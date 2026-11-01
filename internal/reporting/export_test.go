package reporting

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"

	"orchis-go/internal/osquery"
	"testing"
)

func TestReportRedactionEscapingAndCSVFormulaProtection(t *testing.T) {
	raw := `password=hunter2 token=oag_secret Bearer abc.def person@example.com`
	safe := Redact(raw, 1000)
	for _, secret := range []string{"hunter2", "oag_secret", "abc.def", "person@example.com"} {
		if strings.Contains(safe, secret) {
			t.Fatalf("leaked %s", secret)
		}
	}
	evidence := SafeEvidence(map[string]any{"response": "SECRET ENV FILE", "request": "COOKIE", "port": float64(80), "template_id": "demo", "nested": map[string]string{"password": "secret"}})
	if len(evidence) != 2 {
		t.Fatalf("unexpected evidence %+v", evidence)
	}
	snap := Snapshot{Organization: `<script>alert(1)</script>`, Assets: []Asset{{Name: "=1+1", Findings: []Finding{{Title: "+SUM(1)", Value: "@evil", Evidence: map[string]any{}}}}}}
	html, e := HTML(snap)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(html), "<script>alert") {
		t.Fatal("unescaped target HTML")
	}
	csv, e := CSV(snap)
	if e != nil {
		t.Fatal(e)
	}
	for _, value := range []string{"'=1+1", "'+SUM(1)", "'@evil"} {
		if !strings.Contains(string(csv), value) {
			t.Fatalf("unsafe CSV %s", csv)
		}
	}
}
func TestComparisonDoesNotResolveFailedOrChangedCoverage(t *testing.T) {
	id := uuid.New()
	old := Snapshot{Profile: "quick", Assets: []Asset{{ID: id, Status: "finished", Methods: []Method{{Action: "nuclei.scan", Version: 1, Target: "http://example.test", Params: []byte(`{}`), Outcome: "success"}}, Findings: []Finding{{ID: "old"}}}}}
	current := old
	current.Assets = append([]Asset(nil), old.Assets...)
	current.Assets[0].Findings = nil
	if got := Compare(current, old); !strings.Contains(got, "1 resolved") {
		t.Fatal(got)
	}
	current.Assets[0].Status = "incomplete"
	if got := Compare(current, old); strings.Contains(got, "1 resolved") {
		t.Fatal("failed scan resolved finding")
	}
	current.Assets[0].Status = "finished"
	current.Assets[0].Methods = append([]Method(nil), old.Assets[0].Methods...)
	current.Assets[0].Methods[0].Target = "http://another.test"
	if got := Compare(current, old); strings.Contains(got, "1 resolved") {
		t.Fatal("changed scope resolved finding")
	}
}

func TestDemoSnapshotIsLabelledInHTMLAndCSV(t *testing.T) {
	snap := Snapshot{Organization: "Demo Org", IsDemo: true, Label: "DEMO / SYNTHETIC - test label", BySeverity: map[string]int{},
		Assets: []Asset{{Name: "a", Findings: []Finding{{Type: "text", Severity: "low", Title: "t", Value: "v"}}}}}
	html, err := HTML(snap)
	if err != nil || !strings.Contains(string(html), "DEMO / SYNTHETIC - test label") {
		t.Fatalf("HTML demo tanpa label: %v", err)
	}
	csv, err := CSV(snap)
	if err != nil || !strings.Contains(string(csv), "synthetic") || !strings.Contains(string(csv), ",true") {
		t.Fatalf("CSV demo tanpa penanda: %s %v", csv, err)
	}
	snap.IsDemo, snap.Label = false, ""
	html, _ = HTML(snap)
	if strings.Contains(string(html), "demo-banner\">") {
		t.Fatal("laporan nyata tidak boleh berlabel demo")
	}
	csv, _ = CSV(snap)
	if !strings.Contains(string(csv), ",false") {
		t.Fatalf("CSV nyata harus synthetic=false: %s", csv)
	}
}

func TestFindingTechniqueIsAConservativeEstimateRenderedInReport(t *testing.T) {
	vuln := Finding{Type: "vulnerability", Severity: "high", Title: "Exposed panel", Value: "x"}
	got := EstimatedTechniques(vuln)
	if len(got) != 1 || got[0].ID != "T1190" || got[0].Basis != "estimate" || got[0].Name == "" || got[0].Reason == "" {
		t.Fatalf("kerentanan harus dipetakan ke T1190 sebagai perkiraan beralasan: %+v", got)
	}
	for _, typ := range []string{"open_port", "service", "text"} {
		if EstimatedTechniques(Finding{Type: typ}) != nil {
			t.Fatalf("temuan %s tidak boleh dipetakan (hasil discovery, bukan celah)", typ)
		}
	}
	vuln.MITRE = got
	html, err := HTML(Snapshot{Organization: "O", BySeverity: map[string]int{}, Assets: []Asset{{Name: "a", Findings: []Finding{vuln}}}})
	if err != nil || !strings.Contains(string(html), "MITRE ATT&amp;CK (estimate): T1190 Exploit Public-Facing Application") {
		t.Fatalf("laporan harus menandai perkiraan: %v", err)
	}
	// Dibekukan di JSON snapshot: nama dan basis ikut tersimpan.
	raw, _ := json.Marshal(vuln)
	if !strings.Contains(string(raw), `"basis":"estimate"`) || !strings.Contains(string(raw), `"name":"Exploit Public-Facing Application"`) {
		t.Fatalf("snapshot harus membekukan basis dan nama: %s", raw)
	}
}

