package organizations

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/contract"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
)

type AssessmentInput struct {
	NmapParams   json.RawMessage `json:"nmap_params,omitempty"`
	NucleiParams json.RawMessage `json:"nuclei_params,omitempty"`
	Profile      string          `json:"profile"`
	AssetIDs     []uuid.UUID     `json:"asset_ids"`
}
type Assessment struct {
	ID             uuid.UUID          `json:"id"`
	OrganizationID uuid.UUID          `json:"organization_id"`
	Name           string             `json:"name"`
	Profile        string             `json:"profile"`
	Status         string             `json:"status"`
	CreatedAt      time.Time          `json:"created_at"`
	EndedAt        *time.Time         `json:"ended_at"`
	Targets        []AssessmentTarget `json:"targets,omitempty"`
}
type AssessmentTarget struct {
	AssetID      uuid.UUID          `json:"asset_id"`
	Name         string             `json:"name"`
	Addresses    []contract.Address `json:"addresses"`
	SimulationID *uuid.UUID         `json:"simulation_id"`
	Status       string             `json:"status"`
	SkipReason   string             `json:"skip_reason"`
}

const assessmentCols = `id,organization_id,name,profile,status,created_at,ended_at`

func scanAssessment(row pgx.Row) (Assessment, error) {
	var a Assessment
	err := row.Scan(&a.ID, &a.OrganizationID, &a.Name, &a.Profile, &a.Status, &a.CreatedAt, &a.EndedAt)
	return a, err
}
func permitted(p *auth.Principal, perms ...string) error {
	for _, perm := range perms {
		if !authz.Has(p.Permissions, perm) {
			return httpx.Forbidden(perm)
		}
	}
	return nil
}
func assessmentParams(profile string) ([]json.RawMessage, error) {
	switch profile {
	case "quick":
		return []json.RawMessage{json.RawMessage(`{"profile":"top100","service_detection":true}`), json.RawMessage(`{"severity":["medium","high","critical"]}`)}, nil
	case "full":
		return []json.RawMessage{json.RawMessage(`{"profile":"custom","ports":"1-65535","service_detection":true}`), json.RawMessage(`{"severity":["info","low","medium","high","critical"]}`)}, nil
	case "web":
		return []json.RawMessage{json.RawMessage(`{"profile":"web","service_detection":true}`), json.RawMessage(`{"severity":["medium","high","critical"],"tags":["cve"]}`)}, nil
	}
	return nil, invalid("profile", "Choose quick, full or web.")
}
func (s *Service) Launch(ctx context.Context, p *auth.Principal, org uuid.UUID, in AssessmentInput, key, ip, req string) (*Assessment, error) {
	if err := permitted(p, authz.OrganizationRead, authz.AssetRead, authz.SimulationRun); err != nil {
		return nil, err
	}
	if len(key) < 1 || len(key) > 200 {
		return nil, invalid("idempotency_key", "Provide an Idempotency-Key header of 1 to 200 characters.")
	}
	if in.Profile == "" {
		in.Profile = "quick"
	}
	params, err := assessmentParams(in.Profile)
	if err != nil {
		return nil, err
	}
	if len(in.NmapParams) > 0 {
		normalized, e := contract.Normalize(contract.KeyNmapTCP, in.NmapParams)
		if e != nil {
			return nil, invalid("nmap_params", e.Error())
		}
		params[0] = normalized
		in.NmapParams = normalized
	}
	if len(in.NucleiParams) > 0 {
		normalized, e := contract.Normalize(contract.KeyNucleiVS, in.NucleiParams)
		if e != nil {
			return nil, invalid("nuclei_params", e.Error())
		}
		params[1] = normalized
		in.NucleiParams = normalized
	}
	if len(in.AssetIDs) == 0 {
		in.AssetIDs = nil
	}
	if len(in.AssetIDs) > 1000 {
		return nil, invalid("asset_ids", "Select at most 1000 assets.")
	}
	sort.Slice(in.AssetIDs, func(i, j int) bool { return in.AssetIDs[i].String() < in.AssetIDs[j].String() })
	for i := 1; i < len(in.AssetIDs); i++ {
		if in.AssetIDs[i] == in.AssetIDs[i-1] {
			return nil, invalid("asset_ids", "Duplicate asset.")
		}
	}
	body, _ := json.Marshal(in)
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	id := ids.New()
	err = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		// Kunci organisasi menyerialkan permintaan berulang sebelum penulisan idempotensi.
		var name string
		if e := tx.QueryRow(ctx, `SELECT name FROM organizations WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, org).Scan(&name); errors.Is(e, pgx.ErrNoRows) {
			return httpx.NotFound("Organization")
		} else if e != nil {
			return e
		}
		var oldHash string
		e := tx.QueryRow(ctx, `SELECT id,request_hash FROM assessments WHERE tenant_id=$1 AND organization_id=$2 AND idempotency_key=$3`, p.TenantID, org, key).Scan(&id, &oldHash)
		if e == nil {
			if oldHash != hash {
				return httpx.Conflict("This idempotency key was used with different assessment options.")
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		type target struct {
			ID         uuid.UUID
			Name       string
			Authorized bool
			Addrs      []contract.Address
		}
		targets := []target{}
		rows, e := tx.Query(ctx, `SELECT id,name,testing_authorized FROM assets WHERE tenant_id=$1 AND organization_id=$2 AND deleted_at IS NULL AND ($3::uuid[] IS NULL OR id=ANY($3)) ORDER BY id FOR SHARE`, p.TenantID, org, in.AssetIDs)
		if e != nil {
			return e
		}
		for rows.Next() {
			var t target
			if e = rows.Scan(&t.ID, &t.Name, &t.Authorized); e != nil {
				rows.Close()
				return e
			}
			targets = append(targets, t)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		if len(in.AssetIDs) > 0 && len(targets) != len(in.AssetIDs) {
			return invalid("asset_ids", "One or more assets do not belong to this organization.")
		}
		if len(targets) > 1000 {
			return invalid("asset_ids", "Select at most 1000 assets per assessment.")
		}
		if _, e = tx.Exec(ctx, `INSERT INTO assessments(id,tenant_id,organization_id,name,profile,idempotency_key,request_hash,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, p.TenantID, org, name, in.Profile, key, hash, p.UserID); e != nil {
			return e
		}
		keys := []string{contract.KeyNmapTCP, contract.KeyNucleiVS}
		for _, t := range targets {
			ar, e := tx.Query(ctx, `SELECT kind,value,is_primary FROM asset_addresses WHERE tenant_id=$1 AND asset_id=$2 ORDER BY is_primary DESC,created_at`, p.TenantID, t.ID)
			if e != nil {
				return e
			}
			for ar.Next() {
				var a contract.Address
				if e = ar.Scan(&a.Kind, &a.Value, &a.IsPrimary); e != nil {
					ar.Close()
					return e
				}
				t.Addrs = append(t.Addrs, a)
			}
			ar.Close()
			if e = ar.Err(); e != nil {
				return e
			}
			reason := s.targetReason(t.Authorized, t.Addrs)
			var sim *uuid.UUID
			if reason == "" {
				sid := ids.New()
				sim = &sid
				if _, e = tx.Exec(ctx, `INSERT INTO simulations(id,tenant_id,name,status,started_at,launched_by) VALUES($1,$2,$3,'running',now(),$4)`, sid, p.TenantID, name+" / "+t.Name, p.UserID); e != nil {
					return e
				}
				for i, key := range keys {
					var cid uuid.UUID
					var ver int
					if e = tx.QueryRow(ctx, `SELECT c.id,c.version FROM contracts c JOIN integrations i ON i.tenant_id=c.tenant_id AND i.id=c.integration_id WHERE c.tenant_id=$1 AND c.key=$2 AND i.enabled`, p.TenantID, key).Scan(&cid, &ver); errors.Is(e, pgx.ErrNoRows) {
						return httpx.Conflict("The " + key + " integration is disabled or unavailable.")
					} else if e != nil {
						return e
					}
					normalized, e := contract.Normalize(key, params[i])
					if e != nil {
						return e
					}
					if _, e = tx.Exec(ctx, `INSERT INTO simulation_steps(id,tenant_id,simulation_id,key,title,contract_id,contract_version,params,trigger_mode,is_enabled) VALUES($1,$2,$3,$4,$4,$5,$6,$7,'after_parents',true)`, ids.New(), p.TenantID, sid, []string{"nmap", "nuclei"}[i], cid, ver, []byte(normalized)); e != nil {
						return e
					}
				}
				if _, e = tx.Exec(ctx, `INSERT INTO simulation_targets(tenant_id,simulation_id,asset_id) VALUES($1,$2,$3)`, p.TenantID, sid, t.ID); e != nil {
					return e
				}
			}
			addrs, _ := json.Marshal(t.Addrs)
			if string(addrs) == "null" {
				addrs = []byte(`[]`)
			}
			if _, e = tx.Exec(ctx, `INSERT INTO assessment_targets(tenant_id,assessment_id,asset_id,asset_name,addresses,simulation_id,skip_reason) VALUES($1,$2,$3,$4,$5,$6,$7)`, p.TenantID, id, t.ID, t.Name, addrs, sim, reason); e != nil {
				return e
			}
			if sim != nil {
				if e = s.Engine.Advance(ctx, tx, p.TenantID, *sim); e != nil {
					return e
				}
			}
		}
		if e = s.Refresh(ctx, tx, p.TenantID); e != nil {
			return e
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "assessment.launch", ResourceType: "assessment", ResourceID: &id, IP: ip, RequestID: req, Detail: map[string]any{"organization_id": org, "profile": in.Profile, "target_count": len(targets)}})
	})
	if err != nil {
		return nil, err
	}
	return s.Assessment(ctx, p, id)
}

