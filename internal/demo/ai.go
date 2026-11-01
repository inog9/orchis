package demo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/ai"
	"orchis-go/internal/httpx"
	"orchis-go/internal/reporting"
)

func (g *generator) seedAI(ctx context.Context, tx pgx.Tx) error {
	provider, close, err := newLocalFakeAI()
	if err != nil {
		return err
	}
	defer close()
	templates := map[string]uuid.UUID{}
	versions := map[string]int{}
	texts := map[string]string{}
	for _, kind := range []string{"planning", "remediation"} {
		for revision := 1; revision <= 2; revision++ {
			id := g.id("ai-template", kind, fmt.Sprint(revision))
			instructions := fmt.Sprintf("DEMO / SYNTHETIC %s prompt revision %d. Local fake API only; output is an illustrative placeholder, never a real recommendation.", kind, revision)
			if _, err := tx.Exec(ctx, `INSERT INTO ai_templates(id,tenant_id,kind,version,instructions,created_by,is_demo)
SELECT $1,$2,$3,COALESCE(max(version),0)+1,$4,$5,true FROM ai_templates WHERE tenant_id=$2 AND kind=$3 ON CONFLICT(id) DO NOTHING`, id, g.tenant, kind, instructions, g.createdBy); err != nil {
				return err
			}
			var version int
			if err := tx.QueryRow(ctx, `SELECT version,instructions FROM ai_templates WHERE tenant_id=$1 AND id=$2 AND is_demo`, g.tenant, id).Scan(&version, &instructions); err != nil {
				return err
			}
			if revision == 2 {
				templates[kind] = id
				versions[kind] = version
				texts[kind] = instructions
			}
		}
	}
	for oi := 0; oi < orgCount; oi++ {
		org := g.id("org", fmt.Sprint(oi))
		report := g.id("org", fmt.Sprint(oi), "report", "2")
		if _, err := tx.Exec(ctx, `INSERT INTO ai_policies(tenant_id,organization_id,enabled,planning,remediation,planning_model,remediation_model,consent_by,consent_at,is_demo)
VALUES($1,$2,false,true,true,'demo-planner','demo-remediation',$3,$4,true) ON CONFLICT(tenant_id,organization_id) DO NOTHING`, g.tenant, org, g.createdBy, g.now); err != nil {
			return err
		}
		var owned bool
		if err := tx.QueryRow(ctx, `SELECT is_demo FROM ai_policies WHERE tenant_id=$1 AND organization_id=$2`, g.tenant, org).Scan(&owned); err != nil {
			return err
		}
		if !owned {
			return httpx.Conflict("A real AI policy already uses a demo organisation; no settings were overwritten.")
		}
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT snapshot FROM reports WHERE tenant_id=$1 AND id=$2 AND is_demo`, g.tenant, report).Scan(&raw); err != nil {
			return err
		}
		var snapshot reporting.Snapshot
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			return err
		}
		in, mapping, err := ai.Minimize(snapshot)
		if err != nil {
			return err
		}
		input, _ := json.Marshal(in)
		refs, _ := json.Marshal(mapping)
		for _, state := range []string{"review", "completed", "rejected", "canceled"} {
			id := g.id("ai-job", fmt.Sprint(oi), state)
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ai_jobs WHERE tenant_id=$1 AND id=$2 AND is_demo)`, g.tenant, id).Scan(&exists); err != nil {
				return err
			}
			if exists {
				continue
			}
			kind, upstream, model := "planning", "deepseek", "demo-planner"
			if state == "completed" {
				kind, upstream, model = "remediation", "openai", "demo-remediation"
			}
			instructions := ai.Instructions + "\n" + texts[kind]
			claim := ai.Claim{APIKey: fakeAPIKey, MaxOutputTokens: 512, Job: ai.Job{IsDemo: true, Kind: kind, Provider: upstream, Model: model, Input: in, Instructions: instructions, PromptVersion: versions[kind]}}
			out, usage, err := localFakeOutput(ctx, provider, claim)
			if err != nil {
				return err
			}
			output, _ := json.Marshal(out)
			wire, err := ai.ProviderBody(claim)
			if err != nil {
				return err
			}
			marker := "demo-local-fake-" + state + "-" + fmt.Sprint(g.seed)
			if _, err := tx.Exec(ctx, `INSERT INTO ai_jobs(id,tenant_id,organization_id,report_id,kind,provider,model,policy_version,status,idempotency_key,request_hash,input,mapping,instructions,prompt_version,template_id,output,reserved_tokens,reserved_cost_micros,input_bytes,max_output_tokens,usage_tokens,created_by,is_demo)
VALUES($1,$2,$3,$4,$5,$6,$7,1,$8,$9,$19,$10,$11,$12,$13,$14,$15,4096,0,$16,512,$17,$18,true)`, id, g.tenant, org, report, kind, upstream, model, state, marker, input, refs, instructions, versions[kind], templates[kind], output, len(wire), usage, g.createdBy, fmt.Sprintf("%x", hash(marker))); err != nil {
				return err
			}
		}
	}
	return nil
}
