package server_test

import (
	"context"
	"errors"
	"testing"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/execution"
	"orchis-go/internal/settings"
)

type integJSON struct {
	ID, Kind, Key, Status string
	Enabled               bool
	HasToken              bool `json:"has_token"`
	Logs                  []struct{ Level, Message string }
}

func integrations(t *testing.T, a *app, cookie string) map[string]integJSON {
	t.Helper()
	var out struct{ Items []integJSON }
	r := a.do("GET", "/api/v1/integrations", nil, cookie)
	if r.Status != 200 {
		t.Fatalf("integrations: %d %s", r.Status, r.Body)
	}
	r.JSON(&out)
	m := map[string]integJSON{}
	for _, i := range out.Items {
		m[i.Key] = i
	}
	return m
}

func TestIntegrationsStatusTokenRotationAndEnableSwitch(t *testing.T) {
	f := newScanFixture(t, true) // token nmap sudah diterbitkan
	m := integrations(t, f.a, f.analyst)
	if len(m) != 3 || m["agent"].Kind != "executor" || m["nmap"].Status != "offline" || !m["nmap"].HasToken || m["nuclei"].HasToken {
		t.Fatalf("keadaan awal: %+v", m)
	}
	if r := f.inj("POST", "heartbeat", map[string]any{"version": "1.2.3"}, f.token); r.Status != 200 {
		t.Fatalf("heartbeat: %d", r.Status)
	}
	m = integrations(t, f.a, f.analyst)
	if m["nmap"].Status != "online" || len(m["nmap"].Logs) != 1 || !contains(m["nmap"].Logs[0].Message, "connected (version 1.2.3)") {
		t.Fatalf("setelah heartbeat: %+v", m["nmap"])
	}
	f.inj("POST", "heartbeat", map[string]any{"version": "1.2.3"}, f.token)
	if got := integrations(t, f.a, f.analyst)["nmap"]; len(got.Logs) != 1 {
		t.Fatalf("heartbeat beruntun tidak boleh menggandakan log: %+v", got.Logs)
	}
	id := m["nmap"].ID

	// hanya admin yang mengelola
	if r := f.a.do("POST", "/api/v1/integrations/"+id+"/token", nil, f.analyst); r.Status != 403 {
		t.Fatalf("analyst memutar token: %d", r.Status)
	}
	if r := f.a.do("PATCH", "/api/v1/integrations/"+id, map[string]any{"enabled": false}, f.analyst); r.Status != 403 {
		t.Fatalf("analyst menonaktifkan: %d", r.Status)
	}
	r := f.a.do("POST", "/api/v1/integrations/"+id+"/token", nil, f.admin)
	if r.Status != 201 {
		t.Fatalf("putar token: %d %s", r.Status, r.Body)
	}
	var nt struct{ Token string }
	r.JSON(&nt)
	if nt.Token == "" || nt.Token == f.token {
		t.Fatal("token baru harus berbeda")
	}
	if r := f.inj("POST", "heartbeat", map[string]any{}, f.token); r.Status != 401 {
		t.Fatalf("token lama harus hangus: %d", r.Status)
	}
	if r := f.inj("POST", "heartbeat", map[string]any{}, nt.Token); r.Status != 200 {
		t.Fatalf("token baru: %d", r.Status)
	}
	if contains(string(f.a.do("GET", "/api/v1/integrations", nil, f.admin).Body), nt.Token) {
		t.Fatal("token tidak boleh muncul lagi di daftar")
	}

	// nonaktifkan: injector ditolak dan Run ditolak; aktifkan lagi: pulih
	if r := f.a.do("PATCH", "/api/v1/integrations/"+id, map[string]any{"enabled": false}, f.admin); r.Status != 204 {
		t.Fatalf("nonaktifkan: %d", r.Status)
	}
	if got := integrations(t, f.a, f.analyst)["nmap"]; got.Status != "disabled" {
		t.Fatalf("status: %+v", got)
	}
	if r := f.inj("POST", "heartbeat", map[string]any{}, nt.Token); r.Status != 401 {
		t.Fatalf("injector nonaktif tidak boleh berkomunikasi: %d", r.Status)
	}
	if r := f.a.do("POST", "/api/v1/atomic-tests/"+f.test+"/run", nil, f.analyst); r.Status != 409 {
		t.Fatalf("Run ke injector nonaktif harus 409: %d %s", r.Status, r.Body)
	}
	if r := f.a.do("PATCH", "/api/v1/integrations/"+id, map[string]any{}, f.admin); r.Status != 422 {
		t.Fatalf("tanpa enabled: %d", r.Status)
	}
	f.a.do("PATCH", "/api/v1/integrations/"+id, map[string]any{"enabled": true}, f.admin)
	if r := f.inj("POST", "heartbeat", map[string]any{}, nt.Token); r.Status != 200 {
		t.Fatalf("setelah diaktifkan: %d", r.Status)
	}
	logs := integrations(t, f.a, f.analyst)["nmap"].Logs
	if len(logs) < 3 {
		t.Fatalf("log harus memuat rotasi, nonaktif, aktif: %+v", logs)
	}
	if r := f.a.do("POST", "/api/v1/integrations/00000000-0000-0000-0000-000000000000/token", nil, f.admin); r.Status != 404 {
		t.Fatalf("integrasi tak dikenal: %d", r.Status)
	}
}

