package server_test

// Kebijakan pemilik produk: memasang agent berarti menyetujui koleksi DefenseBot. Test di sini
// menjaga dua jalurnya (token organisasi saat enroll, dan sweeper untuk organisasi yang sudah
// ada) beserta garis merahnya: check Endpoint/network tidak pernah ditanam, dan baris yang
// dimatikan admin tidak pernah dinyalakan lagi.

import (
	"context"
	"testing"

	"orchis-go/internal/db"
	"orchis-go/internal/endpoint"
)

func TestEnrollWithOrgTokenSeedsDefenseBotConsent(t *testing.T) {
	f := newAgentFixture(t)
	org := createOrg(t, f.a, f.admin, "autoconsent")
	r := f.a.do("POST", "/api/v1/agents/enrollment-tokens", map[string]any{"os": "linux", "arch": "x86_64", "mode": "service", "organization_id": org.ID.String()}, f.admin)
	if r.Status != 201 {
		t.Fatalf("token: %d %s", r.Status, r.Body)
	}
	var tk struct{ ID, Token string }
	r.JSON(&tk)
	if r := f.a.do("POST", "/api/v1/agent/enroll", f.enrollBody(tk.Token, "autobox"), "", "X-Requested-With", ""); r.Status != 200 {
		t.Fatalf("enroll: %d %s", r.Status, r.Body)
	}
	var enabled []string
	rows, err := f.a.env.Owner.Query(context.Background(), `SELECT query_key FROM osquery_org_settings WHERE organization_id=$1 AND enabled AND consented_by IS NOT NULL`, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		enabled = append(enabled, k)
	}
	rows.Close()
	want := map[string]bool{"osquery.packages": false, "osquery.hardening": false, "osquery.os_release": false}
	for _, k := range enabled {
		if _, ok := want[k]; ok {
			want[k] = true
		}
	}
	for k, ok := range want {
		if !ok {
			t.Fatalf("check DefenseBot %s tidak ditanam saat enroll; aktif: %v", k, enabled)
		}
	}
	for _, k := range []string{"osquery.inventory", "osquery.users", "osquery.network", "osquery.network_ranges"} {
		var n int
		if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1 AND query_key=$2`, org.ID, k).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s berkategori Endpoint/network dan tidak boleh ditanam otomatis", k)
		}
	}
	// Enroll kedua dengan token organisasi yang sama tidak menambah atau mengubah baris.
	r2 := f.a.do("POST", "/api/v1/agents/enrollment-tokens", map[string]any{"os": "linux", "arch": "x86_64", "mode": "service", "organization_id": org.ID.String()}, f.admin)
	if r2.Status != 201 {
		t.Fatalf("token kedua: %d %s", r2.Status, r2.Body)
	}
	var tk2 struct{ Token string }
	r2.JSON(&tk2)
	if r := f.a.do("POST", "/api/v1/agent/enroll", f.enrollBody(tk2.Token, "autobox2"), "", "X-Requested-With", ""); r.Status != 200 {
		t.Fatalf("enroll kedua: %d %s", r.Status, r.Body)
	}
	var total int
	if err := f.a.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1`, org.ID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != len(enabled) {
		t.Fatalf("enroll ulang harus idempoten; baris: %d, semula: %d", total, len(enabled))
	}
}

func TestAutoConsentSweepSeedsMissingAndRespectsAdminDisabled(t *testing.T) {
	f := newAgentFixture(t)
	_, asset := f.authorizedAgent(t, "sweepbox") // enroll tanpa organisasi
	org := f.joinOrg(t, asset)                   // masuk organisasi lewat penugasan aset
	svc := &endpoint.Service{DB: &db.DB{Pool: f.a.env.Owner}, AutoConsent: true}
	ctx := context.Background()
	if err := svc.AutoConsentTick(ctx, f.a.env.TenantA); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1 AND enabled AND query_key='osquery.packages'`, org).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("sweeper harus menanam scope DefenseBot untuk organisasi yang punya agent")
	}
	// Admin mematikan satu check: keputusan ini tidak boleh dibalikkan seeder.
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE osquery_org_settings SET enabled=false WHERE organization_id=$1 AND query_key='osquery.packages'`, org); err != nil {
		t.Fatal(err)
	}
	// Rotasi dokumen pada baris yang masih enabled.
	if _, err := f.a.env.Owner.Exec(ctx, `UPDATE osquery_org_settings SET consent_version='dokumen-lama' WHERE organization_id=$1 AND enabled`, org); err != nil {
		t.Fatal(err)
	}
	if err := svc.AutoConsentTick(ctx, f.a.env.TenantA); err != nil {
		t.Fatal(err)
	}
	var stale int
	if err := f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1 AND enabled AND consent_version='dokumen-lama'`, org).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if stale != 0 {
		t.Fatal("baris enabled yang konsennya kadaluarsa harus disegarkan ke dokumen berlaku")
	}
	var off int
	if err := f.a.env.Owner.QueryRow(ctx, `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1 AND query_key='osquery.packages' AND enabled`, org).Scan(&off); err != nil {
		t.Fatal(err)
	}
	if off != 0 {
		t.Fatal("check yang dimatikan admin tidak boleh dinyalakan seeder")
	}
}
