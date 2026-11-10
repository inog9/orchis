package endpoint

// Penanaman persetujuan otomatis. Kebijakan pemilik produk: memasang agent berarti menyetujui
// koleksi orchis-defense-agent untuk organisasinya, sehingga koleksi tidak pernah menjadi penghambat scan.
// Aturan main:
//   - Lingkup: query kategori orchis-defense-agent plus semua query pack runnable. Check kategori
//     Endpoint/network (osquery.inventory, osquery.users, osquery.network, osquery.network_ranges)
//     tidak pernah ditanam; migration 00037 memang mematikannya secara sengaja.
//   - Baris yang sudah ada TIDAK pernah ditimpa (ON CONFLICT DO NOTHING); keputusan admin, termasuk
//     "Nonaktifkan", selalu menang.
//   - Rotasi dokumen persetujuan: baris scope yang masih enabled tapi konsennya kadaluarsa
//     disegarkan ke dokumen berlaku oleh AutoConsentTick, sesuai kebijakan zero-touch.

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/audit"
	"orchis-go/internal/osquery"
)

// autoConsentKeys mengembalikan kunci lingkup persetujuan otomatis untuk satu OS agent
// (linux|macos|windows): query orchis-defense-agent tanpa pack ditambah semua query pack runnable.
// Windows tidak punya query yang mendukung sehingga hasilnya kosong.
func autoConsentKeys(os string) []string {
	var keys []string
	for i := range osquery.Queries {
		q := &osquery.Queries[i]
		if !q.SupportsOS(os) {
			continue
		}
		if (q.Category == "DefenseBot" && q.Pack == "") || q.Pack != "" {
			keys = append(keys, q.Key)
		}
	}
	return keys
}

// SeedAutoConsent menanam baris persetujuan yang belum ada untuk satu organisasi pada
// nama-nama OS agent (linux|macos|windows). Baris yang sudah ada tidak pernah diubah.
// Mengembalikan jumlah baris yang benar-benar baru dibuat.
func SeedAutoConsent(ctx context.Context, tx pgx.Tx, tenant, org, actor uuid.UUID, agentOS []string) (int, error) {
	uniq := map[string]struct{}{}
	for _, name := range agentOS {
		for _, k := range autoConsentKeys(name) {
			uniq[k] = struct{}{}
		}
	}
	if len(uniq) == 0 {
		return 0, nil
	}
	keys := make([]string, 0, len(uniq))
	for k := range uniq {
		keys = append(keys, k)
	}
	c := osquery.Consent()
	tag, err := tx.Exec(ctx, `INSERT INTO osquery_org_settings(tenant_id,organization_id,query_key,enabled,updated_by,consent_version,consent_sha256,consented_by,consented_at)
SELECT $1,$2,k,true,$5,$3,$4,$5,now() FROM unnest($6::text[]) AS k
ON CONFLICT (tenant_id,organization_id,query_key) DO NOTHING`, tenant, org, c.Version, c.SHA256, actor, keys)
	if err != nil {
		return 0, err
	}
	n := int(tag.RowsAffected())
	if n > 0 {
		if err := audit.Write(ctx, tx, audit.Entry{Tenant: tenant, ActorUser: &actor, Action: "endpoint.collection.autoconsent", ResourceType: "organization", ResourceID: &org,
			Detail: map[string]any{"inserted": n, "keys": keys, "consent_version": c.Version}}); err != nil {
			return n, err
		}
	}
	return n, nil
}

// AutoConsentTick menjalankan kebijakan persetujuan otomatis untuk seluruh organisasi non-demo
// yang punya agent: menanam baris scope yang belum ada dan menyegarkan baris scope yang masih
// enabled tetapi konsennya kadaluarsa. Baris enabled=false (keputusan admin) tidak pernah disentuh.
func (s *Service) AutoConsentTick(ctx context.Context, tenant uuid.UUID) error {
	return s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		var actor uuid.UUID
		err := tx.QueryRow(ctx, `SELECT u.id FROM users u JOIN roles r ON r.id=u.role_id AND r.tenant_id=u.tenant_id
WHERE u.tenant_id=$1 AND u.deleted_at IS NULL AND NOT u.is_demo ORDER BY (r.key='admin') DESC, u.created_at LIMIT 1`, tenant).Scan(&actor)
		if err == pgx.ErrNoRows {
			return nil // belum ada user; tidak ada yang bisa jadi penanda persetujuan
		}
		if err != nil {
			return err
		}
		type orgPlatform struct {
			org uuid.UUID
			os  string
		}
		rows, err := tx.Query(ctx, `SELECT DISTINCT organization_id, os FROM agents
WHERE tenant_id=$1 AND status<>'revoked' AND NOT is_demo AND organization_id IS NOT NULL`, tenant)
		if err != nil {
			return err
		}
		var pairs []orgPlatform
		for rows.Next() {
			var p orgPlatform
			if err := rows.Scan(&p.org, &p.os); err != nil {
				rows.Close()
				return err
			}
			pairs = append(pairs, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		byOrg := map[uuid.UUID]map[string]struct{}{}
		for _, p := range pairs {
			keys := autoConsentKeys(p.os)
			if len(keys) == 0 {
				continue // platform tanpa artefak osquery (mis. windows): bukan urusan koleksi
			}
			c := osquery.Consent()
			if _, err := tx.Exec(ctx, `INSERT INTO osquery_org_settings(tenant_id,organization_id,query_key,enabled,updated_by,consent_version,consent_sha256,consented_by,consented_at)
SELECT $1,$2,k,true,$5,$3,$4,$5,now() FROM unnest($6::text[]) AS k
ON CONFLICT (tenant_id,organization_id,query_key) DO NOTHING`, tenant, p.org, c.Version, c.SHA256, actor, keys); err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `UPDATE osquery_org_settings SET consent_version=$3, consent_sha256=$4, consented_by=$5, consented_at=now(), updated_by=$5, version=version+1
WHERE tenant_id=$1 AND organization_id=$2 AND enabled AND query_key=ANY($6)
AND (consent_version IS DISTINCT FROM $3 OR consent_sha256 IS DISTINCT FROM $4)`, tenant, p.org, c.Version, c.SHA256, actor, keys)
			if err != nil {
				return err
			}
			if tag.RowsAffected() > 0 {
				set := byOrg[p.org]
				if set == nil {
					set = map[string]struct{}{}
					byOrg[p.org] = set
				}
				for _, k := range keys {
					set[k] = struct{}{}
				}
			}
		}
		for org, set := range byOrg {
			keys := make([]string, 0, len(set))
			for k := range set {
				keys = append(keys, k)
			}
			o := org
			if err := audit.Write(ctx, tx, audit.Entry{Tenant: tenant, ActorUser: &actor, Action: "endpoint.collection.autoconsent", ResourceType: "organization", ResourceID: &o,
				Detail: map[string]any{"refreshed": true, "keys": keys}}); err != nil {
				return err
			}
		}
		return nil
	})
}

// maybeAutoConsent menjalankan AutoConsentTick paling sering sekali per menit dari loop koleksi.
func (s *Service) maybeAutoConsent(ctx context.Context, tenant uuid.UUID, last *time.Time) {
	if !s.AutoConsent || time.Since(*last) < time.Minute {
		return
	}
	if err := s.AutoConsentTick(ctx, tenant); err != nil {
		slog.Error("penanaman persetujuan otomatis gagal", "err", err)
	}
	*last = time.Now()
}