func TestSettingsNameAndComponentHealth(t *testing.T) {
	a, env := setup(t)
	admin, analyst := a.login("admin@a.test"), a.login("analyst@a.test")
	var s struct {
		PlatformName string `json:"platform_name"`
		Version      string
		Components   []struct{ Name, Status string }
	}
	a.do("GET", "/api/v1/settings", nil, analyst).JSON(&s)
	if s.PlatformName != "Platform default" || len(s.Components) != 0 {
		t.Fatalf("analyst hanya melihat nama: %+v", s)
	}
	if r := a.do("PATCH", "/api/v1/settings", map[string]any{"platform_name": "Acme Red Team"}, analyst); r.Status != 403 {
		t.Fatalf("analyst mengubah: %d", r.Status)
	}
	if r := a.do("PATCH", "/api/v1/settings", map[string]any{"platform_name": "  "}, admin); r.Status != 422 {
		t.Fatalf("nama kosong: %d", r.Status)
	}
	if r := a.do("PATCH", "/api/v1/settings", map[string]any{"platform_name": "Acme Red Team"}, admin); r.Status != 204 {
		t.Fatalf("ubah nama: %d", r.Status)
	}
	a.do("GET", "/api/v1/settings", nil, analyst).JSON(&s)
	if s.PlatformName != "Acme Red Team" {
		t.Fatalf("nama: %+v", s)
	}
	// pemeriksaan komponen: satu sehat, satu mati, satu tidak dikonfigurasi, satu macet melewati batas waktu
	svc := &settings.Service{DB: env.App, Version: "9.9", Checks: []settings.Check{
		{Name: "ok", Fn: func(context.Context) error { return nil }},
		{Name: "down", Fn: func(context.Context) error { return errors.New("secret internal detail") }},
		{Name: "off"},
	}}
	got, err := svc.Get(context.Background(), &auth.Principal{TenantID: env.TenantA, Permissions: []string{authz.SettingsManage}})
	if err != nil || got.Version != "9.9" || len(got.Components) != 3 {
		t.Fatalf("komponen: %+v %v", got, err)
	}
	want := map[string]string{"ok": "up", "down": "down", "off": "not_configured"}
	for _, c := range got.Components {
		if want[c.Name] != c.Status || (c.Status == "down" && contains(c.Detail, "secret")) {
			t.Fatalf("komponen %s: %+v (detail internal tidak boleh bocor)", c.Name, c)
		}
	}
	noPerm, _ := svc.Get(context.Background(), &auth.Principal{TenantID: env.TenantA})
	if len(noPerm.Components) != 0 {
		t.Fatal("tanpa settings.manage komponen tidak boleh terlihat")
	}
	_ = execution.TopicDispatch
}

func TestTagsRenameDeleteAndGroupGuard(t *testing.T) {
	a, _ := setup(t)
	admin, analyst := a.login("admin@a.test"), a.login("analyst@a.test")
	mk := func(name string) string {
		var tg struct{ ID string }
		r := a.do("POST", "/api/v1/tags", map[string]any{"name": name, "color": "#112233"}, analyst)
		if r.Status != 201 {
			t.Fatalf("buat tag: %d %s", r.Status, r.Body)
		}
		r.JSON(&tg)
		return tg.ID
	}
	crown, temp := mk("crown"), mk("temp")
	if r := a.do("PATCH", "/api/v1/tags/"+crown, map[string]any{"name": "temp", "color": "#112233"}, analyst); r.Status != 422 {
		t.Fatalf("nama ganda: %d %s", r.Status, r.Body)
	}
	if r := a.do("PATCH", "/api/v1/tags/"+crown, map[string]any{"name": "crown-jewel", "color": "#ff0000"}, analyst); r.Status != 200 {
		t.Fatalf("ubah: %d %s", r.Status, r.Body)
	}
	var tl struct {
		Items []struct {
			Name, Color string
			Assets      int
		}
	}
	asset := mkAsset(t, a, analyst, map[string]any{"category": "host", "name": "h", "tag_ids": []string{crown}})
	_ = asset
	a.do("GET", "/api/v1/tags", nil, analyst).JSON(&tl)
	found := false
	for _, x := range tl.Items {
		if x.Name == "crown-jewel" {
			found = x.Assets == 1 && x.Color == "#FF0000"
		}
	}
	if !found {
		t.Fatalf("daftar tag harus memuat jumlah aset dan warna baru: %+v", tl)
	}
	// tag yang dipakai aturan grup tidak boleh dihapus
	if r := a.do("POST", "/api/v1/asset-groups", map[string]any{"name": "Crowns", "rule": map[string]any{"mode": "and", "filters": []map[string]any{{"field": "tag", "operator": "in", "values": []string{crown}}}}}, analyst); r.Status != 201 {
		t.Fatalf("buat grup: %d %s", r.Status, r.Body)
	}
	if r := a.do("DELETE", "/api/v1/tags/"+crown, nil, admin); r.Status != 409 {
		t.Fatalf("tag di aturan grup harus 409: %d %s", r.Status, r.Body)
	}
	if r := a.do("DELETE", "/api/v1/tags/"+temp, nil, analyst); r.Status != 403 {
		t.Fatalf("analyst tidak boleh menghapus tag: %d", r.Status)
	}
	if r := a.do("DELETE", "/api/v1/tags/"+temp, nil, admin); r.Status != 204 {
		t.Fatalf("hapus tag bebas: %d", r.Status)
	}
	if r := a.do("DELETE", "/api/v1/tags/"+temp, nil, admin); r.Status != 404 {
		t.Fatalf("hapus lagi: %d", r.Status)
	}
}
