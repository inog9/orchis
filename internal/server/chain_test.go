package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
	"orchis-go/internal/server"
)

type chainFixture struct {
	*scanFixture
	nucleiToken string
	scenario    string
	version     int
}

func newChainFixture(t *testing.T) *chainFixture {
	f := newScanFixture(t, true)
	tok, err := execution.IssueToken(context.Background(), f.a.env.App, f.a.env.TenantA, "nuclei")
	if err != nil {
		t.Fatal(err)
	}
	c := &chainFixture{scanFixture: f, nucleiToken: tok}
	var sc struct {
		ID      string
		Version int
	}
	r := f.a.do("POST", "/api/v1/scenarios", map[string]any{"name": "Kill chain", "severity": "high"}, f.analyst)
	if r.Status != 201 {
		t.Fatalf("buat scenario: %d %s", r.Status, r.Body)
	}
	r.JSON(&sc)
	c.scenario, c.version = sc.ID, sc.Version
	return c
}

func (c *chainFixture) reload(t *testing.T) {
	var sc struct{ Version int }
	c.a.do("GET", "/api/v1/scenarios/"+c.scenario, nil, c.analyst).JSON(&sc)
	c.version = sc.Version
}

func (c *chainFixture) targets(t *testing.T, assets, groups []string) resp {
	r := c.a.do("PUT", "/api/v1/scenarios/"+c.scenario+"/targets", map[string]any{"version": c.version, "asset_ids": assets, "group_ids": groups}, c.analyst)
	if r.Status == 200 {
		c.reload(t)
	}
	return r
}

func step(key, contractKey, mode string, extra map[string]any) map[string]any {
	m := map[string]any{"key": key, "title": "Step " + key, "contract_key": contractKey, "trigger_mode": mode, "is_enabled": true, "params": map[string]any{}}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func onPort(ports ...string) map[string]any {
	return map[string]any{"trigger": map[string]any{"type": "open_port", "values": ports}}
}

func (c *chainFixture) graph(t *testing.T, steps []map[string]any, edges [][2]string) resp {
	es := []map[string]any{}
	for _, e := range edges {
		es = append(es, map[string]any{"parent": e[0], "child": e[1]})
	}
	r := c.a.do("PUT", "/api/v1/scenarios/"+c.scenario+"/graph", map[string]any{"version": c.version, "steps": steps, "edges": es}, c.analyst)
	if r.Status == 200 {
		c.reload(t)
	}
	return r
}

func (c *chainFixture) mustReady(t *testing.T, steps []map[string]any, edges [][2]string) {
	t.Helper()
	if r := c.targets(t, []string{c.asset}, nil); r.Status != 200 {
		t.Fatalf("targets: %d %s", r.Status, r.Body)
	}
	if r := c.graph(t, steps, edges); r.Status != 200 {
		t.Fatalf("graph: %d %s", r.Status, r.Body)
	}
}

type simJSON struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Code      string `json:"code"`
	StepsDone int    `json:"steps_done"`
	StepCount int    `json:"step_count"`
	Steps     []struct {
		Key, Status string
		Executions  map[string]int
	}
	Targets []struct{ ID, Name string }
}

func (s simJSON) status(key string) string {
	for _, st := range s.Steps {
		if st.Key == key {
			return st.Status
		}
	}
	return "?"
}

func (c *chainFixture) launch(t *testing.T) simJSON {
	t.Helper()
	r := c.a.do("POST", "/api/v1/scenarios/"+c.scenario+"/launch", map[string]any{}, c.analyst)
	if r.Status != 202 {
		t.Fatalf("launch: %d %s", r.Status, r.Body)
	}
	var s simJSON
	r.JSON(&s)
	return s
}

func (c *chainFixture) sim(t *testing.T, id string) simJSON {
	t.Helper()
	r := c.a.do("GET", "/api/v1/simulations/"+id, nil, c.viewer)
	if r.Status != 200 {
		t.Fatalf("get sim: %d %s", r.Status, r.Body)
	}
	var s simJSON
	r.JSON(&s)
	return s
}

