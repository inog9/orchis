// Package demo menyediakan data dummy (seed), penghapusan bersih (purge), dan status data demo.
//
// Setiap baris yang dibuat seeder ditandai is_demo=true pada tabelnya sendiri (organizations,
// assets, agents, findings, assessments, reports, tags, audit_log) sehingga dapat dihitung dan
// dihapus bersih tanpa menyentuh data nyata. Seed deterministik berdasar DEMO_SEED=n: seed yang
// sama tidak menduplikasi (ID turunan dari tenant+seed+jalur), seed berbeda tanpa purge dulu
// ditolak (tidak pernah menimpa diam-diam). Purge transaksional, urut sesuai ketergantungan FK,
// dan gagal tertutup (fail-closed) bila ada data NYATA yang masih bergantung pada data demo
// lewat ON DELETE CASCADE (agent/finding nyata pada aset demo, atau bukti eksekusi nyata pada
// finding demo) — purge dibatalkan seluruhnya, tidak ada penghapusan sebagian.
package demo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/audit"
	"orchis-go/internal/db"
	"orchis-go/internal/execution"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
)

// BlobCleaner menghapus objek MinIO milik data demo (opsional; disuntikkan oleh parent/perakit
// server bila penyimpanan objek dikonfigurasi). Nil berarti tidak ada yang dibersihkan di sisi
// penyimpanan objek, dan Status/Purge TIDAK mengklaim pembersihan itu selesai.
type BlobCleaner interface {
	Remove(ctx context.Context, key string) error
}

// SearchCleaner menghapus dokumen Elasticsearch milik data demo (opsional, sama seperti BlobCleaner).
type SearchCleaner interface {
	RemoveFindings(ctx context.Context, tenant uuid.UUID, ids []uuid.UUID) error
}

type Service struct {
	DB     *db.DB
	Blob   BlobCleaner
	Search SearchCleaner
}

// Status merangkum keberadaan data demo per tenant.
type Status struct {
	FormatVersion  int            `json:"format_version,omitempty"`
	HasDemo        bool           `json:"has_demo"`
	CleanupPending int            `json:"cleanup_pending"`
	Counts         map[string]int `json:"counts"`
	Seed           *int           `json:"seed,omitempty"`
}

// demoTables: tabel yang punya kolom is_demo sendiri (lihat migrations/00016_demo.sql).
var demoTables = []string{"organizations", "assets", "agents", "findings", "assessments", "reports", "tags", "scenarios", "alerts", "endpoint_datasets", "audit_log", "simulations", "executions", "ai_policies", "ai_templates", "ai_jobs", "roles", "users", "api_tokens", "password_reset_links", "people", "teams", "asset_groups", "action_presets", "atomic_tests", "integrations"}

func scanStatus(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (*Status, error) {
	out := &Status{Counts: map[string]int{}}
	for _, table := range demoTables {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE tenant_id=$1 AND is_demo`, tenant).Scan(&n); err != nil {
			return nil, err
		}
		out.Counts[table] = n
		if n > 0 {
			out.HasDemo = true
		}
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM demo_cleanup_tasks WHERE tenant_id=$1`, tenant).Scan(&out.CleanupPending); err != nil {
		return nil, err
	}
	var seed int
	err := tx.QueryRow(ctx, `SELECT seed,format_version FROM demo_seed_state WHERE tenant_id=$1`, tenant).Scan(&seed, &out.FormatVersion)
	if err == nil {
		out.Seed = &seed
		out.HasDemo = true
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return out, nil
}

// Status melaporkan apakah tenant punya data demo, hitungan per tabel, dan seed yang dipakai (bila ada).
func (s *Service) Status(ctx context.Context, tenant uuid.UUID) (*Status, error) {
	var out *Status
	err := s.DB.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
		st, err := scanStatus(ctx, tx, tenant)
		out = st
		return err
	})
	return out, err
}

