package reporting

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
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
