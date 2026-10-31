package reporting

import (
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
