package reporting

import (
	"encoding/json"
	"github.com/google/uuid"
	"orchis-go/internal/contract"
	"reflect"
	"testing"
)

func comparisonFixture() Snapshot {
	m := Method{Action: "nuclei.scan", Version: 1, Target: "http://127.0.0.1:18080", Params: json.RawMessage(`{}`), Outcome: "success"}
	return Snapshot{Profile: "quick", AssessmentID: uuid.New(), Assets: []Asset{{ID: uuid.MustParse("11111111-1111-4111-8111-111111111111"), Name: "fixture", Status: "finished", Methods: []Method{m}, Findings: []Finding{{ID: "v1", Type: "vulnerability", Title: "exposure", Severity: "high", Coverage: []string{CoverageKey(m)}}}}}}
}
func comparisonCopy(s Snapshot) Snapshot {
	raw, _ := json.Marshal(s)
	var out Snapshot
	_ = json.Unmarshal(raw, &out)
	return out
}
func comparisonResult(current, old Snapshot) Comparison {
	b := NewComparison(current)
	b.Add(old, uuid.Nil)
	return b.Result()
}
func TestComparisonStructuredChanges(t *testing.T) {
	old := comparisonFixture()
	current := comparisonCopy(old)
	current.Assets[0].Findings = append(current.Assets[0].Findings, Finding{ID: "v2", Type: "vulnerability", Severity: "critical", Coverage: old.Assets[0].Findings[0].Coverage})
	got := comparisonResult(current, old)
	if got.Totals != (ChangeCounts{New: 1, Persistent: 1}) || got.BySeverity["critical"].New != 1 || got.BySeverity["high"].Persistent != 1 {
		t.Fatalf("changes/severity: %+v", got)
	}
	current.Assets[0].Findings = nil
	got = comparisonResult(current, old)
	if got.Totals.Resolved != 1 || got.BySeverity["high"].Resolved != 1 {
		t.Fatalf("completed same coverage must resolve: %+v", got)
	}
	b := NewComparison(old)
	if b.Result().Totals.New != 1 {
		t.Fatal("first completed coverage must be new")
	}
}
func TestComparisonResolutionGuards(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Snapshot, *Snapshot)
	}{
		{"current failed", func(c, o *Snapshot) { c.Assets[0].Status = "incomplete"; c.Assets[0].Methods[0].Outcome = "failed" }},
		{"current stopped", func(c, o *Snapshot) { c.Assets[0].Status = "canceled"; c.Assets[0].Methods[0].Outcome = "canceled" }},
		{"stopped stale success", func(c, o *Snapshot) {
			c.Assets[0].Status = "canceled"
			c.Assets[0].Methods = append(c.Assets[0].Methods, Method{Action: "nmap.tcp_connect", Version: 1, Target: "127.0.0.1", Params: json.RawMessage(`{}`), Outcome: "failed"})
		}},
		{"current skipped", func(c, o *Snapshot) { c.Assets[0].Status = "skipped" }},
		{"skip reason", func(c, o *Snapshot) { c.Assets[0].SkipReason = "not_authorized" }},
		{"inconsistent incomplete", func(c, o *Snapshot) { c.Assets[0].Status = "incomplete" }},
		{"previous failed", func(c, o *Snapshot) { o.Assets[0].Status = "incomplete"; o.Assets[0].Methods[0].Outcome = "failed" }},
		{"previous stopped", func(c, o *Snapshot) { o.Assets[0].Status = "canceled" }},
		{"profile", func(c, o *Snapshot) { c.Profile = "full" }},
		{"target", func(c, o *Snapshot) { c.Assets[0].Methods[0].Target = "http://127.0.0.2:18080" }},
		{"tool", func(c, o *Snapshot) { c.Assets[0].Methods[0].Action = "nmap.tcp_connect" }},
		{"version", func(c, o *Snapshot) { c.Assets[0].Methods[0].Version = 99 }},
		{"parameters", func(c, o *Snapshot) { c.Assets[0].Methods[0].Params = json.RawMessage(`{"severity":["critical"]}`) }},
		{"invalid parameters", func(c, o *Snapshot) { c.Assets[0].Methods[0].Params = json.RawMessage(`{"bogus":"--exec"}`) }},
		{"asset", func(c, o *Snapshot) { c.Assets[0].ID = uuid.New() }},
		{"redacted target", func(c, o *Snapshot) {
			c.Assets[0].Methods[0].Target = "http://127.0.0.1/?token=[REDACTED]"
			o.Assets[0].Methods[0].Target = c.Assets[0].Methods[0].Target
		}},
		{"duplicate methods", func(c, o *Snapshot) { c.Assets[0].Methods = append(c.Assets[0].Methods, c.Assets[0].Methods[0]) }},
		{"duplicate current assets", func(c, o *Snapshot) { c.Assets = append(c.Assets, c.Assets[0], c.Assets[0]) }},
		{"duplicate baseline assets", func(c, o *Snapshot) { o.Assets = append(o.Assets, o.Assets[0]) }},
		{"foreign producer", func(c, o *Snapshot) { o.Assets[0].Findings[0].Coverage = []string{"foreign"} }},
		{"incompatible producer", func(c, o *Snapshot) { o.Assets[0].Findings[0].Type = "not-supported" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := comparisonFixture()
			current := comparisonCopy(old)
			current.Assets[0].Findings = nil
			tt.mutate(&current, &old)
			got := comparisonResult(current, old)
			if got.Totals.Resolved != 0 {
				t.Fatalf("unsafe resolved (%s): %+v", tt.name, got)
			}
		})
	}
}
func TestComparisonPartialToolsAndAllProducers(t *testing.T) {
	old := comparisonFixture()
	m := Method{Action: "nmap.tcp_connect", Version: 1, Target: "127.0.0.1", Params: json.RawMessage(`{}`), Outcome: "success"}
	old.Assets[0].Methods = append(old.Assets[0].Methods, m)
	old.Assets[0].Findings = append(old.Assets[0].Findings, Finding{ID: "port", Type: "open_port", Severity: "info", Coverage: []string{CoverageKey(m)}})
	current := comparisonCopy(old)
	current.Assets[0].Status = "incomplete"
	current.Assets[0].Methods[0].Outcome = "failed"
	current.Assets[0].Findings = nil
	got := comparisonResult(current, old)
	if got.Totals.Resolved != 1 || got.Totals.Uncompared != 1 || got.BySeverity["high"].Resolved != 0 {
		t.Fatalf("partial tool boundary: %+v", got)
	}
	// Two checks of the same tool can produce one finding; all original checks must finish.
	old = comparisonFixture()
	m = old.Assets[0].Methods[0]
	m.Target = "http://127.0.0.1:18081"
	old.Assets[0].Methods = append(old.Assets[0].Methods, m)
	old.Assets[0].Findings[0].Coverage = append(old.Assets[0].Findings[0].Coverage, CoverageKey(m))
	current = comparisonCopy(old)
	current.Assets[0].Findings = nil
	current.Assets[0].Status = "incomplete"
	current.Assets[0].Methods[1].Outcome = "canceled"
	if got = comparisonResult(current, old); got.Totals.Resolved != 0 || got.Totals.Uncompared != 1 {
		t.Fatalf("all producers required: %+v", got)
	}
	old.Assets[0].Findings[0].Coverage = nil
	current.Assets[0].Methods[1].Outcome = "success"
	current.Assets[0].Status = "finished"
	if got = comparisonResult(current, old); got.Totals.Resolved != 0 {
		t.Fatal("ambiguous legacy producers resolved")
	}
}
func TestComparisonLatestMatchingAttemptAndPerToolHistory(t *testing.T) {
	old := comparisonFixture()
	current := comparisonCopy(old)
	current.Assets[0].Findings = nil
	failed := comparisonCopy(old)
	failed.AssessmentID = uuid.New()
	failed.Assets[0].Status = "incomplete"
	failed.Assets[0].Methods[0].Outcome = "failed"
	failed.Assets[0].Findings = nil
	b := NewComparison(current)
	b.Add(failed, uuid.Nil)
	b.Add(old, uuid.Nil)
	got := b.Result()
	if got.Totals.Resolved != 0 || len(got.Checks) != 1 || got.Checks[0].Reason != "previous_not_completed" {
		t.Fatalf("latest failed must shadow success: %+v", got)
	}
	different := comparisonCopy(old)
	different.Profile = "other"
	different.Assets[0].Methods[0].Target = "http://127.0.0.2"
	b = NewComparison(current)
	b.Add(different, uuid.Nil)
	if b.Done() {
		t.Fatal("unrelated profile is not a baseline")
	}
	b.Add(old, uuid.Nil)
	if !b.Done() || b.Result().Totals.Resolved != 1 {
		t.Fatal("earlier matching check should be used across unrelated runs")
	}
}
func TestComparisonDeterministicAndDoesNotMutate(t *testing.T) {
	old := comparisonFixture()
	current := comparisonCopy(old)
	f := current.Assets[0].Findings[0]
	f.Severity = "critical"
	f.Title = "a"
	current.Assets[0].Findings = append(current.Assets[0].Findings, f)
	before, _ := json.Marshal(current)
	a := comparisonResult(current, old)
	after, _ := json.Marshal(current)
	if string(before) != string(after) {
		t.Fatal("comparison mutated immutable snapshot")
	}
	current.Assets[0].Findings[0], current.Assets[0].Findings[1] = current.Assets[0].Findings[1], current.Assets[0].Findings[0]
	b := comparisonResult(current, old)
	if !reflect.DeepEqual(a, b) || a.BySeverity["critical"].Persistent != 1 {
		t.Fatalf("order-dependent comparison: %+v %+v", a, b)
	}
	if CoverageKey(Method{Action: "nuclei.scan", Version: 1, Target: "http://127.0.0.1", Params: json.RawMessage(`{"x":`)}) != "" {
		t.Fatal("invalid parameters must fail closed")
	}
	m := old.Assets[0].Methods[0]
	k := CoverageKey(m)
	if k == "" {
		t.Fatal("valid fixture must have key")
	}
	m.Params = json.RawMessage(`{ "severity": ["critical", "high", "medium", "low", "info"] }`)
	if CoverageKey(m) == "" {
		t.Fatal("valid normalized parameters required")
	}
}

