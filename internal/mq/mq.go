// Package mq: RabbitMQ untuk pengiriman tugas dan peristiwa. Pengiriman at-least-once: penerima harus idempoten.
//
// Alur: baris outbox ditulis dalam transaksi bisnis -> Relay menerbitkan ke exchange dengan publisher confirm ->
// consumer memproses lalu ack. Pesan yang ditolak (nack tanpa requeue) masuk antrean mati "orchis.dead".
package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	amqp "github.com/rabbitmq/amqp091-go"

	"orchis-go/internal/db"
)

const (
	Exchange = "orchis.events"
	DeadX    = "orchis.dead"
	DeadQ    = "orchis.dead"
)

// Queues = antrean tetap yang dideklarasikan server saat naik, supaya pesan tidak hilang walau konsumen belum ada.
var Queues = []string{"exec.nmap", "exec.nuclei", "index.finding"}

func Dial(url string) (*amqp.Connection, error) {
	return amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(5 * time.Second), Heartbeat: 10 * time.Second})
}

// Declare membuat exchange, antrean tetap, dan antrean mati. Idempoten.
func Declare(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(Exchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(DeadX, "fanout", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(DeadQ, true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind(DeadQ, "", DeadX, false, nil); err != nil {
		return err
	}
	for _, q := range Queues {
		if err := DeclareWork(ch, q); err != nil {
			return err
		}
	}
	return nil
}

func DeclareWork(ch *amqp.Channel, q string) error {
	if _, err := ch.QueueDeclare(q, true, false, false, false, amqp.Table{"x-dead-letter-exchange": DeadX}); err != nil {
		return err
	}
	return ch.QueueBind(q, q, Exchange, false, nil)
}

// Relay menerbitkan baris outbox yang belum terkirim.
type Relay struct {
	DB     *db.DB
	Tenant uuid.UUID
	URL    string
}

type pending struct {
	id        int64
	messageID uuid.UUID
	topic     string
	payload   []byte
	headers   map[string]string
}

// Run berjalan sampai ctx selesai; menyambung ulang bila RabbitMQ putus.
func (r *Relay) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := r.session(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("relay outbox terputus, mencoba lagi", "err", err)
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (r *Relay) session(ctx context.Context) error {
	conn, err := Dial(r.URL)
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	if err := Declare(ch); err != nil {
		return err
	}
	if err := ch.Confirm(false); err != nil {
		return err
	}
	slog.Info("relay outbox berjalan")
	closed := conn.NotifyClose(make(chan *amqp.Error, 1))
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-closed:
			return errors.New("koneksi RabbitMQ ditutup")
		case <-t.C:
			for {
				n, err := r.Once(ctx, ch)
				if err != nil {
					return err
				}
				if n == 0 {
					break
				}
			}
		}
	}
}

// Once menerbitkan satu kelompok pesan. Mengembalikan jumlah pesan yang diproses.
func (r *Relay) Once(ctx context.Context, ch *amqp.Channel) (int, error) {
	n := 0
	err := r.DB.InTenant(ctx, r.Tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
SELECT id, message_id, topic, payload, headers FROM outbox
WHERE tenant_id=$1 AND published_at IS NULL AND available_at <= now()
ORDER BY id LIMIT 50 FOR UPDATE SKIP LOCKED`, r.Tenant)
		if err != nil {
			return err
		}
		var batch []pending
		for rows.Next() {
			var p pending
			var hdr []byte
			if err := rows.Scan(&p.id, &p.messageID, &p.topic, &p.payload, &hdr); err != nil {
				rows.Close()
				return err
			}
			_ = json.Unmarshal(hdr, &p.headers)
			batch = append(batch, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, p := range batch {
			key := p.headers["routing_key"]
			if key == "" {
				key = p.topic
			}
			if perr := publish(ctx, ch, key, p); perr != nil {
				slog.Warn("gagal menerbitkan pesan outbox", "id", p.id, "err", perr)
				if _, err := tx.Exec(ctx, `UPDATE outbox SET attempts=attempts+1, last_error=$2,
available_at = now() + make_interval(secs => LEAST(60, 2 ^ LEAST(attempts, 6))) WHERE id=$1`, p.id, perr.Error()); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at=now(), attempts=attempts+1, last_error=NULL WHERE id=$1`, p.id); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

func publish(ctx context.Context, ch *amqp.Channel, key string, p pending) error {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conf, err := ch.PublishWithDeferredConfirmWithContext(cctx, Exchange, key, true, false, amqp.Publishing{
		ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: p.messageID.String(), Body: p.payload,
		Timestamp: time.Now(),
	})
	if err != nil {
		return err
	}
	ok, err := conf.WaitContext(cctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("pesan ditolak broker (nack)")
	}
	return nil
}

// Message = pesan yang diterima konsumen.
type Message struct {
	ID   uuid.UUID
	Body []byte
}

// Consume memanggil handler untuk setiap pesan; nil -> ack, error -> nack ke antrean mati. Menyambung ulang otomatis.
func Consume(ctx context.Context, url, queue string, prefetch int, handler func(ctx context.Context, m Message) error) {
	for ctx.Err() == nil {
		if err := consumeOnce(ctx, url, queue, prefetch, handler); err != nil && ctx.Err() == nil {
			slog.Warn("konsumen terputus, mencoba lagi", "queue", queue, "err", err)
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func consumeOnce(ctx context.Context, url, queue string, prefetch int, handler func(context.Context, Message) error) error {
	conn, err := Dial(url)
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(Exchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(DeadX, "fanout", true, false, false, false, nil); err != nil {
		return err
	}
	if err := DeclareWork(ch, queue); err != nil {
		return err
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		return err
	}
	msgs, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	closed := conn.NotifyClose(make(chan *amqp.Error, 1))
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-closed:
			return fmt.Errorf("koneksi ditutup: %v", e)
		case d, ok := <-msgs:
			if !ok {
				return errors.New("channel konsumen ditutup")
			}
			id, _ := uuid.Parse(d.MessageId)
			if err := handler(ctx, Message{ID: id, Body: d.Body}); err != nil {
				slog.Error("pesan ditolak, dikirim ke antrean mati", "queue", queue, "message", d.MessageId, "err", err)
				_ = d.Nack(false, false)
				continue
			}
			_ = d.Ack(false)
		}
	}
}
