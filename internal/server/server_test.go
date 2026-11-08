package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"orchis-go/internal/bootstrap"
	"orchis-go/internal/contract"
	"orchis-go/internal/server"
	"orchis-go/internal/testutil"
)

const pw = "Correct-Horse-9"

type app struct {
	t   *testing.T
	env *testutil.Env
	srv *httptest.Server
}

type resp struct {
	Status int
	Header http.Header
	Body   []byte
}

func (r resp) JSON(v any) {
	if err := json.Unmarshal(r.Body, v); err != nil {
		panic(fmt.Sprintf("bukan JSON: %s", r.Body))
	}
}

func newApp(t *testing.T, env *testutil.Env, tenant uuid.UUID) *app {
	return newAppGuard(t, env, tenant, contract.Guard{AllowLoopback: true})
}

// testReleaseDir: direktori rilis agent untuk server uji berikutnya (diisi tes yang membutuhkannya).
var testReleaseDir string

func newAppGuard(t *testing.T, env *testutil.Env, tenant uuid.UUID, g contract.Guard) *app {
	srv := httptest.NewServer(server.New(server.Options{DB: env.App, Tenant: tenant, Guard: g, AgentRelease: testReleaseDir, AutoConsent: true}))
	t.Cleanup(srv.Close)
	return &app{t: t, env: env, srv: srv}
}

func (a *app) do(method, path string, body any, cookie string, headers ...string) resp {
	a.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, a.srv.URL+path, rd)
	req.Header.Set("X-Requested-With", "orchis-web")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "orchis_session", Value: cookie})
	}
	for i := 0; i+1 < len(headers); i += 2 {
		if headers[i+1] == "" {
			req.Header.Del(headers[i])
		} else {
			req.Header.Set(headers[i], headers[i+1])
		}
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return resp{res.StatusCode, res.Header, b}
}

func (a *app) user(tenant uuid.UUID, email, role string) {
	a.t.Helper()
	if _, err := bootstrap.CreateUser(context.Background(), a.env.App, tenant, email, role+" user", role, pw); err != nil {
		a.t.Fatal(err)
	}
}

func (a *app) login(email string) string {
	a.t.Helper()
	r := a.do("POST", "/api/v1/auth/login", map[string]any{"email": email, "password": pw}, "")
	if r.Status != 200 {
		a.t.Fatalf("login %s gagal: %d %s", email, r.Status, r.Body)
	}
	for _, c := range (&http.Response{Header: r.Header}).Cookies() {
		if c.Name == "orchis_session" {
			return c.Value
		}
	}
	a.t.Fatal("cookie sesi tidak ada")
	return ""
}

func setup(t *testing.T) (*app, *testutil.Env) {
	env := testutil.New(t)
	a := newApp(t, env, env.TenantA)
	a.user(env.TenantA, "admin@a.test", "admin")
	a.user(env.TenantA, "analyst@a.test", "analyst")
	a.user(env.TenantA, "viewer@a.test", "viewer")
	return a, env
}

func TestLoginMeLogout(t *testing.T) {
	a, _ := setup(t)
	r := a.do("POST", "/api/v1/auth/login", map[string]any{"email": "ADMIN@a.test ", "password": pw}, "")
	if r.Status != 200 {
		t.Fatalf("login: %d %s", r.Status, r.Body)
	}
	var sc string
	for _, c := range (&http.Response{Header: r.Header}).Cookies() {
		if c.Name == "orchis_session" {
			sc = c.Value
			if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
				t.Errorf("cookie harus HttpOnly dan SameSite=Lax: %+v", c)
			}
		}
	}
	if sc == "" {
		t.Fatal("tidak ada cookie sesi")
	}
	me := a.do("GET", "/api/v1/auth/me", nil, sc)
	var u struct {
		Email       string   `json:"email"`
		Role        string   `json:"role"`
		Permissions []string `json:"permissions"`
	}
	me.JSON(&u)
	if me.Status != 200 || u.Email != "admin@a.test" || u.Role != "admin" || len(u.Permissions) < 10 {
		t.Fatalf("me salah: %d %s", me.Status, me.Body)
	}
	if out := a.do("POST", "/api/v1/auth/logout", nil, sc); out.Status != 204 {
		t.Fatalf("logout: %d", out.Status)
	}
	if after := a.do("GET", "/api/v1/auth/me", nil, sc); after.Status != 401 {
		t.Fatalf("sesi harus tidak berlaku setelah logout, dapat %d", after.Status)
	}
}

