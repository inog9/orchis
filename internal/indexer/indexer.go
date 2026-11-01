// Package indexer menjaga indeks Elasticsearch selaras dengan PostgreSQL. Indeks hanya salinan: bila rusak, Reindex membangunnya ulang.
package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/db"
	"orchis-go/internal/mq"
	"orchis-go/internal/search"
)

const consumerName = "indexer"

type Indexer struct {
	DB     *db.DB
	Tenant uuid.UUID
	ES     *search.Client
}

func (x *Indexer) doc(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*search.FindingDoc, error) {
	d := search.FindingDoc{TenantID: x.Tenant}
	var assetName *string
	err := tx.QueryRow(ctx, `
SELECT f.asset_id, a.name, f.type, f.severity, f.title, f.value, f.last_seen_at
FROM findings f LEFT JOIN assets a ON a.tenant_id=f.tenant_id AND a.id=f.asset_id
WHERE f.tenant_id=$1 AND f.id=$2 FOR KEY SHARE OF f`, x.Tenant, id).Scan(&d.AssetID, &assetName, &d.Type, &d.Severity, &d.Title, &d.Value, &d.LastSeenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if assetName != nil {
		d.AssetName = *assetName
	}
	return &d, err
}

// Handle memproses satu pesan index.finding. Pesan ganda diabaikan lewat processed_messages.
func (x *Indexer) Handle(ctx context.Context, m mq.Message) error {
	var p struct {
		FindingID uuid.UUID `json:"finding_id"`
		TenantID  uuid.UUID `json:"tenant_id"`
	}
	if err := json.Unmarshal(m.Body, &p); err != nil || p.TenantID != x.Tenant {
		return fmt.Errorf("pesan index.finding tidak valid: %v", err)
	}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		lastErr = x.db(ctx, m.ID, p.FindingID)
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}

func (x *Indexer) db(ctx context.Context, msg, finding uuid.UUID) error {
	return x.DB.InTenant(ctx, x.Tenant, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO processed_messages (consumer, message_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, consumerName, msg)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil // sudah diproses
		}
		d, err := x.doc(ctx, tx, finding)
		if err != nil || d == nil {
			return err
		}
		return x.ES.IndexFinding(ctx, finding, *d)
	})
}

// Run menghubungkan indexer ke antrean index.finding sampai ctx selesai.
func (x *Indexer) Run(ctx context.Context, amqpURL string) {
	for ctx.Err() == nil {
		if err := x.ES.EnsureIndex(ctx); err != nil {
			slog.Warn("elasticsearch belum siap", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}
		break
	}
	mq.Consume(ctx, amqpURL, "index.finding", 20, x.Handle)
}

// Reindex membangun ulang seluruh indeks dari PostgreSQL. Mengembalikan jumlah dokumen.
func (x *Indexer) Reindex(ctx context.Context) (int, error) {
	if err := x.ES.EnsureIndex(ctx); err != nil {
		return 0, err
	}
	n := 0
	err := x.DB.InTenant(ctx, x.Tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id FROM findings WHERE tenant_id=$1`, x.Tenant)
		if err != nil {
			return err
		}
		var all []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			all = append(all, id)
		}
		rows.Close()
		for _, id := range all {
			d, err := x.doc(ctx, tx, id)
			if err != nil {
				return err
			}
			if d == nil {
				continue
			}
			if err := x.ES.IndexFinding(ctx, id, *d); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}