func writeDemoAudit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, actor *uuid.UUID, action string, detail map[string]any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_log (id, tenant_id, actor_user_id, action, resource_type, detail, is_demo)
VALUES ($1,$2,$3,$4,'demo',$5,true)`, ids.New(), tenant, actor, action, raw)
	return err
}

// lockTenant serialisasi seed/purge berurutan pada tenant yang sama; terlepas otomatis saat tx berakhir.
func lockTenant(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "demo/"+tenant.String())
	return err
}

// Seed membuat data dummy deterministik berdasar seed. Idempotent untuk seed yang sama (ID
// turunan dari tenant+seed+jalur, INSERT ... ON CONFLICT DO NOTHING); seed berbeda dari yang
// sudah ada ditolak (harus purge dulu) agar tidak pernah menimpa diam-diam.
func (s *Service) Seed(ctx context.Context, tenant uuid.UUID, seed int, actor *uuid.UUID, ip, reqID string) error {
	if seed < 0 || seed > 1_000_000 {
		return httpx.BadRequest("DEMO_SEED must be an integer between 0 and 1000000.")
	}
	return s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := lockTenant(ctx, tx, tenant); err != nil {
			return err
		}
		var pending bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM demo_cleanup_tasks WHERE tenant_id=$1)`, tenant).Scan(&pending); err != nil {
			return err
		}
		if pending {
			return httpx.Conflict("Previous demo artifact cleanup is pending. Retry purge after storage/search recover before reseeding.")
		}
		var existing, format int
		err := tx.QueryRow(ctx, `SELECT seed,format_version FROM demo_seed_state WHERE tenant_id=$1 FOR UPDATE`, tenant).Scan(&existing, &format)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if _, err := tx.Exec(ctx, `INSERT INTO demo_seed_state (tenant_id, seed,format_version) VALUES ($1,$2,3)`, tenant, seed); err != nil {
				return err
			}
		case err != nil:
			return err
		case format != 3:
			return httpx.Conflict("Demo fixture format changed. Run demo-purge before reseeding; existing snapshots are not rewritten.")
		case existing != seed:
			return httpx.Conflict(fmt.Sprintf("Demo data already seeded with seed=%d. Run demo-purge before seeding with a different seed.", existing))
		default:
			if _, err := tx.Exec(ctx, `UPDATE demo_seed_state SET updated_at=now() WHERE tenant_id=$1`, tenant); err != nil {
				return err
			}
		}
		var actorID uuid.UUID
		err = tx.QueryRow(ctx, `SELECT u.id FROM users u JOIN roles r ON r.id=u.role_id AND r.tenant_id=u.tenant_id
WHERE u.tenant_id=$1 AND u.deleted_at IS NULL AND NOT u.is_demo ORDER BY (r.key='admin') DESC, u.created_at LIMIT 1`, tenant).Scan(&actorID)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.Conflict("Demo seed requires at least one existing user in the tenant; create an admin first.")
		}
		if err != nil {
			return err
		}
		// Katalog kontrak bawaan (idempotent, sama dengan yang disemai server saat start) dibutuhkan oleh langkah skenario.
		if err := execution.SeedCatalog(ctx, tx, tenant); err != nil {
			return err
		}
		g := &generator{tenant: tenant, seed: seed, now: time.Now().UTC(), createdBy: actorID}
		if err := g.run(ctx, tx); err != nil {
			return err
		}
		return writeDemoAudit(ctx, tx, tenant, actor, "demo.seed", map[string]any{"seed": seed})
	})
}

