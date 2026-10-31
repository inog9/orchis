package ai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/auth"
	"orchis-go/internal/httpx"
	"time"
)

type Claim struct {
	Job             Job    `json:"job"`
	Token           string `json:"token"`
	APIKey          string `json:"api_key"`
	MaxOutputTokens int    `json:"max_output_tokens"`
}

func (s *Service) Claim(ctx context.Context) (*Claim, error) {
	// Tidak mengulang panggilan provider setelah lease hilang: biaya/hasil panggilan lama mungkin tidak diketahui.
	if e := s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE ai_jobs SET status='failed',error='Worker lease expired. Review provider usage before requesting again.',claim_hash=NULL,version=version+1,updated_at=now() WHERE tenant_id=$1 AND status='running' AND lease_until<now()`, s.Tenant)
		return e
	}); e != nil {
		return nil, e
	}
	var claim *Claim
	e := s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		var org uuid.UUID
		e := tx.QueryRow(ctx, `SELECT p.organization_id FROM ai_policies p WHERE p.tenant_id=$1 AND p.enabled AND EXISTS(SELECT 1 FROM ai_jobs j WHERE j.tenant_id=p.tenant_id AND j.organization_id=p.organization_id AND j.status='queued' AND j.policy_version=p.version) AND NOT EXISTS(SELECT 1 FROM ai_jobs j WHERE j.tenant_id=p.tenant_id AND j.organization_id=p.organization_id AND (j.status='running' OR (j.status IN('review','completed','failed') AND j.updated_at>now()-interval '10 seconds'))) ORDER BY p.organization_id FOR UPDATE OF p SKIP LOCKED LIMIT 1`, s.Tenant).Scan(&org)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		// Snapshot kandidat bisa mendahului commit worker lain. Periksa lagi setelah policy terkunci.
		var busy bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ai_jobs WHERE tenant_id=$1 AND organization_id=$2 AND (status='running' OR (status IN('review','completed','failed') AND updated_at>now()-interval '10 seconds')))`, s.Tenant, org).Scan(&busy); e != nil {
			return e
		}
		if busy {
			return nil
		}
		j, e := scanJob(tx.QueryRow(ctx, `SELECT `+jobCols+` FROM ai_jobs WHERE tenant_id=$1 AND organization_id=$2 AND status='queued' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`, s.Tenant, org))
		if e != nil {
			return e
		}
		pol, e := readPolicy(ctx, tx, s.Tenant, org, false)
		if e != nil {
			return e
		}
		if pol.Version != j.PolicyVersion {
			return httpx.Conflict("AI consent changed.")
		}
		var enc []byte
		var limit int
		if e = tx.QueryRow(ctx, `SELECT p.secrets_enc,j.max_output_tokens FROM ai_policies p JOIN ai_jobs j ON j.tenant_id=p.tenant_id AND j.organization_id=p.organization_id WHERE j.tenant_id=$1 AND j.id=$2`, s.Tenant, j.ID).Scan(&enc, &limit); e != nil {
			return e
		}
		plain, e := Open(s.Key, enc, s.Tenant.String()+"/"+org.String())
		if e != nil {
			return errors.New("AI credentials unavailable")
		}
		var k keys
		if e = json.Unmarshal(plain, &k); e != nil {
			return e
		}
		key := k.DeepSeek
		if j.Provider == "openai" {
			key = k.OpenAI
		}
		if key == "" {
			return errors.New("AI provider credential unavailable")
		}
		tokenBytes := make([]byte, 32)
		if _, e = rand.Read(tokenBytes); e != nil {
			return e
		}
		token := hex.EncodeToString(tokenBytes)
		if _, e = tx.Exec(ctx, `UPDATE ai_jobs SET status='running',claim_hash=$3,lease_until=now()+interval '90 seconds',version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2`, s.Tenant, j.ID, digest([]byte(token))); e != nil {
			return e
		}
		var actor uuid.UUID
		if e = tx.QueryRow(ctx, `SELECT created_by FROM ai_jobs WHERE tenant_id=$1 AND id=$2`, s.Tenant, j.ID).Scan(&actor); e != nil {
			return e
		}
		p := &auth.Principal{TenantID: s.Tenant, UserID: actor}
		if e = note(ctx, tx, p, org, j.ID, "ai.call", map[string]any{"provider": j.Provider, "purpose": j.Kind, "reserved_tokens": j.ReservedTokens, "input_bytes": j.InputBytes}); e != nil {
			return e
		}
		j.Status = "running"
		j.Version++
		claim = &Claim{Job: j, Token: token, APIKey: key, MaxOutputTokens: limit}
		return nil
	})
	return claim, e
}
func (s *Service) Alive(ctx context.Context, id uuid.UUID, token string) (bool, error) {
	var alive bool
	e := s.DB.InTenantRead(ctx, s.Tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ai_jobs j JOIN ai_policies p ON p.tenant_id=j.tenant_id AND p.organization_id=j.organization_id WHERE j.tenant_id=$1 AND j.id=$2 AND j.status='running' AND j.claim_hash=$3 AND j.lease_until>now() AND p.enabled AND p.version=j.policy_version)`, s.Tenant, id, digest([]byte(token))).Scan(&alive)
	})
	return alive, e
}

type Completion struct {
	Token       string          `json:"token"`
	Output      json.RawMessage `json:"output"`
	Failure     string          `json:"failure"`
	UsageTokens int             `json:"usage_tokens"`
}

func (s *Service) Complete(ctx context.Context, id uuid.UUID, c Completion) error {
	p := &auth.Principal{TenantID: s.Tenant}
	j, e := s.Get(ctx, p, id)
	if e != nil {
		return e
	}
	var out Output
	failure := ""
	if c.Failure != "" {
		failure = "AI provider request failed. Check worker configuration and provider usage before trying again."
	} else {
		out, e = ValidateOutput(c.Output, j.Input, j.Kind)
		if e != nil {
			failure = "AI response rejected: invalid schema, mapping or action. No actions were queued."
		}
	}
	if c.UsageTokens < 0 || c.UsageTokens > j.ReservedTokens {
		failure = "Provider usage exceeded the reserved token bound. Review usage before retrying."
	}
	return s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		pol, e := readPolicy(ctx, tx, s.Tenant, j.OrganizationID, true)
		if e != nil {
			return e
		}
		var hash, status string
		var until time.Time
		var actor uuid.UUID
		if e = tx.QueryRow(ctx, `SELECT COALESCE(claim_hash,''),status,COALESCE(lease_until,now()-interval '1 second'),created_by FROM ai_jobs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, s.Tenant, id).Scan(&hash, &status, &until, &actor); e != nil {
			return e
		}
		if status != "running" || hash != digest([]byte(c.Token)) || !until.After(time.Now()) || !pol.Enabled || pol.Version != j.PolicyVersion {
			return httpx.Conflict("AI job is no longer claimable; result discarded.")
		}
		status = "review"
		if j.Kind == "remediation" || len(out.Actions) == 0 {
			status = "completed"
		}
		var raw []byte
		if failure != "" {
			status = "failed"
		} else {
			raw, _ = json.Marshal(out)
		}
		if _, e = tx.Exec(ctx, `UPDATE ai_jobs SET status=$3,output=$4,error=$5,usage_tokens=$6,claim_hash=NULL,lease_until=NULL,version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2`, s.Tenant, id, status, raw, failure, c.UsageTokens); e != nil {
			return e
		}
		p.UserID = actor
		return note(ctx, tx, p, j.OrganizationID, id, "ai.result", map[string]any{"provider": j.Provider, "status": status, "usage_tokens": c.UsageTokens, "output_bytes": len(c.Output)})
	})
}
