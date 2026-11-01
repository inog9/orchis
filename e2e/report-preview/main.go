// Alat QA halaman cetak: snapshot di stdin atau fixture sintetis yang diberi label jelas.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"io"
	"orchis-go/internal/reporting"
	"os"
	"time"
)

func large() reporting.Snapshot {
	now := time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)
	s := reporting.Snapshot{IsDemo: true, Label: "DEMO / SYNTHETIC DATA - layout test, not a real assessment.", Organization: "Demo Meridian Research Cooperative", AssessmentID: uuid.MustParse("00000000-0000-4000-8000-000000000001"), Profile: "quick", CreatedAt: now, Status: "finished", Risk: "High exposure", BySeverity: map[string]int{}, Remediation: "Synthetic examples for layout review. Validate each observation and agree remediation owners before use.", Comparison: "No comparable previous scope/profile. Incomplete checks do not establish resolution."}
	severities := []string{"critical", "high", "medium", "low", "info"}
	for i := 0; i < 12; i++ {
		a := reporting.Asset{ID: uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", i+10)), Name: fmt.Sprintf("Demo endpoint %02d — application and research services", i+1), Status: "finished", Findings: []reporting.Finding{}}
		if i >= 9 {
			a.Status = "skipped"
			a.SkipReason = "Testing has not been authorized by the administrator."
		} else if i == 8 {
			a.Status = "incomplete"
		}
		if i < 9 {
			for j := 0; j < 4; j++ {
				severity := severities[(i+j)%len(severities)]
				s.BySeverity[severity]++
				f := reporting.Finding{ID: fmt.Sprintf("demo-%d-%d", i, j), Type: "vulnerability", Severity: severity, Title: fmt.Sprintf("Synthetic observation %02d.%d: service configuration requires review", i+1, j+1), Value: "DEMO ONLY: a curated example used to verify report layout; no endpoint was scanned for this fixture.", Evidence: map[string]any{"source": "synthetic layout fixture", "matched_at": "http://127.0.0.1/demo/"}}
				if j == 0 {
					f.MITRE = reporting.EstimatedTechniques(f)
				}
				a.Findings = append(a.Findings, f)
			}
			end := now.Add(time.Minute)
			for _, action := range []string{"nmap.tcp_connect", "nuclei.scan"} {
				a.Methods = append(a.Methods, reporting.Method{Action: action, Version: 1, Params: json.RawMessage(`{"profile":"quick","ports":"80,443,8080"}`), Target: "127.0.0.1", Outcome: "success", StartedAt: &now, EndedAt: &end})
			}
			if i == 8 {
				a.Methods[1].Outcome = "error"
				a.Methods[1].Error = "Synthetic scan failure: endpoint could not be reached; no safety conclusion can be made."
			}
		}
		s.Assets = append(s.Assets, a)
	}
	return s
}

func main() {
	fixture := flag.Bool("large", false, "render the synthetic multi-page fixture")
	snapshotOut := flag.String("snapshot-out", "", "optional fixture JSON destination")
	flag.Parse()
	var s reporting.Snapshot
	if *fixture {
		s = large()
	} else {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 16<<20+1))
		if err != nil || len(b) > 16<<20 {
			panic("invalid or oversized snapshot")
		}
		if err = json.Unmarshal(b, &s); err != nil {
			panic(err)
		}
		if s.Organization == "" {
			panic("snapshot requires organisation")
		}
	}
	if *snapshotOut != "" {
		b, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			panic(err)
		}
		if err = os.WriteFile(*snapshotOut, b, 0600); err != nil {
			panic(err)
		}
	}
	html, err := reporting.HTML(s)
	if err != nil {
		panic(err)
	}
	if _, err = os.Stdout.Write(html); err != nil {
		panic(err)
	}
}
