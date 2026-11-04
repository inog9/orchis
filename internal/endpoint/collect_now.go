package endpoint

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/osquery"
	"orchis-go/internal/scancontrol"
)

// CollectNow queues only currently approved fixed queries. Pending queries are
// reused, and every task passes QueueTest and the ordinary claim/result guards.
func (s *Service) CollectNow(ctx context.Context, p *auth.Principal, agent uuid.UUID, ip, req string) ([]uuid.UUID, error) {
	for _, permission := range []string{authz.SettingsManage, authz.AgentManage, authz.AssetRead, authz.OrganizationRead, authz.AtomicRun} {
		if !authz.Has(p.Permissions, permission) {
			return nil, httpx.Forbidden(permission)
		}
	}
	out := []uuid.UUID{}
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if e := scancontrol.Lock(ctx, tx, p.TenantID); e != nil {
			return e
		}
		var asset, org uuid.UUID
		var platform, arch, status string
		var live, paused, authorized bool
		e := tx.QueryRow(ctx, `SELECT g.asset_id,a.organization_id,g.os,g.arch,g.status,COALESCE(g.last_heartbeat_at>now()-interval '90 seconds',false),g.paused OR g.uninstall_status IN('queued','dispatched'),a.testing_authorized
FROM agents g JOIN assets a ON a.tenant_id=g.tenant_id AND a.id=g.asset_id JOIN organizations o ON o.tenant_id=a.tenant_id AND o.id=a.organization_id
WHERE g.tenant_id=$1 AND g.id=$2 AND g.organization_id=a.organization_id AND NOT g.is_demo AND NOT a.is_demo AND NOT o.is_demo AND a.deleted_at IS NULL FOR SHARE OF g,a,o`, p.TenantID, agent).Scan(&asset, &org, &platform, &arch, &status, &live, &paused, &authorized)
		if errors.Is(e, pgx.ErrNoRows) {
			return httpx.NotFound("Agent")
		}
		if e != nil {
			return e
		}
		if status == "revoked" || !live || paused {
			return httpx.Conflict("The agent must be online and resumed before collecting data.")
		}
		if !authorized {
			return httpx.Conflict("Authorize the asset before collecting data.")
		}
		if platform != "linux" || arch != "x86_64" {
			return httpx.Conflict("Endpoint collection is supported on Linux amd64 only.")
		}
		consent := osquery.Consent()
		rows, e := tx.Query(ctx, `SELECT query_key FROM osquery_org_settings WHERE tenant_id=$1 AND organization_id=$2 AND enabled AND consent_version=$3 AND consent_sha256=$4 AND consented_by IS NOT NULL AND consented_at IS NOT NULL ORDER BY query_key FOR SHARE`, p.TenantID, org, consent.Version, consent.SHA256)
		if e != nil {
			return e
		}
		keys := []string{}
		for rows.Next() {
			var key string
			if e = rows.Scan(&key); e != nil {
				rows.Close()
				return e
			}
			keys = append(keys, key)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		for _, key := range keys {
			q := osquery.Find(key)
			if q == nil || !q.SupportsOS(platform) {
				continue
			}
			var pending uuid.UUID
			e = tx.QueryRow(ctx, `SELECT e.id FROM executions e JOIN contracts c ON c.tenant_id=e.tenant_id AND c.id=e.contract_id WHERE e.tenant_id=$1 AND e.agent_id=$2 AND e.target_asset_id=$3 AND c.key=$4 AND e.state IN('ready','running') ORDER BY e.created_at LIMIT 1`, p.TenantID, agent, asset, key).Scan(&pending)
			if e == nil {
				out = append(out, pending)
				continue
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			var contractID uuid.UUID
			var version int
			if e = tx.QueryRow(ctx, `SELECT id,version FROM contracts WHERE tenant_id=$1 AND key=$2 AND is_active ORDER BY version DESC LIMIT 1`, p.TenantID, key).Scan(&contractID, &version); e != nil {
				return e
			}
			test := uuid.NewSHA1(agent, []byte("endpoint-collection:"+key))
			if _, e = tx.Exec(ctx, `INSERT INTO atomic_tests(id,tenant_id,name,contract_id,contract_version,target_asset_id,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(tenant_id,id) DO UPDATE SET contract_id=EXCLUDED.contract_id,contract_version=EXCLUDED.contract_version,deleted_at=NULL`, test, p.TenantID, "Endpoint collection: "+q.Title, contractID, version, asset, p.UserID); e != nil {
				return e
			}
			executionID := ids.New()
			if e = s.Exec.QueueTest(ctx, tx, p, test, executionID, ip, req); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO endpoint_collection_schedule(tenant_id,agent_id,query_key,atomic_test_id) VALUES($1,$2,$3,$4) ON CONFLICT(tenant_id,agent_id,query_key) DO UPDATE SET last_enqueued_at=now()`, p.TenantID, agent, key, test); e != nil {
				return e
			}
			out = append(out, executionID)
		}
		if len(out) == 0 {
			return httpx.Conflict("No endpoint queries have current consent. Review the collection choices first.")
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "agent.collect_now", ResourceType: "agent", ResourceID: &agent, IP: ip, RequestID: req, Detail: map[string]any{"queries": keys, "execution_ids": out}})
	})
	return out, err
}
