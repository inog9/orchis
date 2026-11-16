package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"orchis-go/internal/osquery"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"orchis-go/internal/demo"
	"orchis-go/internal/mitre"
)

func fixtureServer(t *testing.T, body func() ([]byte, int)) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, code := body()
		w.WriteHeader(code)
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func bundle(t *testing.T) []byte {
	b, err := os.ReadFile("../mitre/testdata/bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func importer(env interface{}, c *chainFixture, url string) *mitre.Importer {
	return &mitre.Importer{DB: c.a.env.App, Tenant: c.a.env.TenantA, Source: url}
}

func TestMitreImportIsAtomicIdempotentAndLinksContracts(t *testing.T) {
	c := newChainFixture(t)
	good := bundle(t)
	var mu sync.Mutex
	serve := good
	srv := fixtureServer(t, func() ([]byte, int) { mu.Lock(); defer mu.Unlock(); return serve, 200 })
	im := importer(nil, c, srv.URL)
	ctx := context.Background()
	count := func(q string) int {
		var n int
		c.a.env.Owner.QueryRow(ctx, q).Scan(&n)
		return n
	}

	run, err := im.Import(ctx)
	if err != nil || run.Status != "succeeded" || *run.Techniques != 12 || *run.Tactics != 4 {
		t.Fatalf("impor pertama: %+v %v", run, err)
	}
	if n := count(`SELECT count(*) FROM mitre_techniques`); n != 12 {
		t.Fatalf("12 teknik (revoked tidak ikut, deprecated ikut dengan penanda): %d", n)
	}
	// Fixture memuat T1046, T1595.002 dan T1082 saja: T1046 (nmap), T1595.002 (nuclei), T1082 (agent.system_info,
	// osquery.inventory, osquery.hardening) = lima tautan bawaan.
	// Empat aksi non-osquery (termasuk Hackbot) ditambah rujukan T1082 osquery.
	wantLinks := 4
	for _, q := range osquery.Queries {
		for _, ref := range q.Mitre {
			if ref.ID == "T1082" {
				wantLinks++
			}
		}
	}
	if n := count(`SELECT count(*) FROM contract_techniques`); n != wantLinks {
		t.Fatalf("tautan bawaan %d diharapkan: %d", wantLinks, n)
	}
	if n := count(`SELECT count(*) FROM contract_techniques WHERE reason='' OR basis<>'direct'`); n != 0 {
		t.Fatalf("setiap tautan wajib beralasan dan berbasis langsung: %d tidak", n)
	}
	// ulangi: tidak menggandakan apa pun
	if _, err := im.Import(ctx); err != nil {
		t.Fatal(err)
	}
	if count(`SELECT count(*) FROM mitre_techniques`) != 12 || count(`SELECT count(*) FROM mitre_tactics`) != 4 || count(`SELECT count(*) FROM contract_techniques`) != wantLinks {
		t.Fatal("impor ulang harus idempoten")
	}
	// sumber rusak: gagal, dicatat, dan data lama utuh
	mu.Lock()
	serve = []byte(`{"objects":[{"type":"attack-pattern"`)
	mu.Unlock()
	if run, err := im.Import(ctx); err == nil || run.Status != "failed" || run.Error == nil {
		t.Fatalf("sumber rusak harus gagal: %+v %v", run, err)
	}
	if count(`SELECT count(*) FROM mitre_techniques WHERE NOT is_deprecated`) != 11 { // 12 teknik, satu (T1001) deprecated dari sumber
		t.Fatal("data lama harus utuh setelah impor gagal")
	}
	var status string
	c.a.env.Owner.QueryRow(ctx, `SELECT status FROM mitre_import_runs ORDER BY started_at DESC LIMIT 1`).Scan(&status)
	if status != "failed" {
		t.Fatalf("catatan impor: %s", status)
	}
	// sumber 500
	mu.Lock()
	serve = nil
	mu.Unlock()
	bad := fixtureServer(t, func() ([]byte, int) { return []byte("x"), 503 })
	if _, err := importer(nil, c, bad.URL).Import(ctx); err == nil {
		t.Fatal("HTTP 503 harus gagal")
	}
	// teknik yang hilang dari sumber ditandai deprecated, bukan dihapus (tautan tetap)
	smaller := string(good)
	_ = smaller
	c.a.env.Owner.Exec(ctx, `INSERT INTO mitre_techniques (id, name) VALUES ('T9999','Gone soon')`)
	mu.Lock()
	serve = good
	mu.Unlock()
	if _, err := im.Import(ctx); err != nil {
		t.Fatal(err)
	}
	var dep bool
	c.a.env.Owner.QueryRow(ctx, `SELECT is_deprecated FROM mitre_techniques WHERE id='T9999'`).Scan(&dep)
	if !dep {
		t.Fatal("teknik yang hilang dari sumber harus ditandai deprecated")
	}
}

func TestMitreConcurrentImportsRunOnce(t *testing.T) {
	c := newChainFixture(t)
	gate := make(chan struct{})
	b := bundle(t)
	srv := fixtureServer(t, func() ([]byte, int) { <-gate; return b, 200 })
	im := importer(nil, c, srv.URL)
	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := range errs {
		wg.Add(1)
		go func() { defer wg.Done(); _, errs[i] = im.Import(context.Background()) }()
	}
	time.Sleep(700 * time.Millisecond) // semua sudah mencoba mengklaim; hanya satu yang masuk ke unduhan
	close(gate)
	wg.Wait()
	ok, busy := 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case e == mitre.ErrBusy:
			busy++
		default:
			t.Fatalf("galat tak terduga: %v", e)
		}
	}
	if ok != 1 || busy != 2 {
		t.Fatalf("tiga impor serentak: sukses=%d sibuk=%d (harus 1 dan 2)", ok, busy)
	}
}

