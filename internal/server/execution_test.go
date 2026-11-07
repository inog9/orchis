package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"orchis-go/internal/osquery"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
	"orchis-go/internal/server"
)

type execJSON struct {
	ID           string `json:"id"`
	State        string `json:"state"`
	Outcome      string `json:"outcome"`
	Error        string `json:"error"`
	FindingCount int    `json:"finding_count"`
	Target       string `json:"target_address"`
	Logs         []struct{ Level, Message string }
	Findings     []struct {
		ID, Type, Value, Severity string
		OccurrenceCount           int `json:"occurrence_count"`
	}
}

// scanFixture menyiapkan aset sasaran, atomic test nmap, dan token injector nmap.
type scanFixture struct {
	a                      *app
	admin, analyst, viewer string
	asset, test            string
	token                  string
	rt                     *execution.Runtime
}

func newScanFixture(t *testing.T, authorized bool) *scanFixture {
	a, env := setup(t)
	f := &scanFixture{a: a, admin: a.login("admin@a.test"), analyst: a.login("analyst@a.test"), viewer: a.login("viewer@a.test")}
	f.asset = mkAsset(t, a, f.admin, map[string]any{"category": "host", "name": "target-1", "testing_authorized": authorized,
		"addresses": []map[string]any{{"kind": "ip", "value": "127.0.0.1"}}})
	var tst struct{ ID string }
	r := a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": "Top ports", "contract_key": "nmap.tcp_connect",
		"params": map[string]any{"profile": "top100"}, "target_asset_id": f.asset}, f.analyst)
	if r.Status != 201 {
		t.Fatalf("buat atomic test: %d %s", r.Status, r.Body)
	}
	r.JSON(&tst)
	f.test = tst.ID
	tok, err := execution.IssueToken(context.Background(), env.App, env.TenantA, "nmap")
	if err != nil {
		t.Fatal(err)
	}
	f.token = tok
	f.rt = server.Runtime(server.Options{DB: env.App, Tenant: env.TenantA, Guard: contract.Guard{AllowLoopback: true}})
	return f
}

// inj memanggil API injector: hanya token bearer, tanpa header CSRF dan tanpa cookie.
func (f *scanFixture) inj(method, path string, body any, token string) resp {
	return f.a.do(method, "/api/v1/injector/"+path, body, "", "X-Requested-With", "", "Authorization", "Bearer "+token)
}

func (f *scanFixture) run(t *testing.T) execJSON {
	t.Helper()
	r := f.a.do("POST", "/api/v1/atomic-tests/"+f.test+"/run", nil, f.analyst)
	if r.Status != 202 {
		t.Fatalf("run: %d %s", r.Status, r.Body)
	}
	var e execJSON
	r.JSON(&e)
	return e
}

func (f *scanFixture) exec(t *testing.T, id string) execJSON {
	t.Helper()
	var e execJSON
	r := f.a.do("GET", "/api/v1/executions/"+id, nil, f.analyst)
	if r.Status != 200 {
		t.Fatalf("get execution: %d %s", r.Status, r.Body)
	}
	r.JSON(&e)
	return e
}

func (f *scanFixture) outbox(t *testing.T, topic string) int {
	t.Helper()
	var n int
	if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM outbox WHERE topic=$1`, topic).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

var okResult = map[string]any{"exit_code": 0, "output": "<nmaprun/>", "findings": []map[string]any{
	{"type": "open_port", "value": "tcp/80", "title": "Open port 80/tcp (http)", "severity": "info", "details": map[string]any{"port": 80}},
	{"type": "open_port", "value": "TCP/80 ", "title": "duplicate in same batch", "severity": "info"},
	{"type": "vulnerability", "value": "nope", "title": "not allowed for nmap", "severity": "critical"},
}}

func TestRunIsRefusedUntilAssetIsAuthorized(t *testing.T) {
	f := newScanFixture(t, false)
	r := f.a.do("POST", "/api/v1/atomic-tests/"+f.test+"/run", nil, f.analyst)
	if r.Status != 422 {
		t.Fatalf("aset belum diotorisasi harus 422, dapat %d %s", r.Status, r.Body)
	}
	if f.outbox(t, execution.TopicDispatch) != 0 {
		t.Fatal("penolakan tidak boleh meninggalkan pesan dispatch")
	}
	var n int
	f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM executions`).Scan(&n)
	if n != 0 {
		t.Fatalf("penolakan tidak boleh membuat eksekusi, ada %d", n)
	}
	// analyst tidak boleh mengotorisasi; admin boleh
	var cur struct{ Version int }
	f.a.do("GET", "/api/v1/assets/"+f.asset, nil, f.admin).JSON(&cur)
	if r := f.a.do("PATCH", "/api/v1/assets/"+f.asset, map[string]any{"version": cur.Version, "testing_authorized": true}, f.analyst); r.Status != 403 {
		t.Fatalf("analyst tidak boleh mengotorisasi: %d", r.Status)
	}
	if r := f.a.do("PATCH", "/api/v1/assets/"+f.asset, map[string]any{"version": cur.Version, "testing_authorized": true}, f.admin); r.Status != 200 {
		t.Fatalf("admin mengotorisasi: %d %s", r.Status, r.Body)
	}
	f.run(t)
}

