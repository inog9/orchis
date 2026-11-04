package server_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orchis-go/internal/agents"
	"orchis-go/internal/server"
)

// makeRelease menulis direktori rilis dengan satu biner palsu (isi dikenal) untuk linux/amd64.
func makeRelease(t *testing.T, version string, content string) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "agent-linux-amd64"), []byte(content), 0o755)
	sum := sha256.Sum256([]byte(content))
	m, _ := json.Marshal(map[string]any{"version": version, "targets": map[string]any{
		"linux/amd64": map[string]string{"file": "agent-linux-amd64", "sha256": hex.EncodeToString(sum[:]), "signature": "c2lnbmF0dXJl"}}})
	os.WriteFile(filepath.Join(dir, "manifest.json"), m, 0o644)
	return dir
}

func setRelease(t *testing.T, version, content string) string {
	dir := makeRelease(t, version, content)
	testReleaseDir = dir
	t.Cleanup(func() { testReleaseDir = "" })
	return dir
}

type agentFixture struct {
	*scanFixture
	release string
}

func newAgentFixture(t *testing.T) *agentFixture {
	rel := setRelease(t, "1.0.0", "FAKE-AGENT-BINARY")
	f := newScanFixture(t, true)
	return &agentFixture{scanFixture: f, release: rel}
}

// agentCall memanggil API agent: hanya kredensial bearer, tanpa header CSRF dan tanpa cookie.
func (f *agentFixture) agentCall(method, path string, body any, secret string) resp {
	return f.a.do(method, "/api/v1/agent/"+path, body, "", "X-Requested-With", "", "Authorization", "Bearer "+secret)
}

type enrollOut struct {
	AgentID string `json:"agent_id"`
	Secret  string `json:"secret"`
}

func (f *agentFixture) enrollToken(t *testing.T, osName, arch, mode string) (string, string) {
	t.Helper()
	r := f.a.do("POST", "/api/v1/agents/enrollment-tokens", map[string]any{"os": osName, "arch": arch, "mode": mode}, f.admin)
	if r.Status != 201 {
		t.Fatalf("token pendaftaran: %d %s", r.Status, r.Body)
	}
	var tk struct{ ID, Token string }
	r.JSON(&tk)
	return tk.ID, tk.Token
}

func (f *agentFixture) enrollBody(token, host string) map[string]any {
	return map[string]any{"token": token, "hostname": host, "os": "linux", "arch": "x86_64", "version": "1.0.0", "protocol_version": 1}
}

func (f *agentFixture) enroll(t *testing.T, host string) (enrollOut, string) {
	t.Helper()
	_, tok := f.enrollToken(t, "linux", "x86_64", "service")
	r := f.a.do("POST", "/api/v1/agent/enroll", f.enrollBody(tok, host), "", "X-Requested-With", "")
	if r.Status != 200 {
		t.Fatalf("enroll: %d %s", r.Status, r.Body)
	}
	var out enrollOut
	r.JSON(&out)
	return out, tok
}