func TestMitreRunIfDueSkipsFreshDataAndDedupesPerDay(t *testing.T) {
	c := newChainFixture(t)
	b := bundle(t)
	hits := 0
	srv := fixtureServer(t, func() ([]byte, int) { hits++; return b, 200 })
	im := importer(nil, c, srv.URL)
	ctx := context.Background()
	now := time.Now()
	ran, err := im.RunIfDue(ctx, 7*24*time.Hour, "a", now)
	if err != nil || !ran || hits != 1 {
		t.Fatalf("pertama kali harus mengimpor: ran=%v hits=%d err=%v", ran, hits, err)
	}
	if ran, _ := im.RunIfDue(ctx, 7*24*time.Hour, "b", now); ran || hits != 1 {
		t.Fatalf("data masih segar: tidak boleh mengimpor lagi (hits=%d)", hits)
	}
	// data lama: instance pertama menang klaim hari itu, instance kedua tidak mengulang
	c.a.env.Owner.Exec(ctx, `UPDATE mitre_import_runs SET finished_at = now() - interval '30 days'`)
	c.a.env.Owner.Exec(ctx, `DELETE FROM job_runs`)
	r1, _ := im.RunIfDue(ctx, 7*24*time.Hour, "a", now)
	r2, _ := im.RunIfDue(ctx, 7*24*time.Hour, "b", now)
	if !r1 || r2 || hits != 2 {
		t.Fatalf("hanya satu instance per hari: r1=%v r2=%v hits=%d", r1, r2, hits)
	}
}

type matrixJSON struct {
	Tactics []struct {
		ID, Name   string
		Techniques []struct {
			ID                           string
			Mapped, Exercised, Simulated bool
			Runs                         int
			Subs                         []struct {
				ID                string
				Mapped, Exercised bool
			}
		}
	}
	Total, Mapped, Exercised, Simulated int
	Platforms                           []string
	Imported                            bool
}

func (c *chainFixture) matrix(t *testing.T, cookie string) matrixJSON {
	var m matrixJSON
	r := c.a.do("GET", "/api/v1/mitre/matrix", nil, cookie)
	if r.Status != 200 {
		t.Fatalf("matrix: %d %s", r.Status, r.Body)
	}
	r.JSON(&m)
	return m
}

