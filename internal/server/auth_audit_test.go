package server_test

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http/httptest"
	"orchis-go/internal/audit"
	"orchis-go/internal/auditview"
	"orchis-go/internal/auth"
	"orchis-go/internal/server"
	"strings"
	"sync"
	"testing"
)

func TestAuditPrivacyPaginationAndTenant(t *testing.T) {
	a, env := setup(t)
	ctx := context.Background()
	admin := a.login("admin@a.test")
	for _, email := range []string{"guess-one@invalid.test", "guess-two@invalid.test", "admin@a.test"} {
		if r := a.do("POST", "/api/v1/auth/login", map[string]any{"email": email, "password": "Wrong-Secret-9"}, ""); r.Status != 401 {
			t.Fatal(r.Status)
		}
	}
	var n int
	if e := env.Owner.QueryRow(ctx, `SELECT count(*) FROM login_throttle WHERE email IN ('guess-one@invalid.test','guess-two@invalid.test')`).Scan(&n); e != nil || n != 0 {
		t.Fatalf("guessed identity persisted: %d %v", n, e)
	}
	if e := env.Owner.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE detail::text LIKE '%guess-%' OR detail::text LIKE '%Wrong-Secret-9%'`).Scan(&n); e != nil || n != 0 {
		t.Fatal("audit leaked credentials or guesses", e)
	}
	foreign := uuid.New()
	if e := env.App.InTenant(ctx, env.TenantB, func(tx pgx.Tx) error {
		return audit.Write(ctx, tx, audit.Entry{Tenant: env.TenantB, Action: "auth.login_failed", ResourceType: "session", ResourceID: &foreign, Detail: map[string]any{"reason": "invalid_credentials", "password": "do-not-export"}})
	}); e != nil {
		t.Fatal(e)
	}
	cursor := ""
	seen := map[uuid.UUID]bool{}
	for range 4 {
		path := "/api/v1/audit?action=auth.login_failed&limit=1"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		r := a.do("GET", path, nil, admin)
		if r.Status != 200 {
			t.Fatal(r.Status, string(r.Body))
		}
		if strings.Contains(string(r.Body), "do-not-export") || strings.Contains(string(r.Body), foreign.String()) {
			t.Fatal("audit escaped tenant/detail scope")
		}
		var page auditview.Page
		r.JSON(&page)
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatal("duplicate cursor row")
			}
			seen[item.ID] = true
			if item.Reason != "invalid_credentials" || item.RequestID == "" {
				t.Fatal("missing safe audit metadata")
			}
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if len(seen) != 3 {
		t.Fatalf("audit pagination got %d", len(seen))
	}
	viewer := a.login("viewer@a.test")
	if r := a.do("GET", "/api/v1/audit", nil, viewer); r.Status != 403 {
		t.Fatal("audit permission missing", r.Status)
	}
	if r := a.do("GET", "/api/v1/audit", nil, ""); r.Status != 401 {
		t.Fatal(r.Status)
	}
	for _, path := range []string{"/api/v1/audit?cursor=broken", "/api/v1/audit?limit=101"} {
		if r := a.do("GET", path, nil, admin); r.Status < 400 || r.Status >= 500 {
			t.Fatal("bad pagination accepted", r.Status)
		}
	}
	if r := a.do("POST", "/api/v1/auth/logout", nil, admin); r.Status != 204 {
		t.Fatal(r.Status)
	}
	if e := env.Owner.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='auth.logout'`, env.TenantA).Scan(&n); e != nil || n != 1 {
		t.Fatal("logout not audited", e, n)
	}
}
func TestConcurrentUnknownFailuresDoNotLoseCounts(t *testing.T) {
	_, env := setup(t)
	svc := auth.NewService(env.App)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			_, e := svc.Login(context.Background(), env.TenantA, fmt.Sprintf("guess-%d@invalid.test", i), "wrong", "192.0.2.10", "", false)
			if e == nil {
				t.Error("unexpected login")
			}
		})
	}
	wg.Wait()
	var attempts, count int
	if e := env.Owner.QueryRow(context.Background(), `SELECT failed_attempts FROM login_throttle WHERE tenant_id=$1 AND email='unknown:192.0.2.10'`, env.TenantA).Scan(&attempts); e != nil || attempts != 5 {
		t.Fatalf("lost failures: %d %v", attempts, e)
	}
	if e := env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='auth.login_failed'`, env.TenantA).Scan(&count); e != nil || count != 8 {
		t.Fatal("missing failure audit", count, e)
	}
}
func TestLoginAuditIsAtomic(t *testing.T) {
	a, env := setup(t)
	ctx := context.Background()
	_, e := env.Owner.Exec(ctx, `CREATE FUNCTION fail_login_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='auth.login' THEN RAISE EXCEPTION 'test audit unavailable';END IF;RETURN NEW;END $$;CREATE TRIGGER reject_login_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION fail_login_audit()`)
	if e != nil {
		t.Fatal(e)
	}
	r := a.do("POST", "/api/v1/auth/login", map[string]any{"email": "admin@a.test", "password": pw}, "")
	if r.Status != 500 || r.Header.Get("Set-Cookie") != "" {
		t.Fatal("login succeeded without audit", r.Status)
	}
	var n int
	if e = env.Owner.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE tenant_id=$1`, env.TenantA).Scan(&n); e != nil || n != 0 {
		t.Fatal("session committed without audit", n, e)
	}
}

func TestPublicRateLimitsAvoidCredentialWork(t *testing.T) {
	_, env := setup(t)
	srv := httptest.NewServer(server.New(server.Options{DB: env.App, Tenant: env.TenantA, PublicRateLimit: true}))
	defer srv.Close()
	a := &app{t: t, env: env, srv: srv}
	a.login("admin@a.test")
	for i := range 11 {
		r := a.do("POST", "/api/v1/auth/login", map[string]any{"email": fmt.Sprintf("guess-%d@invalid.test", i), "password": "wrong"}, "")
		if i >= 9 && (r.Status != 429 || !strings.Contains(string(r.Body), "Too many requests") || r.Header.Get("Retry-After") == "" || r.Header.Get("X-Request-Id") == "") {
			t.Fatal("public rate limit not applied", r.Status, string(r.Body))
		}
	}
	var n int
	if e := env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='auth.login_failed'`, env.TenantA).Scan(&n); e != nil || n != 9 {
		t.Fatal("limited calls reached credential work", n, e)
	}
}
