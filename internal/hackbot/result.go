// Package hackbot defines the versioned boundary to the unattended pentest engine.
package hackbot

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Finding struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Severity string          `json:"severity"`
	Value    string          `json:"value"`
	Details  json.RawMessage `json:"details"`
}
type Result struct {
	Schema        int       `json:"schema"`
	SessionID     string    `json:"session_id"`
	Disposition   string    `json:"disposition"`
	Phase         string    `json:"phase"`
	Summary       string    `json:"summary"`
	Phases        []string  `json:"phases"`
	VerifiedTasks int       `json:"verified_tasks"`
	Blockers      []string  `json:"blockers"`
	Findings      []Finding `json:"findings"`
	Report        string    `json:"report"`
	Validation    string    `json:"validation"`
}
type Progress struct {
	SessionID string `json:"session_id,omitempty"`
	Phase     string `json:"phase,omitempty"`
}

func (p Progress) Valid() bool {
	if len(p.SessionID) > 80 || strings.IndexFunc(p.SessionID, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-')
	}) >= 0 {
		return false
	}
	switch p.Phase {
	case "setup", "exploration", "recon", "auth", "authz", "injection", "fuzzing", "endpoint-fuzz-full", "gadgets", "reflection", "evidence", "report":
		return true
	}
	return false
}

func LimitSummary(s string) string {
	words := strings.Fields(s)
	if len(words) > 300 {
		words = words[:300]
	}
	return strings.Join(words, " ")
}
func (r *Result) Validate() error {
	if r.Schema != 1 || r.SessionID == "" || !(Progress{SessionID: r.SessionID, Phase: r.Phase}).Valid() {
		return fmt.Errorf("invalid orchis-redteam-agent result identity")
	}
	switch r.Disposition {
	case "complete", "limited", "input_required", "failed", "cancelled":
	default:
		return fmt.Errorf("invalid orchis-redteam-agent disposition")
	}
	if len(r.Phases) > 13 || r.VerifiedTasks < 0 {
		return fmt.Errorf("invalid orchis-redteam-agent coverage")
	}
	for _, phase := range r.Phases {
		if !(Progress{Phase: phase}).Valid() {
			return fmt.Errorf("invalid orchis-redteam-agent coverage phase")
		}
	}
	if len(r.Report) > 1<<20 || len(r.Validation) > 1<<20 || len(r.Summary) > 32000 || len(r.Findings) > 2000 || len(r.Blockers) > 100 {
		return fmt.Errorf("orchis-redteam-agent result too large")
	}
	if strings.TrimSpace(r.Report) == "" || strings.TrimSpace(r.Validation) == "" {
		return fmt.Errorf("orchis-redteam-agent report artifacts are missing")
	}
	r.Summary = LimitSummary(r.Summary)
	if r.Summary == "" {
		r.Summary = fmt.Sprintf("orchis-redteam-agent completed %d verified tasks and validated %d findings. Delivery: %s. See the report for coverage and limitations.", r.VerifiedTasks, len(r.Findings), r.Disposition)
	}
	for _, f := range r.Findings {
		var details struct {
			Verdict string `json:"verdict"`
		}
		if f.ID == "" || f.Title == "" || len(f.Title) > 300 || f.Value == "" || len(f.Value) > 500 || len(f.Details) > 8<<10 || json.Unmarshal(f.Details, &details) != nil || details.Verdict != "VALIDATED" {
			return fmt.Errorf("invalid orchis-redteam-agent finding")
		}
		switch f.Severity {
		case "info", "low", "medium", "high", "critical":
		default:
			return fmt.Errorf("invalid orchis-redteam-agent severity")
		}
	}
	return nil
}