func TestMitreMatrixMarksExercisedTechniquesPerTenant(t *testing.T) {
	c := newChainFixture(t)
	if m := c.matrix(t, c.analyst); m.Imported || len(m.Tactics) != 0 {
		t.Fatalf("sebelum impor matriks kosong: %+v", m)
	}
	srv := fixtureServer(t, func() ([]byte, int) { return bundle(t), 200 })
	if _, err := importer(nil, c, srv.URL).Import(context.Background()); err != nil {
		t.Fatal(err)
	}
	m := c.matrix(t, c.analyst)
	if !m.Imported || len(m.Tactics) != 4 || m.Tactics[0].Name != "Reconnaissance" || m.Tactics[3].Name != "Discovery" {
		t.Fatalf("kolom: %+v", m)
	}
	// T1595 (induk) dapat dilatih lewat sub-teknik T1595.002 milik Nuclei; T1046 lewat Nmap; T1082 lewat agent.system_info
	if m.Mapped != 3 || m.Exercised != 0 {
		t.Fatalf("sebelum dijalankan: mapped=%d exercised=%d", m.Mapped, m.Exercised)
	}
	// jalankan nmap sukses: T1046 menjadi 'exercised'
	e := c.run(t)
	c.inj("POST", "executions/"+e.ID+"/claim", nil, c.token)
	c.inj("POST", "executions/"+e.ID+"/result", okEmpty, c.token)
	m = c.matrix(t, c.analyst)
	var discovery, recon bool
	for _, col := range m.Tactics {
		for _, tech := range col.Techniques {
			if tech.ID == "T1046" {
				discovery = tech.Exercised && tech.Runs == 1
			}
			if tech.ID == "T1595" {
				recon = tech.Mapped && !tech.Exercised && len(tech.Subs) == 2
			}
		}
	}
	if !discovery || !recon || m.Exercised != 1 {
		t.Fatalf("setelah nmap sukses: %+v", m)
	}
	// eksekusi gagal tidak dihitung
	e2 := c.run(t)
	c.inj("POST", "executions/"+e2.ID+"/claim", nil, c.token)
	c.inj("POST", "executions/"+e2.ID+"/result", failed, c.token)
	for _, col := range c.matrix(t, c.analyst).Tactics {
		for _, tech := range col.Techniques {
			if tech.ID == "T1046" && tech.Runs != 1 {
				t.Fatalf("hanya eksekusi sukses yang dihitung: %+v", tech)
			}
		}
	}
	// sub-teknik yang dilatih menandai induknya
	e3 := c.runNuclei(t)
	c.finish(t, e3, nucleiC, okEmpty)
	for _, col := range c.matrix(t, c.analyst).Tactics {
		for _, tech := range col.Techniques {
			if tech.ID == "T1595" && !(tech.Exercised && tech.Runs == 1) {
				t.Fatalf("induk harus ikut dilatih: %+v", tech)
			}
		}
	}
	// detail teknik
	var d struct {
		ID, Name, URL string
		Actions       []string
		Runs          int
	}
	r := c.a.do("GET", "/api/v1/mitre/techniques/T1595", nil, c.analyst)
	r.JSON(&d)
	if r.Status != 200 || d.URL != "https://attack.mitre.org/techniques/T1595/" || d.Runs != 1 || len(d.Actions) != 2 || d.Actions[0] != "Nuclei vulnerability scan" || d.Actions[1] != "orchis-redteam-agent web pentest" {
		t.Fatalf("detail: %d %s", r.Status, r.Body)
	}
	r = c.a.do("GET", "/api/v1/mitre/techniques/T1595.002", nil, c.analyst)
	r.JSON(&d)
	if d.URL != "https://attack.mitre.org/techniques/T1595/002/" {
		t.Fatalf("URL sub-teknik: %s", d.URL)
	}
	for _, bad := range []string{"T1", "x", "T1595.2", "T1595;DROP", "T9998"} {
		if r := c.a.do("GET", "/api/v1/mitre/techniques/"+bad, nil, c.analyst); r.Status != 404 {
			t.Fatalf("id %q: %d", bad, r.Status)
		}
	}
	// isolasi: tenant B tidak melihat pelatihan tenant A walau data ATT&CK bersama
	pb := principalFor(t, c.a.env.TenantB)
	pb.Permissions = append(pb.Permissions, "scenario.read")
	svc := &mitre.Service{DB: c.a.env.App}
	mb, err := svc.Matrix(context.Background(), pb)
	if err != nil || mb.Exercised != 0 || mb.Mapped != 0 {
		t.Fatalf("tenant B: %+v %v", mb, err)
	}
}