func TestWrongPasswordIsGenericAndHasErrorID(t *testing.T) {
	a, _ := setup(t)
	known := a.do("POST", "/api/v1/auth/login", map[string]any{"email": "admin@a.test", "password": "salah"}, "")
	unknown := a.do("POST", "/api/v1/auth/login", map[string]any{"email": "tidak@ada.test", "password": "salah"}, "")
	if known.Status != 401 || unknown.Status != 401 {
		t.Fatalf("harus 401 keduanya: %d %d", known.Status, unknown.Status)
	}
	var p1, p2 map[string]any
	known.JSON(&p1)
	unknown.JSON(&p2)
	if p1["detail"] != p2["detail"] || p1["title"] != p2["title"] {
		t.Errorf("pesan harus identik agar akun tidak bocor: %v vs %v", p1, p2)
	}
	if !strings.HasPrefix(p1["error_id"].(string), "err_") || p1["request_id"] == "" {
		t.Errorf("error_id/request_id wajib ada: %v", p1)
	}
	if strings.Contains(string(known.Body), "tersisa") || strings.Contains(string(known.Body), "attempts") {
		t.Errorf("jumlah sisa percobaan tidak boleh dibocorkan: %s", known.Body)
	}
	if ct := known.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("content-type: %s", ct)
	}
}

func TestLockoutAfterFiveFailuresForKnownAndUnknownEmail(t *testing.T) {
	a, env := setup(t)
	for _, email := range []string{"admin@a.test", "hantu@a.test"} {
		var last resp
		for i := 0; i < 5; i++ {
			last = a.do("POST", "/api/v1/auth/login", map[string]any{"email": email, "password": "salah"}, "")
		}
		if last.Status != 429 || last.Header.Get("Retry-After") == "" {
			t.Fatalf("%s: percobaan ke-5 harus 429 dengan Retry-After, dapat %d", email, last.Status)
		}
		// password benar pun ditolak selama terkunci
		if r := a.do("POST", "/api/v1/auth/login", map[string]any{"email": email, "password": pw}, ""); r.Status != 429 {
			t.Fatalf("%s: harus tetap terkunci, dapat %d", email, r.Status)
		}
	}
	// kunci kedaluwarsa -> bisa masuk lagi
	if _, err := env.Owner.Exec(context.Background(), `UPDATE login_throttle SET locked_until = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	a.login("admin@a.test")
	var n int
	env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM login_throttle WHERE email='admin@a.test'`).Scan(&n)
	if n != 0 {
		t.Errorf("hitungan harus dihapus setelah login berhasil, tersisa %d", n)
	}
}

func TestCSRFHeaderRequiredForWrites(t *testing.T) {
	a, _ := setup(t)
	r := a.do("POST", "/api/v1/auth/login", map[string]any{"email": "admin@a.test", "password": pw}, "", "X-Requested-With", "")
	if r.Status != 403 {
		t.Fatalf("tanpa header khusus harus 403, dapat %d", r.Status)
	}
}