func TestRunRefusesDangerousAndLoopbackTargets(t *testing.T) {
	f := newScanFixture(t, true)
	meta := mkAsset(t, f.a, f.admin, map[string]any{"category": "host", "name": "metadata", "testing_authorized": true,
		"addresses": []map[string]any{{"kind": "ip", "value": "169.254.169.254"}}})
	var tst struct{ ID string }
	f.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": "meta", "contract_key": "nmap.tcp_connect", "target_asset_id": meta}, f.analyst).JSON(&tst)
	if r := f.a.do("POST", "/api/v1/atomic-tests/"+tst.ID+"/run", nil, f.analyst); r.Status != 422 {
		t.Fatalf("alamat metadata cloud harus ditolak walau aset diotorisasi: %d %s", r.Status, r.Body)
	}
	// Tanpa AllowLoopback, 127.0.0.1 juga ditolak (perilaku bawaan produksi).
	strict := newAppGuard(t, f.a.env, f.a.env.TenantA, contract.Guard{})
	sc := strict.login("analyst@a.test")
	if r := strict.do("POST", "/api/v1/atomic-tests/"+f.test+"/run", nil, sc); r.Status != 422 {
		t.Fatalf("loopback harus ditolak tanpa opt-in: %d %s", r.Status, r.Body)
	}
}

func TestAtomicTestParamsAreValidatedByServer(t *testing.T) {
	f := newScanFixture(t, true)
	for _, params := range []map[string]any{
		{"profile": "custom", "ports": "22; id"},
		{"profile": "top100", "script": "vuln"},
		{"profile": "nope"},
	} {
		r := f.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": "x", "contract_key": "nmap.tcp_connect", "params": params}, f.analyst)
		if r.Status != 422 && r.Status != 400 {
			t.Errorf("params %v harus ditolak, dapat %d %s", params, r.Status, r.Body)
		}
	}
	if r := f.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": "x", "contract_key": "nmap.udp"}, f.analyst); r.Status != 422 {
		t.Errorf("kontrak tak dikenal harus 422: %d", r.Status)
	}
	var tst struct {
		Params json.RawMessage
	}
	f.a.do("GET", "/api/v1/atomic-tests/"+f.test, nil, f.analyst).JSON(&tst)
	if string(tst.Params) != `{"profile":"top100","service_detection":false}` {
		t.Errorf("params harus disimpan dalam bentuk kanonik, dapat %s", tst.Params)
	}
}

