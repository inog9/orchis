package endpoint

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/osquery"
	"orchis-go/internal/scancontrol"
	"time"
)

// CollectionInput menyetujui pilihan query per platform secara atomik dan versioned.
type CollectionInput struct {
	Platform string             `json:"platform"`
	Keys     []string           `json:"keys"`
	Versions map[string]int64   `json:"versions"`
	Consent  osquery.ConsentRef `json:"consent"`
}

func (s *Service) SetCollection(ctx context.Context, p *auth.Principal, org uuid.UUID, in CollectionInput, ip, req string) error {
	if !authz.Has(p.Permissions, authz.SettingsManage) {
		return httpx.Forbidden(authz.SettingsManage)
	}
	if !authz.Has(p.Permissions, authz.OrganizationWrite) {
		return httpx.Forbidden(authz.OrganizationWrite)
	}
	platform := in.Platform
	if platform == "darwin" {
		platform = "macos"
	}
	if !osquery.PlatformHasArtifact(platform) {
		return httpx.Validation(httpx.FieldError{Name: "platform", Message: "This platform has no pinned agent artifact yet."})
	}
	if !in.Consent.Valid() {
		return httpx.Validation(httpx.FieldError{Name: "consent", Message: "Review and accept the current endpoint collection document."})
	}
	selected := map[string]bool{}
	for _, key := range in.Keys {
		q := osquery.Find(key)
		if q == nil || !q.SupportsOS(platform) || selected[key] {
			return httpx.Validation(httpx.FieldError{Name: "keys", Message: "Choose each query supported on this platform once."})
		}
		selected[key] = true
	}
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
		var locked uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE tenant_id=$1 AND id=$2 AND NOT is_demo FOR UPDATE`, p.TenantID, org).Scan(&locked); errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Organisation")
		} else if err != nil {
			return err
		}
		tables := []string{}
		keys := []string{}
		for _, q := range osquery.Queries {
			if !q.SupportsOS(platform) {
				continue
			}
			expected, known := in.Versions[q.Key]
			if !known {
				return httpx.Conflict("Collection choices changed; reload before approval.")
			}
			var current int64
			err := tx.QueryRow(ctx, `SELECT version FROM osquery_org_settings WHERE tenant_id=$1 AND organization_id=$2 AND query_key=$3`, p.TenantID, org, q.Key).Scan(&current)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if current != expected {
				return httpx.Conflict("Collection choices changed; reload before approval.")
			}
			enabled := selected[q.Key]
			if _, err = tx.Exec(ctx, `INSERT INTO osquery_org_settings(tenant_id,organization_id,query_key,enabled,updated_by,consent_version,consent_sha256,consented_by,consented_at) VALUES($1,$2,$3,$4,$5,$6,$7,$5,now()) ON CONFLICT(tenant_id,organization_id,query_key) DO UPDATE SET enabled=EXCLUDED.enabled,version=osquery_org_settings.version+1,updated_by=EXCLUDED.updated_by,updated_at=now(),consent_version=EXCLUDED.consent_version,consent_sha256=EXCLUDED.consent_sha256,consented_by=EXCLUDED.consented_by,consented_at=EXCLUDED.consented_at`, p.TenantID, org, q.Key, enabled, p.UserID, in.Consent.Version, in.Consent.SHA256); err != nil {
				return err
			}
			if enabled {
				keys = append(keys, q.Key)
				tables = append(tables, q.TablesRead()...)
			}
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "endpoint.collection.consent", ResourceType: "organization", ResourceID: &org, IP: ip, RequestID: req, Detail: map[string]any{"platform": in.Platform, "queries": keys, "tables_read": tables, "consent": in.Consent, "osquery_version": osquery.Version, "automatic": true}})
	})
}

// CollectTick hanya mengambil keputusan eksplisit saat ini, bukan default katalog.
// Satu definisi internal dipakai ulang per agent/query; tidak ada shell atau SQL dari pengguna.
func (s *Service) CollectTick(ctx context.Context, tenant uuid.UUID, interval time.Duration) error {
	if interval < time.Minute || interval > 24*time.Hour {
		return errors.New("invalid endpoint collection interval")
	}
	return s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "endpoint-collection:"+tenant.String()); err != nil {
			return err
		}
		c := osquery.Consent()
		type item struct {
			agent, asset, actor uuid.UUID
			key, os, arch       string
		}
		items := []item{}
		rows, err := tx.Query(ctx, `SELECT g.id,a.id,s.consented_by,s.query_key,g.os,g.arch FROM agents g
   JOIN assets a ON a.tenant_id=g.tenant_id AND a.id=g.asset_id AND a.organization_id=g.organization_id
   JOIN organizations o ON o.tenant_id=a.tenant_id AND o.id=a.organization_id
   JOIN osquery_org_settings s ON s.tenant_id=a.tenant_id AND s.organization_id=a.organization_id
   LEFT JOIN endpoint_collection_schedule d ON d.tenant_id=g.tenant_id AND d.agent_id=g.id AND d.query_key=s.query_key
   WHERE g.tenant_id=$1 AND g.os||'/'||g.arch=ANY($5::text[]) AND g.status='online' AND NOT g.paused AND g.uninstall_status NOT IN('queued','dispatched') AND g.last_heartbeat_at>now()-interval '90 seconds' AND NOT g.is_demo
   AND a.deleted_at IS NULL AND NOT a.is_demo AND NOT o.is_demo AND a.testing_authorized
   AND s.enabled AND s.consent_version=$2 AND s.consent_sha256=$3 AND s.consented_by IS NOT NULL AND s.consented_at IS NOT NULL
   AND (d.last_enqueued_at IS NULL OR d.last_enqueued_at<=now()-make_interval(secs=>$4))
   AND NOT EXISTS(SELECT 1 FROM executions e JOIN contracts co ON co.tenant_id=e.tenant_id AND co.id=e.contract_id WHERE e.tenant_id=g.tenant_id AND e.target_asset_id=a.id AND co.key=s.query_key AND e.state IN ('ready','running'))
   ORDER BY g.id,s.query_key LIMIT 100 FOR SHARE OF g,a,s`, tenant, c.Version, c.SHA256, interval.Seconds(), osquery.SupportedPlatforms())
		if err != nil {
			return err
		}
		for rows.Next() {
			var i item
			if err = rows.Scan(&i.agent, &i.asset, &i.actor, &i.key, &i.os, &i.arch); err != nil {
				rows.Close()
				return err
			}
			items = append(items, i)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		for _, i := range items {
			q := osquery.Find(i.key)
			if q == nil || !q.SupportsOS(i.os) || !osquery.CollectionSupported(i.os, i.arch) {
				continue
			}
			if err = s.enqueueCollection(ctx, tx, tenant, i.agent, i.asset, i.actor, q, true); err != nil {
				return err
			}
		}
		return nil
	})
}

// enqueueCollection queues one approved fixed query for one agent. The same definition is reused per agent and query; there
// is no shell and no user-supplied SQL. actor is the administrator whose approval allows the collection.
func (s *Service) enqueueCollection(ctx context.Context, tx pgx.Tx, tenant, agent, asset, actor uuid.UUID, q *osquery.Query, automatic bool) error {
	var contract uuid.UUID
	var version int
	if err := tx.QueryRow(ctx, `SELECT id,version FROM contracts WHERE tenant_id=$1 AND key=$2 AND is_active ORDER BY version DESC LIMIT 1`, tenant, q.Key).Scan(&contract, &version); err != nil {
		return fmt.Errorf("endpoint contract %s: %w", q.Key, err)
	}
	test := uuid.NewSHA1(agent, []byte("endpoint-collection:"+q.Key))
	if _, err := tx.Exec(ctx, `INSERT INTO atomic_tests(id,tenant_id,name,contract_id,contract_version,target_asset_id,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(tenant_id,id) DO UPDATE SET contract_id=EXCLUDED.contract_id,contract_version=EXCLUDED.contract_version,deleted_at=NULL`, test, tenant, "Endpoint collection: "+q.Title, contract, version, asset, actor); err != nil {
		return err
	}
	p := &auth.Principal{TenantID: tenant, UserID: actor}
	execID := ids.New()
	if err := s.Exec.QueueTest(ctx, tx, p, test, execID, "", "endpoint-collection"); err != nil {
		return err
	}
	if err := audit.Write(ctx, tx, audit.Entry{Tenant: tenant, Action: "endpoint.collection.enqueue", ResourceType: "execution", ResourceID: &execID, Detail: map[string]any{"automatic": automatic, "agent_id": agent, "query": q.Key, "consented_by": actor}}); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO endpoint_collection_schedule(tenant_id,agent_id,query_key,atomic_test_id) VALUES($1,$2,$3,$4) ON CONFLICT(tenant_id,agent_id,query_key) DO UPDATE SET last_enqueued_at=now()`, tenant, agent, q.Key, test)
	return err
}

