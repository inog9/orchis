// Package scenario: template chain (scenario = chain): metadata, perimeter sasaran, dan graf langkah.
package scenario

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/chain"
	"orchis-go/internal/contract"
	"orchis-go/internal/db"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
)

type Service struct{ DB *db.DB }

var severities = map[string]bool{"low": true, "medium": true, "high": true, "critical": true}

type Scenario struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Severity    string    `json:"severity"`
	Category    *string   `json:"category"`
	Status      string    `json:"status"` // draft | ready (diturunkan dari kesiapan)
	StepCount   int       `json:"step_count"`
	EdgeCount   int       `json:"edge_count"`
	TargetCount int       `json:"target_count"`
	Simulations int       `json:"simulations"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Version     int       `json:"version"`
}

type TargetRef struct {
	Kind       string    `json:"kind"` // asset | group
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Authorized *bool     `json:"authorized,omitempty"` // hanya untuk aset
}

type StepDTO struct {
	Key          string          `json:"key"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	ContractKey  string          `json:"contract_key"`
	Params       json.RawMessage `json:"params"`
	DelaySeconds int             `json:"delay_seconds"`
	TriggerMode  string          `json:"trigger_mode"`
	Trigger      *chain.Trigger  `json:"trigger"`
	IsEnabled    bool            `json:"is_enabled"`
}

type EdgeDTO = chain.Edge

type Detail struct {
	Scenario
	Targets  []TargetRef `json:"targets"`
	Steps    []StepDTO   `json:"steps"`
	Edges    []EdgeDTO   `json:"edges"`
	Problems []string    `json:"problems"` // alasan scenario belum siap diluncurkan
}

const listSQL = `
SELECT s.id, s.name, s.description, s.severity, s.category, s.created_at, s.updated_at, s.version,
  (SELECT count(*) FROM scenario_steps x WHERE x.tenant_id=s.tenant_id AND x.scenario_id=s.id),
  (SELECT count(*) FROM scenario_step_edges x WHERE x.tenant_id=s.tenant_id AND x.scenario_id=s.id),
  (SELECT count(*) FROM scenario_targets x WHERE x.tenant_id=s.tenant_id AND x.scenario_id=s.id),
  (SELECT count(*) FROM simulations x WHERE x.tenant_id=s.tenant_id AND x.scenario_id=s.id),
  (SELECT count(*) FROM scenario_steps x WHERE x.tenant_id=s.tenant_id AND x.scenario_id=s.id AND x.is_enabled)
FROM scenarios s WHERE s.tenant_id=$1 AND s.deleted_at IS NULL`

func scan(row pgx.Row) (Scenario, error) {
	var s Scenario
	var enabled int
	err := row.Scan(&s.ID, &s.Name, &s.Description, &s.Severity, &s.Category, &s.CreatedAt, &s.UpdatedAt, &s.Version,
		&s.StepCount, &s.EdgeCount, &s.TargetCount, &s.Simulations, &enabled)
	s.Status = "draft"
	if enabled > 0 && s.TargetCount > 0 {
		s.Status = "ready"
	}
	return s, err
}

func (s *Service) List(ctx context.Context, p *auth.Principal) ([]Scenario, error) {
	out := []Scenario{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, listSQL+` ORDER BY s.updated_at DESC, s.id LIMIT 200`, p.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			sc, err := scan(rows)
			if err != nil {
				return err
			}
			out = append(out, sc)
		}
		return rows.Err()
	})
	return out, err
}

type Input struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Severity    string  `json:"severity"`
	Category    *string `json:"category"`
}

func (in *Input) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	var fe []httpx.FieldError
	if in.Name == "" || len([]rune(in.Name)) > 200 {
		fe = append(fe, httpx.FieldError{Name: "name", Message: "Enter a name of 1 to 200 characters."})
	}
	if in.Severity == "" {
		in.Severity = "medium"
	}
	if !severities[in.Severity] {
		fe = append(fe, httpx.FieldError{Name: "severity", Message: "Choose low, medium, high or critical."})
	}
	if in.Description != nil && len(*in.Description) > 2000 {
		fe = append(fe, httpx.FieldError{Name: "description", Message: "At most 2000 characters."})
	}
	if in.Category != nil {
		c := strings.TrimSpace(*in.Category)
		if c == "" {
			in.Category = nil
		} else if len(c) > 80 {
			fe = append(fe, httpx.FieldError{Name: "category", Message: "At most 80 characters."})
		} else {
			in.Category = &c
		}
	}
	if len(fe) > 0 {
		return httpx.Validation(fe...)
	}
	return nil
}