func TestEnrollmentTokenIsAdminOnlyValidatedAndSingleUse(t *testing.T) {
	f := newAgentFixture(t)
	body := map[string]any{"os": "linux", "arch": "x86_64", "mode": "service"}
	if r := f.a.do("POST", "/api/v1/agents/enrollment-tokens", body, f.analyst); r.Status != 403 {
		t.Fatalf("analyst tidak boleh membuat token pendaftaran: %d", r.Status)
	}
	for _, bad := range []map[string]any{{"os": "plan9", "arch": "x86_64", "mode": "service"}, {"os": "linux", "arch": "mips", "mode": "service"}, {"os": "linux", "arch": "arm64", "mode": "root"}} {
		if r := f.a.do("POST", "/api/v1/agents/enrollment-tokens", bad, f.admin); r.Status != 422 {
			t.Fatalf("%v harus 422: %d", bad, r.Status)
		}
	}
	if r := f.a.do("POST", "/api/v1/agents/enrollment-tokens", map[string]any{"os": "windows", "arch": "arm64", "mode": "service"}, f.admin); r.Status != 409 {
		t.Fatalf("platform tanpa build harus 409: %d %s", r.Status, r.Body)
	}
	r := f.a.do("POST", "/api/v1/agents/enrollment-tokens", body, f.admin)
	var tk struct {
		ID, Token, Version, SHA256 string
		FileName                   string `json:"file_name"`
	}
	r.JSON(&tk)
	sum := sha256.Sum256([]byte("FAKE-AGENT-BINARY"))
	if r.Status != 201 || !strings.HasPrefix(tk.Token, "oen_") || tk.Version != "1.0.0" || tk.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("token: %d %s", r.Status, r.Body)
	}
	var st struct {
		Used, Expired bool
		Agent         *struct{ ID string }
	}
	f.a.do("GET", "/api/v1/agents/enrollment-tokens/"+tk.ID, nil, f.admin).JSON(&st)
	if st.Used || st.Expired || st.Agent != nil {
		t.Fatalf("status awal: %+v", st)
	}
	// platform tidak cocok: token TIDAK terpakai
	wrong := f.enrollBody(tk.Token, "box1")
	wrong["os"] = "macos"
	if r := f.a.do("POST", "/api/v1/agent/enroll", wrong, "", "X-Requested-With", ""); r.Status != 400 || !contains(string(r.Body), "created for linux x86_64") {
		t.Fatalf("platform salah: %d %s", r.Status, r.Body)
	}
	f.a.do("GET", "/api/v1/agents/enrollment-tokens/"+tk.ID, nil, f.admin).JSON(&st)
	if st.Used {
		t.Fatal("token tidak boleh hangus karena pendaftaran yang gagal")
	}
	for _, bad := range []map[string]any{{"hostname": "bad host!"}, {"hostname": ""}, {"version": "dev"}, {"protocol_version": 99}} {
		b := f.enrollBody(tk.Token, "box1")
		for k, v := range bad {
			b[k] = v
		}
		if r := f.a.do("POST", "/api/v1/agent/enroll", b, "", "X-Requested-With", ""); r.Status != 422 && r.Status != 426 {
			t.Fatalf("%v harus ditolak: %d %s", bad, r.Status, r.Body)
		}
	}
	// berhasil sekali, lalu token mati
	ok := f.a.do("POST", "/api/v1/agent/enroll", f.enrollBody(tk.Token, "box1"), "", "X-Requested-With", "")
	if ok.Status != 200 {
		t.Fatalf("enroll: %d %s", ok.Status, ok.Body)
	}
	var out enrollOut
	ok.JSON(&out)
	if !strings.HasPrefix(out.Secret, "oag_") {
		t.Fatalf("kredensial: %s", ok.Body)
	}
	if r := f.a.do("POST", "/api/v1/agent/enroll", f.enrollBody(tk.Token, "box1"), "", "X-Requested-With", ""); r.Status != 400 {
		t.Fatalf("token sekali pakai: %d", r.Status)
	}
	f.a.do("GET", "/api/v1/agents/enrollment-tokens/"+tk.ID, nil, f.admin).JSON(&st)
	if !st.Used || st.Agent == nil || st.Agent.ID != out.AgentID {
		t.Fatalf("status setelah dipakai: %+v", st)
	}
	// Aset baru menyimpan identitas agent; tidak ada alamat scan atau testing permission otomatis.
	var al struct {
		Items []struct {
			Name, Source      string
			AgentState        string `json:"agent_state"`
			TestingAuthorized bool   `json:"testing_authorized"`
			Addresses         []struct{ Kind, Value string }
		}
	}
	f.a.do("GET", "/api/v1/assets?q=box1", nil, f.analyst).JSON(&al)
	if len(al.Items) != 1 || al.Items[0].Source != "agent" || al.Items[0].TestingAuthorized || al.Items[0].AgentState != "online" ||
		len(al.Items[0].Addresses) != 0 {
		t.Fatalf("aset dari agent: %+v", al)
	}
	// token kedaluwarsa
	_, tok2 := f.enrollToken(t, "linux", "x86_64", "service")
	f.a.env.Owner.Exec(context.Background(), `UPDATE agent_enrollment_tokens SET created_at=now()-interval '3 hours', expires_at=now()-interval '2 hours' WHERE used_at IS NULL`)
	if r := f.a.do("POST", "/api/v1/agent/enroll", f.enrollBody(tok2, "box2"), "", "X-Requested-With", ""); r.Status != 400 {
		t.Fatalf("token kedaluwarsa: %d", r.Status)
	}
}

