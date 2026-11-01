package indexer_test

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"orchis-go/internal/indexer"
	"orchis-go/internal/mq"
	"orchis-go/internal/search"
	"orchis-go/internal/testutil"
	"sync"
	"testing"
	"time"
)

func TestIndexWriteFinishesBeforeFindingCanBePurged(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	env := testutil.New(t)
	fid := uuid.New()
	if _, err := env.Owner.Exec(ctx, `INSERT INTO findings(id,tenant_id,type,value,value_hash,title) VALUES($1,$2,'text','local','\x01','LOCAL index race fixture')`, fid, env.TenantA); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(201)
		w.Write([]byte(`{"result":"created"}`))
	}))
	defer server.Close()
	defer once.Do(func() { close(release) })
	es := search.New(server.URL)
	es.Index = "fixture"
	worker := &indexer.Indexer{DB: env.App, Tenant: env.TenantA, ES: es}
	body, _ := json.Marshal(map[string]uuid.UUID{"finding_id": fid, "tenant_id": env.TenantA})
	indexDone := make(chan error, 1)
	go func() { indexDone <- worker.Handle(ctx, mq.Message{ID: uuid.New(), Body: body}) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var appPID int32
	if err := env.Owner.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE datname=current_database() AND usename='orchis_app' AND state='idle in transaction' LIMIT 1`).Scan(&appPID); err != nil {
		t.Fatal(err)
	}
	conn, err := env.Owner.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	deletePID := int32(conn.Conn().PgConn().PID())
	deleteDone := make(chan error, 1)
	go func() {
		_, err := conn.Exec(ctx, `DELETE FROM findings WHERE tenant_id=$1 AND id=$2`, env.TenantA, fid)
		deleteDone <- err
	}()
	for {
		var blocked bool
		if err = env.Owner.QueryRow(ctx, `SELECT $1=ANY(pg_blocking_pids($2))`, appPID, deletePID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err := <-deleteDone:
			t.Fatalf("Finding disappeared while an old index put was still in flight: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
	once.Do(func() { close(release) })
	select {
	case err = <-indexDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case err = <-deleteDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var n int
	if err = env.Owner.QueryRow(ctx, `SELECT count(*) FROM findings WHERE id=$1`, fid).Scan(&n); err != nil || n != 0 {
		t.Fatal("Delete did not complete", err)
	}
}