func dupName(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return httpx.Validation(httpx.FieldError{Name: "name", Message: "A scenario with this name already exists."})
	}
	return err
}

func (s *Service) Create(ctx context.Context, p *auth.Principal, in Input, ip, reqID string) (*Detail, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	id := ids.New()
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO scenarios (id, tenant_id, name, description, severity, category, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			id, p.TenantID, in.Name, in.Description, in.Severity, in.Category, p.UserID); err != nil {
			return dupName(err)
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "scenario.create", ResourceType: "scenario", ResourceID: &id, IP: ip, RequestID: reqID, Detail: map[string]any{"name": in.Name}})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

func (s *Service) Get(ctx context.Context, p *auth.Principal, id uuid.UUID) (*Detail, error) {
	var out *Detail
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		d, err := load(ctx, tx, p.TenantID, id)
		out = d
		return err
	})
	return out, err
}

func load(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (*Detail, error) {
	sc, err := scan(tx.QueryRow(ctx, listSQL+` AND s.id=$2`, tenant, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.NotFound("Scenario")
	}
	if err != nil {
		return nil, err
	}
	d := &Detail{Scenario: sc, Targets: []TargetRef{}, Steps: []StepDTO{}, Edges: []EdgeDTO{}, Problems: []string{}}
	rows, err := tx.Query(ctx, `
SELECT 'asset', a.id, a.name, a.testing_authorized FROM scenario_targets t JOIN assets a ON a.tenant_id=t.tenant_id AND a.id=t.asset_id AND a.deleted_at IS NULL
 WHERE t.tenant_id=$1 AND t.scenario_id=$2
UNION ALL
SELECT 'group', g.id, g.name, NULL FROM scenario_targets t JOIN asset_groups g ON g.tenant_id=t.tenant_id AND g.id=t.asset_group_id
 WHERE t.tenant_id=$1 AND t.scenario_id=$2 ORDER BY 1, 3`, tenant, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t TargetRef
		if err := rows.Scan(&t.Kind, &t.ID, &t.Name, &t.Authorized); err != nil {
			rows.Close()
			return nil, err
		}
		d.Targets = append(d.Targets, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sr, err := tx.Query(ctx, `
SELECT st.key, st.title, COALESCE(st.description,''), c.key, st.params, st.delay_seconds, st.trigger_mode, st.trigger, st.is_enabled
FROM scenario_steps st JOIN contracts c ON c.tenant_id=st.tenant_id AND c.id=st.contract_id
WHERE st.tenant_id=$1 AND st.scenario_id=$2 ORDER BY st.created_at, st.key`, tenant, id)
	if err != nil {
		return nil, err
	}
	for sr.Next() {
		var st StepDTO
		var trig []byte
		if err := sr.Scan(&st.Key, &st.Title, &st.Description, &st.ContractKey, &st.Params, &st.DelaySeconds, &st.TriggerMode, &trig, &st.IsEnabled); err != nil {
			sr.Close()
			return nil, err
		}
		if len(trig) > 0 && string(trig) != "null" {
			st.Trigger = &chain.Trigger{}
			if err := json.Unmarshal(trig, st.Trigger); err != nil {
				sr.Close()
				return nil, err
			}
		}
		d.Steps = append(d.Steps, st)
	}
	sr.Close()
	if err := sr.Err(); err != nil {
		return nil, err
	}
	er, err := tx.Query(ctx, `
SELECT p.key, c.key,e.rule FROM scenario_step_edges e
JOIN scenario_steps p ON p.tenant_id=e.tenant_id AND p.id=e.parent_step_id
JOIN scenario_steps c ON c.tenant_id=e.tenant_id AND c.id=e.child_step_id
WHERE e.tenant_id=$1 AND e.scenario_id=$2 ORDER BY 1, 2`, tenant, id)
	if err != nil {
		return nil, err
	}
	for er.Next() {
		var e EdgeDTO
		var rule []byte
		if err := er.Scan(&e.Parent, &e.Child, &rule); err != nil {
			er.Close()
			return nil, err
		}
		if err := json.Unmarshal(rule, &e); err != nil {
			return nil, err
		}
		d.Edges = append(d.Edges, e)
	}
	er.Close()
	if err := er.Err(); err != nil {
		return nil, err
	}
	enabled := 0
	for _, st := range d.Steps {
		if st.IsEnabled {
			enabled++
		}
	}
	if enabled == 0 {
		d.Problems = append(d.Problems, "Add at least one enabled step.")
	}
	if len(d.Targets) == 0 {
		d.Problems = append(d.Problems, "Choose the assets or groups this chain targets.")
	}
	for _, t := range d.Targets {
		if t.Kind == "asset" && t.Authorized != nil && !*t.Authorized {
			d.Problems = append(d.Problems, "Asset "+t.Name+" is not authorized for testing.")
		}
	}
	return d, nil
}

type Patch struct {
	Version int `json:"version"`
	Input
}

// lockCheck mengunci baris scenario dan memastikan versi cocok (kunci optimistik).
func lockCheck(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, version int) error {
	var cur int
	err := tx.QueryRow(ctx, `SELECT version FROM scenarios WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, tenant, id).Scan(&cur)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.NotFound("Scenario")
	}
	if err != nil {
		return err
	}
	if cur != version {
		return httpx.Conflict("This scenario was changed by someone else. Reload and try again.")
	}
	return nil
}

func (s *Service) Patch(ctx context.Context, p *auth.Principal, id uuid.UUID, in Patch, ip, reqID string) (*Detail, error) {
	if in.Version < 1 {
		return nil, httpx.Validation(httpx.FieldError{Name: "version", Message: "Send the version you loaded."})
	}
	if err := in.Input.validate(); err != nil {
		return nil, err
	}
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := lockCheck(ctx, tx, p.TenantID, id, in.Version); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE scenarios SET name=$3, description=$4, severity=$5, category=$6, version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`,
			p.TenantID, id, in.Name, in.Description, in.Severity, in.Category); err != nil {
			return dupName(err)
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "scenario.update", ResourceType: "scenario", ResourceID: &id, IP: ip, RequestID: reqID})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

func (s *Service) Delete(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) error {
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE scenarios SET deleted_at=now(), version=version+1 WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, p.TenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.NotFound("Scenario")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "scenario.delete", ResourceType: "scenario", ResourceID: &id, IP: ip, RequestID: reqID})
	})
}

