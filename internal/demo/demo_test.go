package demo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"orchis-go/internal/demo"
	"orchis-go/internal/ids"
	"orchis-go/internal/testutil"
)

func addAdmin(t *testing.T, env *testutil.Env, tenant uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	var roleID uuid.UUID
	if err := env.Owner.QueryRow(ctx, `SELECT id FROM roles WHERE tenant_id=$1 AND key='admin'`, tenant).Scan(&roleID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `INSERT INTO users (id, tenant_id, email, display_name, role_id) VALUES ($1,$2,$3,'Admin',$4)`,
		ids.New(), tenant, "admin-"+tenant.String()[:8]+"@example.test", roleID); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, env *testutil.Env, query string, args ...any) int {
	t.Helper()
	var n int
	if err := env.Owner.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSeedIsDeterministicIdempotentAndFullyMarked(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	svc := &demo.Service{DB: env.App}

	if err := svc.Seed(ctx, env.TenantA, 7, nil, "", "t"); err != nil {
		t.Fatal(err)
	}
	st, err := svc.Status(ctx, env.TenantA)
	if err != nil || !st.HasDemo || st.Seed == nil || *st.Seed != 7 {
		t.Fatalf("status %+v err %v", st, err)
	}
	for _, table := range []string{"organizations", "assets", "agents", "findings", "assessments", "reports"} {
		if st.Counts[table] == 0 {
			t.Errorf("tabel %s tidak punya baris demo", table)
		}
	}
	if st.Counts["organizations"] != 5 || st.Counts["assets"] != 50 || st.Counts["scenarios"] != 3 || st.Counts["alerts"] != 3 {
		t.Fatalf("diharapkan 5 organisasi/50 aset/3 skenario/3 alert, dapat %+v", st.Counts)
	}
	if st.Counts["endpoint_datasets"] != 40 {
		t.Fatalf("diharapkan 40 dataset osquery sintetis (8 query DefenseBot yang tampil dan didukung Linux x 5 aset Linux): %d", st.Counts["endpoint_datasets"])
	}
	// Data osquery demo hanya di aset Linux demo, bertanda is_demo, dan bukan hasil eksekusi nyata.
	if n := count(t, env, `SELECT count(*) FROM endpoint_datasets WHERE tenant_id=$1 AND (NOT is_demo OR execution_id IS NOT NULL)`, env.TenantA); n != 0 {
		t.Fatalf("%d dataset demo tidak bertanda atau terkait eksekusi", n)
	}
	// Laporan demo membawa postur endpoint sintetis (berlabel) dan ringkasannya.
	if n := count(t, env, `SELECT count(*) FROM reports WHERE tenant_id=$1 AND is_demo AND snapshot->'endpoint_summary'->>'assets_with_data'='1'`, env.TenantA); n != 15 {
		t.Fatalf("15 laporan demo harus memuat ringkasan endpoint: %d", n)
	}
	if n := count(t, env, `SELECT count(*) FROM reports r, jsonb_array_elements(r.snapshot->'assets') a WHERE r.tenant_id=$1 AND r.is_demo AND a->'endpoint'->>'source'='osquery (demo, synthetic)'`, env.TenantA); n != 15 {
		t.Fatalf("setiap laporan demo harus menandai postur sebagai sintetis: %d", n)
	}
	// Langkah skenario memakai kontrak sungguhan dan target hanya aset demo.
	if n := count(t, env, `SELECT count(*) FROM scenario_steps s JOIN contracts c ON c.tenant_id=s.tenant_id AND c.id=s.contract_id WHERE s.tenant_id=$1 AND c.key IN ('nmap.tcp_connect','nuclei.scan','agent.system_info','agent.listening_ports')`, env.TenantA); n != 7 {
		t.Fatalf("diharapkan 7 langkah skenario dengan kontrak nyata, dapat %d", n)
	}
	if n := count(t, env, `SELECT count(*) FROM scenario_targets t WHERE t.tenant_id=$1 AND t.asset_id NOT IN (SELECT id FROM assets WHERE is_demo)`, env.TenantA); n != 0 {
		t.Fatalf("%d target skenario demo menunjuk aset nyata", n)
	}
	// Seed yang sama tidak menduplikasi.
	if err := svc.Seed(ctx, env.TenantA, 7, nil, "", "t"); err != nil {
		t.Fatal(err)
	}
	again, _ := svc.Status(ctx, env.TenantA)
	for k, v := range st.Counts {
		if k != "audit_log" && again.Counts[k] != v {
			t.Errorf("seed ulang mengubah %s: %d -> %d", k, v, again.Counts[k])
		}
	}
	// Seed berbeda tanpa purge ditolak dan tidak mengubah apa pun.
	if err := svc.Seed(ctx, env.TenantA, 8, nil, "", "t"); err == nil || !strings.Contains(err.Error(), "demo-purge") {
		t.Fatalf("seed berbeda tanpa purge harus ditolak dengan petunjuk purge, err=%v", err)
	}
	// Tidak ada kebocoran ke tenant lain.
	other, _ := svc.Status(ctx, env.TenantB)
	if other.HasDemo {
		t.Fatalf("tenant B tidak boleh punya data demo: %+v", other)
	}
	// Semua alamat memakai rentang dokumentasi/invalid, tidak pernah target nyata.
	bad := count(t, env, `SELECT count(*) FROM asset_addresses WHERE tenant_id=$1 AND NOT (value = '127.0.0.1' OR value LIKE '%.example.invalid')`, env.TenantA)
	if bad != 0 {
		t.Fatalf("%d alamat demo bukan rentang dokumentasi", bad)
	}
	// Setiap snapshot laporan demo diberi label sintetis.
	unlabeled := count(t, env, `SELECT count(*) FROM reports WHERE tenant_id=$1 AND is_demo AND (snapshot->>'is_demo')<>'true'`, env.TenantA)
	if unlabeled != 0 {
		t.Fatalf("%d laporan demo tanpa label", unlabeled)
	}
}

func TestSeedDeterministicIDsAcrossRuns(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	svc := &demo.Service{DB: env.App, Blob: emptyArtifacts{}, Search: emptyArtifacts{}}
	if err := svc.Seed(ctx, env.TenantA, 3, nil, "", "t"); err != nil {
		t.Fatal(err)
	}
	first := count(t, env, `SELECT count(*) FROM assets WHERE tenant_id=$1 AND is_demo`, env.TenantA)
	if _, err := svc.Purge(ctx, env.TenantA, nil, "", "t"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Seed(ctx, env.TenantA, 3, nil, "", "t"); err != nil {
		t.Fatal(err)
	}
	if got := count(t, env, `SELECT count(*) FROM assets WHERE tenant_id=$1 AND is_demo`, env.TenantA); got != first {
		t.Fatalf("seed sama setelah purge harus identik: %d vs %d", got, first)
	}
}

func TestPurgeRemovesOnlyDemoAndKeepsRealData(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	svc := &demo.Service{DB: env.App}

	var realOrg uuid.UUID = ids.New()
	if _, err := env.Owner.Exec(ctx, `INSERT INTO organizations (id, tenant_id, name, slug) VALUES ($1,$2,'Real Org','real-org')`, realOrg, env.TenantA); err != nil {
		t.Fatal(err)
	}
	realAsset := ids.New()
	if _, err := env.Owner.Exec(ctx, `INSERT INTO assets (id, tenant_id, organization_id, category, name, source) VALUES ($1,$2,$3,'generic','Real Asset','manual')`, realAsset, env.TenantA, realOrg); err != nil {
		t.Fatal(err)
	}
	realScenario := ids.New()
	if _, err := env.Owner.Exec(ctx, `INSERT INTO scenarios (id, tenant_id, name) VALUES ($1,$2,'Real Scenario')`, realScenario, env.TenantA); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `INSERT INTO alerts (id, tenant_id, kind, title) VALUES ($1,$2,'real','Real alert')`, ids.New(), env.TenantA); err != nil {
		t.Fatal(err)
	}
	if err := svc.Seed(ctx, env.TenantA, 1, nil, "", "t"); err != nil {
		t.Fatal(err)
	}
	st, err := svc.Purge(ctx, env.TenantA, nil, "", "t")
	if err != nil {
		t.Fatal(err)
	}
	if st.HasDemo {
		t.Fatalf("setelah purge masih ada data demo: %+v", st)
	}
	if n := count(t, env, `SELECT count(*) FROM organizations WHERE id=$1`, realOrg); n != 1 {
		t.Fatal("purge menghapus data nyata")
	}
	if n := count(t, env, `SELECT count(*) FROM scenarios WHERE id=$1`, realScenario); n != 1 {
		t.Fatal("purge menghapus skenario nyata")
	}
	if n := count(t, env, `SELECT count(*) FROM alerts WHERE tenant_id=$1 AND NOT is_demo`, env.TenantA); n != 1 {
		t.Fatal("purge menghapus alert nyata")
	}
	if n := count(t, env, `SELECT count(*) FROM assets WHERE id=$1`, realAsset); n != 1 {
		t.Fatal("purge menghapus aset nyata")
	}
	for _, table := range []string{"assets", "agents", "findings", "assessments", "reports", "tags"} {
		if n := count(t, env, `SELECT count(*) FROM `+table+` WHERE tenant_id=$1 AND is_demo`, env.TenantA); n != 0 {
			t.Errorf("%s: %d baris demo tersisa", table, n)
		}
	}
	// Idempotent.
	if _, err := svc.Purge(ctx, env.TenantA, nil, "", "t"); err != nil {
		t.Fatal(err)
	}
	// Catatan audit permanen purge ada dan bukan is_demo.
	if n := count(t, env, `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='demo.purge' AND NOT is_demo`, env.TenantA); n < 1 {
		t.Fatal("jejak audit demo.purge tidak ada")
	}
}

func TestPurgeFailsClosedWhenRealDataDependsOnDemo(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	svc := &demo.Service{DB: env.App}
	if err := svc.Seed(ctx, env.TenantA, 1, nil, "", "t"); err != nil {
		t.Fatal(err)
	}
	var assetID uuid.UUID
	if err := env.Owner.QueryRow(ctx, `SELECT id FROM assets WHERE tenant_id=$1 AND is_demo LIMIT 1`, env.TenantA).Scan(&assetID); err != nil {
		t.Fatal(err)
	}
	// Temuan NYATA pada aset demo: purge akan ikut menghancurkannya lewat cascade, jadi harus ditolak.
	if _, err := env.Owner.Exec(ctx, `INSERT INTO findings (id, tenant_id, asset_id, type, value, value_hash, title, severity, details, first_seen_at, last_seen_at, occurrence_count)
VALUES ($1,$2,$3,'text','real','\x01','Real finding','low','{}',now(),now(),1)`, ids.New(), env.TenantA, assetID); err != nil {
		t.Fatal(err)
	}
	before := count(t, env, `SELECT count(*) FROM assets WHERE tenant_id=$1`, env.TenantA)
	_, err := svc.Purge(ctx, env.TenantA, nil, "", "t")
	if err == nil || !strings.Contains(err.Error(), "Cannot purge") && !strings.Contains(strings.ToLower(err.Error()), "purge") {
		t.Fatalf("purge harus ditolak, err=%v", err)
	}
	if after := count(t, env, `SELECT count(*) FROM assets WHERE tenant_id=$1`, env.TenantA); after != before {
		t.Fatalf("purge sebagian terjadi: %d -> %d", before, after)
	}
}

func TestSeedRequiresExistingUser(t *testing.T) {
	env := testutil.New(t)
	if err := (&demo.Service{DB: env.App}).Seed(context.Background(), env.TenantA, 1, nil, "", "t"); err == nil {
		t.Fatal("seed tanpa pengguna harus ditolak")
	}
}

func TestSeedRejectsOutOfRange(t *testing.T) {
	env := testutil.New(t)
	svc := &demo.Service{DB: env.App}
	for _, s := range []int{-1, 1_000_001} {
		if err := svc.Seed(context.Background(), env.TenantA, s, nil, "", "t"); err == nil {
			t.Fatalf("seed %d harus ditolak", s)
		}
	}
}

func TestPurgeFailsClosedWhenRealSimulationUsesDemoScenario(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	svc := &demo.Service{DB: env.App}
	if err := svc.Seed(ctx, env.TenantA, 1, nil, "", "t"); err != nil {
		t.Fatal(err)
	}
	var sc uuid.UUID
	if err := env.Owner.QueryRow(ctx, `SELECT id FROM scenarios WHERE tenant_id=$1 AND is_demo LIMIT 1`, env.TenantA).Scan(&sc); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `INSERT INTO simulations (id, tenant_id, scenario_id, name) VALUES ($1,$2,$3,'Real run')`, ids.New(), env.TenantA, sc); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Purge(ctx, env.TenantA, nil, "", "t"); err == nil {
		t.Fatal("purge harus ditolak saat simulasi nyata memakai skenario demo")
	}
	if n := count(t, env, `SELECT count(*) FROM scenarios WHERE tenant_id=$1 AND is_demo`, env.TenantA); n != 3 {
		t.Fatalf("purge sebagian: %d skenario demo tersisa", n)
	}
}