func TestAgentHeartbeatUpdateOfferAndRevocation(t *testing.T) {
	f := newAgentFixture(t)
	ag, _ := f.enroll(t, "box1")
	hb := func(version string) (int, struct {
		Latest string `json:"latest_version"`
		Poll   int    `json:"poll_seconds"`
	}) {
		var out struct {
			Latest string `json:"latest_version"`
			Poll   int    `json:"poll_seconds"`
		}
		r := f.agentCall("POST", "heartbeat", map[string]any{"version": version, "protocol_version": 1}, ag.Secret)
		r.JSON(&out)
		return r.Status, out
	}
	if st, out := hb("1.0.0"); st != 200 || out.Latest != "" || out.Poll != 5 {
		t.Fatalf("versi sama: tidak ada tawaran update: %d %+v", st, out)
	}
	os.WriteFile(filepath.Join(f.release, "manifest.json"), mustManifest(t, f.release, "1.1.0", "FAKE-AGENT-BINARY"), 0o644)
	if st, out := hb("1.0.0"); st != 200 || out.Latest != "1.1.0" {
		t.Fatalf("versi lebih baru harus ditawarkan: %d %+v", st, out)
	}
	if _, out := hb("1.1.0"); out.Latest != "" {
		t.Fatalf("sudah terbaru: %+v", out)
	}
	if _, out := hb("2.0.0"); out.Latest != "" {
		t.Fatalf("agent yang lebih baru dari rilis tidak boleh diminta turun: %+v", out)
	}
	if r := f.agentCall("POST", "heartbeat", map[string]any{"version": "dev", "protocol_version": 1}, ag.Secret); r.Status != 422 {
		t.Fatalf("versi tidak valid: %d", r.Status)
	}
	if r := f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 7}, ag.Secret); r.Status != 426 {
		t.Fatalf("protokol berbeda: %d", r.Status)
	}
	if r := f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1}, "oag_salah"); r.Status != 401 {
		t.Fatalf("kredensial salah: %d", r.Status)
	}
	if r := f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1}, f.token); r.Status != 401 {
		t.Fatalf("token injector bukan kredensial agent: %d", r.Status)
	}
	// integrasi Agent terlihat hidup
	if got := integrations(t, f.a, f.analyst)["agent"]; got.Status != "online" {
		t.Fatalf("integrasi agent: %+v", got)
	}
	// daftar agent untuk konsol
	var list struct {
		Items []struct {
			ID, Status, Version string
			UpdateAvailable     bool `json:"update_available"`
		}
	}
	f.a.do("GET", "/api/v1/agents", nil, f.analyst).JSON(&list)
	if len(list.Items) != 1 || list.Items[0].Status != "online" || list.Items[0].Version != "2.0.0" {
		t.Fatalf("daftar: %+v", list)
	}
	// pencabutan: hanya admin; kredensial langsung mati
	if r := f.a.do("POST", "/api/v1/agents/"+ag.AgentID+"/revoke", nil, f.analyst); r.Status != 403 {
		t.Fatalf("analyst tidak boleh mencabut: %d", r.Status)
	}
	if r := f.a.do("POST", "/api/v1/agents/"+ag.AgentID+"/revoke", nil, f.admin); r.Status != 204 {
		t.Fatalf("cabut: %d", r.Status)
	}
	if st, _ := hb("1.0.0"); st != 401 {
		t.Fatalf("agent dicabut harus 401: %d", st)
	}
	if r := f.a.do("POST", "/api/v1/agents/"+ag.AgentID+"/revoke", nil, f.admin); r.Status != 409 {
		t.Fatalf("cabut dua kali: %d", r.Status)
	}
	f.a.do("GET", "/api/v1/agents", nil, f.analyst).JSON(&list)
	if list.Items[0].Status != "revoked" {
		t.Fatalf("status: %+v", list.Items[0])
	}
}

func mustManifest(t *testing.T, dir, version, content string) []byte {
	sum := sha256.Sum256([]byte(content))
	m, _ := json.Marshal(map[string]any{"version": version, "targets": map[string]any{
		"linux/amd64": map[string]string{"file": "agent-linux-amd64", "sha256": hex.EncodeToString(sum[:]), "signature": "c2ln"}}})
	return m
}

