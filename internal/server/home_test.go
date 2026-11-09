package server_test

import (
	"context"
	"testing"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/execution"
	"orchis-go/internal/home"
)

type homeJSON struct {
	Assets   *struct{ Total, Authorized int } `json:"assets"`
	Findings *struct {
		Total, Vulnerabilities int
		OpenPorts              int            `json:"open_ports"`
		VulnerableAssets       int            `json:"vulnerable_assets"`
		BySeverity             map[string]int `json:"by_severity"`
	} `json:"findings"`
	Atomic *struct {
		Tests, Active int
		Runs7d        int `json:"runs_7d"`
		Failed7d      int `json:"failed_7d"`
	} `json:"atomic"`
	LatestRuns []struct {
		TestName string `json:"test_name"`
		Target   string `json:"target"`
		Outcome  string
	} `json:"latest_runs"`
	MostExposed []struct {
		Name                   string
		Critical, High, Medium int
		OpenPorts              int `json:"open_ports"`
	} `json:"most_exposed"`
	Injectors []struct{ Key, Status string }
}

func getHome(t *testing.T, f *scanFixture, cookie string) homeJSON {
	t.Helper()
	r := f.a.do("GET", "/api/v1/home", nil, cookie)
	if r.Status != 200 {
		t.Fatalf("home: %d %s", r.Status, r.Body)
	}
	var h homeJSON
	r.JSON(&h)
	return h
}

func TestHomeSummaryReflectsAssetsRunsAndFindings(t *testing.T) {
	f := newScanFixture(t, true)
	if r := f.a.do("GET", "/api/v1/home", nil, ""); r.Status != 401 {
		t.Fatalf("tanpa login harus 401: %d", r.Status)
	}
	h := getHome(t, f, f.analyst)
	if h.Assets.Total != 1 || h.Assets.Authorized != 1 || h.Findings.Total != 0 || h.Atomic.Tests != 1 || h.Atomic.Runs7d != 0 || len(h.LatestRuns) != 0 || len(h.MostExposed) != 0 {
		t.Fatalf("ringkasan awal salah: %+v", h)
	}
	if len(h.Injectors) != 4 || h.Injectors[0].Status != "offline" {
		t.Fatalf("empat injector offline diharapkan (orchis-defense-agent, Hackbot, Nmap, Nuclei): %+v", h.Injectors)
	}

	// scan nmap sukses (2 port) + scan nuclei sukses (1 kerentanan critical dan 1 medium) pada aset yang sama
	e := f.run(t)
	f.inj("POST", "executions/"+e.ID+"/claim", nil, f.token)
	f.inj("POST", "executions/"+e.ID+"/result", map[string]any{"exit_code": 0, "findings": []map[string]any{
		{"type": "open_port", "value": "tcp/22", "title": "Open port 22/tcp", "severity": "info"},
		{"type": "open_port", "value": "tcp/80", "title": "Open port 80/tcp", "severity": "info"}}}, f.token)

	var nt struct{ ID string }
	f.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": "vulns", "contract_key": "nuclei.scan", "target_asset_id": f.asset}, f.analyst).JSON(&nt)
	ntok, _ := execution.IssueToken(context.Background(), f.a.env.App, f.a.env.TenantA, "nuclei")
	var ne execJSON
	f.a.do("POST", "/api/v1/atomic-tests/"+nt.ID+"/run", nil, f.analyst).JSON(&ne)
	f.inj("POST", "executions/"+ne.ID+"/claim", nil, ntok)
	if r := f.inj("POST", "executions/"+ne.ID+"/result", map[string]any{"exit_code": 0, "findings": []map[string]any{
		{"type": "vulnerability", "value": "cve-1@x", "title": "Bad one", "severity": "critical"},
		{"type": "vulnerability", "value": "cve-2@x", "title": "Meh", "severity": "medium"}}}, ntok); r.Status != 204 {
		t.Fatalf("result nuclei: %d %s", r.Status, r.Body)
	}
	// satu run gagal
	e3 := f.run(t)
	f.inj("POST", "executions/"+e3.ID+"/claim", nil, f.token)
	f.inj("POST", "executions/"+e3.ID+"/result", map[string]any{"exit_code": 1, "error": "boom"}, f.token)

	h = getHome(t, f, f.analyst)
	fs := h.Findings
	if fs.Total != 4 || fs.Vulnerabilities != 2 || fs.OpenPorts != 2 || fs.VulnerableAssets != 1 || fs.BySeverity["critical"] != 1 || fs.BySeverity["medium"] != 1 || fs.BySeverity["info"] != 0 {
		t.Fatalf("statistik finding salah: %+v", fs)
	}
	if h.Atomic.Tests != 2 || h.Atomic.Runs7d != 3 || h.Atomic.Failed7d != 1 || h.Atomic.Active != 0 {
		t.Fatalf("statistik atomic salah: %+v", h.Atomic)
	}
	if len(h.LatestRuns) != 3 || h.LatestRuns[0].Outcome != "error" || h.LatestRuns[0].Target != "target-1" {
		t.Fatalf("run terbaru salah: %+v", h.LatestRuns)
	}
	if len(h.MostExposed) != 1 || h.MostExposed[0].Critical != 1 || h.MostExposed[0].Medium != 1 || h.MostExposed[0].OpenPorts != 2 {
		t.Fatalf("aset paling terpapar salah: %+v", h.MostExposed)
	}

	// menghapus aset menyingkirkannya dari Home dan dari daftar Findings
	if r := f.a.do("DELETE", "/api/v1/assets/"+f.asset, nil, f.admin); r.Status != 204 {
		t.Fatalf("hapus aset: %d", r.Status)
	}
	h = getHome(t, f, f.analyst)
	if h.Assets.Total != 0 || h.Findings.Total != 0 || h.Findings.VulnerableAssets != 0 || len(h.MostExposed) != 0 {
		t.Fatalf("aset terhapus harus hilang dari ringkasan: %+v %+v", h.Assets, h.Findings)
	}
	var list struct{ Items []struct{ ID string } }
	f.a.do("GET", "/api/v1/findings", nil, f.analyst).JSON(&list)
	if len(list.Items) != 0 {
		t.Fatalf("finding aset terhapus tidak boleh tampil: %+v", list)
	}
}