// execsOf mengembalikan ID eksekusi (belum selesai atau semua) milik sebuah langkah.
func (c *chainFixture) execsOf(t *testing.T, sim, key string) []string {
	t.Helper()
	rows, err := c.a.env.Owner.Query(context.Background(), `
SELECT e.id FROM executions e JOIN simulation_steps s ON s.id=e.simulation_step_id WHERE e.simulation_id=$1 AND s.key=$2 ORDER BY e.created_at, e.id`, sim, key)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id uuid.UUID
		rows.Scan(&id)
		out = append(out, id.String())
	}
	return out
}

func (c *chainFixture) tokenFor(contractKey string) string {
	if contractKey == "nuclei.scan" {
		return c.nucleiToken
	}
	return c.token
}

// finish menjalankan sebuah eksekusi sampai selesai lewat API injector.
func (c *chainFixture) finish(t *testing.T, execID, contractKey string, result map[string]any) {
	t.Helper()
	tok := c.tokenFor(contractKey)
	if r := c.inj("POST", "executions/"+execID+"/claim", nil, tok); r.Status != 200 {
		t.Fatalf("claim %s: %d %s", execID, r.Status, r.Body)
	}
	if r := c.inj("POST", "executions/"+execID+"/result", result, tok); r.Status != 204 {
		t.Fatalf("result %s: %d %s", execID, r.Status, r.Body)
	}
}

func ports(p ...string) map[string]any {
	var fs []map[string]any
	for _, x := range p {
		fs = append(fs, map[string]any{"type": "open_port", "value": x, "title": "Open port " + x, "severity": "info"})
	}
	return map[string]any{"exit_code": 0, "findings": fs}
}

var okEmpty = map[string]any{"exit_code": 0, "findings": []map[string]any{}}
var failed = map[string]any{"exit_code": 1, "error": "boom"}

func vuln(sev string) map[string]any {
	return map[string]any{"exit_code": 0, "findings": []map[string]any{{"type": "vulnerability", "value": "t@x", "title": "Bad", "severity": sev}}}
}

const (
	nmapC   = "nmap.tcp_connect"
	nucleiC = "nuclei.scan"
)