func TestAgentDownloadServesOnlyTheCallersPlatformToTheRightCredentials(t *testing.T) {
	f := newAgentFixture(t)
	_, tok := f.enrollToken(t, "linux", "x86_64", "user")
	get := func(secret string, query string) *http.Response {
		req, _ := http.NewRequest("GET", f.a.srv.URL+"/api/v1/agent/download"+query, nil)
		if secret != "" {
			req.Header.Set("Authorization", "Bearer "+secret)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := get(tok, "")
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	sum := sha256.Sum256([]byte("FAKE-AGENT-BINARY"))
	if res.StatusCode != 200 || string(body) != "FAKE-AGENT-BINARY" || res.Header.Get("X-Release-Version") != "1.0.0" || res.Header.Get("X-Sha256") != hex.EncodeToString(sum[:]) || res.Header.Get("X-Signature-Ed25519") == "" {
		t.Fatalf("unduh dengan token pendaftaran: %d %q %v", res.StatusCode, body, res.Header)
	}
	for name, secret := range map[string]string{"tanpa kredensial": "", "token salah": "oen_salah", "token pengguna": "oat_x", "injector": f.token} {
		if r := get(secret, ""); r.StatusCode != 401 {
			t.Fatalf("%s harus 401: %d", name, r.StatusCode)
		}
	}
	if r := get(tok, "?version=9.9.9"); r.StatusCode != 409 {
		t.Fatalf("versi yang diminta berbeda: %d", r.StatusCode)
	}
	// token yang sudah dipakai tidak bisa mengunduh lagi; agent terdaftar memakai kredensialnya sendiri
	b := f.enrollBody(tok, "box9")
	var out enrollOut
	f.a.do("POST", "/api/v1/agent/enroll", b, "", "X-Requested-With", "").JSON(&out)
	if r := get(tok, ""); r.StatusCode != 401 {
		t.Fatalf("token terpakai: %d", r.StatusCode)
	}
	if r := get(out.Secret, "?version=1.0.0"); r.StatusCode != 200 {
		t.Fatalf("agent memperbarui diri: %d", r.StatusCode)
	}
	f.a.do("POST", "/api/v1/agents/"+out.AgentID+"/revoke", nil, f.admin)
	if r := get(out.Secret, ""); r.StatusCode != 401 {
		t.Fatalf("agent dicabut tidak boleh mengunduh: %d", r.StatusCode)
	}
}

func TestMalformedReleaseManifestIsRefusedNotServed(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "evil"), []byte("x"), 0o755)
	sum := sha256.Sum256([]byte("x"))
	h := hex.EncodeToString(sum[:])
	cases := map[string]string{
		"path traversal":     `{"version":"1.0.0","targets":{"linux/amd64":{"file":"../evil","sha256":"` + h + `","signature":"c2ln"}}}`,
		"versi buruk":        `{"version":"1.0","targets":{}}`,
		"tanpa tanda tangan": `{"version":"1.0.0","targets":{"linux/amd64":{"file":"evil","sha256":"` + h + `","signature":""}}}`,
		"sha buruk":          `{"version":"1.0.0","targets":{"linux/amd64":{"file":"evil","sha256":"zz","signature":"c2ln"}}}`,
		"berkas hilang":      `{"version":"1.0.0","targets":{"linux/amd64":{"file":"nothere","sha256":"` + h + `","signature":"c2ln"}}}`,
	}
	for name, body := range cases {
		os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(body), 0o644)
		if _, err := (&agents.Release{Dir: dir}).Manifest(); err == nil {
			t.Errorf("%s: manifest harus ditolak", name)
		}
	}
	if _, err := (&agents.Release{Dir: t.TempDir()}).Manifest(); err == nil {
		t.Error("direktori tanpa manifest harus dianggap tidak ada rilis")
	}
}

