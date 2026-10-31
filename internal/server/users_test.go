package server_test

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type userJSON struct {
	ID, Email, Role, Status string
	Version                 int
	HasPassword             bool `json:"has_password"`
}

type createdJSON struct {
	User      userJSON `json:"user"`
	ResetLink struct {
		Token     string
		ExpiresAt string `json:"expires_at"`
	} `json:"reset_link"`
}

func (a *app) createUser(t *testing.T, admin, email, role string) createdJSON {
	t.Helper()
	r := a.do("POST", "/api/v1/users", map[string]any{"email": email, "display_name": "New " + role, "role": role}, admin)
	if r.Status != 201 {
		t.Fatalf("buat user: %d %s", r.Status, r.Body)
	}
	var c createdJSON
	r.JSON(&c)
	return c
}

func (a *app) loginStatus(email, pass string) int {
	return a.do("POST", "/api/v1/auth/login", map[string]any{"email": email, "password": pass}, "").Status
}

const strong = "Another-Strong-Pass-7"

func TestAdminCreatesUserWhoSetsPasswordThroughOneTimeLink(t *testing.T) {
	a, _ := setup(t)
	admin, analyst := a.login("admin@a.test"), a.login("analyst@a.test")
	if r := a.do("GET", "/api/v1/users", nil, analyst); r.Status != 403 {
		t.Fatalf("analyst tidak boleh melihat daftar user: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/users", map[string]any{"email": "x@a.test", "display_name": "X", "role": "viewer"}, analyst); r.Status != 403 {
		t.Fatalf("analyst tidak boleh membuat user: %d", r.Status)
	}
	c := a.createUser(t, admin, "new@a.test", "analyst")
	if c.User.HasPassword || c.ResetLink.Token == "" {
		t.Fatalf("akun baru tanpa password dan dengan link: %+v", c)
	}
	if st := a.loginStatus("new@a.test", strong); st != 401 {
		t.Fatalf("akun tanpa password tidak boleh bisa masuk: %d", st)
	}
	if r := a.do("POST", "/api/v1/auth/reset/check", map[string]any{"token": c.ResetLink.Token}, ""); r.Status != 204 {
		t.Fatalf("link valid: %d %s", r.Status, r.Body)
	}
	if r := a.do("POST", "/api/v1/auth/reset", map[string]any{"token": c.ResetLink.Token, "password": "short"}, ""); r.Status != 422 {
		t.Fatalf("password lemah harus 422: %d %s", r.Status, r.Body)
	}
	if r := a.do("POST", "/api/v1/auth/reset", map[string]any{"token": c.ResetLink.Token, "password": strong}, ""); r.Status != 204 {
		t.Fatalf("reset: %d %s", r.Status, r.Body)
	}
	if st := a.loginStatus("new@a.test", strong); st != 200 {
		t.Fatalf("setelah reset harus bisa masuk: %d", st)
	}
	if r := a.do("POST", "/api/v1/auth/reset", map[string]any{"token": c.ResetLink.Token, "password": "Yet-Another-Pass-9"}, ""); r.Status != 400 {
		t.Fatalf("link hanya sekali pakai: %d %s", r.Status, r.Body)
	}
	if r := a.do("POST", "/api/v1/auth/reset/check", map[string]any{"token": c.ResetLink.Token}, ""); r.Status != 400 {
		t.Fatalf("link terpakai tidak valid lagi: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/auth/reset", map[string]any{"token": "orl_tebakan", "password": strong}, ""); r.Status != 400 {
		t.Fatalf("token ngawur: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/users", map[string]any{"email": "NEW@a.test", "display_name": "dup", "role": "viewer"}, admin); r.Status != 422 {
		t.Fatalf("email ganda (beda huruf) harus 422: %d %s", r.Status, r.Body)
	}
	if r := a.do("POST", "/api/v1/users", map[string]any{"email": "bad", "display_name": "", "role": "root"}, admin); r.Status != 422 {
		t.Fatalf("masukan salah harus 422: %d", r.Status)
	}
}

func TestResetLinkExpiresIsReplacedAndEndsSessionsAndLockout(t *testing.T) {
	a, env := setup(t)
	admin := a.login("admin@a.test")
	victim := a.login("viewer@a.test")
	// penguncian: 5 kegagalan mengunci akun
	for i := 0; i < 5; i++ {
		a.loginStatus("viewer@a.test", "salah-salah-salah")
	}
	if st := a.loginStatus("viewer@a.test", pw); st != 429 && st != 423 && st != 401 {
		t.Fatalf("akun harus terkunci (status %d)", st)
	}
	var uid string
	for _, u := range listUsers(t, a, admin) {
		if u.Email == "viewer@a.test" {
			uid = u.ID
		}
	}
	r := a.do("POST", "/api/v1/users/"+uid+"/reset-link", nil, admin)
	if r.Status != 201 {
		t.Fatalf("reset-link: %d %s", r.Status, r.Body)
	}
	var first struct{ Token string }
	r.JSON(&first)
	var second struct{ Token string }
	a.do("POST", "/api/v1/users/"+uid+"/reset-link", nil, admin).JSON(&second)
	if r := a.do("POST", "/api/v1/auth/reset/check", map[string]any{"token": first.Token}, ""); r.Status != 400 {
		t.Fatalf("link baru harus membatalkan link lama: %d", r.Status)
	}
	// kedaluwarsa
	env.Owner.Exec(context.Background(), `UPDATE password_reset_links SET created_at = now() - interval '2 hours', expires_at = now() - interval '1 hour'`)
	if r := a.do("POST", "/api/v1/auth/reset", map[string]any{"token": second.Token, "password": strong}, ""); r.Status != 400 {
		t.Fatalf("link kedaluwarsa harus ditolak: %d", r.Status)
	}
	var third struct{ Token string }
	a.do("POST", "/api/v1/users/"+uid+"/reset-link", nil, admin).JSON(&third)
	if r := a.do("POST", "/api/v1/auth/reset", map[string]any{"token": third.Token, "password": strong}, ""); r.Status != 204 {
		t.Fatalf("reset: %d %s", r.Status, r.Body)
	}
	if r := a.do("GET", "/api/v1/auth/me", nil, victim); r.Status != 401 {
		t.Fatalf("sesi lama harus berakhir setelah reset: %d", r.Status)
	}
	if st := a.loginStatus("viewer@a.test", strong); st != 200 {
		t.Fatalf("reset harus membuka kunci akun dan menerima password baru: %d", st)
	}
	// password lama tidak boleh dipakai ulang
	var fourth struct{ Token string }
	a.do("POST", "/api/v1/users/"+uid+"/reset-link", nil, admin).JSON(&fourth)
	if r := a.do("POST", "/api/v1/auth/reset", map[string]any{"token": fourth.Token, "password": strong}, ""); r.Status != 422 {
		t.Fatalf("password yang sama dengan sekarang harus ditolak: %d %s", r.Status, r.Body)
	}
	if r := a.do("POST", "/api/v1/auth/reset", map[string]any{"token": fourth.Token, "password": pw}, ""); r.Status != 422 {
		t.Fatalf("password lama (dari 5 terakhir) harus ditolak: %d %s", r.Status, r.Body)
	}
}

func listUsers(t *testing.T, a *app, admin string) []userJSON {
	var out struct{ Items []userJSON }
	r := a.do("GET", "/api/v1/users", nil, admin)
	if r.Status != 200 {
		t.Fatalf("list users: %d %s", r.Status, r.Body)
	}
	r.JSON(&out)
	return out.Items
}

func findUser(us []userJSON, email string) userJSON {
	for _, u := range us {
		if u.Email == email {
			return u
		}
	}
	return userJSON{}
}

func TestUserAdminGuardrails(t *testing.T) {
	a, _ := setup(t)
	admin := a.login("admin@a.test")
	us := listUsers(t, a, admin)
	me, viewer := findUser(us, "admin@a.test"), findUser(us, "viewer@a.test")
	// tidak boleh mengubah peran/status diri sendiri, dan admin terakhir tidak boleh hilang
	if r := a.do("PATCH", "/api/v1/users/"+me.ID, map[string]any{"version": me.Version, "role": "viewer"}, admin); r.Status != 422 {
		t.Fatalf("ubah peran sendiri harus ditolak: %d %s", r.Status, r.Body)
	}
	if r := a.do("PATCH", "/api/v1/users/"+me.ID, map[string]any{"version": me.Version, "status": "disabled"}, admin); r.Status != 422 {
		t.Fatalf("nonaktifkan diri sendiri harus ditolak: %d", r.Status)
	}
	// admin kedua boleh; lalu admin pertama tidak bisa diturunkan oleh admin kedua bila ia satu-satunya... (ada dua, boleh)
	second := a.createUser(t, admin, "admin2@a.test", "admin")
	if r := a.do("PATCH", "/api/v1/users/"+second.User.ID, map[string]any{"version": second.User.Version, "display_name": "Second"}, admin); r.Status != 200 {
		t.Fatalf("ubah nama: %d %s", r.Status, r.Body)
	}
	// versi usang
	if r := a.do("PATCH", "/api/v1/users/"+viewer.ID, map[string]any{"version": viewer.Version + 5, "role": "analyst"}, admin); r.Status != 409 {
		t.Fatalf("versi usang harus 409: %d", r.Status)
	}
	// perubahan peran langsung berlaku pada sesi yang sedang berjalan
	vs := a.login("viewer@a.test")
	if r := a.do("POST", "/api/v1/assets", map[string]any{"category": "host", "name": "h1"}, vs); r.Status != 403 {
		t.Fatalf("viewer tidak boleh menulis: %d", r.Status)
	}
	if r := a.do("PATCH", "/api/v1/users/"+viewer.ID, map[string]any{"version": viewer.Version, "role": "analyst"}, admin); r.Status != 200 {
		t.Fatalf("promosi: %d %s", r.Status, r.Body)
	}
	if r := a.do("POST", "/api/v1/assets", map[string]any{"category": "host", "name": "h1"}, vs); r.Status != 201 {
		t.Fatalf("setelah dipromosikan sesi yang sama boleh menulis: %d %s", r.Status, r.Body)
	}
	// menonaktifkan mengakhiri sesi saat itu juga dan menolak login
	viewer = findUser(listUsers(t, a, admin), "viewer@a.test")
	if r := a.do("PATCH", "/api/v1/users/"+viewer.ID, map[string]any{"version": viewer.Version, "status": "disabled"}, admin); r.Status != 200 {
		t.Fatalf("nonaktifkan: %d %s", r.Status, r.Body)
	}
	if r := a.do("GET", "/api/v1/auth/me", nil, vs); r.Status != 401 {
		t.Fatalf("sesi akun nonaktif harus berakhir: %d", r.Status)
	}
	if st := a.loginStatus("viewer@a.test", pw); st != 401 {
		t.Fatalf("akun nonaktif tidak boleh masuk: %d", st)
	}
	if r := a.do("POST", "/api/v1/users/"+viewer.ID+"/reset-link", nil, admin); r.Status != 409 {
		t.Fatalf("link untuk akun nonaktif harus 409: %d", r.Status)
	}
	// roles: hanya admin, peran bawaan terlihat dengan jumlah pengguna
	var roles struct {
		Items []struct {
			Key         string
			Users       int
			Permissions []string
		}
	}
	a.do("GET", "/api/v1/roles", nil, admin).JSON(&roles)
	if len(roles.Items) != 3 || roles.Items[0].Key != "admin" || roles.Items[0].Users != 2 {
		t.Fatalf("peran: %+v", roles)
	}
}

// Dua admin yang saling menurunkan pada saat yang sama: tepat satu yang boleh berhasil, platform tidak boleh tanpa admin.
func TestConcurrentAdminDemotionsNeverLeaveZeroAdmins(t *testing.T) {
	for round := 0; round < 4; round++ {
		a, _ := setup(t)
		s1 := a.login("admin@a.test")
		second := a.createUser(t, s1, "admin2@a.test", "admin")
		a.do("POST", "/api/v1/auth/reset", map[string]any{"token": second.ResetLink.Token, "password": strong}, "")
		s2 := loginAs(t, a, "admin2@a.test", strong)
		us := listUsers(t, a, s1)
		one, two := findUser(us, "admin@a.test"), findUser(us, "admin2@a.test")
		results := make(chan int, 2)
		start := make(chan struct{})
		go func() {
			<-start
			results <- a.do("PATCH", "/api/v1/users/"+two.ID, map[string]any{"version": two.Version, "role": "viewer"}, s1).Status
		}()
		go func() {
			<-start
			results <- a.do("PATCH", "/api/v1/users/"+one.ID, map[string]any{"version": one.Version, "role": "viewer"}, s2).Status
		}()
		close(start)
		r1, r2 := <-results, <-results
		var admins int
		a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM users u JOIN roles r ON r.id=u.role_id WHERE r.key='admin' AND u.status='active'`).Scan(&admins)
		if admins < 1 {
			t.Fatalf("putaran %d: platform tanpa admin! status %d dan %d", round, r1, r2)
		}
		if r1 == 500 || r2 == 500 {
			t.Fatalf("putaran %d: tidak boleh ada galat server (deadlock?): %d %d", round, r1, r2)
		}
	}
}

func TestAdminCannotChangeOwnRoleEvenWhenAnotherAdminExists(t *testing.T) {
	a, _ := setup(t)
	admin := a.login("admin@a.test")
	a.createUser(t, admin, "admin2@a.test", "admin")
	me := findUser(listUsers(t, a, admin), "admin@a.test")
	r := a.do("PATCH", "/api/v1/users/"+me.ID, map[string]any{"version": me.Version, "role": "viewer"}, admin)
	if r.Status != 422 || !contains(string(r.Body), "your own") {
		t.Fatalf("ubah peran sendiri harus ditolak walau ada admin lain: %d %s", r.Status, r.Body)
	}
	if r := a.do("PATCH", "/api/v1/users/"+me.ID, map[string]any{"version": me.Version, "status": "disabled"}, admin); r.Status != 422 {
		t.Fatalf("nonaktifkan diri sendiri: %d", r.Status)
	}
}

func TestDisabledAccountsSessionStaysDeadAfterReenabling(t *testing.T) {
	a, _ := setup(t)
	admin := a.login("admin@a.test")
	vs := a.login("viewer@a.test")
	v := findUser(listUsers(t, a, admin), "viewer@a.test")
	a.do("PATCH", "/api/v1/users/"+v.ID, map[string]any{"version": v.Version, "status": "disabled"}, admin)
	v = findUser(listUsers(t, a, admin), "viewer@a.test")
	a.do("PATCH", "/api/v1/users/"+v.ID, map[string]any{"version": v.Version, "status": "active"}, admin)
	if r := a.do("GET", "/api/v1/auth/me", nil, vs); r.Status != 401 {
		t.Fatalf("sesi lama tidak boleh hidup lagi setelah akun diaktifkan kembali: %d", r.Status)
	}
	if st := a.loginStatus("viewer@a.test", pw); st != 200 {
		t.Fatalf("login baru setelah diaktifkan: %d", st)
	}
}

func TestLastActiveAdminCannotBeDemoted(t *testing.T) {
	a, _ := setup(t)
	admin := a.login("admin@a.test")
	second := a.createUser(t, admin, "admin2@a.test", "admin")
	// admin2 menjadi aktif, lalu admin2 menurunkan admin pertama (boleh, ada dua), kemudian admin pertama tidak ada lagi
	var tok struct{ Token string }
	_ = tok
	a.do("POST", "/api/v1/auth/reset", map[string]any{"token": second.ResetLink.Token, "password": strong}, "")
	s2 := loginAs(t, a, "admin2@a.test", strong)
	first := findUser(listUsers(t, a, s2), "admin@a.test")
	if r := a.do("PATCH", "/api/v1/users/"+first.ID, map[string]any{"version": first.Version, "role": "viewer"}, s2); r.Status != 200 {
		t.Fatalf("menurunkan admin pertama saat ada dua: %d %s", r.Status, r.Body)
	}
	// kini admin2 adalah satu-satunya admin aktif; admin yang diturunkan tidak berhak lagi, jadi ujilah lewat jalur langsung layanan DB
	if r := a.do("GET", "/api/v1/users", nil, admin); r.Status != 403 {
		t.Fatalf("admin yang diturunkan kehilangan akses seketika: %d", r.Status)
	}
	self := findUser(listUsers(t, a, s2), "admin2@a.test")
	if r := a.do("PATCH", "/api/v1/users/"+self.ID, map[string]any{"version": self.Version, "role": "viewer"}, s2); r.Status != 422 {
		t.Fatalf("admin terakhir tidak boleh menurunkan dirinya: %d", r.Status)
	}
}

func loginAs(t *testing.T, a *app, email, pass string) string {
	t.Helper()
	r := a.do("POST", "/api/v1/auth/login", map[string]any{"email": email, "password": pass}, "")
	if r.Status != 200 {
		t.Fatalf("login %s: %d %s", email, r.Status, r.Body)
	}
	for _, c := range (&httpResp{r.Header}).cookies() {
		if c.Name == "orchis_session" {
			return c.Value
		}
	}
	t.Fatal("tanpa cookie")
	return ""
}

func TestForgotPasswordIsUniformAndAlertsAdminsOnly(t *testing.T) {
	a, _ := setup(t)
	admin, analyst := a.login("admin@a.test"), a.login("analyst@a.test")
	for _, email := range []string{"nobody@a.test", "viewer@a.test", "viewer@a.test", "not an email"} {
		if r := a.do("POST", "/api/v1/auth/forgot", map[string]any{"email": email}, ""); r.Status != 202 {
			t.Fatalf("respons harus seragam 202 untuk %q: %d %s", email, r.Status, r.Body)
		}
	}
	var al struct {
		Items  []struct{ Kind, Title string }
		Unread int
	}
	a.do("GET", "/api/v1/alerts", nil, admin).JSON(&al)
	if len(al.Items) != 1 || al.Items[0].Kind != "password_reset_requested" || al.Unread != 1 {
		t.Fatalf("admin harus melihat satu alert permintaan reset (email tak dikenal tidak membuat alert, dua permintaan satu jam digabung): %+v", al)
	}
	a.do("GET", "/api/v1/alerts", nil, analyst).JSON(&al)
	if len(al.Items) != 0 {
		t.Fatalf("analyst tidak boleh melihat alert khusus admin: %+v", al)
	}
}

func TestProfilePasswordChangeKeepsCurrentSessionOnly(t *testing.T) {
	a, _ := setup(t)
	s1 := a.login("analyst@a.test")
	s2 := a.login("analyst@a.test")
	if r := a.do("POST", "/api/v1/me/password", map[string]any{"current_password": "salah", "new_password": strong}, s1); r.Status != 422 {
		t.Fatalf("password sekarang salah: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/me/password", map[string]any{"current_password": pw, "new_password": "short"}, s1); r.Status != 422 {
		t.Fatalf("password baru lemah: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/me/password", map[string]any{"current_password": pw, "new_password": pw}, s1); r.Status != 422 {
		t.Fatalf("password sama dengan sekarang: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/me/password", map[string]any{"current_password": pw, "new_password": strong}, s1); r.Status != 204 {
		t.Fatalf("ganti password: %d %s", r.Status, r.Body)
	}
	if r := a.do("GET", "/api/v1/auth/me", nil, s1); r.Status != 200 {
		t.Fatalf("sesi yang dipakai tetap hidup: %d", r.Status)
	}
	if r := a.do("GET", "/api/v1/auth/me", nil, s2); r.Status != 401 {
		t.Fatalf("sesi lain harus berakhir: %d", r.Status)
	}
	if st := a.loginStatus("analyst@a.test", pw); st != 401 {
		t.Fatalf("password lama tidak berlaku: %d", st)
	}
	if r := a.do("PATCH", "/api/v1/me", map[string]any{"display_name": "  Ada L.  "}, s1); r.Status != 204 {
		t.Fatalf("ubah nama: %d", r.Status)
	}
	var me struct {
		DisplayName string `json:"display_name"`
	}
	a.do("GET", "/api/v1/auth/me", nil, s1).JSON(&me)
	if me.DisplayName != "Ada L." {
		t.Fatalf("nama: %q", me.DisplayName)
	}
}

func TestAPITokensActAsTheirOwnerAndDieWithTheAccount(t *testing.T) {
	a, _ := setup(t)
	admin, analyst := a.login("admin@a.test"), a.login("analyst@a.test")
	var tok struct {
		ID, Secret, Prefix string
	}
	r := a.do("POST", "/api/v1/me/tokens", map[string]any{"name": "ci", "expires_in_days": 30}, analyst)
	if r.Status != 201 {
		t.Fatalf("buat token: %d %s", r.Status, r.Body)
	}
	r.JSON(&tok)
	if len(tok.Secret) < 40 || tok.Secret[:4] != "oat_" || tok.Prefix != tok.Secret[:8] {
		t.Fatalf("token: %+v", tok)
	}
	bearer := func(method, path string, body any, secret string) resp {
		return a.do(method, path, body, "", "X-Requested-With", "", "Authorization", "Bearer "+secret)
	}
	if r := bearer("GET", "/api/v1/auth/me", nil, tok.Secret); r.Status != 200 {
		t.Fatalf("token memuat identitas: %d %s", r.Status, r.Body)
	}
	if r := bearer("POST", "/api/v1/assets", map[string]any{"category": "host", "name": "via-token"}, tok.Secret); r.Status != 201 {
		t.Fatalf("token analyst boleh menulis aset tanpa header CSRF: %d %s", r.Status, r.Body)
	}
	if r := bearer("GET", "/api/v1/users", nil, tok.Secret); r.Status != 403 {
		t.Fatalf("token analyst tidak boleh mengelola user: %d", r.Status)
	}
	if r := bearer("GET", "/api/v1/auth/me", nil, "oat_salah"); r.Status != 401 {
		t.Fatalf("token salah: %d", r.Status)
	}
	// header Authorization mengalahkan cookie: token salah + cookie sah tetap 401
	if r := a.do("GET", "/api/v1/auth/me", nil, admin, "Authorization", "Bearer oat_salah"); r.Status != 401 {
		t.Fatalf("Authorization salah tidak boleh jatuh ke cookie: %d", r.Status)
	}
	// daftar tidak membocorkan rahasia
	lr := a.do("GET", "/api/v1/me/tokens", nil, analyst)
	if contains(string(lr.Body), tok.Secret) || !contains(string(lr.Body), tok.Prefix) {
		t.Fatalf("daftar hanya boleh memuat awalan: %s", lr.Body)
	}
	// token orang lain tidak bisa dicabut
	if r := a.do("DELETE", "/api/v1/me/tokens/"+tok.ID, nil, admin); r.Status != 404 {
		t.Fatalf("mencabut token orang lain: %d", r.Status)
	}
	// perubahan peran berlaku langsung pada token
	us := listUsers(t, a, admin)
	an := findUser(us, "analyst@a.test")
	a.do("PATCH", "/api/v1/users/"+an.ID, map[string]any{"version": an.Version, "role": "viewer"}, admin)
	if r := bearer("POST", "/api/v1/assets", map[string]any{"category": "host", "name": "x2"}, tok.Secret); r.Status != 403 {
		t.Fatalf("token mengikuti peran terbaru: %d", r.Status)
	}
	an = findUser(listUsers(t, a, admin), "analyst@a.test")
	a.do("PATCH", "/api/v1/users/"+an.ID, map[string]any{"version": an.Version, "status": "disabled"}, admin)
	_ = analyst
	if r := bearer("GET", "/api/v1/auth/me", nil, tok.Secret); r.Status != 401 {
		t.Fatalf("token akun nonaktif harus mati: %d", r.Status)
	}
	// cabut: setelah dicabut 401
	a2 := a.login("viewer@a.test")
	var t2 struct{ ID, Secret string }
	a.do("POST", "/api/v1/me/tokens", map[string]any{"name": "tmp"}, a2).JSON(&t2)
	if r := a.do("DELETE", "/api/v1/me/tokens/"+t2.ID, nil, a2); r.Status != 204 {
		t.Fatalf("cabut: %d", r.Status)
	}
	if r := bearer("GET", "/api/v1/auth/me", nil, t2.Secret); r.Status != 401 {
		t.Fatalf("token dicabut: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/me/tokens", map[string]any{"name": ""}, a2); r.Status != 422 {
		t.Fatalf("nama kosong: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/me/tokens", map[string]any{"name": "x", "expires_in_days": 9999}, a2); r.Status != 422 {
		t.Fatalf("masa berlaku berlebihan: %d", r.Status)
	}
}

type httpResp struct{ h map[string][]string }

func (r *httpResp) cookies() []*http.Cookie { return (&http.Response{Header: r.h}).Cookies() }

// Penjagaan "admin terakhir" hanya aman terhadap permintaan serentak bila perubahan peran mengunci SEMUA admin aktif.
// Tes ini membuktikannya secara deterministik: bila seorang admin sedang "dipegang" transaksi lain, mengubah admin lain harus menunggu.
func TestRoleChangeWaitsForOtherAdminLocks(t *testing.T) {
	a, env := setup(t)
	s1 := a.login("admin@a.test")
	second := a.createUser(t, s1, "admin2@a.test", "admin")
	a.do("POST", "/api/v1/auth/reset", map[string]any{"token": second.ResetLink.Token, "password": strong}, "")

	cur := findUser(listUsers(t, a, s1), "admin2@a.test") // versi terbaru (reset password menaikkannya)
	ctx := context.Background()
	hold, err := env.Owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Rollback(ctx)
	if _, err := hold.Exec(ctx, `SELECT id FROM users WHERE email='admin@a.test' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() {
		done <- a.do("PATCH", "/api/v1/users/"+cur.ID, map[string]any{"version": cur.Version, "display_name": "Second"}, s1).Status
	}()
	select {
	case st := <-done:
		t.Fatalf("perubahan harus menunggu kunci admin lain, tetapi langsung selesai dengan %d", st)
	case <-time.After(500 * time.Millisecond):
	}
	if err := hold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case st := <-done:
		if st != 200 {
			t.Fatalf("setelah kunci dilepas perubahan harus berhasil: %d", st)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("perubahan tidak selesai setelah kunci dilepas")
	}
}