func TestReportShowsEndpointPostureWithPortComparison(t *testing.T) {
	p := osquery.BuildPosture(map[string]*osquery.Dataset{
		"osquery.inventory": {Query: "osquery.inventory", Status: osquery.StatusOK, Rows: []map[string]string{{"os_name": "Ubuntu", "os_version": "26.04", "kernel": "7.0", "uptime_seconds": "1"}}},
		"osquery.network":   {Query: "osquery.network", Status: osquery.StatusOK, Rows: []map[string]string{{"port": "22", "address": "0.0.0.0", "protocol": "6", "process": "sshd"}, {"port": "5432", "address": "::", "protocol": "6", "process": "pg"}}},
		"osquery.hardening": {Query: "osquery.hardening", Status: osquery.StatusOK, Rows: []map[string]string{{"item": "sysctl", "target": "kernel.randomize_va_space", "value": "0"}}},
	}, []int{22, 80})
	snap := Snapshot{Organization: "O", BySeverity: map[string]int{}, Assets: []Asset{{Name: "web-<b>1", Endpoint: p}, {Name: "no-agent"}}}
	snap.EndpointSummary = summarizeEndpoint(snap.Assets)
	if snap.EndpointSummary == nil || snap.EndpointSummary.AssetsWithData != 1 || snap.EndpointSummary.AssetsTotal != 2 || snap.EndpointSummary.BySeverity["high"] != 1 {
		t.Fatalf("ringkasan: %+v", snap.EndpointSummary)
	}
	html, err := HTML(snap)
	if err != nil {
		t.Fatal(err)
	}
	out := string(html)
	for _, want := range []string{"Endpoint posture", "1 of 2 assets reported data from inside the machine", "Ubuntu 26.04, kernel 7.0", "high: Address space randomisation (ASLR) is not fully enabled",
		"Seen by both: 22. Scan only: 80. Inside only: 5432.", "web-&lt;b&gt;1", "not proof of a problem"} {
		if !strings.Contains(out, want) {
			t.Errorf("laporan tanpa %q", want)
		}
	}
	if strings.Contains(out, "<b>1") {
		t.Error("nama aset harus di-escape")
	}
	// Tanpa data endpoint: bagian tidak muncul.
	plain, _ := HTML(Snapshot{Organization: "O", BySeverity: map[string]int{}, Assets: []Asset{{Name: "a"}}})
	if strings.Contains(string(plain), "Endpoint posture") {
		t.Error("bagian postur tidak boleh muncul tanpa data")
	}
	if summarizeEndpoint([]Asset{{Name: "a"}}) != nil {
		t.Error("tanpa data tidak ada ringkasan")
	}
}

func TestReportCapsLongEndpointFindingLists(t *testing.T) {
	p := &osquery.Posture{Source: "osquery", Attributes: map[string]any{"os_name": "Ubuntu"}}
	for i := 0; i < 12; i++ {
		p.Findings = append(p.Findings, osquery.PostureFinding{Severity: "info", Title: "Port " + strings.Repeat("1", i+1) + "/tcp listens", Query: "osquery.network"})
	}
	snap := Snapshot{Organization: "O", BySeverity: map[string]int{}, Assets: []Asset{{Name: "a", Endpoint: p}}}
	snap.EndpointSummary = summarizeEndpoint(snap.Assets)
	html, err := HTML(snap)
	if err != nil {
		t.Fatal(err)
	}
	out := string(html)
	if strings.Count(out, "info: Port ") != 8 || !strings.Contains(out, "and 4 more") {
		t.Fatalf("daftar harus dibatasi 8 dengan penanda sisanya: %d", strings.Count(out, "info: Port "))
	}
}

func TestPrintCompactsParametersWithoutChangingFrozenSnapshot(t *testing.T) {
	raw := json.RawMessage("{\n  \"ports\": [\n  \"80\",\n  \"443\"\n ]\n}")
	snap := Snapshot{IsDemo: true, Organization: "Demo layout", Assets: []Asset{{Name: "Demo", Methods: []Method{{Action: "nmap.tcp_connect", Version: 1, Params: raw}}}}}
	before, _ := json.Marshal(snap)
	html, err := HTML(snap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), `<pre>{&#34;ports&#34;:[&#34;80&#34;,&#34;443&#34;]}</pre>`) {
		t.Fatal("parameters should render compactly and remain escaped")
	}
	if !strings.Contains(string(html), `content="demo-assessment"`) {
		t.Fatal("synthetic report marker missing")
	}
	after, _ := json.Marshal(snap)
	if string(before) != string(after) {
		t.Fatal("rendering changed the frozen snapshot")
	}
}
