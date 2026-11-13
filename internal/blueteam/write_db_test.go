package blueteam

import (
	"context"
	"testing"

	"orchis-go/internal/ids"
	"orchis-go/internal/testutil"
)

// Semua tipe temuan blue team harus bisa ditulis: findings.type punya FK ke finding_types.
func TestWriteAcceptsEveryBlueteamFindingType(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	org, asset := ids.New(), ids.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, tenant_id, name, slug) VALUES ($1,$2,'O','o')`, []any{org, env.TenantA}},
		{`INSERT INTO assets (id, tenant_id, organization_id, category, name, source) VALUES ($1,$2,$3,'host','h','manual')`, []any{asset, env.TenantA, org}},
	} {
		if _, err := env.Owner.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	types := []string{"agent_tamper", "agent_unresponsive", "new_listening_port", "new_software_installed", "new_local_account", "c2_suspect_connection", "net_connection"}
	var fs []Finding
	for _, ty := range types {
		fs = append(fs, Finding{Type: ty, Value: "v-" + ty, Title: ty, Severity: "high", Details: map[string]any{"x": 1}})
	}
	tx, err := env.Owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	in := Input{Tenant: env.TenantA, Asset: asset, AssetName: "h", OrgID: &org}
	if err := Write(ctx, tx, in, fs); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("blueteam finding type not registered: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := env.Owner.QueryRow(ctx, `SELECT count(*) FROM findings WHERE asset_id=$1`, asset).Scan(&n); err != nil || n != len(types) {
		t.Fatalf("stored %d of %d: %v", n, len(types), err)
	}
}

// Dataset yang belum pernah terkumpul tidak boleh disimpan sebagai baseline kosong.
func TestUpdateBaselinesSkipsDatasetsThatNeverArrived(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	org, asset := ids.New(), ids.New()
	for _, q := range []string{`INSERT INTO organizations (id, tenant_id, name, slug) VALUES ($1,$2,'O','o')`} {
		if _, err := env.Owner.Exec(ctx, q, org, env.TenantA); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := env.Owner.Exec(ctx, `INSERT INTO assets (id, tenant_id, organization_id, category, name, source) VALUES ($1,$2,$3,'host','h','manual')`, asset, env.TenantA, org); err != nil {
		t.Fatal(err)
	}
	tx, err := env.Owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string][]map[string]string{"osquery.packages": {{"name": "tar", "version": "1"}}}
	if err := UpdateBaselines(ctx, tx, env.TenantA, asset, have, []string{"osquery.packages", "osquery.network"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var keys []string
	rows, err := env.Owner.Query(ctx, `SELECT query_key FROM endpoint_baselines WHERE asset_id=$1`, asset)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	rows.Close()
	if len(keys) != 1 || keys[0] != "osquery.packages" {
		t.Fatalf("only datasets that exist get a baseline: %v", keys)
	}
}