func TestComparisonCoverageIdentityIncludesVersionAndTool(t *testing.T) {
	m := comparisonFixture().Assets[0].Methods[0]
	k := CoverageKey(m)
	original := len(contract.Defs)
	defer func() { contract.Defs = contract.Defs[:original] }()
	next := *contract.FindVersion(m.Action, m.Version)
	next.Version = 2
	contract.Defs = append(contract.Defs, next)
	m.Version = 2
	if CoverageKey(m) == k {
		t.Fatal("definition version omitted from coverage identity")
	}
	next.Key = "nuclei.comparison_fixture"
	next.Version = 1
	contract.Defs = append(contract.Defs, next)
	m.Action = next.Key
	m.Version = 1
	if CoverageKey(m) == k {
		t.Fatal("tool omitted from coverage identity")
	}
}
func TestComparisonRequiresEveryProducingCheck(t *testing.T) {
	old := comparisonFixture()
	other := old.Assets[0].Methods[0]
	other.Target = "http://127.0.0.1:18081"
	old.Assets[0].Methods = append(old.Assets[0].Methods, other)
	old.Assets[0].Findings[0].Coverage = append(old.Assets[0].Findings[0].Coverage, CoverageKey(other))
	for failed := 0; failed < 2; failed++ {
		current := comparisonCopy(old)
		current.Assets[0].Status = "incomplete"
		current.Assets[0].Findings = nil
		current.Assets[0].Methods[failed].Outcome = "canceled"
		if got := comparisonResult(current, old); got.Totals.Resolved != 0 {
			t.Fatal("resolution ignored one of the producing checks")
		}
	}
}

func TestComparisonRetentionEpochPreventsFalseResolution(t *testing.T) {
	old := comparisonFixture()
	current := comparisonCopy(old)
	current.FindingsEpoch = 1
	current.Assets[0].Findings = nil
	got := comparisonResult(current, old)
	if got.Totals.Resolved != 0 || got.Totals.Uncompared != 1 || got.Rows[0].Reason != "identity_changed" {
		t.Fatalf("retention identity reset resolved an old identity: %+v", got)
	}
	current.Assets[0].Findings = append(current.Assets[0].Findings, Finding{ID: "rediscovered-uuid", Type: "vulnerability", Severity: "high", Value: old.Assets[0].Findings[0].Value, Coverage: old.Assets[0].Findings[0].Coverage})
	got = comparisonResult(current, old)
	if got.Totals.Resolved != 0 || got.Totals.New != 0 || got.Totals.Uncompared != 2 {
		t.Fatalf("rediscovery after expiry was misclassified: %+v", got)
	}
	subsequent := comparisonCopy(current)
	subsequent.Assets[0].Findings = nil
	got = comparisonResult(subsequent, current)
	if got.Totals.Resolved != 1 {
		t.Fatal("completed checks with the new identity epoch should compare again")
	}
}