func TestAgentRunsAtomicTestsOnItsOwnAssetOnly(t *testing.T) {
	f := newAgentFixture(t)
	// aset sasaran tanpa agent -> 422
	var tst struct{ ID string }
	mk := func(asset string) string {
		var x struct{ ID string }
		r := f.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": "ports", "contract_key": "agent.listening_ports", "target_asset_id": asset}, f.analyst)
		if r.Status != 201 {
			t.Fatalf("buat test: %d %s", r.Status, r.Body)
		}
		r.JSON(&x)
		return x.ID
	}
	tst.ID = mk(f.asset)
	if r := f.a.do("POST", "/api/v1/atomic-tests/"+tst.ID+"/run", nil, f.analyst); r.Status != 422 || !contains(string(r.Body), "no agent") {
		t.Fatalf("aset tanpa agent: %d %s", r.Status, r.Body)
	}
	// parameter tidak diterima oleh aksi agent
	if r := f.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": "x", "contract_key": "agent.system_info", "params": map[string]any{"cmd": "rm -rf /"}, "target_asset_id": f.asset}, f.analyst); r.Status != 422 && r.Status != 400 {
		t.Fatalf("aksi agent tidak boleh menerima parameter: %d", r.Status)
	}
	ag, _ := f.enroll(t, "box1")
	var assetID string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT asset_id FROM agents WHERE id=$1`, ag.AgentID).Scan(&assetID)
	// belum diotorisasi
	t2 := mk(assetID)
	if r := f.a.do("POST", "/api/v1/atomic-tests/"+t2+"/run", nil, f.analyst); r.Status != 422 || !contains(string(r.Body), "authorized") {
		t.Fatalf("belum diotorisasi: %d %s", r.Status, r.Body)
	}
	var cur struct{ Version int }
	f.a.do("GET", "/api/v1/assets/"+assetID, nil, f.admin).JSON(&cur)
	f.a.do("PATCH", "/api/v1/assets/"+assetID, map[string]any{"version": cur.Version, "testing_authorized": true}, f.admin)
	// agent offline -> 409
	f.a.env.Owner.Exec(context.Background(), `UPDATE agents SET last_heartbeat_at = now() - interval '10 minutes' WHERE id=$1`, ag.AgentID)
	if r := f.a.do("POST", "/api/v1/atomic-tests/"+t2+"/run", nil, f.analyst); r.Status != 409 || !contains(string(r.Body), "offline") {
		t.Fatalf("agent offline: %d %s", r.Status, r.Body)
	}
	f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1}, ag.Secret)
	before := f.outbox(t, execution_TopicDispatch)
	r := f.a.do("POST", "/api/v1/atomic-tests/"+t2+"/run", nil, f.analyst)
	if r.Status != 202 {
		t.Fatalf("run: %d %s", r.Status, r.Body)
	}
	var ex execJSON
	r.JSON(&ex)
	if f.outbox(t, execution_TopicDispatch) != before {
		t.Fatal("eksekusi agent tidak boleh membuat pesan antrean (agent mengambil sendiri)")
	}
	var kind, tgt string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT executor_kind, target_address FROM executions WHERE id=$1`, ex.ID).Scan(&kind, &tgt)
	if kind != "agent" || tgt != "local" {
		t.Fatalf("eksekusi: %s %s", kind, tgt)
	}
	// agent lain dan injector tidak bisa mengambil/menyelesaikannya
	other, _ := f.enroll(t, "box2")
	if r := f.agentCall("POST", "executions/claim", nil, other.Secret); r.Status != 204 {
		t.Fatalf("agent lain tidak punya tugas: %d", r.Status)
	}
	if r := f.agentCall("POST", "executions/"+ex.ID+"/result", okEmpty, other.Secret); r.Status != 409 && r.Status != 404 {
		t.Fatalf("agent lain tidak boleh menyelesaikan: %d", r.Status)
	}
	if r := f.inj("POST", "executions/"+ex.ID+"/claim", nil, f.token); r.Status != 404 {
		t.Fatalf("injector tidak boleh mengambil tugas agent: %d", r.Status)
	}
	// pemilik mengambil, tidak bisa diambil dua kali, hasil diterima sekali
	cl := f.agentCall("POST", "executions/claim", nil, ag.Secret)
	var spec struct {
		ExecutionID string `json:"execution_id"`
		ContractKey string `json:"contract_key"`
		Target      string
	}
	cl.JSON(&spec)
	if cl.Status != 200 || spec.ExecutionID != ex.ID || spec.ContractKey != "agent.listening_ports" || spec.Target != "local" {
		t.Fatalf("claim: %d %s", cl.Status, cl.Body)
	}
	if r := f.agentCall("POST", "executions/claim", nil, ag.Secret); r.Status != 204 {
		t.Fatalf("tidak ada tugas lagi: %d", r.Status)
	}
	res := map[string]any{"exit_code": 0, "findings": []map[string]any{
		{"type": "open_port", "value": "tcp/8080", "title": "Open port 8080/tcp (listening)", "severity": "info"},
		{"type": "vulnerability", "value": "x", "title": "agent tidak boleh mengaku menemukan kerentanan", "severity": "critical"}}}
	if r := f.agentCall("POST", "executions/"+ex.ID+"/result", res, ag.Secret); r.Status != 204 {
		t.Fatalf("hasil: %d %s", r.Status, r.Body)
	}
	if r := f.agentCall("POST", "executions/"+ex.ID+"/result", res, ag.Secret); r.Status != 409 {
		t.Fatalf("hasil kedua: %d", r.Status)
	}
	got := f.exec(t, ex.ID)
	if got.Outcome != "success" || len(got.Findings) != 1 || got.Findings[0].Value != "tcp/8080" {
		t.Fatalf("jenis di luar kontrak ditolak: %+v", got)
	}
	// finding terikat ke aset agent itu sendiri
	var fl struct {
		Items []struct {
			AssetID string `json:"asset_id"`
		}
	}
	f.a.do("GET", "/api/v1/findings", nil, f.viewer).JSON(&fl)
	if len(fl.Items) != 1 || fl.Items[0].AssetID != assetID {
		t.Fatalf("finding harus terikat ke aset agent: %+v", fl)
	}
}