func TestChainRunsThroughTriggersJoinsAndSkips(t *testing.T) {
	c := newChainFixture(t)
	c.mustReady(t, []map[string]any{
		step("scan", nmapC, "after_parents", nil),
		step("web", nucleiC, "on_finding", onPort("tcp/80")),
		step("ssh", nucleiC, "on_finding", onPort("tcp/22")),
		step("never", nucleiC, "on_finding", onPort("tcp/9999")),
		step("after-never", nmapC, "after_parents", nil),
		step("report", nmapC, "after_parents", nil),
	}, [][2]string{{"scan", "web"}, {"scan", "ssh"}, {"scan", "never"}, {"never", "after-never"}, {"web", "report"}, {"ssh", "report"}})

	sim := c.launch(t)
	if sim.Status != "running" || sim.Code == "" || sim.StepCount != 6 {
		t.Fatalf("simulasi awal: %+v", sim)
	}
	if sim.status("scan") != "queued" || sim.status("web") != "waiting" || sim.status("report") != "waiting" {
		t.Fatalf("hanya akar yang boleh diantrekan: %+v", sim)
	}
	if c.outbox(t, execution.TopicDispatch) != 1 {
		t.Fatalf("tepat satu dispatch awal diharapkan, dapat %d", c.outbox(t, execution.TopicDispatch))
	}
	// pelaksana nmap selesai dengan port 80 dan 22
	c.finish(t, c.execsOf(t, sim.ID, "scan")[0], nmapC, ports("tcp/80", "tcp/22"))
	sim = c.sim(t, sim.ID)
	if sim.status("scan") != "done" || sim.status("web") != "queued" || sim.status("ssh") != "queued" {
		t.Fatalf("web dan ssh harus terpicu: %+v", sim.Steps)
	}
	if sim.status("never") != "skipped" || sim.status("after-never") != "blocked" {
		t.Fatalf("aturan tak terpenuhi -> skipped, anaknya blocked: %+v", sim.Steps)
	}
	if sim.status("report") != "waiting" {
		t.Fatalf("report menunggu SEMUA induk: %+v", sim.Steps)
	}
	var triggers int
	c.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM finding_triggers`).Scan(&triggers)
	if triggers != 2 {
		t.Fatalf("dua pemicu tercatat (tcp/80 -> web, tcp/22 -> ssh), dapat %d", triggers)
	}
	// web selesai, ssh belum: report tetap menunggu (join)
	c.finish(t, c.execsOf(t, sim.ID, "web")[0], nucleiC, vuln("high"))
	if got := c.sim(t, sim.ID); got.status("report") != "waiting" || len(c.execsOf(t, sim.ID, "report")) != 0 {
		t.Fatalf("join harus menunggu ssh: %+v", got.Steps)
	}
	c.finish(t, c.execsOf(t, sim.ID, "ssh")[0], nucleiC, okEmpty)
	sim = c.sim(t, sim.ID)
	if sim.status("report") != "queued" || len(c.execsOf(t, sim.ID, "report")) != 1 {
		t.Fatalf("report harus diluncurkan setelah semua induk selesai: %+v", sim.Steps)
	}
	c.finish(t, c.execsOf(t, sim.ID, "report")[0], nmapC, okEmpty)
	sim = c.sim(t, sim.ID)
	if sim.Status != "finished" || sim.StepsDone != 6 {
		t.Fatalf("simulasi harus selesai dengan 6 langkah berstatus akhir: %+v", sim)
	}

	// temuan per simulasi
	var fl struct{ Items []struct{ Type string } }
	c.a.do("GET", "/api/v1/findings?simulation_id="+sim.ID, nil, c.viewer).JSON(&fl)
	if len(fl.Items) != 3 {
		t.Fatalf("2 port + 1 kerentanan diharapkan: %+v", fl)
	}
	// attack path diturunkan dari data
	var ap struct {
		Nodes []struct {
			ID, Kind, Status, Severity string
			Column                     int
		}
		Edges []struct{ From, To, Kind string }
	}
	r := c.a.do("GET", "/api/v1/simulations/"+sim.ID+"/attack-path", nil, c.viewer)
	if r.Status != 200 {
		t.Fatalf("attack path: %d %s", r.Status, r.Body)
	}
	r.JSON(&ap)
	kinds := map[string]int{}
	for _, n := range ap.Nodes {
		kinds[n.Kind]++
	}
	if kinds["step"] != 6 || kinds["asset"] != 1 || kinds["finding"] != 3 {
		t.Fatalf("simpul attack path: %v", kinds)
	}
	edge := map[string]int{}
	for _, e := range ap.Edges {
		edge[e.Kind]++
	}
	// 2 pemicu (tcp/80->web, tcp/22->ssh); web/ssh dipicu temuan sehingga tidak digandakan oleh tepi "then" dari scan;
	// "never" (dilewati, tanpa pemicu) tetap terhubung ke induknya; after-never, report: tepi "then" biasa.
	if edge["triggers"] != 2 || edge["found"] != 3 || edge["then"] != 4 || edge["targets"] != 1 {
		t.Fatalf("tepi attack path: %v", edge)
	}
}

func TestDiamondJoinWaitsForBothBranches(t *testing.T) {
	c := newChainFixture(t)
	c.mustReady(t, []map[string]any{
		step("a", nmapC, "after_parents", nil), step("b", nmapC, "after_parents", nil),
		step("c", nmapC, "after_parents", nil), step("d", nmapC, "after_parents", nil),
	}, [][2]string{{"a", "b"}, {"a", "c"}, {"b", "d"}, {"c", "d"}})
	sim := c.launch(t)
	c.finish(t, c.execsOf(t, sim.ID, "a")[0], nmapC, okEmpty)
	c.finish(t, c.execsOf(t, sim.ID, "b")[0], nmapC, okEmpty)
	if len(c.execsOf(t, sim.ID, "d")) != 0 {
		t.Fatal("d tidak boleh jalan sebelum c selesai")
	}
	c.finish(t, c.execsOf(t, sim.ID, "c")[0], nmapC, okEmpty)
	if len(c.execsOf(t, sim.ID, "d")) != 1 {
		t.Fatal("d harus jalan setelah b dan c selesai")
	}
	c.finish(t, c.execsOf(t, sim.ID, "d")[0], nmapC, okEmpty)
	if got := c.sim(t, sim.ID); got.Status != "finished" {
		t.Fatalf("%+v", got)
	}
}

func TestFailedStepBlocksDependentsButSimulationStillFinishes(t *testing.T) {
	c := newChainFixture(t)
	c.mustReady(t, []map[string]any{step("scan", nmapC, "after_parents", nil), step("next", nucleiC, "after_parents", nil)}, [][2]string{{"scan", "next"}})
	sim := c.launch(t)
	c.finish(t, c.execsOf(t, sim.ID, "scan")[0], nmapC, failed)
	got := c.sim(t, sim.ID)
	if got.status("scan") != "failed" || got.status("next") != "blocked" || got.Status != "finished" {
		t.Fatalf("langkah gagal -> anak blocked, simulasi tetap selesai: %+v", got)
	}
	if len(c.execsOf(t, sim.ID, "next")) != 0 {
		t.Fatal("anak yang blocked tidak boleh punya eksekusi")
	}
}

func TestMultipleTargetsFanOutAndGroupsResolveAtLaunch(t *testing.T) {
	c := newChainFixture(t)
	second := mkAsset(t, c.a, c.admin, map[string]any{"category": "host", "name": "second", "testing_authorized": true, "platform": "linux", "arch": "x86_64",
		"addresses": []map[string]any{{"kind": "ip", "value": "127.0.0.2"}}})
	// grup "All Linux x86_64" memuat 'second' saja
	var gl struct{ Items []struct{ ID, Name string } }
	c.a.do("GET", "/api/v1/asset-groups", nil, c.admin).JSON(&gl)
	var group string
	for _, g := range gl.Items {
		if g.Name == "All Linux x86_64" {
			group = g.ID
		}
	}
	if r := c.targets(t, []string{c.asset}, []string{group}); r.Status != 200 {
		t.Fatalf("targets: %d %s", r.Status, r.Body)
	}
	if r := c.graph(t, []map[string]any{step("scan", nmapC, "after_parents", nil), step("then", nmapC, "after_parents", nil)}, [][2]string{{"scan", "then"}}); r.Status != 200 {
		t.Fatalf("graph: %d %s", r.Status, r.Body)
	}
	sim := c.launch(t)
	if len(sim.Targets) != 2 {
		t.Fatalf("aset langsung + anggota grup = 2 sasaran: %+v", sim.Targets)
	}
	execs := c.execsOf(t, sim.ID, "scan")
	if len(execs) != 2 {
		t.Fatalf("satu eksekusi per sasaran: %v", execs)
	}
	c.finish(t, execs[0], nmapC, okEmpty)
	if len(c.execsOf(t, sim.ID, "then")) != 0 {
		t.Fatal("anak menunggu semua eksekusi induk (semua sasaran)")
	}
	c.finish(t, execs[1], nmapC, failed) // satu sasaran gagal, satu sukses -> langkah done
	if got := c.sim(t, sim.ID); got.status("scan") != "done" || len(c.execsOf(t, sim.ID, "then")) != 2 {
		t.Fatalf("langkah done bila ada yang sukses, anak berlipat ke 2 sasaran: %+v", got)
	}
	// grup dibekukan saat peluncuran: aset baru di grup tidak mengubah simulasi yang sudah jalan
	mkAsset(t, c.a, c.admin, map[string]any{"category": "host", "name": "third", "testing_authorized": true, "platform": "linux", "arch": "x86_64", "addresses": []map[string]any{{"kind": "ip", "value": "127.0.0.3"}}})
	if got := c.sim(t, sim.ID); len(got.Targets) != 2 {
		t.Fatalf("sasaran harus beku: %+v", got.Targets)
	}
	_ = second
}

func TestLaunchPreflightRefusals(t *testing.T) {
	c := newChainFixture(t)
	post := func() resp {
		return c.a.do("POST", "/api/v1/scenarios/"+c.scenario+"/launch", map[string]any{}, c.analyst)
	}
	if r := post(); r.Status != 422 {
		t.Fatalf("tanpa sasaran harus 422: %d %s", r.Status, r.Body)
	}
	c.targets(t, []string{c.asset}, nil)
	if r := post(); r.Status != 422 {
		t.Fatalf("tanpa langkah harus 422: %d %s", r.Status, r.Body)
	}
	c.graph(t, []map[string]any{step("scan", nmapC, "after_parents", nil)}, nil)
	// cabut otorisasi aset -> ditolak dengan nama aset
	var cur struct{ Version int }
	c.a.do("GET", "/api/v1/assets/"+c.asset, nil, c.admin).JSON(&cur)
	c.a.do("PATCH", "/api/v1/assets/"+c.asset, map[string]any{"version": cur.Version, "testing_authorized": false}, c.admin)
	r := post()
	if r.Status != 422 || !contains(string(r.Body), "target-1") {
		t.Fatalf("aset tak diotorisasi harus ditolak menyebut namanya: %d %s", r.Status, r.Body)
	}
	if n := countRows(t, c, "simulations"); n != 0 {
		t.Fatalf("penolakan tidak boleh membuat simulasi: %d", n)
	}
	// viewer tidak boleh meluncurkan
	if r := c.a.do("POST", "/api/v1/scenarios/"+c.scenario+"/launch", map[string]any{}, c.viewer); r.Status != 403 {
		t.Fatalf("viewer: %d", r.Status)
	}
	// alamat terlarang (metadata cloud) ditolak sebelum jalan
	meta := mkAsset(t, c.a, c.admin, map[string]any{"category": "host", "name": "meta", "testing_authorized": true, "addresses": []map[string]any{{"kind": "ip", "value": "169.254.169.254"}}})
	c.a.do("GET", "/api/v1/assets/"+c.asset, nil, c.admin).JSON(&cur)
	c.a.do("PATCH", "/api/v1/assets/"+c.asset, map[string]any{"version": cur.Version, "testing_authorized": true}, c.admin)
	c.targets(t, []string{c.asset, meta}, nil)
	if r := post(); r.Status != 422 || !contains(string(r.Body), "meta") {
		t.Fatalf("alamat metadata harus ditolak di muka: %d %s", r.Status, r.Body)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool { return indexOf(s, sub) >= 0 })()
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func countRows(t *testing.T, c *chainFixture, table string) int {
	var n int
	if err := c.a.env.Owner.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPauseResumeCancel(t *testing.T) {
	c := newChainFixture(t)
	c.mustReady(t, []map[string]any{step("scan", nmapC, "after_parents", nil), step("next", nmapC, "after_parents", nil)}, [][2]string{{"scan", "next"}})
	sim := c.launch(t)
	exec := c.execsOf(t, sim.ID, "scan")[0]

	if r := c.a.do("POST", "/api/v1/simulations/"+sim.ID+"/pause", nil, c.viewer); r.Status != 403 {
		t.Fatalf("viewer tidak boleh menjeda: %d", r.Status)
	}
	if r := c.a.do("POST", "/api/v1/simulations/"+sim.ID+"/pause", nil, c.analyst); r.Status != 200 {
		t.Fatalf("pause: %d %s", r.Status, r.Body)
	}
	if r := c.a.do("POST", "/api/v1/simulations/"+sim.ID+"/pause", nil, c.analyst); r.Status != 409 {
		t.Fatalf("menjeda dua kali harus 409: %d", r.Status)
	}
	if r := c.inj("POST", "executions/"+exec+"/claim", nil, c.token); r.Status != 409 {
		t.Fatalf("claim saat dijeda harus 409: %d %s", r.Status, r.Body)
	}
	before := c.outbox(t, execution.TopicDispatch)
	if r := c.a.do("POST", "/api/v1/simulations/"+sim.ID+"/resume", nil, c.analyst); r.Status != 200 {
		t.Fatalf("resume: %d %s", r.Status, r.Body)
	}
	if after := c.outbox(t, execution.TopicDispatch); after != before+1 {
		t.Fatalf("resume harus menerbitkan ulang dispatch eksekusi ready: %d -> %d", before, after)
	}
	if r := c.inj("POST", "executions/"+exec+"/claim", nil, c.token); r.Status != 200 {
		t.Fatalf("claim setelah resume: %d %s", r.Status, r.Body)
	}
	// batal saat berjalan: eksekusi berhenti, hasil terlambat ditolak, simulasi canceled
	if r := c.a.do("POST", "/api/v1/simulations/"+sim.ID+"/cancel", map[string]any{"reason": "oops"}, c.analyst); r.Status != 200 {
		t.Fatalf("cancel: %d %s", r.Status, r.Body)
	}
	var hb struct{ Cancel bool }
	c.inj("POST", "executions/"+exec+"/heartbeat", nil, c.token).JSON(&hb)
	if !hb.Cancel {
		t.Fatal("heartbeat harus memberi tahu injector bahwa eksekusi dibatalkan")
	}
	if r := c.inj("POST", "executions/"+exec+"/result", ports("tcp/80"), c.token); r.Status != 409 {
		t.Fatalf("hasil setelah batal harus 409: %d", r.Status)
	}
	got := c.sim(t, sim.ID)
	if got.Status != "canceled" || got.status("scan") != "canceled" || got.status("next") != "blocked" {
		t.Fatalf("setelah batal: %+v", got)
	}
	if n := len(c.execsOf(t, sim.ID, "next")); n != 0 {
		t.Fatalf("tidak ada eksekusi baru setelah batal: %d", n)
	}
	if r := c.a.do("POST", "/api/v1/simulations/"+sim.ID+"/resume", nil, c.analyst); r.Status != 409 {
		t.Fatalf("simulasi batal tidak bisa dilanjutkan: %d", r.Status)
	}
}

func TestSimulationTimeoutFailsAndStopsExecutions(t *testing.T) {
	c := newChainFixture(t)
	c.mustReady(t, []map[string]any{step("scan", nmapC, "after_parents", nil)}, nil)
	sim := c.launch(t)
	c.a.env.Owner.Exec(context.Background(), `UPDATE simulations SET started_at = now() - interval '2 hours' WHERE id=$1`, sim.ID)
	svc := server.SimulationService(server.Options{DB: c.a.env.App, Tenant: c.a.env.TenantA, Guard: contract.Guard{AllowLoopback: true}})
	adv, to, err := svc.Tick(context.Background(), c.a.env.TenantA)
	if err != nil || to != 1 || adv != 0 {
		t.Fatalf("tick: adv=%d timeout=%d err=%v", adv, to, err)
	}
	got := c.sim(t, sim.ID)
	if got.Status != "failed" {
		t.Fatalf("harus failed: %+v", got)
	}
	if st := c.exec(t, c.execsOf(t, sim.ID, "scan")[0]); st.Outcome != "canceled" {
		t.Fatalf("eksekusi harus dihentikan: %+v", st)
	}
}

func TestStepDelayDefersDispatch(t *testing.T) {
	c := newChainFixture(t)
	c.mustReady(t, []map[string]any{step("slow", nmapC, "after_parents", map[string]any{"delay_seconds": 3600})}, nil)
	c.launch(t)
	var future bool
	c.a.env.Owner.QueryRow(context.Background(), `SELECT available_at > now() + interval '55 minutes' FROM outbox WHERE topic='exec.dispatch'`).Scan(&future)
	if !future {
		t.Fatal("dispatch langkah berjeda harus baru bisa diterbitkan sekitar satu jam lagi")
	}
}

func TestAdvanceIsIdempotentAndSimulationIsASnapshot(t *testing.T) {
	c := newChainFixture(t)
	c.mustReady(t, []map[string]any{step("scan", nmapC, "after_parents", nil), step("next", nmapC, "after_parents", nil)}, [][2]string{{"scan", "next"}})
	sim := c.launch(t)
	svc := server.SimulationService(server.Options{DB: c.a.env.App, Tenant: c.a.env.TenantA, Guard: contract.Guard{AllowLoopback: true}})
	for i := 0; i < 3; i++ {
		svc.Tick(context.Background(), c.a.env.TenantA)
	}
	if n := countRows(t, c, "executions"); n != 1 {
		t.Fatalf("memajukan berulang tidak boleh menggandakan eksekusi: %d", n)
	}
	// mengedit scenario sesudah peluncuran tidak mengubah simulasi
	if r := c.graph(t, []map[string]any{step("only", nucleiC, "after_parents", nil)}, nil); r.Status != 200 {
		t.Fatalf("%d %s", r.Status, r.Body)
	}
	got := c.sim(t, sim.ID)
	if len(got.Steps) != 2 || got.status("scan") != "queued" {
		t.Fatalf("simulasi adalah salinan beku: %+v", got.Steps)
	}
	c.finish(t, c.execsOf(t, sim.ID, "scan")[0], nmapC, okEmpty)
	if len(c.execsOf(t, sim.ID, "next")) != 1 {
		t.Fatal("simulasi terus mengikuti salinannya, bukan scenario yang sudah berubah")
	}
}

func TestScenarioGraphValidationAndLocking(t *testing.T) {
	c := newChainFixture(t)
	r := c.graph(t, []map[string]any{step("a", nmapC, "after_parents", nil), step("b", nmapC, "after_parents", nil)}, [][2]string{{"a", "b"}, {"b", "a"}})
	if r.Status != 422 || !contains(string(r.Body), "loop") {
		t.Fatalf("siklus harus ditolak: %d %s", r.Status, r.Body)
	}
	r = c.graph(t, []map[string]any{step("a", nmapC, "after_parents", map[string]any{"params": map[string]any{"profile": "custom", "ports": "22; id"}})}, nil)
	if r.Status != 422 || !contains(string(r.Body), "ports") {
		t.Fatalf("parameter berbahaya harus ditolak: %d %s", r.Status, r.Body)
	}
	r = c.graph(t, []map[string]any{step("a", "ssh.bruteforce", "after_parents", nil)}, nil)
	if r.Status != 422 {
		t.Fatalf("aksi tak dikenal harus ditolak: %d %s", r.Status, r.Body)
	}
	r = c.graph(t, []map[string]any{step("a", nmapC, "manual", nil)}, nil)
	if r.Status != 422 {
		t.Fatalf("manual belum tersedia: %d", r.Status)
	}
	// graf valid tersimpan; versi lama ditolak (409); viewer tidak boleh
	ok := []map[string]any{step("a", nmapC, "after_parents", nil)}
	stale := c.version
	if r := c.graph(t, ok, nil); r.Status != 200 {
		t.Fatalf("%d %s", r.Status, r.Body)
	}
	if r := c.a.do("PUT", "/api/v1/scenarios/"+c.scenario+"/graph", map[string]any{"version": stale, "steps": ok, "edges": []any{}}, c.analyst); r.Status != 409 {
		t.Fatalf("versi usang harus 409: %d", r.Status)
	}
	if r := c.a.do("PUT", "/api/v1/scenarios/"+c.scenario+"/graph", map[string]any{"version": c.version, "steps": ok, "edges": []any{}}, c.viewer); r.Status != 403 {
		t.Fatalf("viewer: %d", r.Status)
	}
	// nama ganda
	if r := c.a.do("POST", "/api/v1/scenarios", map[string]any{"name": "Kill chain"}, c.analyst); r.Status != 422 {
		t.Fatalf("nama ganda harus 422: %d %s", r.Status, r.Body)
	}
	// params dinormalisasi
	var d struct {
		Steps []struct{ Params json.RawMessage }
	}
	c.a.do("GET", "/api/v1/scenarios/"+c.scenario, nil, c.viewer).JSON(&d)
	if string(d.Steps[0].Params) != `{"profile":"top100","service_detection":false}` {
		t.Fatalf("params harus kanonik: %s", d.Steps[0].Params)
	}
	// hapus: hanya admin
	if r := c.a.do("DELETE", "/api/v1/scenarios/"+c.scenario, nil, c.analyst); r.Status != 403 {
		t.Fatalf("analyst tidak boleh menghapus: %d", r.Status)
	}
	if r := c.a.do("DELETE", "/api/v1/scenarios/"+c.scenario, nil, c.admin); r.Status != 204 {
		t.Fatalf("admin menghapus: %d", r.Status)
	}
	if r := c.a.do("GET", "/api/v1/scenarios/"+c.scenario, nil, c.admin); r.Status != 404 {
		t.Fatalf("setelah dihapus 404: %d", r.Status)
	}
}

var _ = fmt.Sprint
var _ = time.Now

func TestPausedSimulationLetsRunningFinishButStartsNothingNew(t *testing.T) {
	c := newChainFixture(t)
	c.mustReady(t, []map[string]any{step("scan", nmapC, "after_parents", nil), step("next", nmapC, "after_parents", nil)}, [][2]string{{"scan", "next"}})
	sim := c.launch(t)
	scan := c.execsOf(t, sim.ID, "scan")[0]
	if r := c.inj("POST", "executions/"+scan+"/claim", nil, c.token); r.Status != 200 {
		t.Fatalf("claim: %d", r.Status)
	}
	if r := c.a.do("POST", "/api/v1/simulations/"+sim.ID+"/pause", nil, c.analyst); r.Status != 200 {
		t.Fatalf("pause: %d", r.Status)
	}
	// yang sedang berjalan boleh selesai, tetapi langkah berikutnya tidak boleh dimulai selama dijeda
	if r := c.inj("POST", "executions/"+scan+"/result", okEmpty, c.token); r.Status != 204 {
		t.Fatalf("hasil saat dijeda tetap diterima: %d %s", r.Status, r.Body)
	}
	got := c.sim(t, sim.ID)
	if got.Status != "paused" || got.status("scan") != "done" || len(c.execsOf(t, sim.ID, "next")) != 0 {
		t.Fatalf("dijeda: tidak ada eksekusi baru: %+v", got)
	}
	if r := c.a.do("POST", "/api/v1/simulations/"+sim.ID+"/resume", nil, c.analyst); r.Status != 200 {
		t.Fatalf("resume: %d", r.Status)
	}
	if got := c.sim(t, sim.ID); got.status("next") != "queued" || len(c.execsOf(t, sim.ID, "next")) != 1 {
		t.Fatalf("setelah dilanjutkan langkah berikutnya dimulai: %+v", got)
	}
}
