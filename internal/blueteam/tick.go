package blueteam

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/db"
)

// Service menjalankan analisis blueteam berkala per tenant.
type Service struct {
	DB  *db.DB
	Now func() time.Time
}

// diffKeys adalah dataset yang di-baseline untuk deteksi perubahan.
var diffKeys = []string{"osquery.network", "osquery.packages", "osquery.macos_apps", "osquery.programs", "osquery.users", "osquery.user_groups", "osquery.groups"}

// RunOnce menganalisis semua aset yang punya agent aktif pada satu tenant.
func (s *Service) RunOnce(ctx context.Context, tenant uuid.UUID) error {
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	return s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT DISTINCT a.id, a.name, a.organization_id FROM assets a JOIN agents g ON g.tenant_id=a.tenant_id AND g.asset_id=a.id WHERE a.tenant_id=$1 AND a.deleted_at IS NULL AND g.revoked_at IS NULL`, tenant)
		if err != nil {
			return err
		}
		type assetRow struct {
			id   uuid.UUID
			name string
			org  *uuid.UUID
		}
		var assets []assetRow
		for rows.Next() {
			var a assetRow
			var org uuid.UUID
			if err := rows.Scan(&a.id, &a.name, &org); err != nil {
				rows.Close()
				return err
			}
			a.org = &org
			assets = append(assets, a)
		}
		rows.Close()
		for _, a := range assets {
			if err := s.analyzeAsset(ctx, tx, tenant, a.id, a.name, a.org, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) analyzeAsset(ctx context.Context, tx pgx.Tx, tenant, asset uuid.UUID, name string, org *uuid.UUID, now time.Time) error {
	datasets := map[string][]map[string]string{}
	drows, err := tx.Query(ctx, `SELECT query_key, rows, collected_at FROM endpoint_datasets WHERE tenant_id=$1 AND asset_id=$2 AND last_status='ok'`, tenant, asset)
	if err != nil {
		return err
	}
	var networkCollectedAt *time.Time
	for drows.Next() {
		var key string
		var raw []byte
		var collected *time.Time
		if err := drows.Scan(&key, &raw, &collected); err != nil {
			drows.Close()
			return err
		}
		var rows []map[string]string
		if err := json.Unmarshal(raw, &rows); err == nil {
			datasets[key] = rows
			if key == "osquery.network" {
				networkCollectedAt = collected
			}
		}
	}
	drows.Close()

	baselines := map[string][]map[string]string{}
	brows, err := tx.Query(ctx, `SELECT query_key, rows FROM endpoint_baselines WHERE tenant_id=$1 AND asset_id=$2`, tenant, asset)
	if err != nil {
		return err
	}
	for brows.Next() {
		var key string
		var raw []byte
		if err := brows.Scan(&key, &raw); err != nil {
			brows.Close()
			return err
		}
		var rows []map[string]string
		if err := json.Unmarshal(raw, &rows); err == nil && rows != nil { // 'null' = baseline kosong lama: dianggap belum ada
			baselines[key] = rows
		}
	}
	brows.Close()

	var agents []AgentRow
	arows, err := tx.Query(ctx, `SELECT id, hostname, status, uninstall_status, revoked_at, last_heartbeat_at FROM agents WHERE tenant_id=$1 AND asset_id=$2`, tenant, asset)
	if err != nil {
		return err
	}
	for arows.Next() {
		var a AgentRow
		if err := arows.Scan(&a.ID, &a.Hostname, &a.Status, &a.UninstallStatus, &a.RevokedAt, &a.LastHeartbeat); err != nil {
			arows.Close()
			return err
		}
		agents = append(agents, a)
	}
	arows.Close()

	// koneksi keluar terbaru (dari finding net_connection dalam 30 menit terakhir)
	var conns []Connection
	crows, err := tx.Query(ctx, `SELECT value, details, occurrence_count, last_seen_at FROM findings WHERE tenant_id=$1 AND asset_id=$2 AND type='net_connection' AND last_seen_at > now() - interval '30 minutes'`, tenant, asset)
	if err != nil {
		return err
	}
	for crows.Next() {
		var val string
		var det []byte
		var cnt int
		var seen time.Time
		if err := crows.Scan(&val, &det, &cnt, &seen); err != nil {
			crows.Close()
			return err
		}
		var d struct {
			Process string `json:"process"`
			Path    string `json:"path"`
			Proto   string `json:"proto"`
			DstPort int    `json:"dst_port"`
			State   string `json:"state"`
		}
		_ = json.Unmarshal(det, &d)
		c := Connection{Process: d.Process, Path: d.Path, Proto: d.Proto, DstIP: val, DstPort: d.DstPort, State: d.State, Count: cnt}
		conns = append(conns, c)
	}
	crows.Close()

	var offlineMin int
	if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT offline_minutes FROM notification_policies WHERE tenant_id=$1), 15)`, tenant).Scan(&offlineMin); err != nil {
		offlineMin = 15
	}

	in := Input{Tenant: tenant, Asset: asset, AssetName: name, OrgID: org, Now: now, OfflineMinutes: offlineMin, Agents: agents, Datasets: datasets, Connections: conns, Baselines: baselines}
	findings := Run(in)
	if err := Write(ctx, tx, in, findings); err != nil {
		return err
	}
	if err := MarkStale(ctx, tx, in, networkCollectedAt); err != nil {
		return err
	}
	return UpdateBaselines(ctx, tx, tenant, asset, datasets, diffKeys)
}

// RunTicker menjalankan RunOnce berkala.
func (s *Service) RunTicker(ctx context.Context, tenant uuid.UUID, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.RunOnce(ctx, tenant); err != nil {
				slog.Error("analisis blueteam gagal", "err", err)
			}
		}
	}
}