const execution_TopicDispatch = "exec.dispatch"

func TestRevokingAnAgentCancelsItsPendingWork(t *testing.T) {
	f := newAgentFixture(t)
	ag, _ := f.enroll(t, "box1")
	var assetID string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT asset_id FROM agents WHERE id=$1`, ag.AgentID).Scan(&assetID)
	var cur struct{ Version int }
	f.a.do("GET", "/api/v1/assets/"+assetID, nil, f.admin).JSON(&cur)
	f.a.do("PATCH", "/api/v1/assets/"+assetID, map[string]any{"version": cur.Version, "testing_authorized": true}, f.admin)
	var tst struct{ ID string }
	f.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": "info", "contract_key": "agent.system_info", "target_asset_id": assetID}, f.analyst).JSON(&tst)
	var ex execJSON
	f.a.do("POST", "/api/v1/atomic-tests/"+tst.ID+"/run", nil, f.analyst).JSON(&ex)
	f.agentCall("POST", "executions/claim", nil, ag.Secret)
	var hb struct{ Cancel bool }
	f.agentCall("POST", "executions/"+ex.ID+"/heartbeat", nil, ag.Secret).JSON(&hb)
	if hb.Cancel {
		t.Fatal("eksekusi berjalan tidak boleh dibatalkan")
	}
	f.a.do("POST", "/api/v1/agents/"+ag.AgentID+"/revoke", nil, f.admin)
	if got := f.exec(t, ex.ID); got.Outcome != "canceled" {
		t.Fatalf("pencabutan harus membatalkan eksekusi: %+v", got)
	}
	if r := f.agentCall("POST", "executions/"+ex.ID+"/result", okEmpty, ag.Secret); r.Status != 401 {
		t.Fatalf("agent dicabut ditolak di semua endpoint: %d", r.Status)
	}
	// setelah dicabut, aset tidak punya agent aktif lagi
	if r := f.a.do("POST", "/api/v1/atomic-tests/"+tst.ID+"/run", nil, f.analyst); r.Status != 422 {
		t.Fatalf("tanpa agent aktif: %d %s", r.Status, r.Body)
	}
	// pemasangan ulang di mesin yang sama memakai kembali asetnya
	again, _ := f.enroll(t, "box1")
	var again2 string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT asset_id FROM agents WHERE id=$1`, again.AgentID).Scan(&again2)
	if again2 != assetID {
		t.Fatalf("pasang ulang harus memakai kembali aset yang sama: %s vs %s", again2, assetID)
	}
}

func TestAgentSweepMarksOfflineAndRaisesOneAlertPerEpisode(t *testing.T) {
	f := newAgentFixture(t)
	ag, _ := f.enroll(t, "box1")
	svc := server.AgentService(server.Options{DB: f.a.env.App, Tenant: f.a.env.TenantA})
	ctx := context.Background()
	if n, err := svc.Sweep(ctx); err != nil || n != 0 {
		t.Fatalf("agent segar: n=%d err=%v", n, err)
	}
	f.a.env.Owner.Exec(ctx, `UPDATE agents SET last_heartbeat_at = now() - interval '20 minutes' WHERE id=$1`, ag.AgentID)
	if n, err := svc.Sweep(ctx); err != nil || n != 1 {
		t.Fatalf("agent senyap: n=%d err=%v", n, err)
	}
	if n, _ := svc.Sweep(ctx); n != 0 {
		t.Fatalf("sudah offline: tidak diproses lagi: %d", n)
	}
	l := getAlerts(t, f.a, f.analyst, "")
	if len(l.Items) != 1 || l.Items[0].Kind != "agent_offline" {
		t.Fatalf("satu alert offline: %+v", l)
	}
	// kembali hidup lalu senyap lagi = episode baru = alert baru
	f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1}, ag.Secret)
	var st string
	f.a.env.Owner.QueryRow(ctx, `SELECT status FROM agents WHERE id=$1`, ag.AgentID).Scan(&st)
	if st != "online" {
		t.Fatalf("heartbeat harus mengembalikan online: %s", st)
	}
	f.a.env.Owner.Exec(ctx, `UPDATE agents SET last_heartbeat_at = now() - interval '21 minutes' WHERE id=$1`, ag.AgentID)
	svc.Sweep(ctx)
	if n := len(getAlerts(t, f.a, f.analyst, "").Items); n != 2 {
		t.Fatalf("episode baru harus membuat alert baru: %d", n)
	}
	// status di daftar dihitung dari detak, bukan dari kolom yang bisa basi
	var list struct{ Items []struct{ Status string } }
	f.a.do("GET", "/api/v1/agents", nil, f.analyst).JSON(&list)
	if list.Items[0].Status != "offline" {
		t.Fatalf("daftar: %+v", list)
	}
}

