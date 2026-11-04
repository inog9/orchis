package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/contract"
	"orchis-go/internal/groups"
)

// seedRelated creates review fixtures, never usable identities or worker credentials.
// Existing built-in roles, contracts and integration health are left intact.
func (g *generator) seedRelated(ctx context.Context, tx pgx.Tx) error {
	for _, key := range []string{"admin", "analyst", "viewer"} {
		user := g.id("user", key)
		var role uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE tenant_id=$1 AND key=$2 AND is_system`, g.tenant, key).Scan(&role); err != nil {
			return fmt.Errorf("demo requires built-in role %s: %w", key, err)
		}

		if _, err := tx.Exec(ctx, `INSERT INTO users(id,tenant_id,email,display_name,role_id,status,password_hash,is_demo)
VALUES($1,$2,$3,$4,$5,'disabled',NULL,true) ON CONFLICT(id) DO NOTHING`, user, g.tenant, "demo-"+key+"@example.invalid", "Demo: "+key, role); err != nil {
			return err
		}
		// Deliberately invalidated credential examples; only hashes of public fixture labels exist.
		if _, err := tx.Exec(ctx, `INSERT INTO api_tokens(id,tenant_id,user_id,name,token_prefix,token_hash,created_at,expires_at,revoked_at,is_demo)
VALUES($1,$2,$3,'Demo: revoked synthetic API token','DEMO', $4,$5,$6,$6,true) ON CONFLICT(id) DO NOTHING`, g.id("api-token", key), g.tenant, user, hash("inert-demo-api|"+user.String()), g.now.Add(-48*time.Hour), g.now.Add(-24*time.Hour)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO password_reset_links(id,tenant_id,user_id,created_by,token_hash,created_at,expires_at,used_at,is_demo)
VALUES($1,$2,$3,$4,$5,$6,$7,$7,true) ON CONFLICT(id) DO NOTHING`, g.id("reset", key), g.tenant, user, g.createdBy, hash("inert-demo-reset|"+user.String()), g.now.Add(-48*time.Hour), g.now.Add(-47*time.Hour)); err != nil {
			return err
		}
	}
	// Show a revoked demo token in the real operator's Profile without creating
	// an active credential or assigning demo permissions to the operator.
	if _, err := tx.Exec(ctx, `INSERT INTO api_tokens(id,tenant_id,user_id,name,token_prefix,token_hash,created_at,expires_at,revoked_at,is_demo)
 VALUES($1,$2,$3,'Demo: revoked profile example','DEMO',$4,$5,$6,$6,true) ON CONFLICT(id) DO NOTHING`, g.id("api-token", "operator"), g.tenant, g.createdBy, hash("inert-demo-operator-api|"+g.tenant.String()), g.now.Add(-48*time.Hour), g.now.Add(-24*time.Hour)); err != nil {
		return err
	}

	for i := 0; i < 3; i++ {
		team := g.id("team", fmt.Sprint(i))
		if _, err := tx.Exec(ctx, `INSERT INTO teams(id,tenant_id,name,description,kind,is_demo) VALUES($1,$2,$3,'Synthetic training participants; no messages are sent.',$4,true) ON CONFLICT(id) DO NOTHING`, team, g.tenant, fmt.Sprintf("Demo: response team %d", i+1), []string{"internal", "external", "internal"}[i]); err != nil {
			return err
		}
		for j := 0; j < 2; j++ {
			person := g.id("person", fmt.Sprint(i), fmt.Sprint(j))
			if _, err := tx.Exec(ctx, `INSERT INTO people(id,tenant_id,first_name,last_name,email,organization,is_demo) VALUES($1,$2,'Demo',$3,$4,'Fictional training organisation',true) ON CONFLICT(id) DO NOTHING`, person, g.tenant, fmt.Sprintf("Participant %d-%d", i+1, j+1), fmt.Sprintf("demo-person-%d-%d@example.invalid", i, j)); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO team_members(tenant_id,team_id,person_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, g.tenant, team, person); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO scenario_teams(tenant_id,scenario_id,team_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, g.tenant, g.id("scenario", fmt.Sprint(i)), team); err != nil {
			return err
		}
	}
	for i, platform := range []string{"linux", "windows", "macos"} {
		rule := groups.Rule{Mode: "and", Filters: []groups.Filter{{Field: "platform", Operator: "eq", Values: []string{platform}}, {Field: "tag", Operator: "eq", Values: []string{"demo-sample"}}}}
		if err := rule.Normalize(); err != nil {
			return err
		}
		raw, _ := json.Marshal(rule)
		group := g.id("asset-group", platform)
		if _, err := tx.Exec(ctx, `INSERT INTO asset_groups(id,tenant_id,name,description,kind,rule,is_demo) VALUES($1,$2,$3,'Synthetic assets selected by platform and demo tag.','dynamic',$4,true) ON CONFLICT(id) DO NOTHING`, group, g.tenant, "Demo: "+platform+" tagged endpoints", raw); err != nil {
			return err
		}
		// This tag is uniquely owned by this seed. Do not recompute or modify real groups.
		where, args := rule.Where(3)
		if _, err := tx.Exec(ctx, `INSERT INTO asset_group_members(tenant_id,asset_group_id,asset_id,source) SELECT $1,$2,a.id,'rule' FROM assets a WHERE a.tenant_id=$1 AND a.is_demo AND a.deleted_at IS NULL AND `+where+` ON CONFLICT DO NOTHING`, append([]any{g.tenant, group}, args...)...); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO scenario_targets(id,tenant_id,scenario_id,asset_group_id) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO NOTHING`, g.id("group-target", platform), g.tenant, g.id("scenario", fmt.Sprint(i)), group); err != nil {
			return err
		}
	}
	tests := []struct{ name, key, params string }{
		{"Quick exposure", "nmap.tcp_connect", `{"profile":"top100"}`},
		{"Full TCP + services", "nmap.tcp_connect", `{"profile":"custom","ports":"1-65535","service_detection":true}`},
		{"Web CVEs", "nuclei.scan", `{"template_profile":"cves","severity":["high","critical"]}`},
	}
	for i, item := range tests {
		def := contract.Find(item.key)
		if def == nil {
			return fmt.Errorf("missing demo contract %s", item.key)
		}
		params, err := contract.Normalize(item.key, json.RawMessage(item.params))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO action_presets(id,tenant_id,name,contract_key,contract_version,params,is_demo) VALUES($1,$2,$3,$4,$5,$6,true) ON CONFLICT(id) DO NOTHING`, g.id("preset", item.name), g.tenant, "Demo: "+item.name, item.key, def.Version, params); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO atomic_tests(id,tenant_id,name,contract_id,contract_version,params,target_asset_id,created_by,is_demo)
SELECT $1,$2,$3,id,version,$4,$5,$6,true FROM contracts WHERE tenant_id=$2 AND key=$7 AND version=$8 ON CONFLICT(id) DO NOTHING`, g.id("test", item.name), g.tenant, "Demo: "+item.name+" (synthetic fixture)", params, g.id("org", fmt.Sprint(i), "asset", "0"), g.createdBy, item.key, def.Version); err != nil {
			return err
		}
	}
	for i, item := range []struct {
		key, name, status string
		enabled           bool
		age               time.Duration
	}{
		{"demo.nmap.online", "Demo: Nmap online fixture", "running", true, time.Second},
		{"demo.nuclei.offline", "Demo: Nuclei offline fixture", "stopped", true, 24 * time.Hour},
		{"demo.nuclei.disabled", "Demo: Nuclei disabled fixture", "stopped", false, 24 * time.Hour},
	} {
		if _, err := tx.Exec(ctx, `INSERT INTO integrations(id,tenant_id,kind,key,name,description,enabled,status,last_heartbeat_at,config,is_demo) VALUES($1,$2,'injector',$3,$4,'Synthetic health example; never connected to a worker or executable contract.',$5,$6,$7,'{"synthetic":true}',true) ON CONFLICT(id) DO NOTHING`, g.id("integration", fmt.Sprint(i)), g.tenant, item.key, item.name, item.enabled, item.status, g.now.Add(-item.age)); err != nil {
			return err
		}
	}
	return groups.RecomputeAll(ctx, tx, g.tenant)
}
