package server_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/google/uuid"
	"io"
	"net/http"
	"orchis-go/internal/execution"
	"orchis-go/internal/osquery"
	"orchis-go/internal/server"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// agentTask membuat dan mengklaim satu eksekusi aksi agent pada aset agent itu sendiri; mengembalikan id eksekusi.
func (f *agentFixture) agentTask(t *testing.T, secret, assetID, contractKey string) string {
	t.Helper()
	r := f.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": contractKey, "contract_key": contractKey, "target_asset_id": assetID}, f.analyst)
	if r.Status != 201 {
		t.Fatalf("buat test %s: %d %s", contractKey, r.Status, r.Body)
	}
	var tst struct{ ID string }
	r.JSON(&tst)
	f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1}, secret)
	r = f.a.do("POST", "/api/v1/atomic-tests/"+tst.ID+"/run", nil, f.analyst)
	if r.Status != 202 {
		t.Fatalf("run %s: %d %s", contractKey, r.Status, r.Body)
	}
	var ex execJSON
	r.JSON(&ex)
	if cl := f.agentCall("POST", "executions/claim", nil, secret); cl.Status != 200 {
		t.Fatalf("claim %s: %d %s", contractKey, cl.Status, cl.Body)
	}
	return ex.ID
}

func (f *agentFixture) authorizedAgent(t *testing.T, host string) (secret, assetID string) {
	t.Helper()
	ag, _ := f.enroll(t, host)
	f.a.env.Owner.QueryRow(context.Background(), `SELECT asset_id FROM agents WHERE id=$1`, ag.AgentID).Scan(&assetID)
	var cur struct{ Version int }
	f.a.do("GET", "/api/v1/assets/"+assetID, nil, f.admin).JSON(&cur)
	f.a.do("PATCH", "/api/v1/assets/"+assetID, map[string]any{"version": cur.Version, "testing_authorized": true}, f.admin)
	return ag.Secret, assetID
}

// approvedAgent mendaftarkan agent, memasukkan asetnya ke organisasi, dan menyetujui query sensitif yang diberikan.
func (f *agentFixture) approvedAgent(t *testing.T, host string, keys ...string) (secret, assetID string) {
	t.Helper()
	secret, assetID = f.authorizedAgent(t, host)
	org := f.joinOrg(t, assetID)
	for _, k := range keys {
		if r := f.a.do("PUT", "/api/v1/organizations/"+org+"/osquery/"+k, map[string]any{"enabled": true, "consent": osquery.Consent().ConsentRef}, f.admin); r.Status != 204 {
			t.Fatalf("setujui %s: %d %s", k, r.Status, r.Body)
		}
	}
	return
}

func TestOsqueryResultFromAgentIsSanitizedStoredAndMapped(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.approvedAgent(t, "box1", "osquery.users")
	ex := f.agentTask(t, secret, asset, "osquery.users")
	hostile := map[string]any{
		"exit_code": 0,
		// Agent mencoba menyelundupkan temuan kerentanan kritis lewat jalur lama: kontrak tidak mengizinkan tipe itu.
		"output":   "raw-secret-marker",
		"logs":     []map[string]any{{"level": "info", "message": "log-secret-marker"}},
		"findings": []map[string]any{{"type": "text", "value": "forged-text", "title": "forged allowed-type finding", "severity": "critical"}, {"type": "vulnerability", "value": "evil", "title": "agent claims a critical vulnerability", "severity": "critical"}},
		"datasets": []map[string]any{{"query": "osquery.users", "status": "ok", "rows": []map[string]any{
			{"username": "root", "uid": "0", "shell": "/bin/bash", "groups": "root", "password_hash": "$6$leak", "ssh_key": "AAAA"},
			{"username": "toor", "uid": "0", "shell": "/bin/sh", "groups": "root"},
			{"username": "dev", "uid": "1000", "shell": "token=supersecret", "groups": "sudo"},
		}}},
	}
	if r := f.agentCall("POST", "executions/"+ex+"/result", hostile, secret); r.Status != 204 {
		t.Fatalf("hasil: %d %s", r.Status, r.Body)
	}
	got := f.exec(t, ex)
	if got.Outcome != "success" {
		t.Fatalf("outcome: %+v", got)
	}
	var values []string
	for _, fi := range got.Findings {
		values = append(values, fi.Type+":"+fi.Value+":"+fi.Severity)
	}
	all := strings.Join(values, ";")
	if !strings.Contains(all, "text:osquery:uid0:toor:high") || strings.Contains(all, "evil") || strings.Contains(all, "vulnerability") || strings.Contains(all, "forged-text") {
		t.Fatalf("temuan harus berasal dari aturan beku, bukan dari klaim agent: %v", values)
	}
	if n := rowCount(t, f, `SELECT count(*) FROM executions WHERE id=$1 AND raw_output_key IS NOT NULL`, ex); n != 0 {
		t.Fatal("osquery tidak boleh menyimpan output mentah")
	}
	if n := rowCount(t, f, `SELECT count(*) FROM execution_logs WHERE execution_id=$1 AND message LIKE '%log-secret-marker%'`, ex); n != 0 {
		t.Fatal("log bebas agent harus dibuang")
	}
	var title string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT title FROM findings WHERE asset_id=$1 AND value='osquery:uid0:toor'`, asset).Scan(&title)
	if title != "Account toor has superuser (UID 0) rights" {
		t.Fatalf("judul temuan: %q", title)
	}
	var rowsJSON, attrs string
	var status string
	if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT rows::text, attributes::text, last_status FROM endpoint_datasets WHERE asset_id=$1 AND query_key='osquery.users'`, asset).Scan(&rowsJSON, &attrs, &status); err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"password_hash", "$6$leak", "ssh_key", "AAAA", "supersecret"} {
		if strings.Contains(rowsJSON, leaked) {
			t.Fatalf("data tersimpan membocorkan %q: %s", leaked, rowsJSON)
		}
	}
	if status != "ok" || !strings.Contains(rowsJSON, "[redacted]") || !strings.Contains(attrs, `"local_account_count": 3`) {
		t.Fatalf("status/atribut: %s %s %s", status, attrs, rowsJSON)
	}
}

