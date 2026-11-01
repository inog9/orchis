package main

import (
	"orchis-go/internal/reporting"
	"testing"
)

func TestLargeFixtureMatchesItsSummaryAndSyntheticLabel(t *testing.T) {
	s := large()
	c := reporting.Coverage(s.Assets)
	if !s.IsDemo || s.Label == "" || c.Total != 12 || c.Tested != 8 || c.Skipped != 3 || c.Incomplete != 1 {
		t.Fatalf("cakupan fixture tidak konsisten: %+v", c)
	}
	counts := map[string]int{}
	n := 0
	for _, a := range s.Assets {
		for _, f := range a.Findings {
			counts[f.Severity]++
			n++
		}
	}
	if n != 36 {
		t.Fatalf("jumlah temuan: %d", n)
	}
	for key, count := range s.BySeverity {
		if counts[key] != count {
			t.Fatalf("severity %s tidak sesuai", key)
		}
	}
	if s.Risk != "High exposure" {
		t.Fatal("risiko tidak sesuai severity tertinggi")
	}
}