func TestExecutionLifecycleDedupesFindingsAcrossRuns(t *testing.T) {
	f := newScanFixture(t, true)
	e := f.run(t)
	if e.State != "ready" || e.Target != "127.0.0.1" {
		t.Fatalf("eksekusi baru harus ready ke 127.0.0.1: %+v", e)
	}
	if f.outbox(t, execution.TopicDispatch) != 1 {
		t.Fatal("harus ada tepat satu pesan dispatch di outbox")
	}
	if r := f.a.do("POST", "/api/v1/atomic-tests/"+f.test+"/run", nil, f.analyst); r.Status != 409 {
		t.Fatalf("menjalankan ulang saat masih berjalan harus 409: %d", r.Status)
	}
	// injector lain (nuclei) tidak boleh menyentuh eksekusi nmap
	env := f.a.env
	other, _ := execution.IssueToken(context.Background(), env.App, env.TenantA, "nuclei")
	if r := f.inj("POST", "executions/"+e.ID+"/claim", nil, other); r.Status != 404 {
		t.Fatalf("injector lain harus 404: %d %s", r.Status, r.Body)
	}
	if r := f.inj("POST", "executions/"+e.ID+"/claim", nil, "oij_salah"); r.Status != 401 {
		t.Fatalf("token salah harus 401: %d", r.Status)
	}
	if r := f.a.do("POST", "/api/v1/injector/executions/"+e.ID+"/claim", nil, f.admin); r.Status != 401 {
		t.Fatalf("cookie pengguna tidak boleh dipakai di jalur injector: %d", r.Status)
	}
	r := f.inj("POST", "executions/"+e.ID+"/claim", nil, f.token)
	if r.Status != 200 {
		t.Fatalf("claim: %d %s", r.Status, r.Body)
	}
	var spec struct {
		Target  string
		Timeout int `json:"timeout_seconds"`
	}
	r.JSON(&spec)
	if spec.Target != "127.0.0.1" || spec.Timeout != 600 {
		t.Fatalf("spec salah: %s", r.Body)
	}
	if r := f.inj("POST", "executions/"+e.ID+"/claim", nil, f.token); r.Status != 409 {
		t.Fatalf("claim kedua harus 409: %d", r.Status)
	}
	if r := f.inj("POST", "executions/"+e.ID+"/heartbeat", nil, f.token); r.Status != 200 || string(r.Body) == "" {
		t.Fatalf("heartbeat: %d %s", r.Status, r.Body)
	}
	if r := f.inj("POST", "executions/"+e.ID+"/result", okResult, f.token); r.Status != 204 {
		t.Fatalf("result: %d %s", r.Status, r.Body)
	}
	if r := f.inj("POST", "executions/"+e.ID+"/result", okResult, f.token); r.Status != 409 {
		t.Fatalf("result kedua harus 409 (tidak menggandakan finding): %d", r.Status)
	}
	got := f.exec(t, e.ID)
	if got.State != "ended" || got.Outcome != "success" || len(got.Findings) != 1 || got.Findings[0].Value != "tcp/80" {
		t.Fatalf("hasil eksekusi salah: %+v", got)
	}
	var sawReject bool
	for _, l := range got.Logs {
		if l.Level == "warn" && len(l.Message) > 8 && l.Message[:8] == "Rejected" {
			sawReject = true
		}
	}
	if !sawReject {
		t.Errorf("jenis finding yang tidak diizinkan harus tercatat sebagai ditolak: %+v", got.Logs)
	}
	if f.outbox(t, execution.TopicIndexFinder) != 1 {
		t.Errorf("harus ada satu pesan index.finding")
	}

	// Run kedua melihat port yang sama: tetap satu finding, dua kemunculan.
	e2 := f.run(t)
	f.inj("POST", "executions/"+e2.ID+"/claim", nil, f.token)
	if r := f.inj("POST", "executions/"+e2.ID+"/result", okResult, f.token); r.Status != 204 {
		t.Fatalf("result run 2: %d %s", r.Status, r.Body)
	}
	var list struct {
		Items []struct {
			ID              string
			OccurrenceCount int `json:"occurrence_count"`
		}
	}
	f.a.do("GET", "/api/v1/findings?asset_id="+f.asset, nil, f.viewer).JSON(&list)
	if len(list.Items) != 1 || list.Items[0].OccurrenceCount != 2 {
		t.Fatalf("satu finding dengan 2 kemunculan diharapkan: %+v", list)
	}
	var occ int
	f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM finding_occurrences WHERE finding_id=$1`, list.Items[0].ID).Scan(&occ)
	if occ != 2 {
		t.Errorf("occurrence harus 2, dapat %d", occ)
	}
}

func TestFailedScanRecordsNoFindings(t *testing.T) {
	f := newScanFixture(t, true)
	e := f.run(t)
	f.inj("POST", "executions/"+e.ID+"/claim", nil, f.token)
	code := 1
	body := map[string]any{"exit_code": code, "error": "nmap failed: exit status 1", "findings": okResult["findings"]}
	if r := f.inj("POST", "executions/"+e.ID+"/result", body, f.token); r.Status != 204 {
		t.Fatalf("%d %s", r.Status, r.Body)
	}
	got := f.exec(t, e.ID)
	if got.Outcome != "error" || got.Error == "" || len(got.Findings) != 0 {
		t.Fatalf("scan gagal tidak boleh menyimpan finding: %+v", got)
	}
}

func TestCancelStopsRunningExecution(t *testing.T) {
	f := newScanFixture(t, true)
	e := f.run(t)
	f.inj("POST", "executions/"+e.ID+"/claim", nil, f.token)
	if r := f.a.do("POST", "/api/v1/executions/"+e.ID+"/cancel", nil, f.viewer); r.Status != 403 {
		t.Fatalf("viewer tidak boleh membatalkan: %d", r.Status)
	}
	if r := f.a.do("POST", "/api/v1/executions/"+e.ID+"/cancel", nil, f.analyst); r.Status != 200 {
		t.Fatalf("cancel: %d %s", r.Status, r.Body)
	}
	var hb struct{ Cancel bool }
	f.inj("POST", "executions/"+e.ID+"/heartbeat", nil, f.token).JSON(&hb)
	if !hb.Cancel {
		t.Fatal("heartbeat harus memberi tahu injector bahwa eksekusi dibatalkan")
	}
	if r := f.inj("POST", "executions/"+e.ID+"/result", okResult, f.token); r.Status != 409 {
		t.Fatalf("hasil setelah batal harus 409: %d", r.Status)
	}
	if got := f.exec(t, e.ID); got.Outcome != "canceled" || len(got.Findings) != 0 {
		t.Fatalf("eksekusi batal tidak boleh punya finding: %+v", got)
	}
	if r := f.a.do("POST", "/api/v1/executions/"+e.ID+"/cancel", nil, f.analyst); r.Status != 409 {
		t.Fatalf("membatalkan dua kali harus 409: %d", r.Status)
	}
}

func TestAuthorizationRevokedAfterQueueingBlocksTheClaim(t *testing.T) {
	f := newScanFixture(t, true)
	e := f.run(t)
	var cur struct{ Version int }
	f.a.do("GET", "/api/v1/assets/"+f.asset, nil, f.admin).JSON(&cur)
	if r := f.a.do("PATCH", "/api/v1/assets/"+f.asset, map[string]any{"version": cur.Version, "testing_authorized": false}, f.admin); r.Status != 200 {
		t.Fatalf("cabut otorisasi: %d %s", r.Status, r.Body)
	}
	if r := f.inj("POST", "executions/"+e.ID+"/claim", nil, f.token); r.Status != 409 {
		t.Fatalf("claim harus ditolak setelah otorisasi dicabut: %d %s", r.Status, r.Body)
	}
	if got := f.exec(t, e.ID); got.State != "ended" || got.Outcome != "error" {
		t.Fatalf("eksekusi harus berakhir error: %+v", got)
	}
}

func TestSweepTimesOutLostWorkersAndRepublishesStaleReady(t *testing.T) {
	f := newScanFixture(t, true)
	owner := f.a.env.Owner
	ctx := context.Background()

	e := f.run(t)
	f.inj("POST", "executions/"+e.ID+"/claim", nil, f.token)
	if _, err := owner.Exec(ctx, `UPDATE executions SET lease_until = now() - interval '1 second' WHERE id=$1`, e.ID); err != nil {
		t.Fatal(err)
	}
	retried, timedOut, err := f.rt.Sweep(ctx)
	if err != nil || retried != 0 || timedOut != 1 {
		t.Fatalf("sweep: retried=%d timedOut=%d err=%v", retried, timedOut, err)
	}
	if got := f.exec(t, e.ID); got.Outcome != "timeout" {
		t.Fatalf("harus timeout: %+v", got)
	}
	if r := f.inj("POST", "executions/"+e.ID+"/result", okResult, f.token); r.Status != 409 {
		t.Fatalf("hasil terlambat harus 409: %d", r.Status)
	}

	// eksekusi ready yang pesannya hilang diterbitkan ulang sekali
	e2 := f.run(t)
	before := f.outbox(t, execution.TopicDispatch)
	if _, err := owner.Exec(ctx, `UPDATE executions SET updated_at = now() - interval '10 minutes' WHERE id=$1`, e2.ID); err != nil {
		t.Fatal(err)
	}
	f.rt.Sweep(ctx)
	f.rt.Sweep(ctx)
	if after := f.outbox(t, execution.TopicDispatch); after != before+1 {
		t.Fatalf("pesan dispatch harus diterbitkan ulang tepat sekali: before=%d after=%d", before, after)
	}
}

func TestPermissionsAndIsolation(t *testing.T) {
	f := newScanFixture(t, true)
	a := f.a
	if r := a.do("GET", "/api/v1/atomic-tests", nil, f.viewer); r.Status != 403 {
		t.Errorf("viewer cannot read Atomic tests: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/atomic-tests/"+f.test+"/run", nil, f.viewer); r.Status != 403 {
		t.Errorf("viewer tidak boleh menjalankan: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": "x", "contract_key": "nmap.tcp_connect"}, f.viewer); r.Status != 403 {
		t.Errorf("viewer tidak boleh membuat: %d", r.Status)
	}
	if r := a.do("DELETE", "/api/v1/atomic-tests/"+f.test, nil, f.analyst); r.Status != 403 {
		t.Errorf("analyst tidak boleh menghapus: %d", r.Status)
	}
	if r := a.do("GET", "/api/v1/atomic-tests", nil, ""); r.Status != 401 {
		t.Errorf("tanpa login harus 401: %d", r.Status)
	}
	e := f.run(t)
	// tenant B tidak melihat apa pun milik tenant A, bahkan lewat koneksi aplikasi biasa
	var n int
	err := a.env.App.InTenantRead(context.Background(), a.env.TenantB, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM executions WHERE id=$1`, e.ID).Scan(&n)
	})
	if err != nil || n != 0 {
		t.Fatalf("tenant B tidak boleh melihat eksekusi tenant A: n=%d err=%v", n, err)
	}
	if r := a.do("DELETE", "/api/v1/atomic-tests/"+f.test, nil, f.admin); r.Status != 204 {
		t.Fatalf("admin menghapus: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/atomic-tests/"+f.test+"/run", nil, f.analyst); r.Status != 404 {
		t.Errorf("test terhapus tidak bisa dijalankan: %d", r.Status)
	}
}