func TestOsqueryDatasetForAnotherQueryOrInvalidStatusIsRejected(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.approvedAgent(t, "box1", "osquery.users", "osquery.network")
	ex := f.agentTask(t, secret, asset, "osquery.users")
	// Dataset milik query lain tidak dipakai: dianggap tidak ada data, dan hasilnya tercatat gagal.
	body := map[string]any{"exit_code": 0, "datasets": []map[string]any{{"query": "osquery.network", "status": "ok", "rows": []map[string]any{{"port": "22", "address": "0.0.0.0", "protocol": "6", "process": "sshd"}}}}}
	if r := f.agentCall("POST", "executions/"+ex+"/result", body, secret); r.Status != 204 {
		t.Fatalf("hasil: %d %s", r.Status, r.Body)
	}
	var status, reason string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT last_status, last_reason FROM endpoint_datasets WHERE asset_id=$1 AND query_key='osquery.users'`, asset).Scan(&status, &reason)
	if status != "failed" || reason != "the agent returned no data" {
		t.Fatalf("tanpa dataset yang cocok harus tercatat gagal (tidak ada data): %q %q", status, reason)
	}
	if n := rowCount(t, f, `SELECT count(*) FROM endpoint_datasets WHERE asset_id=$1 AND query_key='osquery.network'`, asset); n != 0 {
		t.Fatal("dataset query lain tidak boleh tersimpan")
	}
	// Status asing ditolak sanitasi.
	ex2 := f.agentTask(t, secret, asset, "osquery.network")
	bad := map[string]any{"exit_code": 0, "datasets": []map[string]any{{"query": "osquery.network", "status": "owned", "rows": []map[string]any{}}}}
	f.agentCall("POST", "executions/"+ex2+"/result", bad, secret)
	f.a.env.Owner.QueryRow(context.Background(), `SELECT last_status FROM endpoint_datasets WHERE asset_id=$1 AND query_key='osquery.network'`, asset).Scan(&status)
	if status != "failed" {
		t.Fatalf("status asing harus menjadi gagal: %q", status)
	}
}

func rowCount(t *testing.T, f *agentFixture, q string, args ...any) int {
	t.Helper()
	var n int
	if err := f.a.env.Owner.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestOsquerySkippedAndFailedAttemptsKeepLastGoodData(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.approvedAgent(t, "box1", "osquery.firewall")
	ok := map[string]any{"exit_code": 0, "datasets": []map[string]any{{"query": "osquery.firewall", "status": "ok", "rows": []map[string]any{{"rules": "7"}}}}}
	f.agentCall("POST", "executions/"+f.agentTask(t, secret, asset, "osquery.firewall")+"/result", ok, secret)
	skip := map[string]any{"exit_code": 0, "datasets": []map[string]any{{"query": "osquery.firewall", "status": "skipped", "reason": "needs more privileges (administrator rights)", "rows": []map[string]any{{"rules": "999"}}}}}
	exSkip := f.agentTask(t, secret, asset, "osquery.firewall")
	f.agentCall("POST", "executions/"+exSkip+"/result", skip, secret)
	var rows, status, reason string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT rows::text, last_status, last_reason FROM endpoint_datasets WHERE asset_id=$1 AND query_key='osquery.firewall'`, asset).Scan(&rows, &status, &reason)
	if status != "skipped" || !strings.Contains(reason, "privileges") || !strings.Contains(rows, `"7"`) || strings.Contains(rows, "999") {
		t.Fatalf("dilewati harus mencatat alasan dan mempertahankan data baik terakhir: %s %s %s", status, reason, rows)
	}
	if got := f.exec(t, exSkip); got.Outcome != "success" || len(got.Findings) != 0 || !logHas(got.Logs, "Skipped: needs more privileges") {
		t.Fatalf("dilewati bukan galat, tanpa temuan, tercatat di log: %+v", got)
	}
	// Eksekusi gagal (error agent) mencatat status failed tetapi data baik tetap.
	exFail := f.agentTask(t, secret, asset, "osquery.firewall")
	f.agentCall("POST", "executions/"+exFail+"/result", map[string]any{"exit_code": 1, "error": "osquery is not installed"}, secret)
	f.a.env.Owner.QueryRow(context.Background(), `SELECT rows::text, last_status, last_reason FROM endpoint_datasets WHERE asset_id=$1 AND query_key='osquery.firewall'`, asset).Scan(&rows, &status, &reason)
	if status != "failed" || !strings.Contains(reason, "not installed") || !strings.Contains(rows, `"7"`) {
		t.Fatalf("gagal: %s %s %s", status, reason, rows)
	}
}

