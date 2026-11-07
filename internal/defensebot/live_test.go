//go:build live

package defensebot

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"orchis-go/internal/osquery"
)

// Manual smoke test against the real world: real osquery on this machine and the real public services.
//
//	ORCHIS_LIVE_OSQUERY=/path/to/osqueryd go test -tags live -run TestLive -v -timeout 30m ./internal/defensebot
//
// It makes network calls to OSV, CISA and FIRST. It is never part of CI.
func TestLiveRunOnThisMachine(t *testing.T) {
	bin := os.Getenv("ORCHIS_LIVE_OSQUERY")
	if bin == "" {
		t.Skip("set ORCHIS_LIVE_OSQUERY to a pinned osqueryd binary")
	}
	runner := &osquery.Runner{Bin: bin}
	in := Input{Datasets: map[string]DatasetInput{}, Language: "en"}
	for _, key := range Queries {
		q := osquery.Find(key)
		if q == nil || !q.SupportsOS("linux") {
			continue
		}
		ds := runner.Run(context.Background(), q)
		at := ds.CollectedAt
		in.Datasets[key] = DatasetInput{Status: string(ds.Status), Reason: ds.Reason, CollectedAt: &at, Truncated: ds.Truncated, Rows: ds.Rows}
		t.Logf("%-22s %-8s rows=%d truncated=%v %s", key, ds.Status, len(ds.Rows), ds.Truncated, ds.Reason)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	started := time.Now()
	out := Run(ctx, in, Deps{OSV: &OSV{Concurrency: 12}, NVD: &NVD{}, Enrich: &Enricher{}})
	s := out.Summary
	t.Logf("done in %s: disposition=%s software=%d unmapped=%d findings=%d omitted=%d kev=%d exposed=%d reboot=%d hardening=%d", time.Since(started).Round(time.Second),
		s.Disposition, s.SoftwareChecked, s.SoftwareUnmapped, s.Findings, s.Omitted, s.KnownExploited, s.Exposed, s.RebootRequired, s.Hardening)
	t.Logf("by severity: %v", s.BySeverity)
	t.Logf("provenance: %+v", s.Provenance)
	for _, n := range s.Notes {
		t.Logf("note: %+v", n)
	}
	titles := []string{}
	for _, f := range out.Findings {
		titles = append(titles, f.Severity+" "+f.Title)
	}
	sort.Strings(titles)
	for i, title := range titles {
		if i >= 12 {
			break
		}
		t.Logf("finding: %s", title)
	}
	if s.SoftwareChecked == 0 {
		t.Fatal("no software was checked")
	}
	if s.Provenance.KEVDate == "" || s.Provenance.EPSSDate == "" {
		t.Errorf("exploitation data missing: %+v", s.Provenance)
	}
}