func TestHomeOnlyIncludesSectionsTheCallerMayRead(t *testing.T) {
	f := newScanFixture(t, true)
	svc := &home.Service{DB: f.a.env.App}
	p := &auth.Principal{TenantID: f.a.env.TenantA, Permissions: []string{authz.AssetRead}}
	s, err := svc.Summary(context.Background(), p)
	if err != nil || s.Assets == nil || s.Findings != nil || s.Atomic != nil || len(s.Injectors) != 0 || len(s.LatestRuns) != 0 {
		t.Fatalf("hanya aset yang boleh terisi: %+v %v", s, err)
	}
	// tenant lain melihat nol walau punya semua izin
	pb := &auth.Principal{TenantID: f.a.env.TenantB, Permissions: []string{authz.AssetRead, authz.FindingRead, authz.AtomicRead}}
	sb, err := svc.Summary(context.Background(), pb)
	if err != nil || sb.Assets.Total != 0 || sb.Atomic.Tests != 0 {
		t.Fatalf("tenant B harus kosong: %+v %v", sb, err)
	}
}

func TestHomeCountsScenariosAndSimulations(t *testing.T) {
	c := newChainFixture(t)
	c.mustReady(t, []map[string]any{step("scan", nmapC, "after_parents", nil)}, nil)
	sim := c.launch(t)
	var h struct {
		Chains *struct{ Scenarios, Simulations, Running int } `json:"chains"`
		Latest []struct{ Code, Name, Status string }          `json:"latest_simulations"`
	}
	c.a.do("GET", "/api/v1/home", nil, c.analyst).JSON(&h)
	if h.Chains == nil || h.Chains.Scenarios != 1 || h.Chains.Simulations != 1 || h.Chains.Running != 1 || len(h.Latest) != 1 || h.Latest[0].Code != "OP-0001" || h.Latest[0].Status != "running" {
		t.Fatalf("ringkasan chain salah: %+v %+v", h.Chains, h.Latest)
	}
	c.finish(t, c.execsOf(t, sim.ID, "scan")[0], nmapC, okEmpty)
	c.a.do("GET", "/api/v1/home", nil, c.analyst).JSON(&h)
	if h.Chains.Running != 0 || h.Latest[0].Status != "finished" {
		t.Fatalf("setelah selesai: %+v %+v", h.Chains, h.Latest)
	}
	svc := &home.Service{DB: c.a.env.App}
	s, err := svc.Summary(context.Background(), &auth.Principal{TenantID: c.a.env.TenantA, Permissions: []string{authz.AssetRead}})
	if err != nil || s.Chains != nil || len(s.LatestSims) != 0 {
		t.Fatalf("tanpa izin scenario/simulasi bagian chain harus kosong: %+v %v", s.Chains, err)
	}
}