// guardChecks: pemeriksaan fail-closed sebelum purge. Setiap kueri menghitung baris NYATA (atau
// baris yang hanya bisa ada karena eksekusi nyata) yang masih bergantung pada data demo lewat
// ON DELETE CASCADE. Bila ditemukan satu saja, purge dibatalkan seluruhnya (transaksi rollback).
var guardChecks = []struct{ label, query string }{
	{"kebijakan AI nyata pada organisasi demo", `SELECT count(*) FROM ai_policies WHERE tenant_id=$1 AND NOT is_demo AND organization_id IN(SELECT id FROM organizations WHERE tenant_id=$1 AND is_demo)`},
	{"job AI nyata memakai organisasi/laporan/template demo", `SELECT count(*) FROM ai_jobs WHERE tenant_id=$1 AND NOT is_demo AND (organization_id IN(SELECT id FROM organizations WHERE tenant_id=$1 AND is_demo) OR report_id IN(SELECT id FROM reports WHERE tenant_id=$1 AND is_demo) OR template_id IN(SELECT id FROM ai_templates WHERE tenant_id=$1 AND is_demo))`},
	{"eksekusi nyata pada tes demo", `SELECT count(*) FROM executions WHERE tenant_id=$1 AND NOT is_demo AND atomic_test_id IN(SELECT id FROM atomic_tests WHERE tenant_id=$1 AND is_demo)`},
	{"finding nyata pada eksekusi demo", `SELECT count(*) FROM finding_occurrences o JOIN findings f ON f.tenant_id=o.tenant_id AND f.id=o.finding_id WHERE o.tenant_id=$1 AND NOT f.is_demo AND o.execution_id IN(SELECT id FROM executions WHERE tenant_id=$1 AND is_demo)`},
	{"dataset nyata pada eksekusi demo", `SELECT count(*) FROM endpoint_datasets WHERE tenant_id=$1 AND NOT is_demo AND execution_id IN(SELECT id FROM executions WHERE tenant_id=$1 AND is_demo)`},
	{"assessment nyata memakai simulasi demo", `SELECT count(*) FROM assessment_targets t JOIN assessments a ON a.tenant_id=t.tenant_id AND a.id=t.assessment_id WHERE t.tenant_id=$1 AND NOT a.is_demo AND t.simulation_id IN(SELECT id FROM simulations WHERE tenant_id=$1 AND is_demo)`},
	{"eksekusi nyata pada simulasi demo", `SELECT count(*) FROM executions WHERE tenant_id=$1 AND NOT is_demo AND simulation_id IN(SELECT id FROM simulations WHERE tenant_id=$1 AND is_demo)`},
	{"peran demo digunakan pengguna nyata", `SELECT count(*) FROM users WHERE tenant_id=$1 AND NOT is_demo AND role_id IN(SELECT id FROM roles WHERE tenant_id=$1 AND is_demo)`},
	{"anggota tim nyata pada orang demo", `SELECT count(*) FROM team_members m JOIN teams t ON t.tenant_id=m.tenant_id AND t.id=m.team_id WHERE m.tenant_id=$1 AND NOT t.is_demo AND m.person_id IN(SELECT id FROM people WHERE tenant_id=$1 AND is_demo)`},
	{"orang nyata pada tim demo", `SELECT count(*) FROM team_members m JOIN people p ON p.tenant_id=m.tenant_id AND p.id=m.person_id WHERE m.tenant_id=$1 AND NOT p.is_demo AND m.team_id IN(SELECT id FROM teams WHERE tenant_id=$1 AND is_demo)`},
	{"grup demo menampung aset nyata", `SELECT count(*) FROM asset_group_members m JOIN assets a ON a.tenant_id=m.tenant_id AND a.id=m.asset_id WHERE m.tenant_id=$1 AND NOT a.is_demo AND m.asset_group_id IN(SELECT id FROM asset_groups WHERE tenant_id=$1 AND is_demo)`},
	{"skenario nyata memakai grup demo", `SELECT count(*) FROM scenario_targets t JOIN scenarios s ON s.tenant_id=t.tenant_id AND s.id=t.scenario_id WHERE t.tenant_id=$1 AND NOT s.is_demo AND t.asset_group_id IN(SELECT id FROM asset_groups WHERE tenant_id=$1 AND is_demo)`},
	{"skenario nyata memakai tim demo", `SELECT count(*) FROM scenario_teams t JOIN scenarios s ON s.tenant_id=t.tenant_id AND s.id=t.scenario_id WHERE t.tenant_id=$1 AND NOT s.is_demo AND t.team_id IN(SELECT id FROM teams WHERE tenant_id=$1 AND is_demo)`},
	{"token nyata pengguna demo", `SELECT count(*) FROM api_tokens WHERE tenant_id=$1 AND NOT is_demo AND user_id IN(SELECT id FROM users WHERE tenant_id=$1 AND is_demo)`},
	{"reset nyata pengguna demo", `SELECT count(*) FROM password_reset_links WHERE tenant_id=$1 AND NOT is_demo AND user_id IN(SELECT id FROM users WHERE tenant_id=$1 AND is_demo)`},
	{"sesi pengguna demo", `SELECT count(*) FROM sessions WHERE tenant_id=$1 AND user_id IN(SELECT id FROM users WHERE tenant_id=$1 AND is_demo)`},
	{"aset nyata pada organisasi demo", `SELECT count(*) FROM assets WHERE tenant_id=$1 AND NOT is_demo AND organization_id IN(SELECT id FROM organizations WHERE tenant_id=$1 AND is_demo)`},
	{"agent nyata pada organisasi demo", `SELECT count(*) FROM agents WHERE tenant_id=$1 AND NOT is_demo AND organization_id IN(SELECT id FROM organizations WHERE tenant_id=$1 AND is_demo)`},
	{"assessment nyata pada organisasi demo", `SELECT count(*) FROM assessments WHERE tenant_id=$1 AND NOT is_demo AND organization_id IN(SELECT id FROM organizations WHERE tenant_id=$1 AND is_demo)`},
	{"laporan nyata pada organisasi demo", `SELECT count(*) FROM reports WHERE tenant_id=$1 AND NOT is_demo AND organization_id IN(SELECT id FROM organizations WHERE tenant_id=$1 AND is_demo)`},
	{"tag demo pada aset nyata", `SELECT count(*) FROM asset_tags t JOIN assets a ON a.tenant_id=t.tenant_id AND a.id=t.asset_id WHERE t.tenant_id=$1 AND NOT a.is_demo AND t.tag_id IN(SELECT id FROM tags WHERE tenant_id=$1 AND is_demo)`},
	{"tag demo pada skenario nyata", `SELECT count(*) FROM scenario_tags t JOIN scenarios s ON s.tenant_id=t.tenant_id AND s.id=t.scenario_id WHERE t.tenant_id=$1 AND NOT s.is_demo AND t.tag_id IN(SELECT id FROM tags WHERE tenant_id=$1 AND is_demo)`},
	{"tes tersimpan memakai aset demo", `SELECT count(*) FROM atomic_tests WHERE tenant_id=$1 AND NOT is_demo AND target_asset_id IN(SELECT id FROM assets WHERE tenant_id=$1 AND is_demo)`},

	{"agent nyata pada aset demo", `SELECT count(*) FROM agents a WHERE a.tenant_id=$1 AND NOT a.is_demo AND a.asset_id IN (SELECT id FROM assets WHERE tenant_id=$1 AND is_demo)`},
	{"finding nyata pada aset demo", `SELECT count(*) FROM findings f WHERE f.tenant_id=$1 AND NOT f.is_demo AND f.asset_id IN (SELECT id FROM assets WHERE tenant_id=$1 AND is_demo)`},
	{"bukti eksekusi (finding_occurrences) pada finding demo", `SELECT count(*) FROM finding_occurrences o JOIN executions e ON e.tenant_id=o.tenant_id AND e.id=o.execution_id WHERE o.tenant_id=$1 AND NOT e.is_demo AND o.finding_id IN (SELECT id FROM findings WHERE tenant_id=$1 AND is_demo)`},
	{"follow-up eksekusi (finding_triggers) pada finding demo", `SELECT count(*) FROM finding_triggers t JOIN executions e ON e.tenant_id=t.tenant_id AND e.id=t.execution_id WHERE t.tenant_id=$1 AND NOT e.is_demo AND t.finding_id IN (SELECT id FROM findings WHERE tenant_id=$1 AND is_demo)`},
	{"target skenario nyata pada aset demo", `SELECT count(*) FROM scenario_targets t WHERE t.tenant_id=$1 AND t.asset_id IN (SELECT id FROM assets WHERE tenant_id=$1 AND is_demo) AND t.scenario_id NOT IN (SELECT id FROM scenarios WHERE tenant_id=$1 AND is_demo)`},
	{"simulasi nyata memakai skenario demo", `SELECT count(*) FROM simulations s WHERE s.tenant_id=$1 AND NOT s.is_demo AND s.scenario_id IN (SELECT id FROM scenarios WHERE tenant_id=$1 AND is_demo)`},
	{"data endpoint nyata pada aset demo", `SELECT count(*) FROM endpoint_datasets d WHERE d.tenant_id=$1 AND NOT d.is_demo AND d.asset_id IN (SELECT id FROM assets WHERE tenant_id=$1 AND is_demo)`},
	{"assessment nyata menyasar aset demo", `SELECT count(*) FROM assessment_targets t WHERE t.tenant_id=$1 AND t.asset_id IN (SELECT id FROM assets WHERE tenant_id=$1 AND is_demo) AND t.assessment_id NOT IN (SELECT id FROM assessments WHERE tenant_id=$1 AND is_demo)`},
	{"eksekusi nyata menyasar aset demo", `SELECT count(*) FROM executions e WHERE e.tenant_id=$1 AND NOT e.is_demo AND e.target_asset_id IN (SELECT id FROM assets WHERE tenant_id=$1 AND is_demo)`},
}