// QueueDefenseCollection queues a fresh collection of every approved DefenseBot check for one asset, inside the caller's
// transaction. It fails with a clear conflict when no agent can collect or nothing is approved, so a run never starts empty.
func (s *Service) QueueDefenseCollection(ctx context.Context, tx pgx.Tx, tenant, asset uuid.UUID, keys []string) (int, error) {
	var agent, org uuid.UUID
	var osName, arch string
	err := tx.QueryRow(ctx, `SELECT g.id,g.organization_id,g.os,g.arch FROM agents g WHERE g.tenant_id=$1 AND g.asset_id=$2 AND NOT g.is_demo AND g.status<>'revoked' ORDER BY g.last_heartbeat_at DESC NULLS LAST LIMIT 1`, tenant, asset).Scan(&agent, &org, &osName, &arch)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, httpx.Conflict("No agent is installed on this asset.")
	}
	if err != nil {
		return 0, err
	}
	if !osquery.CollectionSupported(osName, arch) {
		return 0, httpx.Conflict("DefenseBot supports Linux and macOS agents only.")
	}
	c := osquery.Consent()
	rows, err := tx.Query(ctx, `SELECT query_key,consented_by FROM osquery_org_settings WHERE tenant_id=$1 AND organization_id=$2 AND enabled AND consent_version=$3 AND consent_sha256=$4 AND consented_by IS NOT NULL AND consented_at IS NOT NULL AND query_key=ANY($5) ORDER BY query_key`, tenant, org, c.Version, c.SHA256, keys)
	if err != nil {
		return 0, err
	}
	type item struct {
		key   string
		actor uuid.UUID
	}
	var items []item
	for rows.Next() {
		var i item
		if err = rows.Scan(&i.key, &i.actor); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, i)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	queued := 0
	for _, i := range items {
		q := osquery.Find(i.key)
		if q == nil || !q.SupportsOS(osName) {
			continue
		}
		var pending int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM executions e JOIN contracts co ON co.tenant_id=e.tenant_id AND co.id=e.contract_id WHERE e.tenant_id=$1 AND e.agent_id=$2 AND co.key=$3 AND e.state IN ('ready','running')`, tenant, agent, i.key).Scan(&pending); err != nil {
			return 0, err
		}
		if pending > 0 {
			queued++
			continue
		}
		if err = s.enqueueCollection(ctx, tx, tenant, agent, asset, i.actor, q, false); err != nil {
			return 0, err
		}
		queued++
	}
	if queued == 0 {
		return 0, httpx.Conflict("DefenseBot collection is not approved for this organisation. Review the collection choices and the current consent document first.")
	}
	return queued, nil
}
func (s *Service) RunCollection(ctx context.Context, tenant uuid.UUID, interval time.Duration) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		if err := s.CollectTick(ctx, tenant, interval); err != nil {
			slog.Error("endpoint collection failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