func logHas(logs []struct{ Level, Message string }, sub string) bool {
	for _, l := range logs {
		if strings.Contains(l.Message, sub) {
			return true
		}
	}
	return false
}

func writeTools(t *testing.T, dir string, manifest string, files map[string]string) {
	t.Helper()
	td := filepath.Join(dir, "tools")
	if err := os.MkdirAll(td, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		os.WriteFile(filepath.Join(td, name), []byte(content), 0o644)
	}
	os.WriteFile(filepath.Join(td, "manifest.json"), []byte(manifest), 0o644)
}

func TestAgentToolDownloadServesOnlyTheCallersPlatformToAnAgent(t *testing.T) {
	f := newAgentFixture(t)
	ag, _ := f.enroll(t, "box1")
	sum := sha256.Sum256([]byte("FAKE-OSQUERY-TARBALL"))
	good := `{"tools":{"osquery":{"version":"5.24.0","targets":{"linux/amd64":{"file":"osq.tgz","sha256":"` + hex.EncodeToString(sum[:]) + `","signature":"c2ln"}}}}}`
	writeTools(t, f.release, good, map[string]string{"osq.tgz": "FAKE-OSQUERY-TARBALL"})
	get := func(secret, query string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", f.a.srv.URL+"/api/v1/agent/tool"+query, nil)
		if secret != "" {
			req.Header.Set("Authorization", "Bearer "+secret)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return res, string(b)
	}
	res, body := get(ag.Secret, "?name=osquery&version=5.24.0")
	if res.StatusCode != 200 || body != "FAKE-OSQUERY-TARBALL" || res.Header.Get("X-Tool-Version") != "5.24.0" || res.Header.Get("X-Sha256") != hex.EncodeToString(sum[:]) || res.Header.Get("X-Signature-Ed25519") != "c2ln" {
		t.Fatalf("unduh alat: %d %q %v", res.StatusCode, body, res.Header)
	}
	for name, secret := range map[string]string{"tanpa kredensial": "", "token pengguna": "oat_x", "token pendaftaran": "oen_x", "injector": f.token} {
		if r, _ := get(secret, "?name=osquery"); r.StatusCode != 401 {
			t.Errorf("%s harus 401: %d", name, r.StatusCode)
		}
	}
	if r, _ := get(ag.Secret, "?name=curl"); r.StatusCode != 404 {
		t.Errorf("alat di luar daftar izin harus 404: %d", r.StatusCode)
	}
	if r, _ := get(ag.Secret, "?name=../manifest"); r.StatusCode != 404 {
		t.Errorf("nama berbahaya harus 404: %d", r.StatusCode)
	}
	if r, _ := get(ag.Secret, "?name=osquery&version=4.0.0"); r.StatusCode != 409 {
		t.Errorf("versi berbeda harus 409: %d", r.StatusCode)
	}
	// Manifest alat yang rusak atau menunjuk jalur di luar direktori tidak pernah disajikan.
	for name, bad := range map[string]string{
		"traversal":          `{"tools":{"osquery":{"version":"5.24.0","targets":{"linux/amd64":{"file":"../manifest.json","sha256":"` + hex.EncodeToString(sum[:]) + `","signature":"c2ln"}}}}}`,
		"alat tak dikenal":   `{"tools":{"curl":{"version":"1","targets":{}}}}`,
		"hash buruk":         `{"tools":{"osquery":{"version":"5.24.0","targets":{"linux/amd64":{"file":"osq.tgz","sha256":"zz","signature":"c2ln"}}}}}`,
		"tanpa tanda tangan": `{"tools":{"osquery":{"version":"5.24.0","targets":{"linux/amd64":{"file":"osq.tgz","sha256":"` + hex.EncodeToString(sum[:]) + `","signature":""}}}}}`,
		"JSON rusak":         `{"tools":`,
	} {
		writeTools(t, f.release, bad, nil)
		if r, _ := get(ag.Secret, "?name=osquery"); r.StatusCode != 404 {
			t.Errorf("manifest %s tidak boleh disajikan: %d", name, r.StatusCode)
		}
	}
}

func (f *agentFixture) joinOrg(t *testing.T, assetID string) string {
	t.Helper()
	org := createOrg(t, f.a, f.admin, "osq-"+assetID[:8])
	var cur struct{ Version int }
	f.a.do("GET", "/api/v1/assets/"+assetID, nil, f.admin).JSON(&cur)
	body := map[string]any{"assets": []map[string]any{{"id": assetID, "version": cur.Version}}, "testing_authorized": true}
	if r := f.a.do("PUT", "/api/v1/organizations/"+org.ID.String()+"/assets", body, f.admin); r.Status != 204 {
		t.Fatalf("masuk organisasi: %d %s", r.Status, r.Body)
	}
	return org.ID.String()
}

func (f *agentFixture) datasetStatus(t *testing.T, asset, key string) (status, reason string) {
	t.Helper()
	f.a.env.Owner.QueryRow(context.Background(), `SELECT last_status, last_reason FROM endpoint_datasets WHERE asset_id=$1 AND query_key=$2`, asset, key).Scan(&status, &reason)
	return
}

func okUsers() map[string]any {
	return map[string]any{"exit_code": 0, "datasets": []map[string]any{{"query": "osquery.users", "status": "ok", "rows": []map[string]any{
		{"username": "root", "uid": "0", "shell": "/bin/bash", "groups": "root"}}}}}
}

func TestOsqueryCatalogIsReadableAndDescribesWhatItReads(t *testing.T) {
	f := newAgentFixture(t)
	var cat struct {
		OsqueryVersion string   `json:"osquery_version"`
		ProvenOS       []string `json:"proven_os"`
		Packs          []struct{ Name string }
		Queries        []struct {
			Key, Sensitivity string
			Tables           []string
			DefaultEnabled   bool `json:"default_enabled"`
			Mitre            []struct{ ID, Reason string }
			Findings         []struct{ Rule, Explain string }
			Attributes       []string
			Example          []map[string]string
			ExampleFindings  []struct{ Rule, Severity, Title string } `json:"example_findings"`
		}
	}
	r := f.a.do("GET", "/api/v1/osquery/catalog", nil, f.viewer)
	if r.Status != 200 {
		t.Fatalf("katalog: %d %s", r.Status, r.Body)
	}
	r.JSON(&cat)
	if cat.OsqueryVersion != "5.24.0" || len(cat.ProvenOS) != 1 || cat.ProvenOS[0] != "linux" || len(cat.Queries) != 7+len(osquery.PackQueryKeys()) || len(cat.Packs) != 5 {
		t.Fatalf("katalog: %+v", cat)
	}
	for _, q := range cat.Queries {
		if len(q.Tables) == 0 || len(q.Mitre) == 0 || q.Mitre[0].Reason == "" || q.DefaultEnabled {
			t.Errorf("metadata %s tidak lengkap: %+v", q.Key, q)
		}
		for _, fr := range q.Findings {
			if fr.Explain == "" {
				t.Errorf("%s/%s tanpa penjelasan", q.Key, fr.Rule)
			}
		}
	}
	for _, q := range cat.Queries {
		if len(q.Example) == 0 {
			t.Errorf("%s: contoh data wajib ada di katalog", q.Key)
		}
		if q.Key == "osquery.network" && (len(q.ExampleFindings) == 0 || q.ExampleFindings[0].Rule != "listen-all-interfaces") {
			t.Errorf("pratinjau temuan dari data contoh hilang: %+v", q.ExampleFindings)
		}
	}
	if r := f.a.do("GET", "/api/v1/osquery/catalog", nil, ""); r.Status != 401 {
		t.Fatalf("tanpa login: %d", r.Status)
	}
	// SQL tidak pernah ada di katalog yang dikirim ke klien.
	if strings.Contains(strings.ToLower(string(r.Body)), "select ") {
		t.Fatal("teks SQL tidak boleh dikirim ke klien")
	}
}

func TestOsqueryUnapprovedQueriesAreSkippedWithReasonAndApprovalIsAdminOnly(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.authorizedAgent(t, "box1")
	org := f.joinOrg(t, asset)
	// osquery.users bersensitivitas sedang: belum disetujui -> server menutupnya sebagai dilewati; agent tidak mendapat tugas.
	ex := f.createRun(t, secret, asset, "osquery.users")
	if cl := f.agentCall("POST", "executions/claim", nil, secret); cl.Status != 204 {
		t.Fatalf("tugas belum disetujui tidak boleh sampai ke agent: %d %s", cl.Status, cl.Body)
	}
	got := f.exec(t, ex)
	if got.Outcome != "success" || len(got.Findings) != 0 || !logHas(got.Logs, "not approved for this organisation") {
		t.Fatalf("harus berhasil-dilewati dengan alasan di log: %+v", got)
	}
	if st, reason := f.datasetStatus(t, asset, "osquery.users"); st != "skipped" || reason != "not approved for this organisation" {
		t.Fatalf("dataset: %q %q", st, reason)
	}
	// Persetujuan: hanya admin.
	put := func(cookie string, enabled bool) int {
		return f.a.do("PUT", "/api/v1/organizations/"+org+"/osquery/osquery.users", map[string]any{"enabled": enabled, "consent": osquery.Consent().ConsentRef}, cookie).Status
	}
	if put(f.analyst, true) != 403 || put(f.viewer, true) != 403 || put("", true) != 401 {
		t.Fatal("persetujuan hanya untuk admin")
	}
	if r := f.a.do("PUT", "/api/v1/organizations/"+org+"/osquery/osquery.nope", map[string]any{"enabled": true, "consent": osquery.Consent().ConsentRef}, f.admin); r.Status != 404 {
		t.Fatalf("query tak dikenal: %d", r.Status)
	}
	if r := f.a.do("PUT", "/api/v1/organizations/"+org+"/osquery/osquery.users", map[string]any{}, f.admin); r.Status != 422 && r.Status != 400 {
		t.Fatalf("tanpa nilai enabled: %d", r.Status)
	}
	if put(f.admin, true) != 204 {
		t.Fatal("admin harus bisa menyetujui")
	}
	var settings struct {
		Items []struct {
			Key               string
			Enabled, Explicit bool
		}
	}
	f.a.do("GET", "/api/v1/organizations/"+org+"/osquery", nil, f.viewer).JSON(&settings)
	byKey := map[string][2]bool{}
	for _, it := range settings.Items {
		byKey[it.Key] = [2]bool{it.Enabled, it.Explicit}
	}
	if len(settings.Items) != 7+len(osquery.PackQueryKeys()) || byKey["osquery.users"] != [2]bool{true, true} || byKey["osquery.network"] != [2]bool{false, false} || byKey["osquery.inventory"] != [2]bool{false, false} {
		t.Fatalf("pengaturan efektif (bawaan vs keputusan admin): %v", byKey)
	}
	// Setelah disetujui, tugas sampai ke agent dan hasilnya diproses.
	ex2 := f.createRun(t, secret, asset, "osquery.users")
	if cl := f.agentCall("POST", "executions/claim", nil, secret); cl.Status != 200 {
		t.Fatalf("setelah disetujui harus diambil agent: %d", cl.Status)
	}
	if r := f.agentCall("POST", "executions/"+ex2+"/result", okUsers(), secret); r.Status != 204 {
		t.Fatalf("hasil: %d %s", r.Status, r.Body)
	}
	if st, _ := f.datasetStatus(t, asset, "osquery.users"); st != "ok" {
		t.Fatalf("setelah disetujui dataset ok: %q", st)
	}
	// Dicabut lagi: dilewati, data baik terakhir tetap.
	if put(f.admin, false) != 204 {
		t.Fatal("admin harus bisa mencabut")
	}
	f.createRun(t, secret, asset, "osquery.users")
	if cl := f.agentCall("POST", "executions/claim", nil, secret); cl.Status != 204 {
		t.Fatalf("dicabut: agent tidak boleh mendapat tugas: %d", cl.Status)
	}
	var rows string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT rows::text FROM endpoint_datasets WHERE asset_id=$1 AND query_key='osquery.users'`, asset).Scan(&rows)
	if st, _ := f.datasetStatus(t, asset, "osquery.users"); st != "skipped" || !strings.Contains(rows, "root") {
		t.Fatalf("dicabut: status %q rows %s", st, rows)
	}
	// Jejak audit untuk setiap keputusan, termasuk tabel yang dibaca.
	var n int
	f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action='osquery.setting' AND detail->>'query'='osquery.users' AND detail ? 'tables_read'`).Scan(&n)
	if n != 2 {
		t.Fatalf("jejak audit persetujuan: %d", n)
	}
	// Sensitivitas rendah juga memerlukan persetujuan dokumen saat ini.
	f.createRun(t, secret, asset, "osquery.firewall")
	if cl := f.agentCall("POST", "executions/claim", nil, secret); cl.Status != 204 {
		t.Fatalf("query sensitivitas rendah tanpa consent harus dilewati: %d", cl.Status)
	}
}