func guardRealReferences(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	for _, c := range guardChecks {
		var n int
		if err := tx.QueryRow(ctx, c.query, tenant).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return httpx.Conflict(fmt.Sprintf("Cannot purge demo data: %d row(s) of real data (%s) would be destroyed. Detach or remove them first.", n, c.label))
		}
	}
	return nil
}

// purgeOrder: urutan DELETE yang menghormati kunci asing (anak sebelum induk).
var purgeOrder = []string{
	`DELETE FROM ai_job_actions WHERE tenant_id=$1 AND job_id IN(SELECT id FROM ai_jobs WHERE tenant_id=$1 AND is_demo)`,
	`DELETE FROM ai_jobs WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM ai_templates WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM ai_policies WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM simulation_decisions WHERE tenant_id=$1 AND simulation_id IN(SELECT id FROM simulations WHERE tenant_id=$1 AND is_demo)`,
	`DELETE FROM executions WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM password_reset_links WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM api_tokens WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM atomic_tests WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM action_presets WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM reports WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM assessment_targets WHERE tenant_id=$1 AND assessment_id IN (SELECT id FROM assessments WHERE tenant_id=$1 AND is_demo)`,
	`DELETE FROM assessments WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM simulations WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM findings WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM agents WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM scenarios WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM alerts WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM assets WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM tags WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM organizations WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM asset_groups WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM teams WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM people WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM integrations WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM users WHERE tenant_id=$1 AND is_demo`,
	`DELETE FROM roles WHERE tenant_id=$1 AND is_demo`,
}