func TestFindingSearchFallsBackToDatabaseAndEscapesWildcards(t *testing.T) {
	f := newScanFixture(t, true)
	e := f.run(t)
	f.inj("POST", "executions/"+e.ID+"/claim", nil, f.token)
	f.inj("POST", "executions/"+e.ID+"/result", map[string]any{"exit_code": 0, "findings": []map[string]any{
		{"type": "open_port", "value": "tcp/8080", "title": "Open port 8080/tcp (http-proxy)", "severity": "info"},
		{"type": "open_port", "value": "tcp/22", "title": "Open port 22/tcp (ssh)", "severity": "info"},
	}}, f.token)
	var list struct {
		Items  []struct{ Value string }
		Search string
	}
	rr := f.a.do("GET", "/api/v1/findings?q=proxy", nil, f.viewer)
	rr.JSON(&list)
	t.Logf("%d %s", rr.Status, rr.Body)
	if list.Search != "database" || len(list.Items) != 1 || list.Items[0].Value != "tcp/8080" {
		t.Fatalf("pencarian database salah: %+v", list)
	}
	list.Items = nil
	f.a.do("GET", "/api/v1/findings?q=%25", nil, f.viewer).JSON(&list) // "%" harus dianggap huruf biasa
	if len(list.Items) != 0 {
		t.Fatalf("wildcard %% harus di-escape, dapat %+v", list)
	}
	if r := f.a.do("GET", "/api/v1/findings?severity=bogus", nil, f.viewer); r.Status != 422 {
		t.Errorf("severity tak dikenal harus 422: %d", r.Status)
	}
	// paginasi
	f.a.do("GET", "/api/v1/findings?limit=1", nil, f.viewer).JSON(&list)
	var page struct {
		Items      []struct{ Value string }
		NextCursor *string `json:"next_cursor"`
	}
	f.a.do("GET", "/api/v1/findings?limit=1", nil, f.viewer).JSON(&page)
	if len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("halaman pertama harus punya next_cursor: %+v", page)
	}
	var page2 struct{ Items []struct{ Value string } }
	f.a.do("GET", "/api/v1/findings?limit=1&cursor="+*page.NextCursor, nil, f.viewer).JSON(&page2)
	if len(page2.Items) != 1 || page2.Items[0].Value == page.Items[0].Value {
		t.Fatalf("halaman kedua harus berbeda: %+v vs %+v", page2, page)
	}
}

