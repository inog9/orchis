package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/contract"
	"orchis-go/internal/db"
	"orchis-go/internal/execution"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/reporting"
	"orchis-go/internal/scancontrol"
	"time"
)

type Service struct {
	Now         func() time.Time
	DB          *db.DB
	Tenant      uuid.UUID
	Key         []byte
	WorkerToken string
	Guard       contract.Guard
}
type Policy struct {
	IsDemo           bool   `json:"is_demo"`
	Version          int    `json:"version"`
	Enabled          bool   `json:"enabled"`
	Planning         bool   `json:"planning"`
	Remediation      bool   `json:"remediation"`
	PlanningModel    string `json:"planning_model"`
	RemediationModel string `json:"remediation_model"`
	MaxInputBytes    int    `json:"max_input_bytes"`
	MaxOutputTokens  int    `json:"max_output_tokens"`
	DailyTokens      int    `json:"daily_tokens"`
	DailyCalls       int    `json:"daily_calls"`
	DailyCostMicros  int64  `json:"daily_cost_micros"`
	InputRateMicros  int64  `json:"input_rate_micros"`
	OutputRateMicros int64  `json:"output_rate_micros"`
	HasKeys          bool   `json:"has_keys"`
	Configured       bool   `json:"configured"`
}
type PolicyInput struct {
	Policy
	DeepSeekKey string `json:"deepseek_key"`
	OpenAIKey   string `json:"openai_key"`
	Consent     bool   `json:"consent"`
}
type keys struct {
	DeepSeek string `json:"deepseek"`
	OpenAI   string `json:"openai"`
}

func defaults() Policy {
	return Policy{MaxInputBytes: 32000, MaxOutputTokens: 2000, DailyTokens: 20000, DailyCalls: 10}
}

const policyCols = `is_demo,version,enabled,planning,remediation,planning_model,remediation_model,max_input_bytes,max_output_tokens,daily_tokens,daily_calls,daily_cost_micros,input_rate_micros,output_rate_micros,secrets_enc IS NOT NULL`

