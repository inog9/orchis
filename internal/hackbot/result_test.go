package hackbot

import (
	"strings"
	"testing"
)

func TestResultBoundary(t *testing.T) {
	r := Result{Schema: 1, SessionID: "session-1", Disposition: "limited", Phase: "report", Summary: strings.Repeat("word ", 400), Report: "# Partial report", Validation: "# Ledger"}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(strings.Fields(r.Summary)) != 300 {
		t.Fatal("summary was not capped")
	}
	r.Report = ""
	if r.Validate() == nil {
		t.Fatal("missing report accepted")
	}
	r.Report = "report"
	r.Findings = []Finding{{ID: "f", Title: "Candidate", Value: "target", Severity: "high", Details: []byte(`{"verdict":"NEEDS_MORE_EVIDENCE"}`)}}
	if r.Validate() == nil {
		t.Fatal("unvalidated finding accepted")
	}
	r.Findings[0].Details = []byte(`{"verdict":"VALIDATED"}`)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}
