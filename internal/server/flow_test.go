package server_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"orchis-go/internal/contract"
	"orchis-go/internal/osquery"
	"orchis-go/internal/scans"
	"orchis-go/internal/server"
)

// fullAssessmentAsset menyiapkan satu aset yang siap untuk kedua engine: agent terdaftar + koleksi disetujui
// + URL web disetujui. Mengembalikan secret agent, id aset, dan id organisasi.
func fullAssessmentAsset(t *testing.T, f *agentFixture, host string) (secret, asset, org string) {
	t.Helper()
	secret, asset = f.approvedAgent(t, host, defenseKeys()...)
	if _, err := f.a.env.Owner.Exec(context.Background(), `INSERT INTO asset_addresses(id,tenant_id,asset_id,kind,value,scan_approved) SELECT gen_random_uuid(),tenant_id,$1,'url','http://127.0.0.1:8080/',true FROM assets WHERE id=$1`, asset); err != nil {
		t.Fatal(err)
	}
	if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT organization_id FROM assets WHERE id=$1`, asset).Scan(&org); err != nil {
		t.Fatal(err)
	}
	return
}

func TestFullAssessmentLaunchOrderingAndGuard(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	env := f.a.env
	secret, asset, orgID := fullAssessmentAsset(t, f, "full-box")
	assetID := uuid.MustParse(asset)
	org := uuid.MustParse(orgID)
	f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1}, secret)

	full := scans.Input{OrganizationID: org, AssetIDs: []uuid.UUID{assetID}, Tools: []string{"nmap", "nuclei", "hackbot", "defensebot"}}
	if r := f.a.do("POST", "/api/v1/scans", full, f.analyst, "Idempotency-Key", "full-1"); r.Status != 201 {
		t.Fatalf("full launch: %d %s", r.Status, r.Body)
	}
	var scan scans.Scan
	f.a.do("POST", "/api/v1/scans", full, f.analyst, "Idempotency-Key", "full-1").JSON(&scan)
	tools := map[string]int{}
	for _, j := range scan.Jobs {
		tools[j.Tool]++
	}
	if len(tools) != 4 || tools["nmap"] != 1 || tools["nuclei"] != 1 || tools["hackbot"] != 1 || tools["defensebot"] != 1 {
		t.Fatalf("full assessment must create one job per engine: %+v", tools)
	}

	// Full assessment campuran per aset: aset URL-only tanpa agent hanya mendapat Hackbot (tanpa job orchis-defense-agent).
	urlOnly := mkAsset(t, f.a, f.admin, map[string]any{"category": "web_application", "name": "web-only", "organization_id": org, "testing_authorized": true, "addresses": []map[string]any{{"kind": "url", "value": "http://127.0.0.2:8080/"}}})
	mixed := full
	mixed.AssetIDs = []uuid.UUID{uuid.MustParse(urlOnly)}
	var mixedScan scans.Scan
	if r := f.a.do("POST", "/api/v1/scans", mixed, f.analyst, "Idempotency-Key", "full-mixed"); r.Status != 201 {
		t.Fatalf("full on URL-only asset must run Hackbot only: %d %s", r.Status, r.Body)
	} else {
		r.JSON(&mixedScan)
	}
	for _, j := range mixedScan.Jobs {
		if j.Tool == "defensebot" {
			t.Fatalf("URL-only asset must not get a orchis-defense-agent job: %+v", mixedScan.Jobs)
		}
	}

	// Urutan: Nmap dan Nuclei berjalan; Hackbot menunggu keduanya; orchis-defense-agent menunggu Nmap.
	svc := server.ScanService(server.Options{DB: env.App, Guard: contract.Guard{AllowLoopback: true}})
	if err := svc.Tick(ctx, env.TenantA); err != nil {
		t.Fatal(err)
	}
	running := map[string]string{}
	rows, err := env.Owner.Query(ctx, `SELECT tool,status FROM scan_jobs WHERE assessment_id=$1`, scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var tool, status string
		rows.Scan(&tool, &status)
		running[tool] = status
	}
	rows.Close()
	if running["nmap"] != "running" || running["nuclei"] != "running" {
		t.Fatalf("preliminary scanners must run first: %+v", running)
	}
	if running["hackbot"] != "waiting" || running["defensebot"] != "waiting" {
		t.Fatalf("both engines must wait: %+v", running)
	}

	// Selesaikan Nmap+Nuclei, jawab koleksi agent, lalu kedua engine berjalan.
	finishPreliminaryScanners(t, f.a, scan.ID, "")
	answerCollections(t, f, secret, len(defenseKeys()))
	if err = svc.Tick(ctx, env.TenantA); err != nil {
		t.Fatal(err)
	}
	after := map[string]string{}
	rows, err = env.Owner.Query(ctx, `SELECT tool,status FROM scan_jobs WHERE assessment_id=$1`, scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var tool, status string
		rows.Scan(&tool, &status)
		after[tool] = status
	}
	rows.Close()
	if after["hackbot"] != "running" || after["defensebot"] != "running" {
		t.Fatalf("both engines must run after scanners and collection: %+v", after)
	}

	// Guard Hackbot: run web kedua pada aset yang sama ditolak selama run pertama masih hidup.
	second := scans.Input{OrganizationID: org, AssetIDs: []uuid.UUID{assetID}}
	if r := f.a.do("POST", "/api/v1/scans", second, f.analyst, "Idempotency-Key", "overlap"); r.Status != 409 || !strings.Contains(string(r.Body), "already testing") {
		t.Fatalf("second Hackbot run must be refused: %d %s", r.Status, r.Body)
	}

	// Cancel berantai: assessment kedua hanya untuk orchis-defense-agent yang menunggu Nmap aset sama dibatalkan
	// ketika Nmap dari assessment pertama dibatalkan. Gunakan aset kedua agar tidak menyentuh run hidup.
	host2 := mkAsset(t, f.a, f.admin, map[string]any{"category": "host", "name": "cancel-box", "organization_id": org, "testing_authorized": true, "addresses": []map[string]any{{"kind": "url", "value": "http://127.0.0.3:8080/"}}})
	var dummy int
	if err := env.Owner.QueryRow(ctx, `SELECT count(*) FROM agents WHERE asset_id=$1`, host2).Scan(&dummy); err != nil {
		t.Fatal(err)
	}
	pipeline := scans.Input{OrganizationID: org, AssetIDs: []uuid.UUID{uuid.MustParse(host2)}}
	if r := f.a.do("POST", "/api/v1/scans", pipeline, f.analyst, "Idempotency-Key", "cancel-cascade"); r.Status != 201 {
		t.Fatalf("pipeline launch: %d %s", r.Status, r.Body)
	}
	var cascade scans.Scan
	f.a.do("POST", "/api/v1/scans", pipeline, f.analyst, "Idempotency-Key", "cancel-cascade").JSON(&cascade)
	if err = svc.Tick(ctx, env.TenantA); err != nil {
		t.Fatal(err)
	}
	if r := f.a.do("POST", "/api/v1/scans/"+cascade.ID.String()+"/cancel", nil, f.analyst); r.Status != 200 {
		t.Fatalf("cancel: %d %s", r.Status, r.Body)
	}
	var waiting int
	if err := env.Owner.QueryRow(ctx, `SELECT count(*) FROM scan_jobs WHERE assessment_id=$1 AND status='canceled' AND tool IN ('nmap','nuclei','hackbot')`, cascade.ID).Scan(&waiting); err != nil {
		t.Fatal(err)
	}
	if waiting != 3 {
		t.Fatalf("cancel must stop the whole pipeline: %d", waiting)
	}
}

func TestReadinessCoversBothEngines(t *testing.T) {
	f := newAgentFixture(t)
	// Agent tanpa persetujuan koleksi: orchis-defense-agent harus menjelaskan kenapa belum siap.
	secret, asset := f.authorizedAgent(t, "ready-box")
	org := f.joinOrg(t, asset)
	orgID := uuid.MustParse(org)
	f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1}, secret)

	r := f.a.do("GET", "/api/v1/organizations/"+org+"/readiness", nil, f.analyst)
	if r.Status != 200 {
		t.Fatalf("readiness: %d %s", r.Status, r.Body)
	}
	var state struct {
		Total          int  `json:"total"`
		ScanWindowOpen bool `json:"scan_window_open"`
		EmergencyStop  bool `json:"emergency_stop"`
		Items          []struct {
			ID            string `json:"id"`
			WebReason     string `json:"web_reason"`
			DefenseReason string `json:"defense_reason"`
			AgentStatus   string `json:"agent_status"`
			OS            string `json:"os"`
		} `json:"items"`
	}
	r.JSON(&state)
	if state.Total != 1 || len(state.Items) != 1 {
		t.Fatalf("readiness items: %+v", state)
	}
	item := state.Items[0]
	if item.AgentStatus != "online" || item.OS == "" {
		t.Fatalf("agent must be visible: %+v", item)
	}
	if !strings.Contains(item.DefenseReason, "not approved") {
		t.Fatalf("unapproved collection must be explained: %q", item.DefenseReason)
	}
	if !strings.Contains(item.WebReason, "URL") {
		t.Fatalf("host asset without URL must be told the web pentest needs one: %q", item.WebReason)
	}
	for _, k := range defenseKeys() {
		if q := osquery.Find(k); q == nil || !q.SupportsOS("linux") {
			continue
		}
		if rr := f.a.do("PUT", "/api/v1/organizations/"+org+"/osquery/"+k, f.collectionChoice(t, org, k, true), f.admin); rr.Status != 204 {
			t.Fatalf("approve %s: %d %s", k, rr.Status, rr.Body)
		}
	}
	r = f.a.do("GET", "/api/v1/organizations/"+org+"/readiness", nil, f.analyst)
	r.JSON(&state)
	if state.Items[0].DefenseReason != "" {
		t.Fatalf("approved collection must clear the defense reason: %q", state.Items[0].DefenseReason)
	}

	// Stop darurat menutup jendela dan memberi alasan.
	if rr := f.a.do("POST", "/api/v1/organizations/"+org+"/scan-control/stop", nil, f.admin); rr.Status != 200 {
		t.Fatalf("emergency stop: %d %s", rr.Status, rr.Body)
	}
	r = f.a.do("GET", "/api/v1/organizations/"+org+"/readiness", nil, f.analyst)
	r.JSON(&state)
	if !state.EmergencyStop || state.ScanWindowOpen {
		t.Fatalf("emergency stop must close the window: %+v", state)
	}
	_ = orgID
}

func insertFinding(t *testing.T, f *agentFixture, asset, method string) string {
	t.Helper()
	value := "retest-" + method + "-" + asset
	details := map[string]any{"source": "fixture"}
	if method != "" {
		details["method"] = method
	}
	raw, _ := json.Marshal(details)
	hash := sha256.Sum256([]byte("vulnerability\x00" + strings.ToLower(strings.TrimSpace(value))))
	var id string
	if err := f.a.env.Owner.QueryRow(context.Background(), `INSERT INTO findings(id,tenant_id,asset_id,type,value,value_hash,title,severity,details,organization_id) SELECT gen_random_uuid(),tenant_id,$1,'vulnerability',$2,$3,'Fixture finding','high',$4::jsonb,organization_id FROM assets WHERE id=$1 RETURNING id`, asset, value, hash[:], string(raw)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestFindingRetestLaunchesSameEngine(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset, _ := fullAssessmentAsset(t, f, "retest-box")
	f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0", "protocol_version": 1}, secret)

	outside := insertFinding(t, f, asset, "")
	r := f.a.do("POST", "/api/v1/findings/"+outside+"/retest", nil, f.analyst)
	if r.Status != 201 {
		t.Fatalf("outside retest: %d %s", r.Status, r.Body)
	}
	var scan scans.Scan
	r.JSON(&scan)
	tools := map[string]bool{}
	for _, j := range scan.Jobs {
		tools[j.Tool] = true
	}
	if !tools["hackbot"] || tools["defensebot"] {
		t.Fatalf("outside retest must run the Hackbot pipeline only: %+v", scan.Jobs)
	}

	inside := insertFinding(t, f, asset, "osv-exact")
	r = f.a.do("POST", "/api/v1/findings/"+inside+"/retest", nil, f.analyst)
	if r.Status != 201 {
		t.Fatalf("inside retest: %d %s", r.Status, r.Body)
	}
	r.JSON(&scan)
	if len(scan.Jobs) != 1 || scan.Jobs[0].Tool != "defensebot" {
		t.Fatalf("inside retest must run orchis-defense-agent only: %+v", scan.Jobs)
	}

}

func TestFindingTriageLifecycle(t *testing.T) {
	f := newAgentFixture(t)
	_, asset, org := fullAssessmentAsset(t, f, "triage-box")
	id := insertFinding(t, f, asset, "")

	// Viewer tidak boleh men-triage.
	if r := f.a.do("PUT", "/api/v1/findings/"+id+"/triage", map[string]any{"status": "false_positive"}, f.viewer); r.Status != 403 {
		t.Fatalf("viewer triage: %d %s", r.Status, r.Body)
	}
	// Risiko diterima butuh alasan dan tanggal habis.
	if r := f.a.do("PUT", "/api/v1/findings/"+id+"/triage", map[string]any{"status": "accepted_risk"}, f.analyst); r.Status != 422 {
		t.Fatalf("risk without reason: %d %s", r.Status, r.Body)
	}
	past := time.Now().Add(-time.Hour).Format(time.RFC3339)
	if r := f.a.do("PUT", "/api/v1/findings/"+id+"/triage", map[string]any{"status": "accepted_risk", "reason": "comps", "expires_at": past}, f.analyst); r.Status != 422 {
		t.Fatalf("risk with past expiry: %d %s", r.Status, r.Body)
	}
	future := time.Now().Add(72 * time.Hour).Format(time.RFC3339)
	r := f.a.do("PUT", "/api/v1/findings/"+id+"/triage", map[string]any{"status": "accepted_risk", "reason": "Compensating control reviewed", "expires_at": future}, f.analyst)
	if r.Status != 200 {
		t.Fatalf("accept risk: %d %s", r.Status, r.Body)
	}
	var finding struct {
		TriageStatus string `json:"triage_status"`
		TriageReason string `json:"triage_reason"`
	}
	r.JSON(&finding)
	if finding.TriageStatus != "accepted_risk" || finding.TriageReason == "" {
		t.Fatalf("triage fields: %+v", finding)
	}

	// Daftar aksi menyembunyikan risiko yang diterima sampai tanggalnya habis.
	open := func() int {
		var page struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		f.a.do("GET", "/api/v1/findings?organization_id="+org+"&triage=open", nil, f.analyst).JSON(&page)
		n := 0
		for _, it := range page.Items {
			if it.ID == id {
				n++
			}
		}
		return n
	}
	if open() != 0 {
		t.Fatal("accepted risk must be hidden from the actionable list")
	}
	if _, err := f.a.env.Owner.Exec(context.Background(), `UPDATE findings SET triage_expires_at=now()-interval '1 second' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if open() != 1 {
		t.Fatal("expired accepted risk must be actionable again")
	}
	// Audit mencatat siapa dan kapan.
	var n int
	if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action='finding.triage' AND resource_id=$1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("triage must be audited")
	}
	// Kembali ke open membersihkan kolom.
	if r := f.a.do("PUT", "/api/v1/findings/"+id+"/triage", map[string]any{"status": "open"}, f.analyst); r.Status != 200 {
		t.Fatalf("reopen: %d %s", r.Status, r.Body)
	}
	if open() != 1 {
		t.Fatal("reopened finding must be actionable")
	}
}
