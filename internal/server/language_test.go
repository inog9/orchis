package server_test

import (
	"context"
	"orchis-go/internal/auth"
	"testing"
)

func TestLanguagePreferenceOwnAccountPersistenceAndValidation(t *testing.T) {
	a, env := setup(t)
	ctx := context.Background()
	admin := a.login("admin@a.test")
	viewer := a.login("viewer@a.test")
	read := func(cookie, want string) {
		t.Helper()
		r := a.do("GET", "/api/v1/auth/me", nil, cookie)
		var u struct {
			Language string `json:"ui_language"`
		}
		r.JSON(&u)
		if r.Status != 200 || u.Language != want {
			t.Fatalf("language: %d %q want %q", r.Status, u.Language, want)
		}
	}
	read(admin, "")
	read(viewer, "")
	for _, language := range []string{"id", "en", ""} {
		r := a.do("PATCH", "/api/v1/auth/preferences", map[string]any{"ui_language": language}, viewer)
		if r.Status != 200 {
			t.Fatal("Viewer must manage own preference", r.Status, string(r.Body))
		}
		read(viewer, language)
		read(a.login("viewer@a.test"), language)
		read(admin, "")
	}
	for _, language := range []string{"ID", "en-US", "<script>", "fr"} {
		if r := a.do("PATCH", "/api/v1/auth/preferences", map[string]any{"ui_language": language}, viewer); r.Status != 422 {
			t.Fatal("invalid locale accepted", r.Status)
		}
	}
	if r := a.do("PATCH", "/api/v1/auth/preferences", map[string]any{"ui_language": "id"}, viewer, "X-Requested-With", ""); r.Status != 403 {
		t.Fatal("CSRF", r.Status)
	}
	if r := a.do("PATCH", "/api/v1/auth/preferences", map[string]any{"ui_language": "id"}, ""); r.Status != 401 {
		t.Fatal("anonymous preference", r.Status)
	}
	if r := a.do("PATCH", "/api/v1/auth/preferences", map[string]any{"ui_language": "id", "user_id": "another-user"}, viewer); r.Status != 400 {
		t.Fatal("preference accepts target identity", r.Status)
	}
	svc := auth.NewService(env.App)
	// Use the authenticated HTTP identity, independent of the fixture password.
	var uid string
	if e := env.Owner.QueryRow(ctx, `SELECT id::text FROM users WHERE tenant_id=$1 AND email='viewer@a.test'`, env.TenantA).Scan(&uid); e != nil {
		t.Fatal(e)
	}
	var foreign auth.Principal
	foreign.TenantID = env.TenantB
	if e := foreign.UserID.UnmarshalText([]byte(uid)); e != nil {
		t.Fatal(e)
	}
	if e := svc.SetLanguage(ctx, &foreign, "id", ""); e == nil {
		t.Fatal("cross-tenant preference accepted")
	}
	var audited int
	if e := env.Owner.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='user.language' AND resource_id=$2`, env.TenantA, uid).Scan(&audited); e != nil || audited < 3 {
		t.Fatal("language change not audited", audited, e)
	}
	read(viewer, "")
	if _, e := env.Owner.Exec(ctx, `UPDATE users SET ui_language='fr' WHERE id=$1`, uid); e == nil {
		t.Fatal("database locale allowlist absent")
	}
}

func TestScanLanguageLaunchSelectionAndIdempotency(t *testing.T) {
	f := newScanFixture(t, true)
	o := createOrg(t, f.a, f.admin, "language")
	if r := f.a.do("PUT", "/api/v1/organizations/"+o.ID.String()+"/assets", map[string]any{"assets": []map[string]any{{"id": f.asset, "version": 1}}}, f.admin); r.Status != 204 {
		t.Fatal(r.Status)
	}
	if r := f.a.do("PATCH", "/api/v1/auth/preferences", map[string]any{"ui_language": "id"}, f.analyst); r.Status != 200 {
		t.Fatal(r.Status)
	}
	web := mkAsset(t, f.a, f.admin, map[string]any{"category": "web_application", "name": "Language web", "organization_id": o.ID, "testing_authorized": true, "addresses": []map[string]any{{"kind": "url", "value": "http://127.0.0.1:8080/"}}})
	in := map[string]any{"organization_id": o.ID, "asset_ids": []string{web}}
	r := f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "language-fallback")
	var s struct{ ID string }
	r.JSON(&s)
	if r.Status != 201 {
		t.Fatal(r.Status, string(r.Body))
	}
	var lang string
	if e := f.a.env.Owner.QueryRow(context.Background(), `SELECT report_language FROM assessments WHERE id=$1`, s.ID).Scan(&lang); e != nil || lang != "id" {
		t.Fatal("preference fallback", lang, e)
	}
	in["language"] = "en"
	if r = f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "language-fallback"); r.Status != 409 {
		t.Fatal("changed language reused launch", r.Status)
	}
	for _, bad := range []string{"en-US", "fr", "<script>"} {
		in["language"] = bad
		if r = f.a.do("POST", "/api/v1/scans", in, f.analyst, "Idempotency-Key", "language-invalid"); r.Status != 422 {
			t.Fatal("invalid report language", r.Status)
		}
	}
}
