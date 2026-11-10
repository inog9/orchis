package schedules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/contract"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/scancontrol"
	"orchis-go/internal/scans"
)

type Config struct {
	Rule              Rule        `json:"rule"`
	Kind              string      `json:"kind"`
	AllAssets         bool        `json:"all_assets"`
	AssetIDs          []uuid.UUID `json:"asset_ids"`
	Language          string      `json:"language"`
	MaxRuntimeMinutes int         `json:"max_runtime_minutes"`
	NotifyChanges     bool        `json:"notify_changes"`
	NotifyGaps        bool        `json:"notify_gaps"`
	WeeklyDigest      bool        `json:"weekly_digest"`
}
type Input struct {
	RequestKey     uuid.UUID `json:"request_key"`
	Name           string    `json:"name"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Config         Config    `json:"config"`
	Paused         bool      `json:"paused"`
	Version        int       `json:"version"`
}
type Schedule struct {
	ID             uuid.UUID   `json:"id"`
	OrganizationID uuid.UUID   `json:"organization_id"`
	Organization   string      `json:"organization"`
	Name           string      `json:"name"`
	Config         Config      `json:"config"`
	Paused         bool        `json:"paused"`
	NextRunAt      time.Time   `json:"next_run_at"`
	NextRuns       []time.Time `json:"next_runs"`
	RunAs          string      `json:"run_as"`
	Version        int         `json:"version"`
	LastRun        *Run        `json:"last_run,omitempty"`
}
type Run struct {
	ID          uuid.UUID  `json:"id"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	Status      string     `json:"status"`
	ScanID      *uuid.UUID `json:"scan_id"`
	Reason      string     `json:"reason"`
	Summary     Summary    `json:"summary"`
	StartedAt   *time.Time `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at"`
}
type Summary struct {
	New        int `json:"new"`
	Reappeared int `json:"reappeared"`
	Persistent int `json:"persistent"`
	Resolved   int `json:"resolved"`
	Uncompared int `json:"uncompared"`
	Completed  int `json:"completed"`
	Total      int `json:"total"`
	Attention  int `json:"attention"`
}
type Service struct {
	DB    *db.DB
	Scans *scans.Service
	Now   func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func access(p *auth.Principal, write bool) error {
	for _, perm := range []string{authz.OrganizationRead, authz.AssetRead, authz.AssessmentRead, authz.SimulationRead, authz.FindingRead, authz.ReportRead} {
		if !authz.Has(p.Permissions, perm) {
			return httpx.Forbidden(perm)
		}
	}
	if write {
		for _, perm := range []string{authz.AssessmentRun, authz.SimulationRun} {
			if !authz.Has(p.Permissions, perm) {
				return httpx.Forbidden(perm)
			}
		}
	}
	return nil
}
func validation(field, msg string) error {
	return httpx.Validation(httpx.FieldError{Name: field, Message: msg})
}
func (s *Service) validate(in *Input) error {
	in.Name = strings.TrimSpace(in.Name)
	if utf8.RuneCountInString(in.Name) < 1 || utf8.RuneCountInString(in.Name) > 120 {
		return validation("name", "Use a schedule name of 1 to 120 characters.")
	}
	if in.OrganizationID == uuid.Nil {
		return validation("organization_id", "Choose an organisation.")
	}
	c := &in.Config
	if e := c.Rule.Validate(); e != nil {
		return validation("rule", e.Error())
	}
	if c.Kind != "web" && c.Kind != "defense" && c.Kind != "full" {
		return validation("kind", "Choose web pentest, installed software check, or full assessment.")
	}
	if c.Language != "en" && c.Language != "id" {
		return validation("language", "Choose English or Indonesia.")
	}
	if c.MaxRuntimeMinutes < 15 || c.MaxRuntimeMinutes > 1440 {
		return validation("max_runtime_minutes", "Choose a runtime limit from 15 to 1440 minutes.")
	}
	max := contract.DefaultScanMaxHosts
	if s.Scans != nil && s.Scans.MaxHosts > 0 {
		max = s.Scans.MaxHosts
	}
	if !c.AllAssets && (len(c.AssetIDs) == 0 || len(c.AssetIDs) > max) {
		return validation("asset_ids", "Select authorised assets within the scan host limit.")
	}
	if c.AllAssets && len(c.AssetIDs) > 0 {
		return validation("asset_ids", "Choose selected assets or all authorised assets, not both.")
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range c.AssetIDs {
		if id == uuid.Nil || seen[id] {
			return validation("asset_ids", "Select each asset once.")
		}
		seen[id] = true
	}
	return nil
}
func checkTargets(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, in Input) error {
	var demo bool
	e := tx.QueryRow(ctx, `SELECT is_demo FROM organizations WHERE tenant_id=$1 AND id=$2`, tenant, in.OrganizationID).Scan(&demo)
	if errors.Is(e, pgx.ErrNoRows) {
		return httpx.NotFound("Organisation")
	}
	if e != nil {
		return e
	}
	if demo {
		return httpx.Conflict("Synthetic organisations cannot run scheduled audits.")
	}
	if !in.Config.AllAssets {
		var n int
		e = tx.QueryRow(ctx, `SELECT count(*) FROM assets WHERE tenant_id=$1 AND organization_id=$2 AND id=ANY($3::uuid[]) AND testing_authorized AND deleted_at IS NULL AND NOT is_demo`, tenant, in.OrganizationID, in.Config.AssetIDs).Scan(&n)
		if e != nil {
			return e
		}
		if n != len(in.Config.AssetIDs) {
			return validation("asset_ids", "Every selected asset must belong to this organisation and have testing authorisation.")
		}
	}
	return nil
}
func (s *Service) Save(ctx context.Context, p *auth.Principal, id uuid.UUID, in Input, ip, req string) (*Schedule, error) {
	if e := access(p, true); e != nil {
		return nil, e
	}
	if e := s.validate(&in); e != nil {
		return nil, e
	}
	now := s.now()
	next, e := in.Config.Rule.Next(now)
	if e != nil {
		return nil, e
	}
	create := id == uuid.Nil
	if create && in.RequestKey == uuid.Nil {
		return nil, validation("request_key", "Provide a request key before creating a schedule.")
	}
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	if create {
		id = ids.New()
	}
	e = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if e := scancontrol.Lock(ctx, tx, p.TenantID); e != nil {
			return e
		}
		if e := checkTargets(ctx, tx, p.TenantID, in); e != nil {
			return e
		}
		if create {
			// Serialize the per-tenant cap, also for concurrent requests.
			if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "schedule-cap:"+p.TenantID.String()); e != nil {
				return e
			}
			var oldHash string
			var deleted *time.Time
			existingErr := tx.QueryRow(ctx, `SELECT id,creation_hash,deleted_at FROM audit_schedules WHERE tenant_id=$1 AND creation_key=$2`, p.TenantID, in.RequestKey).Scan(&id, &oldHash, &deleted)
			if existingErr == nil {
				if deleted != nil || oldHash != hash {
					return httpx.Conflict("This request key belongs to different schedule settings. Reopen the form before creating another schedule.")
				}
				return nil
			}
			if !errors.Is(existingErr, pgx.ErrNoRows) {
				return existingErr
			}
			var n int
			if e := tx.QueryRow(ctx, `SELECT count(*) FROM audit_schedules WHERE tenant_id=$1 AND deleted_at IS NULL`, p.TenantID).Scan(&n); e != nil {
				return e
			}
			if n >= 1000 {
				return httpx.Conflict("This workspace has reached its schedule limit.")
			}
			_, e = tx.Exec(ctx, `INSERT INTO audit_schedules(id,tenant_id,organization_id,name,config,paused,next_run_at,next_digest_at,created_by,run_as,creation_key,creation_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$9,$10,$11)`, id, p.TenantID, in.OrganizationID, in.Name, in.Config, in.Paused, next, now.Add(7*24*time.Hour), p.UserID, in.RequestKey, hash)
		} else {
			tag, err := tx.Exec(ctx, `UPDATE audit_schedules SET name=$3,config=$4,paused=$5,next_run_at=$6,run_as=$7,version=version+1,updated_at=$8 WHERE tenant_id=$1 AND id=$2 AND organization_id=$9 AND version=$10 AND deleted_at IS NULL`, p.TenantID, id, in.Name, in.Config, in.Paused, next, p.UserID, now, in.OrganizationID, in.Version)
			e = err
			if e == nil && tag.RowsAffected() == 0 {
				return httpx.Conflict("The schedule changed or is unavailable. Refresh before saving.")
			}
		}
		if e != nil {
			return e
		}
		action := "schedule.updated"
		if create {
			action = "schedule.created"
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: action, ResourceType: "audit_schedule", ResourceID: &id, IP: ip, RequestID: req})
	})
	if e != nil {
		return nil, e
	}
	return s.Get(ctx, p, id)
}