func TestMitreManualImportNeedsPermissionAndConfig(t *testing.T) {
	c := newChainFixture(t)
	if r := c.a.do("POST", "/api/v1/mitre/import", nil, c.analyst); r.Status != 403 {
		t.Fatalf("analyst: %d", r.Status)
	}
	if r := c.a.do("POST", "/api/v1/mitre/import", nil, c.admin); r.Status != 409 {
		t.Fatalf("tanpa sumber terkonfigurasi harus 409: %d %s", r.Status, r.Body)
	}
	if r := c.a.do("GET", "/api/v1/mitre/matrix", nil, ""); r.Status != 401 {
		t.Fatalf("tanpa login: %d", r.Status)
	}
}

// runNuclei membuat dan menjalankan atomic test nuclei, mengembalikan ID eksekusinya.
func (c *chainFixture) runNuclei(t *testing.T) string {
	var tst struct{ ID string }
	c.a.do("POST", "/api/v1/atomic-tests", map[string]any{"name": "nuc", "contract_key": nucleiC, "target_asset_id": c.asset}, c.analyst).JSON(&tst)
	var e execJSON
	c.a.do("POST", "/api/v1/atomic-tests/"+tst.ID+"/run", nil, c.analyst).JSON(&e)
	return e.ID
}

// Bundel Enterprise ATT&CK penuh yang dibekukan: dimuat dari berkas lokal dengan hash dipatok (jalur produksi).
func TestMitreFullFrozenBundleImportAndThreeCoverageLevels(t *testing.T) {
	m, ds, err := mitre.Verify("../../deploy/mitre")
	if err != nil {
		t.Skipf("bundel beku belum ada (make mitre-update): %v", err)
	}
	c := newChainFixture(t)
	ctx := context.Background()
	im := &mitre.Importer{DB: c.a.env.App, Tenant: c.a.env.TenantA, Source: filepath.Join("../../deploy/mitre", m.File), ExpectedSHA256: m.SHA256}
	run, err := im.Import(ctx)
	if err != nil || run.Status != "succeeded" || run.SHA256 == nil || *run.SHA256 != m.SHA256 {
		t.Fatalf("impor bundel beku: %+v %v", run, err)
	}
	count := func(q string) int {
		var n int
		if err := c.a.env.Owner.QueryRow(ctx, q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	want := ds.Count()
	if n := count(`SELECT count(*) FROM mitre_tactics`); n != want.Tactics {
		t.Fatalf("taktik di DB %d, bundel %d", n, want.Tactics)
	}
	if n := count(`SELECT count(*) FROM mitre_techniques WHERE parent_id IS NULL AND NOT is_deprecated`); n != want.Parents {
		t.Fatalf("teknik induk aktif di DB %d, bundel %d", n, want.Parents)
	}
	if n := count(`SELECT count(*) FROM mitre_techniques WHERE parent_id IS NOT NULL AND NOT is_deprecated`); n != want.Subs {
		t.Fatalf("sub-teknik aktif di DB %d, bundel %d", n, want.Subs)
	}
	if n := count(`SELECT count(*) FROM mitre_techniques WHERE is_deprecated`); n != want.Deprecated {
		t.Fatalf("deprecated di DB %d, bundel %d", n, want.Deprecated)
	}
	// Idempoten: impor ulang tidak mengubah jumlah apa pun.
	before := count(`SELECT count(*) FROM mitre_techniques`)
	if _, err := im.Import(ctx); err != nil {
		t.Fatal(err)
	}
	if after := count(`SELECT count(*) FROM mitre_techniques`); after != before {
		t.Fatalf("impor ulang mengubah jumlah teknik: %d -> %d", before, after)
	}
	// Hash salah ditolak SEBELUM menyentuh data, dan data lama utuh.
	bad := &mitre.Importer{DB: c.a.env.App, Tenant: c.a.env.TenantA, Source: im.Source, ExpectedSHA256: "00" + m.SHA256[2:]}
	if _, err := bad.Import(ctx); err == nil {
		t.Fatal("hash salah harus ditolak")
	}
	if after := count(`SELECT count(*) FROM mitre_techniques`); after != before {
		t.Fatal("impor yang ditolak mengubah data")
	}

	mx := c.matrix(t, c.analyst)
	if !mx.Imported || len(mx.Tactics) != want.Tactics || mx.Total != want.Parents {
		t.Fatalf("matriks: taktik=%d total=%d", len(mx.Tactics), mx.Total)
	}
	if len(mx.Platforms) < 5 {
		t.Fatalf("daftar platform untuk filter kosong/terlalu kecil: %v", mx.Platforms)
	}
	// Tingkat 1 (mapped): aksi bawaan (T1046, T1595, T1082, T1049, T1518, T1071, T1087, T1120, T1016 sebagai induk); tingkat 2 (trained) belum ada
	// eksekusi nyata; tingkat 3 (simulated) belum ada demo.
	if mx.Mapped != 9 || mx.Exercised != 0 || mx.Simulated != 0 {
		t.Fatalf("sebelum demo: mapped=%d exercised=%d simulated=%d", mx.Mapped, mx.Exercised, mx.Simulated)
	}

	// Data demo menambah tingkat "simulated" (T1190 dirujuk temuan demo) tanpa pernah menjadi mapped/trained.
	if err := (&demo.Service{DB: c.a.env.App}).Seed(ctx, c.a.env.TenantA, 1, nil, "", "t"); err != nil {
		t.Fatal(err)
	}
	mx = c.matrix(t, c.analyst)
	if mx.Simulated == 0 || mx.Exercised != 0 || mx.Mapped != 9 {
		t.Fatalf("setelah demo: mapped=%d exercised=%d simulated=%d (demo tidak boleh jadi trained/mapped)", mx.Mapped, mx.Exercised, mx.Simulated)
	}
	var t1190 *struct {
		ID                           string
		Mapped, Exercised, Simulated bool
		Runs                         int
		Subs                         []struct {
			ID                string
			Mapped, Exercised bool
		}
	}
	for ti := range mx.Tactics {
		for i := range mx.Tactics[ti].Techniques {
			if mx.Tactics[ti].Techniques[i].ID == "T1190" {
				t1190 = &mx.Tactics[ti].Techniques[i]
			}
		}
	}
	if t1190 == nil || !t1190.Simulated || t1190.Mapped || t1190.Exercised {
		t.Fatalf("T1190 harus simulated saja: %+v", t1190)
	}

	// Detail teknik: tautan sumber, mitigasi, sub-teknik, dan pemetaan beralasan.
	var det struct {
		ID, URL     string
		Mitigations []struct{ ID, Name string }
		Mappings    []struct{ Action, Reason, Basis, Via string }
		Subs        []struct{ ID string }
	}
	r := c.a.do("GET", "/api/v1/mitre/techniques/T1046", nil, c.analyst)
	r.JSON(&det)
	if r.Status != 200 || det.ID != "T1046" || det.URL != "https://attack.mitre.org/techniques/T1046" || len(det.Mitigations) == 0 ||
		len(det.Mappings) < 1 || det.Mappings[0].Reason == "" || det.Mappings[0].Basis != "direct" {
		t.Fatalf("detail T1046: %d %s", r.Status, r.Body)
	}
	det.Subs, det.Mappings = nil, nil
	r = c.a.do("GET", "/api/v1/mitre/techniques/T1595", nil, c.analyst)
	r.JSON(&det)
	if r.Status != 200 || len(det.Subs) != 3 || len(det.Mappings) != 2 || det.Mappings[0].Via != "T1595.002" || det.Mappings[1].Via != "T1595.002" {
		t.Fatalf("detail T1595 harus memuat 3 sub-teknik dan pemetaan via T1595.002: %d %s", r.Status, r.Body)
	}
}
