package blueteam

import (
	"context"
	"testing"
	"time"

	"orchis-go/internal/ids"
	"orchis-go/internal/testutil"
)

type staleEnv struct {
	t     *testing.T
	env   *testutil.Env
	ctx   context.Context
	asset [16]byte
	in    Input
	fid   string
}

func newStale(t *testing.T) (*staleEnv, Input) {
	env := testutil.New(t)
	ctx := context.Background()
	org, asset := ids.New(), ids.New()
	if _, err := env.Owner.Exec(ctx, `INSERT INTO organizations (id, tenant_id, name, slug) VALUES ($1,$2,'O','o')`, org, env.TenantA); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `INSERT INTO assets (id, tenant_id, organization_id, category, name, source) VALUES ($1,$2,$3,'host','h','manual')`, asset, env.TenantA, org); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	hb := now
	in := Input{Tenant: env.TenantA, Asset: asset, AssetName: "h", OrgID: &org, Now: now,
		Agents: []AgentRow{{ID: ids.New(), Hostname: "h", Status: "online", LastHeartbeat: &hb}}}
	f := Finding{Type: "new_listening_port", Value: "0.0.0.0:9999", Title: "New listening port 9999 on 0.0.0.0", Severity: "critical",
		Details: map[string]any{"row": map[string]string{"port": "9999", "address": "0.0.0.0"}}}
	tx, err := env.Owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(ctx, tx, in, []Finding{f}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return &staleEnv{t: t, env: env, ctx: ctx, in: in}, in
}

func (s *staleEnv) status() (string, int) {
	var st string
	var n int
	if err := s.env.Owner.QueryRow(s.ctx, `SELECT triage_status, absent_count FROM findings WHERE asset_id=$1 AND type='new_listening_port'`, s.in.Asset).Scan(&st, &n); err != nil {
		s.t.Fatal(err)
	}
	return st, n
}

func (s *staleEnv) mark(rows []map[string]string, collected time.Time) {
	in := s.in
	in.Datasets = map[string][]map[string]string{"osquery.network": rows}
	tx, err := s.env.Owner.Begin(s.ctx)
	if err != nil {
		s.t.Fatal(err)
	}
	if err := MarkStale(s.ctx, tx, in, &collected); err != nil {
		_ = tx.Rollback(s.ctx)
		s.t.Fatal(err)
	}
	if err := tx.Commit(s.ctx); err != nil {
		s.t.Fatal(err)
	}
}

func TestPortBecomesStaleAfterTwoConsecutiveCollectionsAndNeverBefore(t *testing.T) {
	s, _ := newStale(t)
	other := []map[string]string{{"port": "445", "address": "0.0.0.0"}}
	t0 := time.Now()
	s.mark(other, t0)
	if st, n := s.status(); st != "open" || n != 1 {
		t.Fatalf("after one miss: %s/%d", st, n)
	}
	s.mark(other, t0) // pengambilan yang SAMA tidak dihitung dua kali
	if st, n := s.status(); st != "open" || n != 1 {
		t.Fatalf("same collection counted twice: %s/%d", st, n)
	}
	s.mark(other, t0.Add(time.Minute))
	if st, _ := s.status(); st != "stale" {
		t.Fatalf("expected stale after two consecutive collections, got %s", st)
	}
}

func TestPortSeenAgainResetsAndReopens(t *testing.T) {
	s, _ := newStale(t)
	other := []map[string]string{{"port": "445", "address": "0.0.0.0"}}
	back := []map[string]string{{"port": "9999", "address": "0.0.0.0"}}
	t0 := time.Now()
	s.mark(other, t0)
	s.mark(back, t0.Add(time.Minute))
	if st, n := s.status(); st != "open" || n != 0 {
		t.Fatalf("a port seen again must reset the counter: %s/%d", st, n)
	}
	s.mark(other, t0.Add(2*time.Minute))
	s.mark(other, t0.Add(3*time.Minute))
	if st, _ := s.status(); st != "stale" {
		t.Fatalf("want stale, got %s", st)
	}
	s.mark(back, t0.Add(4*time.Minute))
	if st, n := s.status(); st != "open" || n != 0 {
		t.Fatalf("a stale port that returns must reopen: %s/%d", st, n)
	}
}

func TestOfflineAgentNeverCausesStale(t *testing.T) {
	s, _ := newStale(t)
	// Tidak ada pengambilan data baru: dataset tidak ada / collectedAt tidak diberikan.
	in := s.in
	tx, err := s.env.Owner.Begin(s.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := MarkStale(s.ctx, tx, in, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(s.ctx); err != nil {
		t.Fatal(err)
	}
	if st, n := s.status(); st != "open" || n != 0 {
		t.Fatalf("no new collection must change nothing: %s/%d", st, n)
	}
}

func TestManualTriageIsNeverOverriddenByStale(t *testing.T) {
	s, _ := newStale(t)
	if _, err := s.env.Owner.Exec(s.ctx, `UPDATE findings SET triage_status='false_positive' WHERE asset_id=$1`, s.in.Asset); err != nil {
		t.Fatal(err)
	}
	other := []map[string]string{{"port": "445", "address": "0.0.0.0"}}
	t0 := time.Now()
	s.mark(other, t0)
	s.mark(other, t0.Add(time.Minute))
	if st, _ := s.status(); st != "false_positive" {
		t.Fatalf("manual triage must be kept, got %s", st)
	}
}
