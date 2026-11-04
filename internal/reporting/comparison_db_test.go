package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/ids"
	"orchis-go/internal/testutil"
	"testing"
	"time"
)

func TestComparisonHistoryIsolationLatestFailureAndFreeze(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	org, otherOrg, actor := ids.New(), ids.New(), ids.New()
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := env.Owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO organizations(id,tenant_id,name,slug) VALUES($1,$2,'Comparison','comparison'),($3,$2,'Other','other')`, org, env.TenantA, otherOrg)
	must(`INSERT INTO users(id,tenant_id,email,display_name,role_id) SELECT $1,$2,'comparison@a.test','Fixture',id FROM roles WHERE tenant_id=$2 AND key='admin'`, actor, env.TenantA)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := Service{DB: env.App}
	n := 0
	save := func(snapshot Snapshot, organisation uuid.UUID) Snapshot {
		t.Helper()
		n++
		snapshot.AssessmentID = ids.New()
		snapshot.ScanID = &snapshot.AssessmentID
		snapshot.CreatedAt = start.Add(time.Duration(n) * time.Minute)
		for i := range snapshot.Assets {
			snapshot.Assets[i].Discovered = true
		}
		must(`INSERT INTO assessments(id,tenant_id,organization_id,name,profile,idempotency_key,request_hash,created_by,is_scan,status,created_at) VALUES($1,$2,$3,'Comparison',$4,$7,'fixture',$5,true,'finished',$6)`, snapshot.AssessmentID, env.TenantA, organisation, snapshot.Profile, actor, snapshot.CreatedAt, snapshot.AssessmentID.String())
		if err := env.App.InTenant(ctx, env.TenantA, func(tx pgx.Tx) error {
			return svc.FreezeScanSnapshot(ctx, tx, env.TenantA, organisation, snapshot.AssessmentID, snapshot)
		}); err != nil {
			t.Fatal(err)
		}
		var raw []byte
		if err := env.Owner.QueryRow(ctx, `SELECT snapshot FROM reports WHERE assessment_id=$1`, snapshot.AssessmentID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var frozen Snapshot
		if err := json.Unmarshal(raw, &frozen); err != nil {
			t.Fatal(err)
		}
		return frozen
	}
	old := save(comparisonFixture(), org)
	if old.ComparisonDetails == nil || old.ComparisonDetails.Totals.New != 1 {
		t.Fatal("first snapshot missing structured initial finding")
	}
	foreign := comparisonCopy(old)
	foreign.Assets[0].Findings = nil
	save(foreign, otherOrg)
	unrelated := comparisonCopy(old)
	unrelated.Profile = "different"
	unrelated.Assets[0].Findings = nil
	save(unrelated, org)
	current := comparisonCopy(old)
	current.Assets[0].Findings = nil
	frozen := save(current, org)
	if frozen.ComparisonDetails.Totals.Resolved != 1 || frozen.ComparisonDetails.Checks[0].BaselineAssessmentID == nil || *frozen.ComparisonDetails.Checks[0].BaselineAssessmentID != old.AssessmentID {
		t.Fatalf("wrong organisation/profile baseline: %+v", frozen.ComparisonDetails)
	}
	failed := comparisonCopy(old)
	failed.Assets[0].Status = "incomplete"
	failed.Assets[0].Methods[0].Outcome = "failed"
	failed = save(failed, org)
	if failed.ComparisonDetails.Totals.Resolved != 0 {
		t.Fatal("failed latest scan resolved observations")
	}
	later := save(current, org)
	if later.ComparisonDetails.Totals.Resolved != 0 || later.ComparisonDetails.Checks[0].Reason != "previous_not_completed" || *later.ComparisonDetails.Checks[0].BaselineAssessmentID != failed.AssessmentID {
		t.Fatalf("failed matching attempt was skipped: %+v", later.ComparisonDetails)
	}
	before, _ := json.Marshal(frozen.ComparisonDetails)
	must(`UPDATE reports SET snapshot=jsonb_set(snapshot,'{assets}','[]') WHERE assessment_id=$1`, old.AssessmentID)
	var raw []byte
	if err := env.Owner.QueryRow(ctx, `SELECT snapshot->'comparison_details' FROM reports WHERE assessment_id=$1`, frozen.AssessmentID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var actual Comparison
	if err := json.Unmarshal(raw, &actual); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(actual)
	if string(before) != string(after) {
		t.Fatal("frozen comparison changed when baseline changed")
	}
	var count int
	if err := env.App.InTenantRead(ctx, env.TenantB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM reports WHERE organization_id=$1`, org).Scan(&count)
	}); err != nil || count != 0 {
		t.Fatalf("comparison tenant leak: %d %v", count, err)
	}
	save(old, org)
	must(`UPDATE organizations SET findings_epoch=findings_epoch+1 WHERE tenant_id=$1 AND id=$2`, env.TenantA, org)
	reset := save(current, org)
	if reset.FindingsEpoch != 1 || reset.ComparisonDetails.Totals.Resolved != 0 || reset.ComparisonDetails.Totals.Uncompared != 1 {
		t.Fatalf("retention identity reset lost at freeze: %+v", reset.ComparisonDetails)
	}
	// A foreign organisation is invisible through RLS, including its identity epoch.
	if err := env.App.InTenantRead(ctx, env.TenantB, func(tx pgx.Tx) error {
		probe := comparisonCopy(current)
		probe.CreatedAt = start.Add(time.Hour)
		probe.AssessmentID = uuid.New()
		return freezeComparison(ctx, tx, env.TenantB, org, &probe, true)
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign organisation must remain invisible: %v", err)
	}
}