// Refresh aman diulang oleh worker; selesai tidak berarti semua pemindaian berhasil.
func (s *Service) Refresh(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE assessments a SET status='finished',ended_at=now() WHERE a.tenant_id=$1 AND a.status='running' AND NOT EXISTS(SELECT 1 FROM assessment_targets t JOIN simulations s ON s.tenant_id=t.tenant_id AND s.id=t.simulation_id WHERE t.tenant_id=a.tenant_id AND t.assessment_id=a.id AND s.status IN ('running','paused','draft','scheduled'))`, tenant)
	return err
}
func (s *Service) Assessment(ctx context.Context, p *auth.Principal, id uuid.UUID) (*Assessment, error) {
	if err := permitted(p, authz.OrganizationRead, authz.AssetRead, authz.SimulationRead); err != nil {
		return nil, err
	}
	var out Assessment
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		var e error
		out, e = scanAssessment(tx.QueryRow(ctx, `SELECT `+assessmentCols+` FROM assessments WHERE tenant_id=$1 AND id=$2`, p.TenantID, id))
		if errors.Is(e, pgx.ErrNoRows) {
			return httpx.NotFound("Assessment")
		}
		if e != nil {
			return e
		}
		out.Targets = []AssessmentTarget{}
		rows, e := tx.Query(ctx, `SELECT t.asset_id,t.asset_name,t.addresses,t.simulation_id,COALESCE(s.status,'skipped'),t.skip_reason FROM assessment_targets t LEFT JOIN simulations s ON s.tenant_id=t.tenant_id AND s.id=t.simulation_id WHERE t.tenant_id=$1 AND t.assessment_id=$2 ORDER BY t.asset_name,t.asset_id`, p.TenantID, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var t AssessmentTarget
			if e = rows.Scan(&t.AssetID, &t.Name, &t.Addresses, &t.SimulationID, &t.Status, &t.SkipReason); e != nil {
				return e
			}
			out.Targets = append(out.Targets, t)
		}
		return rows.Err()
	})
	return &out, err
}
func (s *Service) Assessments(ctx context.Context, p *auth.Principal, org uuid.UUID, raw string) (map[string]any, error) {
	if e := permitted(p, authz.OrganizationRead); e != nil {
		return nil, e
	}
	id, err := cursor(raw)
	if err != nil {
		return nil, err
	}
	items := []Assessment{}
	var next *string
	err = s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT `+assessmentCols+` FROM assessments WHERE tenant_id=$1 AND organization_id=$2 AND ($3::uuid='00000000-0000-0000-0000-000000000000' OR id<$3) ORDER BY id DESC LIMIT 26`, p.TenantID, org, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			a, e := scanAssessment(rows)
			if e != nil {
				return e
			}
			items = append(items, a)
		}
		return rows.Err()
	})
	if len(items) > 25 {
		items = items[:25]
		v := base64Cursor(items[24].ID)
		next = &v
	}
	return map[string]any{"items": items, "next_cursor": next}, err
}

func base64Cursor(id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id.String()))
}
