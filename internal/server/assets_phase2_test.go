package server_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"orchis-go/internal/testutil"
)

type groupJSON struct {
	ID          string                      `json:"id"`
	Name        string                      `json:"name"`
	IsSystem    bool                        `json:"is_system"`
	MemberCount int                         `json:"member_count"`
	Version     int                         `json:"version"`
	Members     []struct{ ID, Name string } `json:"members"`
	TotalAssets int                         `json:"total_assets"`
}

func mkAsset(t *testing.T, a *app, sc string, body map[string]any) string {
	t.Helper()
	r := a.do("POST", "/api/v1/assets", body, sc)
	var out struct{ ID string }
	r.JSON(&out)
	if r.Status != 201 {
		t.Fatalf("buat aset: %d %s", r.Status, r.Body)
	}
	return out.ID
}

func listGroups(t *testing.T, a *app, sc string) map[string]groupJSON {
	t.Helper()
	var out struct{ Items []groupJSON }
	r := a.do("GET", "/api/v1/asset-groups", nil, sc)
	r.JSON(&out)
	if r.Status != 200 {
		t.Fatalf("daftar grup: %d %s", r.Status, r.Body)
	}
	m := map[string]groupJSON{}
	for _, g := range out.Items {
		m[g.Name] = g
	}
	return m
}

func TestSystemGroupsAreSeededAndTrackAssets(t *testing.T) {
	a, _ := setup(t)
	sc := a.login("admin@a.test")
	g := listGroups(t, a, sc)
	for _, n := range []string{"All endpoints", "All Linux x86_64", "All Linux arm64", "All macOS arm64", "All macOS x86_64", "All Windows arm64", "All Windows x86_64"} {
		if gr, ok := g[n]; !ok || !gr.IsSystem || gr.MemberCount != 0 {
			t.Fatalf("grup bawaan %q hilang atau tidak kosong: %+v", n, gr)
		}
	}
	id := mkAsset(t, a, sc, map[string]any{"category": "host", "name": "WINTERFELL", "platform": "windows", "arch": "x86_64"})
	mkAsset(t, a, sc, map[string]any{"category": "host", "name": "nix", "platform": "linux", "arch": "arm64"})
	mkAsset(t, a, sc, map[string]any{"category": "web_application", "name": "portal"}) // tanpa platform
	g = listGroups(t, a, sc)
	if g["All endpoints"].MemberCount != 2 || g["All Windows x86_64"].MemberCount != 1 || g["All Linux arm64"].MemberCount != 1 || g["All Linux x86_64"].MemberCount != 0 {
		t.Fatalf("keanggotaan bawaan salah: %+v", g)
	}
	// mengubah platform memindahkan aset antar grup
	var cur struct{ Version int }
	a.do("GET", "/api/v1/assets/"+id, nil, sc).JSON(&cur)
	if r := a.do("PATCH", "/api/v1/assets/"+id, map[string]any{"version": cur.Version, "platform": "linux"}, sc); r.Status != 200 {
		t.Fatalf("patch: %d %s", r.Status, r.Body)
	}
	g = listGroups(t, a, sc)
	if g["All Windows x86_64"].MemberCount != 0 || g["All Linux x86_64"].MemberCount != 1 {
		t.Fatalf("aset harus pindah grup setelah platform berubah: %+v", g)
	}
	// grup bawaan tidak bisa diubah atau dihapus
	sys := g["All endpoints"]
	if r := a.do("DELETE", "/api/v1/asset-groups/"+sys.ID, nil, sc); r.Status != 409 {
		t.Errorf("hapus grup bawaan harus 409: %d", r.Status)
	}
	if r := a.do("PUT", "/api/v1/asset-groups/"+sys.ID, map[string]any{"name": "x", "version": sys.Version, "rule": map[string]any{}}, sc); r.Status != 409 {
		t.Errorf("ubah grup bawaan harus 409: %d", r.Status)
	}
}

