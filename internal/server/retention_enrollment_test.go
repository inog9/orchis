package server_test

import (
	"context"
	"orchis-go/internal/retention"
	"orchis-go/internal/scancontrol"
	"testing"
	"time"
)

func TestEnrollmentWaitsForDeletionLockBeforeTokenRow(t *testing.T) {
	f := newAgentFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	org := createOrg(t, f.a, f.admin, "enroll-delete-lock")
	r := f.a.do("POST", "/api/v1/agents/enrollment-tokens", map[string]any{"os": "linux", "arch": "x86_64", "mode": "service", "organization_id": org.ID}, f.admin)
	if r.Status != 201 {
		t.Fatal(r.Status, string(r.Body))
	}
	var token struct{ ID, Token string }
	r.JSON(&token)
	tx, err := f.a.env.App.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err = scancontrol.Lock(ctx, tx, f.a.env.TenantA); err != nil {
		t.Fatal(err)
	}
	response := make(chan resp, 1)
	go func() {
		response <- f.a.do("POST", "/api/v1/agent/enroll", f.enrollBody(token.Token, "fixture-lock-host"), "", "X-Requested-With", "")
	}()
	blocked := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		select {
		case r := <-response:
			t.Fatalf("enrollment bypassed deletion serialization: %d", r.Status)
		default:
		}
		if err = f.a.env.Owner.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks l JOIN pg_stat_activity a ON a.pid=l.pid WHERE a.datname=current_database() AND l.locktype='advisory' AND NOT l.granted)`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("enrollment did not wait for deletion lock")
	}
	// A waiting enrollment must not hold the token row first (the opposite order deadlocks).
	probe, err := f.a.env.Owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = probe.Exec(ctx, `SELECT id FROM agent_enrollment_tokens WHERE id=$1 FOR UPDATE NOWAIT`, token.ID)
	_ = probe.Rollback(ctx)
	if err != nil {
		t.Fatal("enrollment locked token before tenant deletion lock", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case r = <-response:
		if r.Status != 200 {
			t.Fatal(r.Status, string(r.Body))
		}
	case <-ctx.Done():
		t.Fatal("enrollment did not resume")
	}
	var enrolled enrollOut
	r.JSON(&enrolled)
	path := "/api/v1/organizations/" + org.ID.String()
	r = f.a.do("GET", path+"/deletion-preview", nil, f.admin)
	var preview retention.Preview
	r.JSON(&preview)
	r = f.a.do("DELETE", path, retention.DeleteInput{Name: preview.Name, Version: preview.Version, Counts: preview.Counts}, f.admin)
	if r.Status != 200 {
		t.Fatal(r.Status, string(r.Body))
	}
	r = f.agentCall("POST", "heartbeat", map[string]any{"version": "1.0.0"}, enrolled.Secret)
	if r.Status != 401 {
		t.Fatal("deleted agent credentials remain valid", r.Status)
	}
	r = f.a.do("POST", "/api/v1/agent/enroll", f.enrollBody(token.Token, "fixture-reuse"), "", "X-Requested-With", "")
	if r.Status != 400 {
		t.Fatal("deleted token can enroll again", r.Status)
	}
}