const scheduleCols = `s.id,s.organization_id,o.name,s.name,s.config,s.paused,s.next_run_at,u.display_name,s.version`

func scheduleRow(row pgx.Row) (Schedule, error) {
	var v Schedule
	e := row.Scan(&v.ID, &v.OrganizationID, &v.Organization, &v.Name, &v.Config, &v.Paused, &v.NextRunAt, &v.RunAs, &v.Version)
	return v, e
}

const runCols = `id,scheduled_at,status,scan_id,reason,summary,started_at,ended_at`

func runRow(row pgx.Row) (Run, error) {
	var v Run
	e := row.Scan(&v.ID, &v.ScheduledAt, &v.Status, &v.ScanID, &v.Reason, &v.Summary, &v.StartedAt, &v.EndedAt)
	return v, e
}
func (s *Service) List(ctx context.Context, p *auth.Principal, org *uuid.UUID, cursor string) ([]Schedule, *string, error) {
	if e := access(p, false); e != nil {
		return nil, nil, e
	}
	var before *uuid.UUID
	if cursor != "" {
		v, e := uuid.Parse(cursor)
		if e != nil {
			return nil, nil, httpx.BadRequest("Invalid cursor.")
		}
		before = &v
	}
	out := []Schedule{}
	var next *string
	e := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT `+scheduleCols+` FROM audit_schedules s JOIN organizations o ON o.tenant_id=s.tenant_id AND o.id=s.organization_id JOIN users u ON u.tenant_id=s.tenant_id AND u.id=s.run_as WHERE s.tenant_id=$1 AND s.deleted_at IS NULL AND ($2::uuid IS NULL OR s.organization_id=$2) AND ($3::uuid IS NULL OR s.id<$3) ORDER BY s.id DESC LIMIT 51`, p.TenantID, org, before)
		if e != nil {
			return e
		}
		for rows.Next() {
			v, e := scheduleRow(rows)
			if e != nil {
				rows.Close()
				return e
			}
			out = append(out, v)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out) > 50 {
			out = out[:50]
			v := out[49].ID.String()
			next = &v
		}
		for i := range out {
			if e = s.enrich(ctx, tx, p.TenantID, &out[i]); e != nil {
				return e
			}
		}
		return nil
	})
	return out, next, e
}
func (s *Service) enrich(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, v *Schedule) error {
	var e error
	v.NextRuns = []time.Time{}
	if !v.Paused {
		v.NextRuns, e = v.Config.Rule.Preview(v.NextRunAt.Add(-time.Nanosecond))
		if e != nil {
			return e
		}
	}
	last, e := runRow(tx.QueryRow(ctx, `SELECT `+runCols+` FROM audit_schedule_runs WHERE tenant_id=$1 AND schedule_id=$2 ORDER BY scheduled_at DESC LIMIT 1`, tenant, v.ID))
	if e == nil {
		v.LastRun = &last
	}
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	return e
}
func (s *Service) Get(ctx context.Context, p *auth.Principal, id uuid.UUID) (*Schedule, error) {
	if e := access(p, false); e != nil {
		return nil, e
	}
	var out Schedule
	e := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		v, e := scheduleRow(tx.QueryRow(ctx, `SELECT `+scheduleCols+` FROM audit_schedules s JOIN organizations o ON o.tenant_id=s.tenant_id AND o.id=s.organization_id JOIN users u ON u.tenant_id=s.tenant_id AND u.id=s.run_as WHERE s.tenant_id=$1 AND s.id=$2 AND s.deleted_at IS NULL`, p.TenantID, id))
		if errors.Is(e, pgx.ErrNoRows) {
			return httpx.NotFound("Schedule")
		}
		if e != nil {
			return e
		}
		out = v
		return s.enrich(ctx, tx, p.TenantID, &out)
	})
	return &out, e
}
func (s *Service) Pause(ctx context.Context, p *auth.Principal, id uuid.UUID, paused bool, version int, ip, req string) (*Schedule, error) {
	if e := access(p, true); e != nil {
		return nil, e
	}
	if !paused {
		v, e := s.Get(ctx, p, id)
		if e != nil {
			return nil, e
		}
		if v.Version != version {
			return nil, httpx.Conflict("The schedule changed. Refresh before saving.")
		}
		return s.Save(ctx, p, id, Input{Name: v.Name, OrganizationID: v.OrganizationID, Config: v.Config, Paused: false, Version: version}, ip, req)
	}
	// Pausing remains possible when assets lose authorisation or leave the scope.
	e := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if e := scancontrol.Lock(ctx, tx, p.TenantID); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE audit_schedules SET paused=true,version=version+1,updated_at=$4 WHERE tenant_id=$1 AND id=$2 AND version=$3 AND deleted_at IS NULL`, p.TenantID, id, version, s.now())
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return httpx.Conflict("The schedule changed or is unavailable. Refresh before saving.")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "schedule.paused", ResourceType: "audit_schedule", ResourceID: &id, IP: ip, RequestID: req})
	})
	if e != nil {
		return nil, e
	}
	return s.Get(ctx, p, id)
}
func (s *Service) Delete(ctx context.Context, p *auth.Principal, id uuid.UUID, version int, ip, req string) error {
	if e := access(p, true); e != nil {
		return e
	}
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if e := scancontrol.Lock(ctx, tx, p.TenantID); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE audit_schedules SET paused=true,deleted_at=$4,version=version+1 WHERE tenant_id=$1 AND id=$2 AND version=$3 AND deleted_at IS NULL`, p.TenantID, id, version, s.now())
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return httpx.Conflict("The schedule changed or is unavailable. Refresh before deleting.")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "schedule.deleted", ResourceType: "audit_schedule", ResourceID: &id, IP: ip, RequestID: req})
	})
}
func (s *Service) History(ctx context.Context, p *auth.Principal, id uuid.UUID, cursor string) ([]Run, *string, error) {
	if _, e := s.Get(ctx, p, id); e != nil {
		return nil, nil, e
	}
	var before *uuid.UUID
	if cursor != "" {
		v, e := uuid.Parse(cursor)
		if e != nil {
			return nil, nil, httpx.BadRequest("Invalid cursor.")
		}
		before = &v
	}
	out := []Run{}
	var next *string
	e := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT `+runCols+` FROM audit_schedule_runs WHERE tenant_id=$1 AND schedule_id=$2 AND ($3::uuid IS NULL OR id<$3) ORDER BY id DESC LIMIT 31`, p.TenantID, id, before)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := runRow(rows)
			if e != nil {
				return e
			}
			out = append(out, v)
		}
		if len(out) > 30 {
			out = out[:30]
			v := out[29].ID.String()
			next = &v
		}
		return rows.Err()
	})
	return out, next, e
}