// Purge menghapus HANYA data demo (is_demo=true) dalam satu transaksi, dalam urutan yang
// menghormati kunci asing. Idempotent: bila tidak ada data demo, ini adalah no-op yang aman.
// Fail-closed: bila data nyata masih bergantung pada data demo lewat cascade, seluruh transaksi
// dibatalkan tanpa menghapus apa pun.
func (s *Service) Purge(ctx context.Context, tenant uuid.UUID, actor *uuid.UUID, ip, reqID string) (*Status, error) {
	err := s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := lockTenant(ctx, tx, tenant); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM demo_seed_state WHERE tenant_id=$1 FOR UPDATE)`, tenant).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return nil
		}
		// FK inserts take key-share locks. Hold parent update locks before
		// checking real references so a new real child cannot slip into cascade.
		for _, table := range []string{"organizations", "assets", "agents", "assessments", "reports", "findings", "scenarios", "tags", "alerts", "simulations", "executions", "ai_templates", "ai_jobs", "roles", "users", "api_tokens", "password_reset_links", "people", "teams", "asset_groups", "action_presets", "atomic_tests", "integrations"} {
			if _, err := tx.Exec(ctx, `SELECT id FROM `+table+` WHERE tenant_id=$1 AND is_demo ORDER BY id FOR UPDATE`, tenant); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `SELECT organization_id FROM ai_policies WHERE tenant_id=$1 AND is_demo ORDER BY organization_id FOR UPDATE`, tenant); err != nil {
			return err
		}
		if err := queueCleanup(ctx, tx, tenant); err != nil {
			return err
		}
		if err := guardRealReferences(ctx, tx, tenant); err != nil {
			return err
		}
		for _, q := range purgeOrder {
			if _, err := tx.Exec(ctx, q, tenant); err != nil {
				return err
			}
		}
		detail := map[string]any{"artifact_cleanup": "durable_after_commit"}
		if _, err := tx.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id=$1 AND is_demo`, tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM demo_seed_state WHERE tenant_id=$1`, tenant); err != nil {
			return err
		}
		// Entri audit permanen (bukan is_demo) mencatat siapa yang membersihkan data demo dan kapan.
		if err := audit.Write(ctx, tx, audit.Entry{Tenant: tenant, ActorUser: actor, Action: "demo.purge", ResourceType: "demo", IP: ip, RequestID: reqID, Detail: detail}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Fast local deletions normally finish in this request. Slow/unavailable
	// backends leave explicit pending state for the background worker/retry.
	clean, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	for i := 0; i < 8; i++ {
		if err := s.CleanupOnce(clean, tenant); err != nil {
			break
		}
		st, e := s.Status(clean, tenant)
		if e != nil {
			break
		}
		_ = st
		if st.CleanupPending == 0 || s.Blob == nil || s.Search == nil {
			break
		}
	}
	return s.Status(ctx, tenant)
}
