package reporting

import "testing"

func TestCoverageCountsRequireSuccessfulMethods(t *testing.T) {
	assets := []Asset{
		{Status: "finished", Methods: []Method{{Outcome: "success"}}},
		{Status: "skipped", SkipReason: "Not authorized"},
		{Status: "finished", Methods: []Method{{Outcome: "error"}}},
		{Status: "finished"},
	}
	got := Coverage(assets)
	if got.Total != 4 || got.Tested != 1 || got.Skipped != 1 || got.Incomplete != 2 {
		t.Fatalf("misleading coverage: %+v", got)
	}
}
func TestExposureDistinguishesZeroFindingsFromNoCoverage(t *testing.T) {
	s := Snapshot{Assets: []Asset{{Status: "finished", Methods: []Method{{Outcome: "success"}}}}}
	if got := exposureRisk(s); got != "No observed vulnerabilities" {
		t.Fatal(got)
	}
	s.Assets[0].Status = "incomplete"
	if got := exposureRisk(s); got != "No completed scan coverage" {
		t.Fatal(got)
	}
	s.BySeverity = map[string]int{"info": 99, "low": 4, "medium": 1, "high": 1}
	if got := exposureRisk(s); got != "Elevated exposure" {
		t.Fatal(got)
	}
	s.BySeverity["critical"] = 1
	if got := exposureRisk(s); got != "High exposure" {
		t.Fatal(got)
	}
}
