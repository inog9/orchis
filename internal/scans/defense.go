package scans

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/contract"
	"orchis-go/internal/defensebot"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/scancontrol"
)

// DefenseCollector queues a fresh collection of the approved orchis-defense-agent checks for one asset inside the caller's transaction.
type DefenseCollector interface {
	QueueDefenseCollection(ctx context.Context, tx pgx.Tx, tenant, asset uuid.UUID, keys []string) (int, error)
}

// launchDefense starts a orchis-defense-agent run for enrolled endpoints. Unlike a web pentest it has no network target: it asks the
// agent for a fresh collection of the approved checks and then maps the stored data to known vulnerabilities.
func (s *Service) launchDefense(ctx context.Context, p *auth.Principal, in Input, language, key, ip, req string) (*Scan, error) {
	if in.OrganizationID == uuid.Nil {
		return nil, invalid("organization_id", "Choose an organisation.")
	}
	if len(key) < 1 || len(key) > 200 {
		return nil, invalid("idempotency_key", "Provide an Idempotency-Key of 1 to 200 characters.")
	}
	if len(in.RangeIDs) > 0 {
		return nil, invalid("range_ids", "orchis-defense-agent runs on enrolled endpoints, not on network ranges.")
	}
	in.Tools = []string{"defensebot"}
	in.Preset = "defense"
	in.Language = language
	if len(in.AssetIDs) == 0 || len(in.AssetIDs) > s.hostLimit() {
		return nil, invalid("targets", "Select enrolled endpoints.")
	}
	sort.Slice(in.AssetIDs, func(i, j int) bool { return in.AssetIDs[i].String() < in.AssetIDs[j].String() })
	for i, id := range in.AssetIDs {
		if id == uuid.Nil || i > 0 && in.AssetIDs[i-1] == id {
			return nil, invalid("targets", "Select each target once.")
		}
	}
	if s.Collector == nil {
		return nil, httpx.Conflict("orchis-defense-agent collection is not available.")
	}
	body, _ := json.Marshal(in)
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	id := ids.New()
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
		var name string
		var demo bool
		err := tx.QueryRow(ctx, `SELECT name,is_demo FROM organizations WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, in.OrganizationID).Scan(&name, &demo)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Organisation")
		}
		if err != nil {
			return err
		}
		if demo {
			return httpx.Conflict("Synthetic organisations cannot run scans.")
		}
		var oldHash string
		err = tx.QueryRow(ctx, `SELECT id,request_hash FROM assessments WHERE tenant_id=$1 AND organization_id=$2 AND idempotency_key=$3`, p.TenantID, in.OrganizationID, key).Scan(&id, &oldHash)
		if err == nil {
			if oldHash != hash {
				return httpx.Conflict("This idempotency key belongs to different scan options.")
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if s.BeforeLaunch != nil {
			if err := s.BeforeLaunch(ctx, tx, p, in, key); err != nil {
				return err
			}
		}
		if err := preventOverlap(ctx, tx, p.TenantID, in.AssetIDs); err != nil {
			return err
		}
		if err := scancontrol.Check(ctx, tx, p.TenantID, &in.OrganizationID, scancontrol.Now(s.Now)); err != nil {
			return err
		}
		type target struct {
			id   uuid.UUID
			name string
		}
		var targets []target
		for _, asset := range in.AssetIDs {
			t := target{id: asset}
			var authorized bool
			err = tx.QueryRow(ctx, `SELECT name,testing_authorized FROM assets WHERE tenant_id=$1 AND id=$2 AND organization_id=$3 AND deleted_at IS NULL AND NOT is_demo FOR SHARE`, p.TenantID, asset, in.OrganizationID).Scan(&t.name, &authorized)
			if errors.Is(err, pgx.ErrNoRows) {
				return invalid("asset_ids", "An asset is unavailable or belongs to another organisation.")
			}
			if err != nil {
				return err
			}
			if !authorized {
				return invalid("asset_ids", "Every selected asset requires testing authorization.")
			}
			targets = append(targets, t)
		}
		var cid uuid.UUID
		var version int
		err = tx.QueryRow(ctx, `SELECT c.id,c.version FROM contracts c JOIN integrations i ON i.tenant_id=c.tenant_id AND i.id=c.integration_id WHERE c.tenant_id=$1 AND c.key=$2 AND c.is_active AND i.enabled ORDER BY c.version DESC LIMIT 1`, p.TenantID, contract.KeyDefenseBot).Scan(&cid, &version)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("tools", "The orchis-defense-agent integration is disabled.")
		}
		if err != nil {
			return err
		}
		params, _ := json.Marshal(map[string]string{"language": language})
		params, err = contract.NormalizeVersion(contract.KeyDefenseBot, version, params)
		if err != nil {
			return err
		}
		scopeJSON, _ := json.Marshal(contract.ScanScope{Target: "local", TestingAuthorized: true, MaxHosts: s.hostLimit()})
		if _, err = tx.Exec(ctx, `INSERT INTO assessments(id,tenant_id,organization_id,name,profile,idempotency_key,request_hash,created_by,is_scan,runner,scan_options,report_language) VALUES($1,$2,$3,$4,$5,$6,$7,$8,true,'injector',$9,$10)`, id, p.TenantID, in.OrganizationID, name, in.Preset, key, hash, p.UserID, body, language); err != nil {
			return err
		}
		for _, t := range targets {
			// A fresh collection is queued in the same transaction, so an offline or unapproved endpoint fails the launch.
			if _, err = s.Collector.QueueDefenseCollection(ctx, tx, p.TenantID, t.id, defensebot.Queries); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO scan_jobs(id,tenant_id,assessment_id,control_asset_id,asset_id,range_id,range_version,name,target,addresses,tool,contract_id,contract_version,params,scope,status,error) VALUES($1,$2,$3,$4,$4,NULL,NULL,$5,'local','[]','defensebot',$6,$7,$8,$9,'waiting','')`, ids.New(), p.TenantID, id, t.id, t.name, cid, version, []byte(params), scopeJSON); err != nil {
				return err
			}
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "scan.launch", ResourceType: "scan", ResourceID: &id, IP: ip, RequestID: req, Detail: map[string]any{"organization_id": in.OrganizationID, "asset_ids": in.AssetIDs, "tools": in.Tools, "preset": in.Preset, "runner": "injector"}})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}