// createRun membuat dan menjalankan eksekusi tanpa mengklaimnya (klaim dilakukan pemanggil).
func (f *agentFixture) createRun(t *testing.T, secret, assetID, contractKey string) string {
	t.Helper()
	r := f.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": contractKey + "-" + assetID[:4] + "-" + time.Now().Format("150405.000000"), "contract_key": contractKey, "target_asset_id": assetID}, f.analyst)
	if r.Status != 201 {
		t.Fatalf("buat test: %d %s", r.Status, r.Body)
	}
	var tst struct{ ID string }
	r.JSON(&tst)
	f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1}, secret)
	r = f.a.do("POST", "/api/v1/atomic-tests/"+tst.ID+"/run", nil, f.analyst)
	if r.Status != 202 {
		t.Fatalf("run: %d %s", r.Status, r.Body)
	}
	var ex execJSON
	r.JSON(&ex)
	return ex.ID
}

func TestEndpointDataAndRefreshEndpoints(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.authorizedAgent(t, "box1")
	org := f.joinOrg(t, asset)
	// Aset tanpa data: semua query "never", bendera aktif sesuai bawaan, tidak ada temuan.
	var d struct {
		HasAgent bool `json:"has_agent"`
		Items    []struct {
			Key, Status string
			Enabled     bool
			Synthetic   bool
		}
		Findings []struct{ Value, Query string }
	}
	if r := f.a.do("GET", "/api/v1/assets/"+asset+"/endpoint-data", nil, f.viewer); r.Status != 200 {
		t.Fatalf("data endpoint: %d %s", r.Status, r.Body)
	} else {
		r.JSON(&d)
	}
	if !d.HasAgent || len(d.Items) != 7+len(osquery.PackQueryKeys()) || len(d.Findings) != 0 {
		t.Fatalf("awal: %+v", d)
	}
	for _, it := range d.Items {
		if it.Status != "never" || it.Synthetic || it.Enabled {
			t.Errorf("item awal %s: %+v", it.Key, it)
		}
	}
	if r := f.a.do("GET", "/api/v1/assets/"+asset+"/endpoint-data", nil, ""); r.Status != 401 {
		t.Fatalf("tanpa login: %d", r.Status)
	}
	if r := f.a.do("GET", "/api/v1/assets/00000000-0000-0000-0000-000000000000/endpoint-data", nil, f.viewer); r.Status != 404 {
		t.Fatalf("aset tak ada: %d", r.Status)
	}
	// Tombol "jalankan sekarang": analyst boleh, viewer tidak, query tak dikenal 404.
	refresh := func(cookie, key string) resp {
		return f.a.do("POST", "/api/v1/assets/"+asset+"/endpoint-data/refresh", map[string]any{"query_key": key}, cookie)
	}
	if refresh(f.viewer, "osquery.firewall").Status != 403 || refresh(f.analyst, "osquery.nope").Status != 404 {
		t.Fatal("izin/validasi refresh")
	}
	if approval := f.a.do("PUT", "/api/v1/organizations/"+org+"/osquery/osquery.firewall", map[string]any{"enabled": true, "consent": osquery.Consent().ConsentRef}, f.admin); approval.Status != 204 {
		t.Fatalf("refresh approval: %d %s", approval.Status, approval.Body)
	}
	f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1}, secret)
	r := refresh(f.analyst, "osquery.firewall")
	if r.Status != 202 {
		t.Fatalf("refresh: %d %s", r.Status, r.Body)
	}
	var ex execJSON
	r.JSON(&ex)
	if cl := f.agentCall("POST", "executions/claim", nil, secret); cl.Status != 200 {
		t.Fatalf("claim: %d", cl.Status)
	}
	res := map[string]any{"exit_code": 0, "datasets": []map[string]any{{"query": "osquery.firewall", "status": "ok", "rows": []map[string]any{{"rules": "0"}}}}}
	if rr := f.agentCall("POST", "executions/"+ex.ID+"/result", res, secret); rr.Status != 204 {
		t.Fatalf("hasil: %d %s", rr.Status, rr.Body)
	}
	// Refresh kedua memakai ulang atomic test yang sama (tidak menumpuk).
	if rr := refresh(f.analyst, "osquery.firewall"); rr.Status != 202 {
		t.Fatalf("refresh kedua: %d %s", rr.Status, rr.Body)
	}
	if n := rowCount(t, f, `SELECT count(*) FROM atomic_tests WHERE target_asset_id=$1`, asset); n != 1 {
		t.Fatalf("atomic test harus dipakai ulang: %d", n)
	}
	f.a.do("GET", "/api/v1/assets/"+asset+"/endpoint-data", nil, f.viewer).JSON(&d)
	var fw string
	for _, it := range d.Items {
		if it.Key == "osquery.firewall" {
			fw = it.Status
		}
	}
	if fw != "ok" || len(d.Findings) != 1 || d.Findings[0].Query != "osquery.firewall" || d.Findings[0].Value != "osquery:no-iptables-rules" {
		t.Fatalf("setelah hasil: status=%s temuan=%+v", fw, d.Findings)
	}
}

