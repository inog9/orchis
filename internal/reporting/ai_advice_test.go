package reporting

import (
	"strings"
	"testing"
)

func TestAIAdviceRendersInRemediationSection(t *testing.T) {
	s := Snapshot{
		Assets: []Asset{{Name: "macbook", Findings: []Finding{{ID: "f1", Type: "vulnerability", Severity: "critical", Title: "CVE-2026-59873 in tar 7.5.15"}}}},
		AIAdvice: &AIAdvice{
			JobID: "j1", Model: "test-model", Summary: "Upgrade the npm packages.",
			Items: []AIAdviceItem{{FindingID: "f1", Title: "CVE-2026-59873 in tar 7.5.15", Severity: "critical", Recommendation: "Run npm update tar to 7.5.21.", Validation: "Re-run the scan after upgrading."}},
		},
	}
	h := aiAdviceHTML("en", s)
	if !strings.Contains(string(h), "AI-generated remediation advice") || !strings.Contains(string(h), "Run npm update tar to 7.5.21.") {
		t.Fatalf("blok AI tidak ter-render: %s", string(h)[:200])
	}
	if got := aiAdviceHTML("en", Snapshot{}); got != "" {
		t.Fatalf("tanpa advice harus kosong, dapat %q", string(got))
	}
	full, err := HTML(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(full), "AI-generated remediation advice") {
		t.Fatal("laporan penuh tidak memuat blok AI")
	}
}