type TargetsInput struct {
	Version  int         `json:"version"`
	AssetIDs []uuid.UUID `json:"asset_ids"`
	GroupIDs []uuid.UUID `json:"group_ids"`
}

func (s *Service) SetTargets(ctx context.Context, p *auth.Principal, id uuid.UUID, in TargetsInput, ip, reqID string) (*Detail, error) {
	if in.Version < 1 {
		return nil, httpx.Validation(httpx.FieldError{Name: "version", Message: "Send the version you loaded."})
	}
	if len(in.AssetIDs)+len(in.GroupIDs) > 200 {
		return nil, httpx.Validation(httpx.FieldError{Name: "asset_ids", Message: "At most 200 targets."})
	}
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := lockCheck(ctx, tx, p.TenantID, id, in.Version); err != nil {
			return err
		}
		assets, groups := uniq(in.AssetIDs), uniq(in.GroupIDs)
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM assets WHERE tenant_id=$1 AND deleted_at IS NULL AND id = ANY($2)`, p.TenantID, assets).Scan(&n); err != nil {
			return err
		}
		if n != len(assets) {
			return httpx.Validation(httpx.FieldError{Name: "asset_ids", Message: "One of the assets does not exist."})
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM asset_groups WHERE tenant_id=$1 AND id = ANY($2)`, p.TenantID, groups).Scan(&n); err != nil {
			return err
		}
		if n != len(groups) {
			return httpx.Validation(httpx.FieldError{Name: "group_ids", Message: "One of the groups does not exist."})
		}
		if _, err := tx.Exec(ctx, `DELETE FROM scenario_targets WHERE tenant_id=$1 AND scenario_id=$2`, p.TenantID, id); err != nil {
			return err
		}
		for _, a := range assets {
			if _, err := tx.Exec(ctx, `INSERT INTO scenario_targets (id, tenant_id, scenario_id, asset_id) VALUES ($1,$2,$3,$4)`, ids.New(), p.TenantID, id, a); err != nil {
				return err
			}
		}
		for _, g := range groups {
			if _, err := tx.Exec(ctx, `INSERT INTO scenario_targets (id, tenant_id, scenario_id, asset_group_id) VALUES ($1,$2,$3,$4)`, ids.New(), p.TenantID, id, g); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE scenarios SET version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "scenario.targets", ResourceType: "scenario", ResourceID: &id, IP: ip, RequestID: reqID,
			Detail: map[string]any{"assets": len(assets), "groups": len(groups)}})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