func TestSessionExpiresWhenIdle(t *testing.T) {
	a, env := setup(t)
	sc := a.login("admin@a.test")
	if _, err := env.Owner.Exec(context.Background(), `UPDATE sessions SET idle_expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if r := a.do("GET", "/api/v1/auth/me", nil, sc); r.Status != 401 {
		t.Fatalf("sesi idle yang lewat harus 401, dapat %d", r.Status)
	}
}

func TestDisabledUserCannotUseExistingSession(t *testing.T) {
	a, env := setup(t)
	sc := a.login("viewer@a.test")
	env.Owner.Exec(context.Background(), `UPDATE users SET status='disabled' WHERE email='viewer@a.test'`)
	if r := a.do("GET", "/api/v1/auth/me", nil, sc); r.Status != 401 {
		t.Fatalf("akun nonaktif harus 401, dapat %d", r.Status)
	}
}

func TestUnauthenticatedAndUnknownRoutes(t *testing.T) {
	a, _ := setup(t)
	if r := a.do("GET", "/api/v1/assets", nil, ""); r.Status != 401 {
		t.Fatalf("tanpa sesi harus 401, dapat %d", r.Status)
	}
	r := a.do("GET", "/api/v1/tidak-ada", nil, "")
	var p map[string]any
	r.JSON(&p)
	if r.Status != 404 || p["error_id"] == nil {
		t.Fatalf("jalur tak dikenal harus 404 problem+json: %d %s", r.Status, r.Body)
	}
	if h := a.do("GET", "/api/healthz", nil, ""); h.Status != 200 {
		t.Fatalf("healthz: %d", h.Status)
	}
	if sec := a.do("GET", "/api/healthz", nil, ""); sec.Header.Get("X-Content-Type-Options") != "nosniff" || sec.Header.Get("X-Frame-Options") != "DENY" {
		t.Errorf("header keamanan hilang: %v", sec.Header)
	}
}

func TestAssetPermissionsPerRole(t *testing.T) {
	a, _ := setup(t)
	admin, analyst, viewer := a.login("admin@a.test"), a.login("analyst@a.test"), a.login("viewer@a.test")
	body := map[string]any{"category": "host", "name": "srv-01", "platform": "linux", "arch": "x86_64",
		"addresses": []map[string]any{{"kind": "ip", "value": "10.0.0.5"}}}

	if r := a.do("GET", "/api/v1/assets", nil, viewer); r.Status != 403 {
		t.Errorf("Viewer cannot read live assets: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/assets", body, viewer); r.Status != 403 {
		t.Errorf("viewer tidak boleh menulis: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/assets", body, analyst); r.Status != 201 {
		t.Errorf("analyst boleh menulis: %d %s", r.Status, r.Body)
	}
	auth := map[string]any{"category": "host", "name": "srv-02", "testing_authorized": true}
	if r := a.do("POST", "/api/v1/assets", auth, analyst); r.Status != 403 {
		t.Errorf("analyst TIDAK boleh mengotorisasi pengujian: %d", r.Status)
	}
	r := a.do("POST", "/api/v1/assets", auth, admin)
	var got struct {
		TestingAuthorized bool   `json:"testing_authorized"`
		ID                string `json:"id"`
	}
	r.JSON(&got)
	if r.Status != 201 || !got.TestingAuthorized {
		t.Errorf("admin boleh mengotorisasi: %d %s", r.Status, r.Body)
	}
	if loc := r.Header.Get("Location"); loc != "/api/v1/assets/"+got.ID {
		t.Errorf("Location salah: %s", loc)
	}
}

func TestAssetValidation(t *testing.T) {
	a, _ := setup(t)
	sc := a.login("admin@a.test")
	r := a.do("POST", "/api/v1/assets", map[string]any{
		"category": "komputer", "name": " ", "criticality": "gawat",
		"addresses": []map[string]any{{"kind": "ip", "value": "999.1.1.1"}, {"kind": "ip", "value": "10.0.0.1"}, {"kind": "ip", "value": "10.0.0.1"}},
	}, sc)
	var p struct {
		Fields []struct{ Name, Message string } `json:"fields"`
	}
	r.JSON(&p)
	got := map[string]bool{}
	for _, f := range p.Fields {
		got[f.Name] = true
	}
	if r.Status != 422 || !got["category"] || !got["name"] || !got["criticality"] || !got["addresses"] {
		t.Fatalf("validasi kurang lengkap: %d %s", r.Status, r.Body)
	}
	if r := a.do("POST", "/api/v1/assets", map[string]any{"category": "host", "name": "x", "tidak_dikenal": 1}, sc); r.Status != 400 {
		t.Errorf("kolom tak dikenal harus 400, dapat %d", r.Status)
	}
	// normalisasi: IPv6, hostname huruf besar
	ok := a.do("POST", "/api/v1/assets", map[string]any{"category": "web_application", "name": "Portal",
		"addresses": []map[string]any{{"kind": "hostname", "value": "PORTAL.Example.COM."}, {"kind": "ip", "value": "::ffff:10.1.2.3"}}}, sc)
	var asset struct {
		Addresses []struct{ Kind, Value string } `json:"addresses"`
	}
	ok.JSON(&asset)
	if ok.Status != 201 || len(asset.Addresses) != 2 {
		t.Fatalf("buat gagal: %d %s", ok.Status, ok.Body)
	}
	vals := asset.Addresses[0].Value + "," + asset.Addresses[1].Value
	if !strings.Contains(vals, "portal.example.com") || !strings.Contains(vals, "10.1.2.3") || strings.Contains(vals, "ffff") {
		t.Errorf("alamat harus dinormalisasi: %s", vals)
	}
}

func TestAssetPaginationFiltersAndWildcards(t *testing.T) {
	a, _ := setup(t)
	sc := a.login("admin@a.test")
	names := []string{"web-01", "web-02", "db_01", "db-02", "100%-uji", "app-01", "app-02"}
	for i, n := range names {
		cat := "host"
		if strings.HasPrefix(n, "web") {
			cat = "web_application"
		}
		if r := a.do("POST", "/api/v1/assets", map[string]any{"category": cat, "name": n}, sc); r.Status != 201 {
			t.Fatalf("buat %d: %d %s", i, r.Status, r.Body)
		}
	}
	seen := map[string]bool{}
	cursor, pages := "", 0
	for {
		path := "/api/v1/assets?limit=3"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		r := a.do("GET", path, nil, sc)
		var pg struct {
			Items      []struct{ ID, Name string } `json:"items"`
			NextCursor *string                     `json:"next_cursor"`
		}
		r.JSON(&pg)
		if r.Status != 200 {
			t.Fatalf("list: %d %s", r.Status, r.Body)
		}
		pages++
		for _, it := range pg.Items {
			if seen[it.ID] {
				t.Fatalf("aset muncul dua kali di halaman berbeda: %s", it.Name)
			}
			seen[it.ID] = true
		}
		if pg.NextCursor == nil {
			break
		}
		cursor = *pg.NextCursor
	}
	if len(seen) != 7 || pages != 3 {
		t.Fatalf("harus 7 aset unik dalam 3 halaman, dapat %d aset / %d halaman", len(seen), pages)
	}
	count := func(qs string) int {
		var pg struct{ Items []any }
		a.do("GET", "/api/v1/assets?"+qs, nil, sc).JSON(&pg)
		return len(pg.Items)
	}
	if n := count("category=web_application"); n != 2 {
		t.Errorf("filter kategori: %d", n)
	}
	if n := count("q=app"); n != 2 {
		t.Errorf("pencarian teks: %d", n)
	}
	if n := count("q=%25"); n != 1 { // '%' literal, bukan wildcard
		t.Errorf("tanda %% harus dicari secara literal, dapat %d", n)
	}
	if n := count("q=_"); n != 1 { // '_' literal: hanya db_01
		t.Errorf("tanda _ harus dicari secara literal, dapat %d", n)
	}
	if r := a.do("GET", "/api/v1/assets?cursor=@@@", nil, sc); r.Status != 400 {
		t.Errorf("cursor rusak harus 400, dapat %d", r.Status)
	}
	if r := a.do("GET", "/api/v1/assets?limit=0", nil, sc); r.Status != 422 {
		t.Errorf("limit 0 harus 422, dapat %d", r.Status)
	}
}

func TestTenantIsolationThroughAPI(t *testing.T) {
	env := testutil.New(t)
	appA := newApp(t, env, env.TenantA)
	appB := newApp(t, env, env.TenantB)
	appA.user(env.TenantA, "admin@a.test", "admin")
	appB.user(env.TenantB, "admin@b.test", "admin")
	scA, scB := appA.login("admin@a.test"), appB.login("admin@b.test")

	r := appA.do("POST", "/api/v1/assets", map[string]any{"category": "host", "name": "rahasia-A"}, scA)
	var created struct{ ID string }
	r.JSON(&created)
	if r.Status != 201 {
		t.Fatal(r.Status, string(r.Body))
	}
	if g := appB.do("GET", "/api/v1/assets/"+created.ID, nil, scB); g.Status != 404 {
		t.Errorf("tenant B tidak boleh membaca aset A berdasarkan ID: %d", g.Status)
	}
	var pg struct{ Items []any }
	appB.do("GET", "/api/v1/assets", nil, scB).JSON(&pg)
	if len(pg.Items) != 0 {
		t.Errorf("tenant B tidak boleh melihat aset A di daftar")
	}
	// admin A tidak bisa masuk ke tenant B dengan kredensial A
	if l := appB.do("POST", "/api/v1/auth/login", map[string]any{"email": "admin@a.test", "password": pw}, ""); l.Status != 401 {
		t.Errorf("kredensial tenant A tidak berlaku di tenant B: %d", l.Status)
	}
}

func TestAuditLogRecordsCreation(t *testing.T) {
	a, env := setup(t)
	sc := a.login("analyst@a.test")
	r := a.do("POST", "/api/v1/assets", map[string]any{"category": "identity", "name": "svc-account"}, sc)
	var created struct{ ID string }
	r.JSON(&created)
	var action, actor, reqID string
	err := env.Owner.QueryRow(context.Background(), `SELECT a.action, u.email::text, coalesce(a.request_id,'')
FROM audit_log a JOIN users u ON u.id = a.actor_user_id WHERE a.resource_id = $1`, created.ID).Scan(&action, &actor, &reqID)
	if err != nil || action != "asset.create" || actor != "analyst@a.test" || !strings.HasPrefix(reqID, "req_") {
		t.Fatalf("audit tidak sesuai: %v %q %q %q", err, action, actor, reqID)
	}
}