func TestContractsReportInjectorStatus(t *testing.T) {
	f := newScanFixture(t, true)
	var out struct {
		Items []struct {
			Key               string
			IntegrationStatus string `json:"integration_status"`
		}
	}
	f.a.do("GET", "/api/v1/contracts", nil, f.analyst).JSON(&out)
	keys := map[string]bool{}
	for _, c := range out.Items {
		keys[c.Key] = true
	}
	// Nmap, Nuclei, Hackbot, dua aksi agent lama, aksi osquery lama dan aksi osquery DefenseBot.
	want := []string{"nmap.tcp_connect", "nuclei.scan", "hackbot.pentest", "defensebot.match", "agent.listening_ports", "agent.system_info",
		"osquery.inventory", "osquery.packages", "osquery.users", "osquery.network", "osquery.hardening", "osquery.firewall", "osquery.network_ranges",
		"osquery.os_release", "osquery.kernel", "osquery.npm_global", "osquery.pip_packages", "osquery.macos_apps", "osquery.homebrew", "osquery.exposure"}
	// Query pack resmi yang dibekukan masing-masing punya kontrak (kunci = kunci query).
	want = append(want, osquery.PackQueryKeys()...)
	if len(out.Items) != len(want) {
		t.Fatalf("%d kontrak diharapkan: %+v", len(want), out)
	}
	for _, k := range want {
		if !keys[k] {
			t.Errorf("kontrak %s hilang", k)
		}
	}
	for _, c := range out.Items {
		if c.IntegrationStatus != "offline" {
			t.Errorf("tanpa heartbeat status harus offline: %+v", c)
		}
	}
	if r := f.inj("POST", "heartbeat", map[string]any{"version": "1.2.3"}, f.token); r.Status != 200 {
		t.Fatalf("heartbeat: %d %s", r.Status, r.Body)
	}
	f.a.do("GET", "/api/v1/contracts", nil, f.analyst).JSON(&out)
	for _, c := range out.Items {
		want := "offline"
		if c.Key == "nmap.tcp_connect" {
			want = "online"
		}
		if c.IntegrationStatus != want {
			t.Errorf("%s: status %q, harapan %q", c.Key, c.IntegrationStatus, want)
		}
	}
}