func TestOsqueryPackIsOptInPerOrganisationAndOnlyRunnableQueriesAreApproved(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.authorizedAgent(t, "box1")
	org := f.joinOrg(t, asset)
	// Katalog memuat pack beku dengan query yang dilewati beserta alasannya, dan tabel yang akan dibaca.
	var cat struct {
		Packs []struct {
			Name       string
			SHA256     string   `json:"sha256"`
			TablesRead []string `json:"tables_read"`
			Queries    []struct{ Name, Key, Status, Reason string }
		}
	}
	f.a.do("GET", "/api/v1/osquery/catalog", nil, f.analyst).JSON(&cat)
	var runKey, skippedReason string
	for _, p := range cat.Packs {
		if p.Name == "ossec-rootkit" {
			for _, q := range p.Queries {
				if q.Status == "runnable" || q.Key != "" {
					t.Fatalf("ossec-rootkit membaca berkas dan tidak boleh berjalan: %+v", q)
				}
				skippedReason = q.Reason
			}
		}
		if p.Name == "it-compliance" {
			if len(p.TablesRead) == 0 || len(p.SHA256) != 64 {
				t.Fatalf("pack harus menampilkan tabel dan hash: %+v", p)
			}
			for _, q := range p.Queries {
				if q.Status == "runnable" {
					runKey = q.Key
				}
			}
		}
	}
	if runKey == "" || !strings.Contains(skippedReason, "sensitive") {
		t.Fatalf("pack tidak lengkap: key=%q alasan=%q", runKey, skippedReason)
	}
	// Tanpa persetujuan: dilewati dengan alasan, tanpa data.
	var tst struct{ ID string }
	f.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": runKey, "contract_key": runKey, "target_asset_id": asset}, f.analyst).JSON(&tst)
	f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1}, secret)
	var run execJSON
	f.a.do("POST", "/api/v1/atomic-tests/"+tst.ID+"/run", nil, f.analyst).JSON(&run)
	if cl := f.agentCall("POST", "executions/claim", nil, secret); cl.Status != 204 {
		t.Fatalf("pack belum disetujui tidak boleh sampai ke agent: %d", cl.Status)
	}
	if got := f.exec(t, run.ID); got.Outcome != "success" || !logHas(got.Logs, "Skipped") {
		t.Fatalf("pack belum disetujui harus dilewati: %+v", got)
	}
	// Hanya admin (settings.manage) yang dapat menyetujui; query yang dilewati tidak pernah aktif.
	if r := f.a.do("PUT", "/api/v1/organizations/"+org+"/osquery-packs/it-compliance", map[string]any{"enabled": true, "consent": osquery.Consent().ConsentRef}, f.analyst); r.Status != 403 {
		t.Fatalf("analis tidak boleh menyetujui pack: %d", r.Status)
	}
	if r := f.a.do("PUT", "/api/v1/organizations/"+org+"/osquery-packs/nope", map[string]any{"enabled": true, "consent": osquery.Consent().ConsentRef}, f.admin); r.Status != 404 {
		t.Fatalf("pack tidak dikenal: %d", r.Status)
	}
	if r := f.a.do("PUT", "/api/v1/organizations/"+org+"/osquery-packs/it-compliance", map[string]any{"enabled": true, "consent": osquery.Consent().ConsentRef}, f.admin); r.Status != 204 {
		t.Fatalf("setujui pack: %d %s", r.Status, r.Body)
	}
	if n := rowCount(t, f, `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1 AND enabled AND query_key LIKE 'osquery.pack.it_compliance.%'`, org); n < 2 {
		t.Fatalf("semua query pack yang dapat berjalan harus disetujui: %d", n)
	}
	if n := rowCount(t, f, `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1 AND query_key NOT LIKE 'osquery.pack.it_compliance.%'`, org); n != 0 {
		t.Fatalf("pack lain tidak boleh ikut: %d", n)
	}
	var detail string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT detail::text FROM audit_log WHERE action='osquery.pack' ORDER BY at DESC LIMIT 1`).Scan(&detail)
	if !strings.Contains(detail, "pack_sha256") || !strings.Contains(detail, "tables_read") {
		t.Fatalf("audit harus memuat hash pack dan tabel: %s", detail)
	}
	// Setelah disetujui, hasil dari agent diterima dan disanitasi menurut skema query pack.
	ex2 := f.agentTask(t, secret, asset, runKey)
	body := map[string]any{"exit_code": 0, "datasets": []map[string]any{{"query": runKey, "status": "ok", "rows": []map[string]any{{"name": "x", "evil_column": "y"}}}}}
	f.agentCall("POST", "executions/"+ex2+"/result", body, secret)
	var rows string
	f.a.env.Owner.QueryRow(context.Background(), `SELECT rows::text FROM endpoint_datasets WHERE asset_id=$1 AND query_key=$2 AND last_status='ok'`, asset, runKey).Scan(&rows)
	if strings.Contains(rows, "evil_column") {
		t.Fatalf("kolom di luar skema lolos: %s", rows)
	}
	// Mencabut menutup lagi.
	f.a.do("PUT", "/api/v1/organizations/"+org+"/osquery-packs/it-compliance", map[string]any{"enabled": false}, f.admin)
	if n := rowCount(t, f, `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1 AND enabled`, org); n != 0 {
		t.Fatalf("pencabutan harus menonaktifkan semua: %d", n)
	}
}

func TestOsqueryFailureDiagnosticsCannotBypassDatasetRedaction(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.approvedAgent(t, "diagnostics", "osquery.inventory")
	ex := f.agentTask(t, secret, asset, "osquery.inventory")
	body := map[string]any{
		"exit_code": 1, "error": "Authorization: Bearer diagnostic-secret-marker",
		"output": "raw-secret-marker", "logs": []map[string]any{{"level": "error", "message": "log-secret-marker"}},
		"datasets": []map[string]any{{"query": "osquery.inventory", "status": "failed", "reason": "password=\"dataset-secret-marker with spaces\"", "osquery_version": "token=version-secret-marker"}},
	}
	if r := f.agentCall("POST", "executions/"+ex+"/result", body, secret); r.Status != 204 {
		t.Fatalf("hasil: %d %s", r.Status, r.Body)
	}
	var diagnostic string
	if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT COALESCE(e.error,'') || COALESCE(e.raw_output_key,'') || d.last_reason || COALESCE(d.osquery_version,'') || COALESCE((SELECT string_agg(l.message,' ') FROM execution_logs l WHERE l.execution_id=e.id),'')
FROM executions e JOIN endpoint_datasets d ON d.tenant_id=e.tenant_id AND d.asset_id=e.target_asset_id AND d.query_key='osquery.inventory' WHERE e.id=$1`, ex).Scan(&diagnostic); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(diagnostic, "secret-marker") || !strings.Contains(diagnostic, "[redacted]") {
		t.Fatalf("diagnostik harus disanitasi: %q", diagnostic)
	}
}

