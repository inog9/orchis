package recommendations

import (
	"encoding/json"
	"fmt"
	"orchis-go/internal/reporting"
	"strings"
	"testing"
)

func TestPrepareLimitsAndRedactsThePriorityQueue(t *testing.T) {
	s := reporting.Snapshot{Language: "id", Status: "incomplete", Assets: []reporting.Asset{{Name: "confidential-host", Findings: []reporting.Finding{}}}}
	for i := 0; i < MaxFindings+5; i++ {
		s.Assets[0].Findings = append(s.Assets[0].Findings, reporting.Finding{ID: fmt.Sprint(i), Type: "vulnerability", Title: "Injection token=private", Severity: "low", Evidence: map[string]any{"raw": "private response", "kev": i == MaxFindings+4}})
	}
	in := Prepare(s)
	if in.Language != "id" || in.Coverage != "incomplete" || len(in.Findings) != MaxFindings || in.Omitted != 5 || in.Findings[0].ID != fmt.Sprint(MaxFindings+4) {
		t.Fatalf("wrong bounded input: %+v", in)
	}
	raw, _ := json.Marshal(in)
	for _, secret := range []string{"confidential-host", "private response", "token=private"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("unnecessary data leaked: %s", secret)
		}
	}
}
func TestValidateRequiresExactFindingCoverage(t *testing.T) {
	in := Input{Findings: []Finding{{ID: "f1"}}}
	good := `{"summary":"Review the recorded issue.","items":[{"finding_id":"f1","priority":"now","reason":"Critical observation.","action":"Review the vulnerable component.","validation":"Retest the same check."}]}`
	if _, err := Validate([]byte(good), in); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(good, "f1", "invented", 1), strings.Replace(good, "now", "critical", 1), strings.Replace(good, `"validation":"Retest the same check."`, `"validation":""`, 1), strings.Replace(good, `"summary":`, `"commands":[],"summary":`, 1), good + ` {}`, `{"summary":"No issue","items":[]}`} {
		if _, err := Validate([]byte(bad), in); err == nil {
			t.Fatalf("accepted unsupported output: %s", bad)
		}
	}
}