func TestPurgeFailsClosedWhenRealEndpointDataSitsOnDemoAsset(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	svc := &demo.Service{DB: env.App}
	if err := svc.Seed(ctx, env.TenantA, 1, nil, "", "t"); err != nil {
		t.Fatal(err)
	}
	var asset uuid.UUID
	env.Owner.QueryRow(ctx, `SELECT id FROM assets WHERE tenant_id=$1 AND is_demo AND platform='macos' LIMIT 1`, env.TenantA).Scan(&asset)
	if _, err := env.Owner.Exec(ctx, `INSERT INTO endpoint_datasets (tenant_id, asset_id, query_key, last_status, collected_at) VALUES ($1,$2,'osquery.inventory','ok',now())`, env.TenantA, asset); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Purge(ctx, env.TenantA, nil, "", "t"); err == nil {
		t.Fatal("purge harus ditolak saat ada data endpoint nyata pada aset demo")
	}
	if n := count(t, env, `SELECT count(*) FROM endpoint_datasets WHERE tenant_id=$1`, env.TenantA); n != 41 {
		t.Fatalf("purge sebagian: %d dataset", n)
	}
}

// This metadata-only fixture has no external objects/documents; make that
// explicit when testing a complete purge followed by deterministic reseeding.
type emptyArtifacts struct{}

func (emptyArtifacts) Remove(context.Context, string) error                         { return nil }
func (emptyArtifacts) RemoveFindings(context.Context, uuid.UUID, []uuid.UUID) error { return nil }