type endpointStoreSpy struct{ calls int }

func (s *endpointStoreSpy) Put(context.Context, string, []byte, string) error { s.calls++; return nil }

func TestOsqueryNeverWritesRawObjectsAndRejectedResultsHaveNoSideEffects(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.approvedAgent(t, "raw-object", "osquery.inventory")
	ex := f.agentTask(t, secret, asset, "osquery.inventory")
	var in execution.Injector
	if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT integration_id,agent_id FROM executions WHERE id=$1`, ex).Scan(&in.ID, &in.AgentID); err != nil {
		t.Fatal(err)
	}
	store := &endpointStoreSpy{}
	rt := server.Runtime(server.Options{DB: f.a.env.App, Tenant: f.a.env.TenantA, Store: store})
	zero := 0
	result := execution.Result{ExitCode: &zero, Output: "must-not-be-written", Datasets: []osquery.Dataset{{Query: "osquery.inventory", Status: osquery.StatusOK, Rows: []map[string]string{{"hostname": "test"}}}}}
	if err := rt.Complete(context.Background(), &in, uuid.MustParse(ex), result); err != nil {
		t.Fatal(err)
	}
	if store.calls != 0 {
		t.Fatal("output osquery tidak boleh ditulis ke object store")
	}
	if err := rt.Complete(context.Background(), &in, uuid.MustParse(ex), result); err == nil {
		t.Fatal("hasil kedua harus ditolak")
	}
	if store.calls != 0 {
		t.Fatal("hasil duplikat menulis objek")
	}
	// ID acak tidak boleh membuat objek walaupun caller terautentikasi.
	if err := rt.Complete(context.Background(), &in, uuid.New(), result); err == nil {
		t.Fatal("ID tidak dikenal harus ditolak")
	}
	if store.calls != 0 {
		t.Fatal("hasil tidak dikenal menulis objek")
	}
}