var _ = fmt.Sprint
var _ = time.Now
var _ = uuid.Nil

type brokenSearch struct{}

func (brokenSearch) SearchFindings(context.Context, uuid.UUID, string, int) ([]uuid.UUID, error) {
	return nil, fmt.Errorf("elasticsearch down")
}

type fixedSearch struct{ ids []uuid.UUID }

func (f fixedSearch) SearchFindings(context.Context, uuid.UUID, string, int) ([]uuid.UUID, error) {
	return f.ids, nil
}

func TestFindingSearchSurvivesElasticsearchOutageAndNeverTrustsItForData(t *testing.T) {
	f := newScanFixture(t, true)
	e := f.run(t)
	f.inj("POST", "executions/"+e.ID+"/claim", nil, f.token)
	f.inj("POST", "executions/"+e.ID+"/result", map[string]any{"exit_code": 0, "findings": []map[string]any{
		{"type": "open_port", "value": "tcp/8080", "title": "Open port 8080/tcp", "severity": "info"}}}, f.token)

	env := f.a.env
	svc := &execution.Service{DB: env.App}
	p := principalFor(t, env.TenantA)
	page, err := svc.ListFindings(context.Background(), p, execution.FindingFilter{Query: "8080"}, brokenSearch{})
	if err != nil || page.Search != "database" || len(page.Items) != 1 {
		t.Fatalf("ES mati harus jatuh ke database: %+v %v", page, err)
	}
	// ES yang "berbohong" (mengembalikan ID tenant lain / ID yang tidak ada) tidak boleh membocorkan atau mengarang data.
	foreign := uuid.New()
	page, err = svc.ListFindings(context.Background(), p, execution.FindingFilter{Query: "8080"}, fixedSearch{ids: []uuid.UUID{foreign}})
	if err != nil || page.Search != "elasticsearch" || len(page.Items) != 0 {
		t.Fatalf("ID yang tidak ada di tenant ini tidak boleh menghasilkan baris: %+v %v", page, err)
	}
	// Tenant B tidak melihat finding tenant A walau ES mengembalikan ID-nya.
	var fid uuid.UUID
	env.Owner.QueryRow(context.Background(), `SELECT id FROM findings LIMIT 1`).Scan(&fid)
	pb := principalFor(t, env.TenantB)
	page, err = svc.ListFindings(context.Background(), pb, execution.FindingFilter{Query: "8080"}, fixedSearch{ids: []uuid.UUID{fid}})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("RLS harus menyembunyikan finding tenant lain: %+v %v", page, err)
	}
}

func principalFor(t *testing.T, tenant uuid.UUID) *auth.Principal {
	t.Helper()
	return &auth.Principal{TenantID: tenant, UserID: uuid.New(), Email: "t@x.test", Permissions: []string{authz.FindingRead}}
}