// CheckLaunch runs inside the scan-control transaction, serializing dispatch
// against schedule edits, pause and deletion. It also reloads the actor's role.
func (s *Service) CheckLaunch(ctx context.Context, tx pgx.Tx, p *auth.Principal, in scans.Input, key string) error {
	id, e := uuid.Parse(strings.TrimPrefix(key, "schedule:"))
	if e != nil || !strings.HasPrefix(key, "schedule:") {
		return httpx.Conflict("Scheduled occurrence is unavailable.")
	}
	var paused bool
	var deleted *time.Time
	var at time.Time
	e = tx.QueryRow(ctx, `SELECT s.paused,s.deleted_at,r.scheduled_at FROM audit_schedule_runs r JOIN audit_schedules s ON s.tenant_id=r.tenant_id AND s.id=r.schedule_id WHERE r.tenant_id=$1 AND r.id=$2 AND r.status='queued' AND r.run_as=$3 AND s.organization_id=$4 FOR SHARE OF s`, p.TenantID, id, p.UserID, in.OrganizationID).Scan(&paused, &deleted, &at)
	if errors.Is(e, pgx.ErrNoRows) {
		return httpx.Conflict("Scheduled occurrence is unavailable.")
	}
	if e != nil {
		return e
	}
	if paused || deleted != nil {
		return httpx.Conflict("The schedule was paused or deleted before dispatch.")
	}
	if s.now().Sub(at) > 15*time.Minute {
		return httpx.Conflict("The audit missed its 15-minute start window.")
	}
	current := &auth.Principal{}
	e = tx.QueryRow(ctx, `SELECT r.permissions FROM users u JOIN roles r ON r.tenant_id=u.tenant_id AND r.id=u.role_id WHERE u.tenant_id=$1 AND u.id=$2 AND u.status='active' AND u.deleted_at IS NULL AND u.kind='human' AND NOT u.is_demo FOR SHARE OF u,r`, p.TenantID, p.UserID).Scan(&current.Permissions)
	if errors.Is(e, pgx.ErrNoRows) {
		return httpx.Conflict("The schedule's responsible user is no longer active.")
	}
	if e != nil {
		return e
	}
	return access(current, true)
}