func TestCustomGroupLifecycle(t *testing.T) {
	a, _ := setup(t)
	admin, analyst, viewer := a.login("admin@a.test"), a.login("analyst@a.test"), a.login("viewer@a.test")
	tag := struct{ ID string }{}
	if r := a.do("POST", "/api/v1/tags", map[string]any{"name": "crown-jewel", "color": "#ff0019"}, analyst); r.Status != 201 {
		t.Fatalf("buat tag: %d %s", r.Status, r.Body)
	} else {
		r.JSON(&tag)
	}
	if r := a.do("POST", "/api/v1/tags", map[string]any{"name": "crown-jewel"}, analyst); r.Status != 409 {
		t.Errorf("tag ganda harus 409: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/tags", map[string]any{"name": "x", "color": "merah"}, analyst); r.Status != 422 {
		t.Errorf("warna tidak valid harus 422: %d", r.Status)
	}
	mkAsset(t, a, analyst, map[string]any{"category": "host", "name": "db-01", "platform": "windows", "tag_ids": []string{tag.ID}, "criticality": "critical"})
	mkAsset(t, a, analyst, map[string]any{"category": "host", "name": "web-01", "platform": "windows", "criticality": "low"})

	rule := map[string]any{"mode": "and", "filters": []map[string]any{
		{"field": "tag", "operator": "eq", "values": []string{"crown-jewel"}},
		{"field": "platform", "operator": "eq", "values": []string{"windows"}}}}

	// pratinjau tidak menyimpan apa pun
	var pv struct {
		Count, Total int
		Sample       []struct{ Name string }
	}
	r := a.do("POST", "/api/v1/asset-groups/preview", map[string]any{"rule": rule}, viewer)
	r.JSON(&pv)
	if r.Status != 200 || pv.Count != 1 || pv.Total != 2 || pv.Sample[0].Name != "db-01" {
		t.Fatalf("pratinjau salah: %d %s", r.Status, r.Body)
	}
	if n := len(listGroups(t, a, admin)); n != 7 {
		t.Fatalf("pratinjau tidak boleh membuat grup (jumlah=%d)", n)
	}

	// izin: viewer tidak boleh membuat; analyst boleh; hanya admin boleh menghapus
	if r := a.do("POST", "/api/v1/asset-groups", map[string]any{"name": "Crown jewels", "rule": rule}, viewer); r.Status != 403 {
		t.Errorf("viewer tidak boleh membuat grup: %d", r.Status)
	}
	var g groupJSON
	r = a.do("POST", "/api/v1/asset-groups", map[string]any{"name": "Crown jewels", "description": "aset utama", "rule": rule}, analyst)
	r.JSON(&g)
	if r.Status != 201 || g.MemberCount != 1 || len(g.Members) != 1 || g.Members[0].Name != "db-01" || g.TotalAssets != 2 {
		t.Fatalf("buat grup: %d %s", r.Status, r.Body)
	}
	if r := a.do("POST", "/api/v1/asset-groups", map[string]any{"name": "crown JEWELS ", "rule": rule}, analyst); r.Status != 201 && r.Status != 422 {
		t.Errorf("nama berbeda huruf besar boleh atau ditolak jelas: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/asset-groups", map[string]any{"name": "Crown jewels", "rule": rule}, analyst); r.Status != 422 {
		t.Errorf("nama grup ganda harus 422: %d %s", r.Status, r.Body)
	}
	if r := a.do("POST", "/api/v1/asset-groups", map[string]any{"name": "buruk", "rule": map[string]any{"filters": []map[string]any{{"field": "x", "operator": "eq", "values": []string{"1"}}}}}, analyst); r.Status != 422 {
		t.Errorf("aturan salah harus 422: %d", r.Status)
	}

	// ubah aturan -> keanggotaan dihitung ulang; versi lama ditolak 409
	rule2 := map[string]any{"mode": "and", "filters": []map[string]any{{"field": "criticality", "operator": "in", "values": []string{"low", "medium"}}}}
	if r := a.do("PUT", "/api/v1/asset-groups/"+g.ID, map[string]any{"name": "Crown jewels", "rule": rule2, "version": g.Version + 7}, analyst); r.Status != 409 {
		t.Errorf("versi basi harus 409: %d", r.Status)
	}
	var g2 groupJSON
	r = a.do("PUT", "/api/v1/asset-groups/"+g.ID, map[string]any{"name": "Low risk", "rule": rule2, "version": g.Version}, analyst)
	r.JSON(&g2)
	if r.Status != 200 || g2.MemberCount != 1 || g2.Members[0].Name != "web-01" || g2.Version != g.Version+1 {
		t.Fatalf("ubah grup: %d %s", r.Status, r.Body)
	}
	if r := a.do("DELETE", "/api/v1/asset-groups/"+g.ID, nil, analyst); r.Status != 403 {
		t.Errorf("analyst tidak boleh menghapus grup: %d", r.Status)
	}
	if r := a.do("DELETE", "/api/v1/asset-groups/"+g.ID, nil, admin); r.Status != 204 {
		t.Errorf("admin boleh menghapus grup: %d", r.Status)
	}
	if r := a.do("GET", "/api/v1/asset-groups/"+g.ID, nil, admin); r.Status != 404 {
		t.Errorf("grup terhapus harus 404: %d", r.Status)
	}
}

func TestAssetPatchDeleteAndOptimisticLocking(t *testing.T) {
	a, env := setup(t)
	admin, analyst, viewer := a.login("admin@a.test"), a.login("analyst@a.test"), a.login("viewer@a.test")
	id := mkAsset(t, a, analyst, map[string]any{"category": "host", "name": "srv", "platform": "linux", "arch": "x86_64",
		"addresses": []map[string]any{{"kind": "ip", "value": "10.0.0.1"}}})
	var cur struct {
		Version   int
		Addresses []struct{ Value string }
		Groups    []struct{ Name string }
	}
	a.do("GET", "/api/v1/assets/"+id, nil, analyst).JSON(&cur)
	if len(cur.Groups) != 2 { // All endpoints + All Linux x86_64
		t.Fatalf("detail harus memuat grup anggota: %+v", cur.Groups)
	}
	patch := func(sc string, body map[string]any) resp { return a.do("PATCH", "/api/v1/assets/"+id, body, sc) }
	if r := patch(viewer, map[string]any{"version": cur.Version, "name": "x"}); r.Status != 403 {
		t.Errorf("viewer tidak boleh mengubah: %d", r.Status)
	}
	if r := patch(analyst, map[string]any{"name": "tanpa-versi"}); r.Status != 422 {
		t.Errorf("tanpa version harus 422: %d", r.Status)
	}
	if r := patch(analyst, map[string]any{"version": cur.Version, "name": "srv-2", "addresses": []map[string]any{{"kind": "hostname", "value": "SRV-2.corp"}}}); r.Status != 200 {
		t.Fatalf("patch sah: %d %s", r.Status, r.Body)
	}
	if r := patch(analyst, map[string]any{"version": cur.Version, "name": "balapan"}); r.Status != 409 {
		t.Errorf("versi lama harus 409 (ada yang mengubah lebih dulu): %d", r.Status)
	}
	if r := patch(analyst, map[string]any{"version": cur.Version + 1, "criticality": "gawat"}); r.Status != 422 {
		t.Errorf("nilai tidak valid harus 422: %d", r.Status)
	}
	// otorisasi pengujian: analyst tidak boleh; admin boleh; mengubah kembali ke false membersihkan penanda
	if r := patch(analyst, map[string]any{"version": cur.Version + 1, "testing_authorized": true}); r.Status != 403 {
		t.Errorf("analyst tidak boleh mengotorisasi: %d", r.Status)
	}
	if r := patch(admin, map[string]any{"version": cur.Version + 1, "testing_authorized": true}); r.Status != 200 {
		t.Fatalf("admin mengotorisasi: %d %s", r.Status, r.Body)
	}
	var by *string
	env.Owner.QueryRow(context.Background(), `SELECT authorized_by::text FROM assets WHERE id=$1`, id).Scan(&by)
	if by == nil {
		t.Error("authorized_by harus terisi")
	}
	if r := patch(admin, map[string]any{"version": cur.Version + 2, "testing_authorized": false}); r.Status != 200 {
		t.Fatalf("cabut otorisasi: %d", r.Status)
	}
	env.Owner.QueryRow(context.Background(), `SELECT authorized_by::text FROM assets WHERE id=$1`, id).Scan(&by)
	if by != nil {
		t.Error("authorized_by harus dibersihkan saat otorisasi dicabut")
	}
	// tag tak dikenal ditolak
	if r := patch(analyst, map[string]any{"version": cur.Version + 3, "tag_ids": []string{"00000000-0000-0000-0000-000000000001"}}); r.Status != 422 {
		t.Errorf("tag tak dikenal harus 422: %d", r.Status)
	}
	// hapus: hanya admin; setelah itu 404 dan keluar dari grup
	if r := a.do("DELETE", "/api/v1/assets/"+id, nil, analyst); r.Status != 403 {
		t.Errorf("analyst tidak boleh menghapus: %d", r.Status)
	}
	if r := a.do("DELETE", "/api/v1/assets/"+id, nil, admin); r.Status != 204 {
		t.Fatalf("admin menghapus: %d", r.Status)
	}
	if r := a.do("GET", "/api/v1/assets/"+id, nil, admin); r.Status != 404 {
		t.Errorf("aset terhapus harus 404: %d", r.Status)
	}
	if r := a.do("DELETE", "/api/v1/assets/"+id, nil, admin); r.Status != 404 {
		t.Errorf("hapus dua kali harus 404: %d", r.Status)
	}
	if g := listGroups(t, a, admin); g["All endpoints"].MemberCount != 0 {
		t.Errorf("aset terhapus harus keluar dari grup: %+v", g["All endpoints"])
	}
	var n int
	env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE resource_id=$1 AND action IN ('asset.create','asset.update','asset.delete')`, id).Scan(&n)
	// 1 create + 3 update yang berhasil (nama, otorisasi, pencabutan) + 1 delete. Permintaan yang ditolak tidak dicatat.
	if n != 5 {
		t.Errorf("audit harus mencatat 1 create + 3 update + 1 delete, dapat %d", n)
	}
}

func TestGroupsAndTagsAreTenantIsolated(t *testing.T) {
	env := testutil.New(t)
	appA, appB := newApp(t, env, env.TenantA), newApp(t, env, env.TenantB)
	appA.user(env.TenantA, "admin@a.test", "admin")
	appB.user(env.TenantB, "admin@b.test", "admin")
	scA, scB := appA.login("admin@a.test"), appB.login("admin@b.test")
	var tag struct{ ID string }
	appA.do("POST", "/api/v1/tags", map[string]any{"name": "rahasia-A"}, scA).JSON(&tag)
	rule := map[string]any{"filters": []map[string]any{{"field": "tag", "operator": "eq", "values": []string{"rahasia-A"}}}}
	var g groupJSON
	appA.do("POST", "/api/v1/asset-groups", map[string]any{"name": "grup-A", "rule": rule}, scA).JSON(&g)
	aid := mkAsset(t, appA, scA, map[string]any{"category": "host", "name": "a-1", "platform": "linux", "arch": "arm64", "tag_ids": []string{tag.ID}})
	_ = aid

	var tags struct{ Items []struct{ Name string } }
	appB.do("GET", "/api/v1/tags", nil, scB).JSON(&tags)
	if len(tags.Items) != 0 {
		t.Errorf("tenant B tidak boleh melihat tag A: %+v", tags.Items)
	}
	if _, leaked := listGroups(t, appB, scB)["grup-A"]; leaked {
		t.Error("tenant B tidak boleh melihat grup A")
	}
	if r := appB.do("GET", "/api/v1/asset-groups/"+g.ID, nil, scB); r.Status != 404 {
		t.Errorf("grup A lewat ID di tenant B harus 404: %d", r.Status)
	}
	if r := appB.do("DELETE", "/api/v1/asset-groups/"+g.ID, nil, scB); r.Status != 404 {
		t.Errorf("hapus grup A dari tenant B harus 404: %d", r.Status)
	}
	// B tidak boleh memakai tag A pada asetnya
	bid := mkAsset(t, appB, scB, map[string]any{"category": "host", "name": "b-1"})
	if r := appB.do("PATCH", "/api/v1/assets/"+bid, map[string]any{"version": 1, "tag_ids": []string{tag.ID}}, scB); r.Status != 422 {
		t.Errorf("tag milik tenant lain harus ditolak: %d %s", r.Status, r.Body)
	}
	// pratinjau B dengan aturan tag yang sama tidak menemukan aset A
	var pv struct{ Count int }
	appB.do("POST", "/api/v1/asset-groups/preview", map[string]any{"rule": rule}, scB).JSON(&pv)
	if pv.Count != 0 {
		t.Errorf("pratinjau tenant B tidak boleh menghitung aset A: %d", pv.Count)
	}
}

func csvPayload(csv string, mapping map[string]string, skip bool) map[string]any {
	return map[string]any{"csv": csv, "mapping": mapping, "skip_invalid": skip}
}

func TestCSVImportPreviewAndCommit(t *testing.T) {
	a, env := setup(t)
	admin, analyst, viewer := a.login("admin@a.test"), a.login("analyst@a.test"), a.login("viewer@a.test")
	mkAsset(t, a, admin, map[string]any{"category": "host", "name": "srv-app-01"}) // sudah ada -> duplikat
	csv := "hostname,os,arch,criticality,tags,owner\n" +
		"srv-app-02,windows,x64,high,web;prod,j.doe\n" +
		"srv-app-01,linux,x86_64,low,,x\n" + // duplikat dari DB
		"srv-app-03,Solaris,x86_64,low,,x\n" + // platform tak didukung
		"srv-app-02,linux,arm64,low,,x\n" + // duplikat dalam berkas
		"10.1.2.3,macos,aarch64,critical,prod,x\n" +
		",linux,arm64,low,,x\n" // nama kosong
	// tanpa mapping: hanya saran kolom
	var pv struct {
		Rows                int
		Validated           bool
		SuggestedMapping    map[string]string `json:"suggested_mapping"`
		Valid               int
		Invalid, Duplicates []struct {
			Row     int
			Message string
		}
	}
	r := a.do("POST", "/api/v1/assets/import/preview", csvPayload(csv, nil, false), analyst)
	r.JSON(&pv)
	if r.Status != 200 || pv.Validated || pv.Rows != 6 || pv.SuggestedMapping["hostname"] != "host" || pv.SuggestedMapping["os"] != "platform" ||
		pv.SuggestedMapping["arch"] != "arch" || pv.SuggestedMapping["tags"] != "tags" || pv.SuggestedMapping["owner"] != "ignore" {
		t.Fatalf("saran kolom salah: %d %s", r.Status, r.Body)
	}
	mapping := map[string]string{"hostname": "host", "os": "platform", "arch": "arch", "criticality": "criticality", "tags": "tags", "owner": "ignore"}
	r = a.do("POST", "/api/v1/assets/import/preview", csvPayload(csv, mapping, false), analyst)
	r.JSON(&pv)
	if r.Status != 200 || !pv.Validated || pv.Valid != 2 || len(pv.Invalid) != 2 || len(pv.Duplicates) != 2 {
		t.Fatalf("pratinjau salah (valid=%d invalid=%d dup=%d): %s", pv.Valid, len(pv.Invalid), len(pv.Duplicates), r.Body)
	}
	foundSolaris := false
	for _, i := range pv.Invalid {
		if i.Row == 4 && strings.Contains(i.Message, "isn't supported. Use windows, linux or macos") {
			foundSolaris = true
		}
	}
	if !foundSolaris {
		t.Errorf("pesan platform tak didukung / nomor baris salah: %+v", pv.Invalid)
	}
	var n int
	env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM assets WHERE source='csv'`).Scan(&n)
	if n != 0 {
		t.Fatal("pratinjau tidak boleh menulis data")
	}
	// komit: viewer ditolak; baris tak valid menahan impor kecuali dilewati
	if r := a.do("POST", "/api/v1/assets/import", csvPayload(csv, mapping, true), viewer); r.Status != 403 {
		t.Errorf("viewer tidak boleh mengimpor: %d", r.Status)
	}
	if r := a.do("POST", "/api/v1/assets/import", csvPayload(csv, mapping, false), analyst); r.Status != 422 {
		t.Errorf("baris tak valid tanpa skip harus 422: %d", r.Status)
	}
	var res struct {
		Created           int `json:"created"`
		SkippedInvalid    int `json:"skipped_invalid"`
		SkippedDuplicates int `json:"skipped_duplicates"`
		TagsCreated       int `json:"tags_created"`
	}
	r = a.do("POST", "/api/v1/assets/import", csvPayload(csv, mapping, true), analyst)
	r.JSON(&res)
	if r.Status != 201 || res.Created != 2 || res.SkippedInvalid != 2 || res.SkippedDuplicates != 2 || res.TagsCreated != 2 {
		t.Fatalf("komit salah: %d %s", r.Status, r.Body)
	}
	var authorized, hasIP int
	env.Owner.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE testing_authorized), count(*) FILTER (WHERE id IN (SELECT asset_id FROM asset_addresses WHERE value='10.1.2.3')) FROM assets WHERE source='csv'`).Scan(&authorized, &hasIP)
	if authorized != 0 || hasIP != 1 {
		t.Errorf("aset impor tidak boleh berstatus authorized (=%d) dan alamat IP harus tersimpan (=%d)", authorized, hasIP)
	}
	if g := listGroups(t, a, admin); g["All Windows x86_64"].MemberCount != 1 || g["All macOS arm64"].MemberCount != 1 {
		t.Errorf("grup harus mencerminkan aset impor: %+v / %+v", g["All Windows x86_64"], g["All macOS arm64"])
	}
	// mengulang impor yang sama: semuanya kini duplikat -> tidak ada baris baru
	if r := a.do("POST", "/api/v1/assets/import", csvPayload(csv, mapping, true), analyst); r.Status != 422 {
		t.Errorf("impor ulang harus ditolak karena tidak ada baris baru: %d %s", r.Status, r.Body)
	}
}

func TestCSVImportRejectsBadInput(t *testing.T) {
	a, _ := setup(t)
	sc := a.login("analyst@a.test")
	post := func(body map[string]any) resp { return a.do("POST", "/api/v1/assets/import/preview", body, sc) }
	if r := post(csvPayload("", nil, false)); r.Status != 422 {
		t.Errorf("berkas kosong: %d", r.Status)
	}
	if r := post(csvPayload("name\n", nil, false)); r.Status != 422 {
		t.Errorf("hanya header: %d", r.Status)
	}
	if r := post(csvPayload("name,name2\na,\"b\n", nil, false)); r.Status != 422 {
		t.Errorf("CSV rusak (kutip tak tertutup): %d", r.Status)
	}
	if r := post(csvPayload("name\nx\n", map[string]string{"tidak-ada": "host"}, false)); r.Status != 422 {
		t.Errorf("kolom pemetaan tak ada: %d", r.Status)
	}
	if r := post(csvPayload("name\nx\n", map[string]string{"name": "ignore"}, false)); r.Status != 422 {
		t.Errorf("tanpa kolom nama: %d", r.Status)
	}
	if r := post(csvPayload("a,b\n1,2\n", map[string]string{"a": "host", "b": "host"}, false)); r.Status != 422 {
		t.Errorf("dua kolom ke target yang sama: %d", r.Status)
	}
	if r := post(csvPayload("a\n1\n", map[string]string{"a": "rahasia"}, false)); r.Status != 422 {
		t.Errorf("target tak dikenal: %d", r.Status)
	}
	var sb strings.Builder
	sb.WriteString("name\n")
	for i := 0; i < 1001; i++ {
		fmt.Fprintf(&sb, "h%d\n", i)
	}
	if r := post(csvPayload(sb.String(), nil, false)); r.Status != 422 {
		t.Errorf("lebih dari 1000 baris: %d", r.Status)
	}
	// pemisah titik koma + BOM + CRLF (ekspor Excel regional)
	var pv struct{ Valid int }
	r := post(csvPayload("\xef\xbb\xbfhost;os\r\nsrv-a;linux\r\nsrv-b;mac\r\n", map[string]string{"host": "host", "os": "platform"}, false))
	r.JSON(&pv)
	if r.Status != 200 || pv.Valid != 2 {
		t.Errorf("titik koma/BOM/CRLF harus terbaca: %d %s", r.Status, r.Body)
	}
}
