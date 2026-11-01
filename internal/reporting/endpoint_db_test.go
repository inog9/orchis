package reporting

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/ids"
	"orchis-go/internal/osquery"
	"orchis-go/internal/testutil"
)

func TestEndpointPostureUsesOnlyCurrentlyApprovedDataAndComparesPorts(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	org, asset := ids.New(), ids.New()
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := env.Owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO organizations (id, tenant_id, name, slug) VALUES ($1,$2,'O','o')`, org, env.TenantA)
	must(`INSERT INTO assets (id, tenant_id, organization_id, category, name, source) VALUES ($1,$2,$3,'host','h','manual')`, asset, env.TenantA, org)
	actor := ids.New()
	must(`INSERT INTO users(id,tenant_id,email,display_name,role_id) SELECT $1,$2,'approval@a.test','Approval fixture',id FROM roles WHERE tenant_id=$2 AND key='admin'`, actor, env.TenantA)
	put := func(key, rows string, collected bool, status string) {
		var at any
		if collected {
			at = time.Now().UTC()
		}
		must(`INSERT INTO endpoint_datasets (tenant_id, asset_id, query_key, rows, attributes, collected_at, osquery_version, last_status) VALUES ($1,$2,$3,$4::jsonb,'{}',$5,'5.24.0',$6)`, env.TenantA, asset, key, rows, at, status)
	}
	put("osquery.inventory", `[{"os_name":"Ubuntu","os_version":"26.04","kernel":"7.0","uptime_seconds":"5"}]`, true, "ok")
	put("osquery.network", `[{"port":"22","address":"0.0.0.0","protocol":"6","process":"sshd"},{"port":"5432","address":"::","protocol":"6","process":"pg"}]`, true, "ok")
	put("osquery.users", `[{"username":"toor","uid":"0","shell":"/bin/sh","groups":"root"}]`, true, "ok")
	put("osquery.firewall", `[]`, false, "failed") // tidak pernah berhasil: tidak boleh masuk

	a := Asset{ID: asset, Name: "h", Findings: []Finding{{Type: "open_port", Value: "tcp/22"}, {Type: "open_port", Value: "tcp/80"}, {Type: "vulnerability", Value: "x"}}}
	posture := func() *struct {
		attrs, ports string
		findings     int
	} {
		var out *struct {
			attrs, ports string
			findings     int
		}
		err := env.App.InTenant(ctx, env.TenantA, func(tx pgx.Tx) error {
			p, err := endpointPosture(ctx, tx, env.TenantA, a)
			if err != nil || p == nil {
				return err
			}
			out = &struct {
				attrs, ports string
				findings     int
			}{attrs: asString(p.Attributes["os_name"]), findings: len(p.Findings)}
			if p.Ports != nil {
				out.ports = intsString(p.Ports.Both) + "|" + intsString(p.Ports.ScanOnly) + "|" + intsString(p.Ports.HostOnly)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	// Bawaan: users dan network bersensitivitas sedang dan belum disetujui -> tidak ikut; hanya inventory (rendah).
	got := posture()
	if got == nil || got.attrs != "Ubuntu" || got.ports != "" || got.findings != 0 {
		t.Fatalf("bawaan: %+v", got)
	}
	must(`INSERT INTO osquery_org_settings (tenant_id, organization_id, query_key, enabled,consent_version,consent_sha256,consented_by,consented_at) VALUES ($1,$2,'osquery.network',true,$3,$4,$5,now())`, env.TenantA, org, osquery.ConsentVersion, osquery.Consent().SHA256, actor)
	if got = posture(); got == nil || got.ports != "22|80|5432" || got.findings != 2 {
		t.Fatalf("network disetujui: %+v", got)
	}
	// Setelah admin menyetujui users: temuan akun UID 0 masuk.
	must(`INSERT INTO osquery_org_settings (tenant_id, organization_id, query_key, enabled,consent_version,consent_sha256,consented_by,consented_at) VALUES ($1,$2,'osquery.users',true,$3,$4,$5,now())`, env.TenantA, org, osquery.ConsentVersion, osquery.Consent().SHA256, actor)
	if got = posture(); got == nil || got.findings != 3 {
		t.Fatalf("setelah disetujui: %+v", got)
	}
	// Dicabut: datanya keluar lagi dari laporan baru.
	must(`UPDATE osquery_org_settings SET enabled=false WHERE query_key='osquery.users'`)
	if got = posture(); got == nil || got.findings != 2 {
		t.Fatalf("setelah dicabut: %+v", got)
	}
	// Inventory dicabut (query bawaan aktif dimatikan admin): tidak ada lagi atribut OS.
	must(`INSERT INTO osquery_org_settings (tenant_id, organization_id, query_key, enabled) VALUES ($1,$2,'osquery.inventory',false)`, env.TenantA, org)
	if got = posture(); got == nil || got.attrs != "" {
		t.Fatalf("inventory dimatikan: %+v", got)
	}
	// Aset tanpa data sama sekali: nil.
	var none *struct{}
	_ = none
	other := Asset{ID: uuid.New(), Name: "x"}
	must(`INSERT INTO assets (id, tenant_id, organization_id, category, name, source) VALUES ($1,$2,$3,'host','h2','manual')`, other.ID, env.TenantA, org)
	if err := env.App.InTenant(ctx, env.TenantA, func(tx pgx.Tx) error {
		p, err := endpointPosture(ctx, tx, env.TenantA, other)
		if p != nil {
			t.Fatalf("aset tanpa data harus nil: %+v", p)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intsString(v []int) string {
	s := ""
	for i, n := range v {
		if i > 0 {
			s += ","
		}
		s += itoa(n)
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	d := ""
	for n > 0 {
		d = string(rune('0'+n%10)) + d
		n /= 10
	}
	return d
}
