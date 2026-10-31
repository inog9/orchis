package injector_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"orchis-go/internal/blob"
	"orchis-go/internal/bootstrap"
	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
	"orchis-go/internal/indexer"
	"orchis-go/internal/injector"
	"orchis-go/internal/mq"
	"orchis-go/internal/search"
	"orchis-go/internal/server"
	"orchis-go/internal/testutil"
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// TestFullPipeline menjalankan jalur sungguhan: API -> outbox -> RabbitMQ -> injector -> nmap -> API -> PostgreSQL,
// lalu indeks Elasticsearch dan keluaran mentah di MinIO. Tanpa tiruan: butuh `make dev-up-full` dan nmap terpasang.
func TestFullPipeline(t *testing.T) {
	if _, err := exec.LookPath("nmap"); err != nil {
		t.Skip("nmap tidak terpasang; tes pipeline membutuhkannya (apt install nmap)")
	}
	amqpURL := envOr("ORCHIS_TEST_AMQP_URL", "amqp://orchis:orchis@127.0.0.1:15672/")
	esURL := envOr("ORCHIS_TEST_ES_URL", "http://127.0.0.1:19200")
	conn, err := mq.Dial(amqpURL)
	if err != nil {
		t.Fatalf("RabbitMQ tidak terjangkau di %s (jalankan: make dev-up-full): %v", amqpURL, err)
	}
	conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	env := testutil.New(t)

	es := search.New(esURL)
	es.Index = "orchis-findings-test-" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if err := es.EnsureIndex(ctx); err != nil {
		t.Fatalf("Elasticsearch tidak terjangkau di %s (jalankan: make dev-up-full): %v", esURL, err)
	}
	t.Cleanup(func() { es.DeleteIndex(context.Background()) })
	store, err := blob.Open(ctx, blob.Options{Endpoint: envOr("ORCHIS_TEST_S3_ENDPOINT", "127.0.0.1:19000"),
		AccessKey: "orchis", SecretKey: "orchis-dev-secret", Bucket: "orchis-test"})
	if err != nil {
		t.Fatalf("MinIO tidak terjangkau (jalankan: make dev-up-full): %v", err)
	}

	guard := contract.Guard{AllowLoopback: true}
	srv := httptest.NewServer(server.New(server.Options{DB: env.App, Tenant: env.TenantA, Guard: guard, Search: es, Store: store}))
	t.Cleanup(srv.Close)

	// Target: port TCP sungguhan yang sedang mendengarkan.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port

	// Pengguna & permintaan API
	if _, err := bootstrap.CreateUser(ctx, env.App, env.TenantA, "admin@a.test", "Admin", "admin", "Correct-Horse-9"); err != nil {
		t.Fatal(err)
	}
	cookie := login(t, srv.URL)
	api := func(method, path string, body any) (int, []byte) {
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = strings.NewReader(string(b))
		}
		req, _ := http.NewRequest(method, srv.URL+path, rd)
		req.Header.Set("X-Requested-With", "orchis-web")
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, b
	}
	var asset, test struct{ ID string }
	code, b := api("POST", "/api/v1/assets", map[string]any{"category": "host", "name": "pipeline-target", "testing_authorized": true,
		"addresses": []map[string]any{{"kind": "ip", "value": "127.0.0.1"}}})
	must(t, code == 201, "buat aset", b)
	json.Unmarshal(b, &asset)
	code, b = api("POST", "/api/v1/atomic-tests", map[string]any{"name": "pipeline", "contract_key": "nmap.tcp_connect",
		"params": map[string]any{"profile": "custom", "ports": fmt.Sprint(port)}, "target_asset_id": asset.ID})
	must(t, code == 201, "buat test", b)
	json.Unmarshal(b, &test)

	// Jalankan SEBELUM relay hidup: pesan harus menunggu di outbox (tidak hilang) dan terkirim saat relay naik.
	code, b = api("POST", "/api/v1/atomic-tests/"+test.ID+"/run", nil)
	must(t, code == 202, "run", b)
	var ex struct{ ID, State string }
	json.Unmarshal(b, &ex)
	var unpublished int
	env.Owner.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE topic='exec.dispatch' AND published_at IS NULL`).Scan(&unpublished)
	if unpublished != 1 {
		t.Fatalf("pesan dispatch harus menunggu di outbox, dapat %d", unpublished)
	}

	token, err := execution.IssueToken(ctx, env.App, env.TenantA, "nmap")
	if err != nil {
		t.Fatal(err)
	}
	go (&mq.Relay{DB: env.App, Tenant: env.TenantA, URL: amqpURL}).Run(ctx)
	go (&indexer.Indexer{DB: env.App, Tenant: env.TenantA, ES: es}).Run(ctx, amqpURL)
	go (&injector.Worker{Kind: "nmap", Client: injector.NewClient(srv.URL, token), AMQPURL: amqpURL, Version: "test", Guard: guard}).Run(ctx)

	var got struct {
		State, Outcome, Error string
		RawOutputKey          *string `json:"raw_output_key"`
		Findings              []struct{ Type, Value, Severity string }
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		_, b = api("GET", "/api/v1/executions/"+ex.ID, nil)
		json.Unmarshal(b, &got)
		if got.State == "ended" || time.Now().After(deadline) {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if got.State != "ended" || got.Outcome != "success" {
		t.Fatalf("eksekusi tidak selesai sukses: %s", b)
	}
	wantValue := fmt.Sprintf("tcp/%d", port)
	if len(got.Findings) != 1 || got.Findings[0].Type != "open_port" || got.Findings[0].Value != wantValue {
		t.Fatalf("finding open_port %s diharapkan: %s", wantValue, b)
	}
	if got.RawOutputKey == nil {
		t.Fatal("keluaran mentah harus tersimpan di MinIO")
	}
	raw, err := store.Get(ctx, *got.RawOutputKey)
	if err != nil || !strings.Contains(string(raw), "<nmaprun") {
		t.Fatalf("objek MinIO salah: %v %.80s", err, raw)
	}

	// Elasticsearch: finding muncul lewat konsumen index.finding, lalu pencarian API memakainya.
	var page struct {
		Items  []struct{ Value string }
		Search string
	}
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		es.Refresh(ctx)
		_, b = api("GET", "/api/v1/findings?q=pipeline-target", nil)
		json.Unmarshal(b, &page)
		if page.Search == "elasticsearch" && len(page.Items) == 1 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if page.Search != "elasticsearch" || len(page.Items) != 1 || page.Items[0].Value != wantValue {
		t.Fatalf("pencarian lewat Elasticsearch gagal: %s", b)
	}
	// Tenant lain tidak mencari apa-apa dari indeks yang sama.
	other, err := es.SearchFindings(ctx, env.TenantB, "pipeline-target", 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("tenant B tidak boleh melihat dokumen tenant A: %v %v", other, err)
	}
	// Indeks bisa dibangun ulang dari PostgreSQL.
	es.DeleteIndex(ctx)
	n, err := (&indexer.Indexer{DB: env.App, Tenant: env.TenantA, ES: es}).Reindex(ctx)
	if err != nil || n != 1 {
		t.Fatalf("reindex: n=%d err=%v", n, err)
	}
	es.Refresh(ctx)
	if ids, err := es.SearchFindings(ctx, env.TenantA, "pipeline-target", 10); err != nil || len(ids) != 1 {
		t.Fatalf("setelah reindex dokumen harus ada: %v %v", ids, err)
	}
	var left int
	env.Owner.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE published_at IS NULL`).Scan(&left)
	if left != 0 {
		t.Errorf("semua pesan outbox harus terkirim, tersisa %d", left)
	}
}

func must(t *testing.T, ok bool, what string, body []byte) {
	t.Helper()
	if !ok {
		t.Fatalf("%s gagal: %s", what, body)
	}
}

func login(t *testing.T, base string) *http.Cookie {
	t.Helper()
	req, _ := http.NewRequest("POST", base+"/api/v1/auth/login", strings.NewReader(`{"email":"admin@a.test","password":"Correct-Horse-9"}`))
	req.Header.Set("X-Requested-With", "orchis-web")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	for _, c := range res.Cookies() {
		if c.Name == "orchis_session" {
			return c
		}
	}
	t.Fatalf("login gagal: %d", res.StatusCode)
	return nil
}