func uniq(in []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	out := make([]uuid.UUID, 0, len(in))
	for _, x := range in {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

type GraphInput struct {
	Version int       `json:"version"`
	Steps   []StepDTO `json:"steps"`
	Edges   []EdgeDTO `json:"edges"`
}

func isFindingType(t string) bool {
	switch t {
	case "open_port", "port_scan", "service", "vulnerability", "url", "text":
		return true
	}
	return false
}

// SetGraph mengganti seluruh graf (langkah + tepi) secara atomik setelah validasi penuh: DAG, parameter per kontrak, aturan trigger.
func (s *Service) SetGraph(ctx context.Context, p *auth.Principal, id uuid.UUID, in GraphInput, ip, reqID string) (*Detail, error) {
	if in.Version < 1 {
		return nil, httpx.Validation(httpx.FieldError{Name: "version", Message: "Send the version you loaded."})
	}
	steps := make([]chain.Step, len(in.Steps))
	norm := make([]json.RawMessage, len(in.Steps))
	var fe []httpx.FieldError
	for i, st := range in.Steps {
		steps[i] = chain.Step{Key: st.Key, Enabled: st.IsEnabled, Mode: st.TriggerMode, Trigger: st.Trigger, DelaySeconds: st.DelaySeconds}
		if t := strings.TrimSpace(st.Title); t == "" || len([]rune(t)) > 200 {
			fe = append(fe, httpx.FieldError{Name: "steps[" + itoa(i) + "].title", Message: "Enter a title of 1 to 200 characters."})
		}
		if len(st.Description) > 2000 {
			fe = append(fe, httpx.FieldError{Name: "steps[" + itoa(i) + "].description", Message: "At most 2000 characters."})
		}
		if contract.Find(st.ContractKey) == nil {
			fe = append(fe, httpx.FieldError{Name: "steps[" + itoa(i) + "].contract_key", Message: "Unknown action."})
			continue
		}
		n, err := contract.Normalize(st.ContractKey, st.Params)
		if err != nil {
			var pe *contract.ParamError
			if errors.As(err, &pe) {
				fe = append(fe, httpx.FieldError{Name: "steps[" + itoa(i) + "]." + pe.Field, Message: pe.Message})
				continue
			}
			return nil, err
		}
		norm[i] = n
	}
	edges := make([]chain.Edge, len(in.Edges))
	for i, e := range in.Edges {
		edges[i] = e
	}
	mapperChildren := map[string]bool{}
	for _, edge := range edges {
		if edge.Mapper != nil {
			if mapperChildren[edge.Child] {
				fe = append(fe, httpx.FieldError{Name: "edges", Message: "Only one mapper may supply each step."})
			}
			mapperChildren[edge.Child] = true
		}
	}
	for _, e := range chain.Validate(steps, edges, isFindingType) {
		fe = append(fe, httpx.FieldError{Name: e.Field, Message: e.Message})
	}
	if len(fe) > 0 {
		return nil, httpx.Validation(fe...)
	}
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := lockCheck(ctx, tx, p.TenantID, id, in.Version); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM scenario_step_edges WHERE tenant_id=$1 AND scenario_id=$2`, p.TenantID, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM scenario_steps WHERE tenant_id=$1 AND scenario_id=$2`, p.TenantID, id); err != nil {
			return err
		}
		stepID := map[string]uuid.UUID{}
		for i, st := range in.Steps {
			def := contract.Find(st.ContractKey)
			sid := ids.New()
			stepID[st.Key] = sid
			var trig []byte
			if st.Trigger != nil {
				trig, _ = json.Marshal(st.Trigger)
			}
			// created_at berurutan agar urutan tampilan sama dengan urutan yang dikirim
			if _, err := tx.Exec(ctx, `
INSERT INTO scenario_steps (id, tenant_id, scenario_id, key, title, description, contract_id, contract_version, params, delay_seconds, trigger_mode, trigger, is_enabled, created_at)
VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),(SELECT id FROM contracts WHERE tenant_id=$2 AND key=$7 AND version=$8),$8,$9,$10,$11,$12,$13, now() + ($14 * interval '1 millisecond'))`,
				sid, p.TenantID, id, st.Key, strings.TrimSpace(st.Title), st.Description, def.Key, def.Version, []byte(norm[i]), st.DelaySeconds, st.TriggerMode, trig, st.IsEnabled, i); err != nil {
				return err
			}
		}
		for _, e := range in.Edges {
			rule, _ := json.Marshal(e)
			if _, err := tx.Exec(ctx, `INSERT INTO scenario_step_edges (tenant_id, scenario_id, parent_step_id, child_step_id,rule) VALUES ($1,$2,$3,$4,$5)`,
				p.TenantID, id, stepID[e.Parent], stepID[e.Child], rule); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE scenarios SET version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "scenario.graph", ResourceType: "scenario", ResourceID: &id, IP: ip, RequestID: reqID,
			Detail: map[string]any{"steps": len(in.Steps), "edges": len(in.Edges)}})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