func readPolicy(ctx context.Context, tx pgx.Tx, tenant, org uuid.UUID, lock bool) (Policy, error) {
	p := defaults()
	sql := `SELECT ` + policyCols + ` FROM ai_policies WHERE tenant_id=$1 AND organization_id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	e := tx.QueryRow(ctx, sql, tenant, org).Scan(&p.IsDemo, &p.Version, &p.Enabled, &p.Planning, &p.Remediation, &p.PlanningModel, &p.RemediationModel, &p.MaxInputBytes, &p.MaxOutputTokens, &p.DailyTokens, &p.DailyCalls, &p.DailyCostMicros, &p.InputRateMicros, &p.OutputRateMicros, &p.HasKeys)
	if errors.Is(e, pgx.ErrNoRows) {
		return defaults(), nil
	}
	return p, e
}
func orgExists(ctx context.Context, tx pgx.Tx, tenant, org uuid.UUID) error {
	var id uuid.UUID
	if e := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE tenant_id=$1 AND id=$2`, tenant, org).Scan(&id); errors.Is(e, pgx.ErrNoRows) {
		return httpx.NotFound("Organisation")
	} else {
		return e
	}
}
func (s *Service) Policy(ctx context.Context, p *auth.Principal, org uuid.UUID) (Policy, error) {
	var out Policy
	e := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		if e := orgExists(ctx, tx, p.TenantID, org); e != nil {
			return e
		}
		var e error
		out, e = readPolicy(ctx, tx, p.TenantID, org, false)
		return e
	})
	if out.IsDemo {
		out.Enabled = false
	}
	out.Configured = !out.IsDemo && len(s.Key) == 32 && len(s.WorkerToken) >= 32
	return out, e
}
func note(ctx context.Context, tx pgx.Tx, p *auth.Principal, org, id uuid.UUID, action string, detail map[string]any) error {
	if detail == nil {
		detail = map[string]any{}
	}
	if org != uuid.Nil {
		detail["organization_id"] = org
	}
	return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: action, ResourceType: "ai", ResourceID: &id, Detail: detail})
}
func (s *Service) SavePolicy(ctx context.Context, p *auth.Principal, org uuid.UUID, in PolicyInput) (Policy, error) {
	current, err := s.Policy(ctx, p, org)
	if err != nil {
		return Policy{}, err
	}
	if current.IsDemo {
		return Policy{}, httpx.Conflict("Synthetic AI policy is read-only. No external provider can be enabled for this fixture.")
	}

	if in.MaxInputBytes < 1024 || in.MaxInputBytes > 64000 || in.MaxOutputTokens < 128 || in.MaxOutputTokens > 4096 || in.DailyTokens < 128 || in.DailyTokens > 1000000 || in.DailyCalls < 1 || in.DailyCalls > 1000 || in.DailyCostMicros < 0 || in.DailyCostMicros > 100000000 || in.InputRateMicros < 0 || in.InputRateMicros > 1000000000 || in.OutputRateMicros < 0 || in.OutputRateMicros > 1000000000 {
		return Policy{}, httpx.BadRequest("Invalid AI limits.")
	}
	if in.Enabled && (!in.Consent || (!in.Planning && !in.Remediation) || in.DailyCostMicros == 0 || in.InputRateMicros == 0 || in.OutputRateMicros == 0 || len(s.Key) != 32 || len(s.WorkerToken) < 32) {
		return Policy{}, httpx.Conflict("Explicit consent, encryption, worker credentials and positive cost limits/rates are required.")
	}
	if (in.Planning && !modelRE.MatchString(in.PlanningModel)) || (in.Remediation && !modelRE.MatchString(in.RemediationModel)) {
		return Policy{}, httpx.BadRequest("Specify an exact model identifier for each enabled purpose.")
	}
	e := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
		if e := orgExists(ctx, tx, p.TenantID, org); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO ai_policies(tenant_id,organization_id,version) VALUES($1,$2,0) ON CONFLICT DO NOTHING`, p.TenantID, org); e != nil {
			return e
		}
		old, e := readPolicy(ctx, tx, p.TenantID, org, true)
		if e != nil {
			return e
		}
		if old.IsDemo {
			return httpx.Conflict("Synthetic AI policy is read-only.")
		}
		if old.Version != in.Version {
			return httpx.Conflict("AI settings changed. Reload and review consent again.")
		}
		var enc []byte
		if e = tx.QueryRow(ctx, `SELECT secrets_enc FROM ai_policies WHERE tenant_id=$1 AND organization_id=$2`, p.TenantID, org).Scan(&enc); e != nil {
			return e
		}
		k := keys{}
		aad := p.TenantID.String() + "/" + org.String()
		if len(enc) > 0 && (in.Enabled || in.DeepSeekKey != "" || in.OpenAIKey != "") {
			plain, e := Open(s.Key, enc, aad)
			if e != nil {
				return errors.New("AI credentials could not be decrypted")
			}
			if e = json.Unmarshal(plain, &k); e != nil {
				return e
			}
		}
		if in.DeepSeekKey != "" {
			if !validText(in.DeepSeekKey, 512) {
				return httpx.BadRequest("Invalid credential.")
			}
			k.DeepSeek = in.DeepSeekKey
		}
		if in.OpenAIKey != "" {
			if !validText(in.OpenAIKey, 512) {
				return httpx.BadRequest("Invalid credential.")
			}
			k.OpenAI = in.OpenAIKey
		}
		if in.Enabled && ((in.Planning && k.DeepSeek == "") || (in.Remediation && k.OpenAI == "")) {
			return httpx.Conflict("Configure credentials for every enabled provider.")
		}
		if k.DeepSeek != "" || k.OpenAI != "" {
			b, _ := json.Marshal(k)
			enc, e = Seal(s.Key, b, aad)
			if e != nil {
				return e
			}
		}
		_, e = tx.Exec(ctx, `UPDATE ai_policies SET version=version+1,enabled=$3,planning=$4,remediation=$5,planning_model=$6,remediation_model=$7,max_input_bytes=$8,max_output_tokens=$9,daily_tokens=$10,daily_calls=$11,daily_cost_micros=$12,input_rate_micros=$13,output_rate_micros=$14,secrets_enc=$15,consent_by=$16,consent_at=CASE WHEN $3 THEN now() ELSE NULL END,updated_at=now() WHERE tenant_id=$1 AND organization_id=$2`, p.TenantID, org, in.Enabled, in.Planning, in.Remediation, in.PlanningModel, in.RemediationModel, in.MaxInputBytes, in.MaxOutputTokens, in.DailyTokens, in.DailyCalls, in.DailyCostMicros, in.InputRateMicros, in.OutputRateMicros, enc, p.UserID)
		if e != nil {
			return e
		}
		// Setiap perubahan kebijakan mencabut pekerjaan yang belum selesai, termasuk klaim lama.
		if _, e = tx.Exec(ctx, `UPDATE ai_jobs SET status='canceled',claim_hash=NULL,version=version+1,error='Organisation AI policy changed.',updated_at=now() WHERE tenant_id=$1 AND organization_id=$2 AND status IN('queued','running','review')`, p.TenantID, org); e != nil {
			return e
		}
		return note(ctx, tx, p, org, org, "ai.policy", map[string]any{"enabled": in.Enabled, "planning": in.Planning, "remediation": in.Remediation, "version": in.Version + 1})
	})
	if e != nil {
		return Policy{}, e
	}
	return s.Policy(ctx, p, org)
}

type Job struct {
	IsDemo             bool      `json:"is_demo"`
	ID                 uuid.UUID `json:"id"`
	OrganizationID     uuid.UUID `json:"organization_id"`
	ReportID           uuid.UUID `json:"report_id"`
	Kind               string    `json:"kind"`
	Provider           string    `json:"provider"`
	Model              string    `json:"model"`
	Status             string    `json:"status"`
	Version            int       `json:"version"`
	PolicyVersion      int       `json:"policy_version"`
	PromptVersion      int       `json:"prompt_version"`
	Instructions       string    `json:"instructions"`
	Input              Input     `json:"input"`
	Output             *Output   `json:"output"`
	Error              string    `json:"error"`
	ReservedTokens     int       `json:"reserved_tokens"`
	ReservedCostMicros int64     `json:"reserved_cost_micros"`
	CreatedAt          time.Time `json:"created_at"`
	InputBytes         int       `json:"input_bytes"`
	References         Mapping   `json:"references"`
}

const jobCols = `is_demo,id,organization_id,report_id,kind,provider,model,status,version,policy_version,prompt_version,instructions,input,output,error,reserved_tokens,reserved_cost_micros,input_bytes,created_at`

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	var in, out []byte
	e := row.Scan(&j.IsDemo, &j.ID, &j.OrganizationID, &j.ReportID, &j.Kind, &j.Provider, &j.Model, &j.Status, &j.Version, &j.PolicyVersion, &j.PromptVersion, &j.Instructions, &in, &out, &j.Error, &j.ReservedTokens, &j.ReservedCostMicros, &j.InputBytes, &j.CreatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return j, httpx.NotFound("AI job")
	}
	if e != nil {
		return j, e
	}
	if e = json.Unmarshal(in, &j.Input); e != nil {
		return j, e
	}
	if len(out) > 0 {
		e = json.Unmarshal(out, &j.Output)
	}
	return j, e
}
func (s *Service) Get(ctx context.Context, p *auth.Principal, id uuid.UUID) (Job, error) {
	var j Job
	e := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		var e error
		j, e = scanJob(tx.QueryRow(ctx, `SELECT `+jobCols+` FROM ai_jobs WHERE tenant_id=$1 AND id=$2`, p.TenantID, id))
		if e != nil {
			return e
		}
		var refs []byte
		if e = tx.QueryRow(ctx, `SELECT mapping FROM ai_jobs WHERE tenant_id=$1 AND id=$2`, p.TenantID, id).Scan(&refs); e != nil {
			return e
		}
		return json.Unmarshal(refs, &j.References)
	})
	return j, e
}
func (s *Service) List(ctx context.Context, p *auth.Principal, org uuid.UUID, before string) ([]Job, error) {
	list := []Job{}
	var cursor uuid.UUID
	if before != "" {
		var e error
		cursor, e = uuid.Parse(before)
		if e != nil {
			return nil, httpx.BadRequest("Invalid cursor.")
		}
	}
	e := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		if e := orgExists(ctx, tx, p.TenantID, org); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+jobCols+` FROM ai_jobs WHERE tenant_id=$1 AND organization_id=$2 AND ($3::uuid='00000000-0000-0000-0000-000000000000' OR id<$3) ORDER BY id DESC LIMIT 50`, p.TenantID, org, cursor)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			j, e := scanJob(rows)
			if e != nil {
				return e
			}
			list = append(list, j)
		}
		return rows.Err()
	})
	return list, e
}

type Request struct {
	ReportID   uuid.UUID  `json:"report_id"`
	Kind       string     `json:"kind"`
	TemplateID *uuid.UUID `json:"template_id"`
}

func (s *Service) Preview(ctx context.Context, p *auth.Principal, org, report uuid.UUID) (Input, Mapping, error) {
	r, e := (&reporting.Service{DB: s.DB}).Get(ctx, p.TenantID, report)
	if e != nil {
		return Input{}, Mapping{}, e
	}
	if r.OrganizationID != org {
		return Input{}, Mapping{}, httpx.NotFound("Report")
	}
	return Minimize(r.Snapshot)
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (s *Service) Create(ctx context.Context, p *auth.Principal, org uuid.UUID, r Request, key string) (Job, error) {
	if (r.Kind != "planning" && r.Kind != "remediation") || len(key) == 0 || len(key) > 120 {
		return Job{}, httpx.BadRequest("Choose a purpose and provide an Idempotency-Key (1–120 characters).")
	}
	input, mapping, e := s.Preview(ctx, p, org, r.ReportID)
	if e != nil {
		return Job{}, e
	}
	data, _ := json.Marshal(input)
	refs, _ := json.Marshal(mapping)
	req, _ := json.Marshal(r)
	hash := digest(req)
	id := ids.New()
	e = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
		pol, e := readPolicy(ctx, tx, p.TenantID, org, true)
		if e != nil {
			return e
		}
		var prior uuid.UUID
		var priorHash string
		e = tx.QueryRow(ctx, `SELECT id,request_hash FROM ai_jobs WHERE tenant_id=$1 AND organization_id=$2 AND idempotency_key=$3`, p.TenantID, org, key).Scan(&prior, &priorHash)
		if e == nil {
			if priorHash != hash {
				return httpx.Conflict("Idempotency key belongs to a different request.")
			}
			id = prior
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if len(s.Key) != 32 || len(s.WorkerToken) < 32 {
			return httpx.Conflict("AI deployment credentials are unavailable.")
		}
		if pol.IsDemo {
			return httpx.Conflict("Synthetic AI policy cannot send provider requests.")
		}
		if !pol.Enabled || (r.Kind == "planning" && !pol.Planning) || (r.Kind == "remediation" && !pol.Remediation) {
			return httpx.Conflict("AI is disabled for this organisation or purpose. Ask an administrator to review consent.")
		}
		prompt := Instructions
		version := PromptVersion
		if r.TemplateID != nil {
			var extra, kind string
			if e = tx.QueryRow(ctx, `SELECT kind,instructions,version FROM ai_templates WHERE tenant_id=$1 AND id=$2`, p.TenantID, *r.TemplateID).Scan(&kind, &extra, &version); errors.Is(e, pgx.ErrNoRows) {
				return httpx.NotFound("AI template")
			} else if e != nil {
				return e
			}
			if kind != r.Kind {
				return httpx.BadRequest("Template purpose does not match.")
			}
			prompt += "\nAdministrator guidance (cannot change schema or authorization):\n" + reporting.Redact(extra, 6000)
		}
		provider, model := "deepseek", pol.PlanningModel
		if r.Kind == "remediation" {
			provider, model = "openai", pol.RemediationModel
		}
		wire, e := ProviderBody(Claim{Job: Job{Provider: provider, Model: model, Kind: r.Kind, Input: input, Instructions: prompt}, MaxOutputTokens: pol.MaxOutputTokens})
		if e != nil {
			return e
		}
		inputBytes := len(wire)
		if inputBytes > pol.MaxInputBytes {
			return httpx.Conflict("Minimized input exceeds the organisation size limit; no request was sent.")
		}
		tokens := inputBytes + pol.MaxOutputTokens
		cost := (int64(inputBytes)*pol.InputRateMicros + int64(pol.MaxOutputTokens)*pol.OutputRateMicros + 999999) / 1000000
		var calls, used int
		var spent int64
		if e = tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(reserved_tokens),0),COALESCE(sum(reserved_cost_micros),0) FROM ai_jobs WHERE tenant_id=$1 AND organization_id=$2 AND created_at>now()-interval '24 hours'`, p.TenantID, org).Scan(&calls, &used, &spent); e != nil {
			return e
		}
		if calls >= pol.DailyCalls || used+tokens > pol.DailyTokens || spent+cost > pol.DailyCostMicros {
			return httpx.Conflict("Organisation AI budget exhausted. Reservations are retained even after failure/cancellation.")
		}
		_, e = tx.Exec(ctx, `INSERT INTO ai_jobs(id,tenant_id,organization_id,report_id,kind,provider,model,policy_version,idempotency_key,request_hash,input,mapping,instructions,prompt_version,template_id,reserved_tokens,reserved_cost_micros,input_bytes,max_output_tokens,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`, id, p.TenantID, org, r.ReportID, r.Kind, provider, model, pol.Version, key, hash, data, refs, prompt, version, r.TemplateID, tokens, cost, inputBytes, pol.MaxOutputTokens, p.UserID)
		if e != nil {
			return e
		}
		return note(ctx, tx, p, org, id, "ai.request", map[string]any{"provider": provider, "purpose": r.Kind, "input_bytes": inputBytes, "reserved_tokens": tokens, "reserved_cost_micros": cost, "prompt_version": version})
	})
	if e != nil {
		return Job{}, e
	}
	return s.Get(ctx, p, id)
}
func (s *Service) Cancel(ctx context.Context, p *auth.Principal, id uuid.UUID, version int) (Job, error) {
	j, e := s.Get(ctx, p, id)
	if e != nil {
		return j, e
	}
	if j.Status == "approved" && !authz.Has(p.Permissions, authz.AtomicRun) {
		return j, httpx.Forbidden(authz.AtomicRun)
	}
	e = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
		tag, e := tx.Exec(ctx, `UPDATE ai_jobs SET status='canceled',claim_hash=NULL,version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND version=$3 AND NOT is_demo AND status IN('queued','running','review','approved')`, p.TenantID, id, version)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return httpx.Conflict("AI job changed or is already final. Reload it.")
		}
		if _, e = tx.Exec(ctx, `UPDATE executions SET state='ended',outcome='canceled',ended_at=now(),lease_until=NULL,version=version+1,updated_at=now() WHERE tenant_id=$1 AND state IN('ready','running') AND id IN(SELECT execution_id FROM ai_job_actions WHERE tenant_id=$1 AND job_id=$2)`, p.TenantID, id); e != nil {
			return e
		}
		return note(ctx, tx, p, j.OrganizationID, id, "ai.cancel", nil)
	})
	if e != nil {
		return j, e
	}
	return s.Get(ctx, p, id)
}