func TestChainStepsCanRunOnAgents(t *testing.T) {
	f := newAgentFixture(t)
	c := &chainFixture{scanFixture: f.scanFixture}
	c.nucleiToken = ""
	var sc struct {
		ID      string
		Version int
	}
	f.a.do("POST", "/api/v1/scenarios", map[string]any{"name": "On-host", "severity": "low"}, f.analyst).JSON(&sc)
	c.scenario, c.version = sc.ID, sc.Version
	ag, _ := f.enroll(t, "box1")
	var assetID string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT asset_id FROM agents WHERE id=$1`, ag.AgentID).Scan(&assetID)
	var cur struct{ Version int }
	f.a.do("GET", "/api/v1/assets/"+assetID, nil, f.admin).JSON(&cur)
	f.a.do("PATCH", "/api/v1/assets/"+assetID, map[string]any{"version": cur.Version, "testing_authorized": true}, f.admin)
	// graf: ports -> info (agent) ; target = aset agent, tetapi launch pertama ke aset TANPA agent harus ditolak
	steps := []map[string]any{step("ports", "agent.listening_ports", "after_parents", nil), step("info", "agent.system_info", "after_parents", nil)}
	if r := c.targets(t, []string{f.asset}, nil); r.Status != 200 {
		t.Fatalf("targets: %d %s", r.Status, r.Body)
	}
	if r := c.graph(t, steps, [][2]string{{"ports", "info"}}); r.Status != 200 {
		t.Fatalf("graph: %d %s", r.Status, r.Body)
	}
	r := f.a.do("POST", "/api/v1/scenarios/"+c.scenario+"/launch", map[string]any{}, f.analyst)
	if r.Status != 422 || !contains(string(r.Body), "has no agent") {
		t.Fatalf("aset tanpa agent harus ditolak di muka: %d %s", r.Status, r.Body)
	}
	c.targets(t, []string{assetID}, nil)
	before := f.outbox(t, execution_TopicDispatch)
	sim := c.launch(t)
	if f.outbox(t, execution_TopicDispatch) != before {
		t.Fatal("langkah agent tidak boleh membuat pesan antrean")
	}
	runNext := func(findings []map[string]any) {
		cl := f.agentCall("POST", "executions/claim", nil, ag.Secret)
		var spec struct {
			ExecutionID string `json:"execution_id"`
		}
		cl.JSON(&spec)
		if cl.Status != 200 {
			t.Fatalf("claim: %d %s", cl.Status, cl.Body)
		}
		if r := f.agentCall("POST", "executions/"+spec.ExecutionID+"/result", map[string]any{"exit_code": 0, "findings": findings}, ag.Secret); r.Status != 204 {
			t.Fatalf("hasil: %d %s", r.Status, r.Body)
		}
	}
	if got := c.sim(t, sim.ID); got.status("ports") != "queued" || got.status("info") != "waiting" {
		t.Fatalf("awal: %+v", got.Steps)
	}
	runNext([]map[string]any{{"type": "open_port", "value": "tcp/22", "title": "Open port 22/tcp", "severity": "info"}})
	if got := c.sim(t, sim.ID); got.status("ports") != "done" || got.status("info") != "queued" {
		t.Fatalf("setelah ports: %+v", got.Steps)
	}
	runNext([]map[string]any{{"type": "text", "value": "system:box1", "title": "System information", "severity": "info"}})
	if got := c.sim(t, sim.ID); got.Status != "finished" {
		t.Fatalf("simulasi agent harus selesai: %+v", got)
	}
	// agent dicabut di tengah simulasi: langkah yang tersisa gagal dan simulasi tetap berakhir (tidak menggantung)
	sim2 := c.launch(t)
	f.a.do("POST", "/api/v1/agents/"+ag.AgentID+"/revoke", nil, f.admin)
	got := c.sim(t, sim2.ID)
	if got.status("ports") != "canceled" || got.Status != "finished" {
		t.Fatalf("pencabutan di tengah simulasi: %+v", got)
	}
}

// Runtime.Claim sendiri harus menolak eksekusi milik agent lain (pertahanan berlapis; endpoint HTTP hanya menawarkan ClaimNext).
func TestRuntimeClaimRefusesAnotherAgentsExecution(t *testing.T) {
	f := newAgentFixture(t)
	a1, _ := f.enroll(t, "box1")
	a2, _ := f.enroll(t, "box2")
	var asset1, integ, ag2 string
	ctx := context.Background()
	f.a.env.Owner.QueryRow(ctx, `SELECT asset_id FROM agents WHERE id=$1`, a1.AgentID).Scan(&asset1)
	f.a.env.Owner.QueryRow(ctx, `SELECT id FROM integrations WHERE key='agent'`).Scan(&integ)
	ag2 = a2.AgentID
	var cur struct{ Version int }
	f.a.do("GET", "/api/v1/assets/"+asset1, nil, f.admin).JSON(&cur)
	f.a.do("PATCH", "/api/v1/assets/"+asset1, map[string]any{"version": cur.Version, "testing_authorized": true}, f.admin)
	var tst struct{ ID string }
	f.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": "info", "contract_key": "agent.system_info", "target_asset_id": asset1}, f.analyst).JSON(&tst)
	var ex execJSON
	f.a.do("POST", "/api/v1/atomic-tests/"+tst.ID+"/run", nil, f.analyst).JSON(&ex)
	rt := server.Runtime(server.Options{DB: f.a.env.App, Tenant: f.a.env.TenantA})
	other := mustAgentWorker(t, integ, ag2)
	if _, err := rt.Claim(ctx, other, mustUUID(t, ex.ID)); err == nil {
		t.Fatal("agent lain tidak boleh mengambil eksekusi milik agent pertama")
	}
	if got := f.exec(t, ex.ID); got.State != "ready" {
		t.Fatalf("eksekusi harus tetap ready: %+v", got)
	}
}

// Agent dicabut SETELAH peluncuran tetapi sebelum langkah agent dimulai: langkah itu gagal dengan jelas, simulasi tidak menggantung.
func TestAgentStepFailsClearlyWhenTheAgentVanishedBeforeItStarts(t *testing.T) {
	f := newAgentFixture(t)
	c := &chainFixture{scanFixture: f.scanFixture}
	ag, _ := f.enroll(t, "box1")
	ctx := context.Background()
	var assetID string
	f.a.env.Owner.QueryRow(ctx, `SELECT asset_id FROM agents WHERE id=$1`, ag.AgentID).Scan(&assetID)
	// aset ini juga punya alamat IP agar langkah nmap bisa berjalan
	f.a.env.Owner.Exec(ctx, `INSERT INTO asset_addresses (id, tenant_id, asset_id, kind, value) SELECT gen_random_uuid(), tenant_id, id, 'ip', '127.0.0.1' FROM assets WHERE id=$1`, assetID)
	var cur struct{ Version int }
	f.a.do("GET", "/api/v1/assets/"+assetID, nil, f.admin).JSON(&cur)
	f.a.do("PATCH", "/api/v1/assets/"+assetID, map[string]any{"version": cur.Version, "testing_authorized": true}, f.admin)
	var sc struct {
		ID      string
		Version int
	}
	f.a.do("POST", "/api/v1/scenarios", map[string]any{"name": "Mixed", "severity": "low"}, f.analyst).JSON(&sc)
	c.scenario, c.version = sc.ID, sc.Version
	c.targets(t, []string{assetID}, nil)
	if r := c.graph(t, []map[string]any{step("scan", nmapC, "after_parents", nil), step("info", "agent.system_info", "after_parents", nil)}, [][2]string{{"scan", "info"}}); r.Status != 200 {
		t.Fatalf("graph: %d %s", r.Status, r.Body)
	}
	sim := c.launch(t)
	f.a.do("POST", "/api/v1/agents/"+ag.AgentID+"/revoke", nil, f.admin)
	c.finish(t, c.execsOf(t, sim.ID, "scan")[0], nmapC, okEmpty)
	got := c.sim(t, sim.ID)
	if got.status("scan") != "done" || got.status("info") != "failed" || got.Status != "finished" {
		t.Fatalf("langkah agent tanpa agent harus gagal dan simulasi tetap selesai: %+v", got.Steps)
	}
	var msg string
	f.a.env.Owner.QueryRow(ctx, `SELECT error FROM executions WHERE id=$1`, c.execsOf(t, sim.ID, "info")[0]).Scan(&msg)
	if !contains(msg, "no agent") {
		t.Fatalf("alasan harus jelas: %q", msg)
	}
}