// Approve mengantrekan tindakan secara atomik. Semua guard Run lama tetap dijalankan dalam transaksi ini.
func (s *Service) Approve(ctx context.Context, p *auth.Principal, id uuid.UUID, version int) (Job, error) {
	for _, perm := range []string{authz.AtomicWrite, authz.AtomicRun, authz.AssetRead} {
		if !authz.Has(p.Permissions, perm) {
			return Job{}, httpx.Forbidden(perm)
		}
	}
	j, e := s.Get(ctx, p, id)
	if e != nil {
		return j, e
	}
	e = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
		pol, e := readPolicy(ctx, tx, p.TenantID, j.OrganizationID, true)
		if e != nil {
			return e
		}
		j, e = scanJob(tx.QueryRow(ctx, `SELECT `+jobCols+` FROM ai_jobs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, id))
		if e != nil {
			return e
		}
		if j.IsDemo {
			return httpx.Conflict("Synthetic AI plans cannot dispatch checks.")
		}
		if j.Status == "approved" {
			return nil
		}
		if j.Version != version || j.Status != "review" || !pol.Enabled || pol.Version != j.PolicyVersion {
			return httpx.Conflict("Review changed or consent was revoked. Reload before approving.")
		}
		raw, _ := json.Marshal(j.Output)
		out, e := ValidateOutput(raw, j.Input, j.Kind)
		if e != nil {
			return httpx.Conflict("Stored plan no longer validates.")
		}
		var refs []byte
		var m Mapping
		if e = tx.QueryRow(ctx, `SELECT mapping FROM ai_jobs WHERE tenant_id=$1 AND id=$2`, p.TenantID, id).Scan(&refs); e != nil {
			return e
		}
		if e = json.Unmarshal(refs, &m); e != nil {
			return e
		}
		for i, a := range out.Actions {
			asset, e := uuid.Parse(m.Assets[a.AssetID])
			if e != nil {
				return e
			}
			var member bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM assets WHERE tenant_id=$1 AND id=$2 AND organization_id=$3 AND deleted_at IS NULL)`, p.TenantID, asset, j.OrganizationID).Scan(&member); e != nil {
				return e
			}
			if !member {
				return httpx.Conflict("A proposed asset moved or was deleted; no actions were queued.")
			}
			testID, execID := ids.New(), ids.New()
			_, e = tx.Exec(ctx, `INSERT INTO atomic_tests(id,tenant_id,name,contract_id,contract_version,params,target_asset_id,created_by) VALUES($1,$2,$3,(SELECT id FROM contracts WHERE tenant_id=$2 AND key=$4 AND version=$5),$5,$6,$7,$8)`, testID, p.TenantID, fmt.Sprintf("AI reviewed %s action %d", id.String()[:8], i+1), a.ContractKey, a.Version, a.Params, asset, p.UserID)
			if e != nil {
				return e
			}
			if e = (&execution.Service{DB: s.DB, Guard: s.Guard, Now: s.Now}).QueueTest(ctx, tx, p, testID, execID, "", ""); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO ai_job_actions(tenant_id,job_id,ordinal,asset_id,atomic_test_id,execution_id) VALUES($1,$2,$3,$4,$5,$6)`, p.TenantID, id, i, asset, testID, execID); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE ai_jobs SET status='approved',approved_by=$3,approved_at=now(),version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id, p.UserID); e != nil {
			return e
		}
		return note(ctx, tx, p, j.OrganizationID, id, "ai.approve", map[string]any{"actions": len(out.Actions)})
	})
	if e != nil {
		return j, e
	}
	return s.Get(ctx, p, id)
}
