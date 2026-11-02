package scans

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/contract"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/reporting"
	"orchis-go/internal/scanscope"
	"time"
)

// Tick menyimpan reservasi slot dan dispatch atomik; restart tidak menduplikasi pasangan target/tool.
func (s *Service) Tick(ctx context.Context, tenant uuid.UUID) error {
	return s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "scan-dispatch:"+tenant.String()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE scan_jobs j SET status=CASE WHEN x.outcome='success' THEN 'finished' WHEN x.outcome='canceled' THEN 'canceled' ELSE 'failed' END,error=COALESCE(x.error,''),ended_at=x.ended_at FROM executions x WHERE j.tenant_id=$1 AND x.tenant_id=j.tenant_id AND x.simulation_id=j.simulation_id AND x.state='ended' AND j.status='running'`, tenant); err != nil {
			return err
		}
		var active int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM scan_jobs WHERE tenant_id=$1 AND status='running'`, tenant).Scan(&active); err != nil {
			return err
		}
		available := s.slots() - active
		if available < 0 {
			available = 0
		}
		type waiting struct {
			id, root, asset, contract uuid.UUID
			target, name, tool        string
			version                   int
			params, addresses         []byte
		}
		pending := []waiting{}
		rows, err := tx.Query(ctx, `SELECT j.id,j.assessment_id,j.control_asset_id,j.contract_id,j.target,j.name,j.tool,j.contract_version,j.params,j.addresses FROM scan_jobs j JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id WHERE j.tenant_id=$1 AND j.status='waiting' AND a.status='running' ORDER BY j.id LIMIT $2 FOR UPDATE OF j SKIP LOCKED`, tenant, available)
		if err != nil {
			return err
		}
		for rows.Next() {
			var j waiting
			if err = rows.Scan(&j.id, &j.root, &j.asset, &j.contract, &j.target, &j.name, &j.tool, &j.version, &j.params, &j.addresses); err != nil {
				rows.Close()
				return err
			}
			pending = append(pending, j)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		for _, j := range pending {
			sim := ids.New()
			if _, err = tx.Exec(ctx, `INSERT INTO simulations(id,tenant_id,name,status,started_at,launched_by) SELECT $1,$2,$3,'running',now(),created_by FROM assessments WHERE tenant_id=$2 AND id=$4`, sim, tenant, j.name+" / "+j.tool, j.root); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE scan_jobs SET status='running',simulation_id=$3 WHERE tenant_id=$1 AND id=$2`, tenant, j.id, sim); err != nil {
				return err
			}
			if _, err = scanscope.CheckJob(ctx, tx, tenant, sim, j.target, s.Guard, s.hostLimit()); err != nil {
				if _, err = tx.Exec(ctx, `UPDATE scan_jobs SET status='failed',error=$3,ended_at=now() WHERE tenant_id=$1 AND id=$2`, tenant, j.id, err.Error()); err != nil {
					return err
				}
				if _, err = tx.Exec(ctx, `UPDATE simulations SET status='failed',ended_at=now() WHERE tenant_id=$1 AND id=$2`, tenant, sim); err != nil {
					return err
				}
				continue
			}
			if _, err = tx.Exec(ctx, `INSERT INTO simulation_steps(id,tenant_id,simulation_id,key,title,contract_id,contract_version,params,trigger_mode,is_enabled) VALUES($1,$2,$3,$4,$4,$5,$6,$7,'after_parents',true)`, ids.New(), tenant, sim, j.tool, j.contract, j.version, j.params); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO simulation_targets(tenant_id,simulation_id,asset_id,addresses) VALUES($1,$2,$3,$4)`, tenant, sim, j.asset, j.addresses); err != nil {
				return err
			}
			if err = s.Engine.Advance(ctx, tx, tenant, sim); err != nil {
				return err
			}
		}
		// Penutupan dan snapshot satu transaksi; renderer boleh gagal/coba ulang tanpa scan baru.
		rows, err = tx.Query(ctx, `UPDATE assessments a SET status='finished',ended_at=now() WHERE a.tenant_id=$1 AND a.is_scan AND a.status='running' AND NOT EXISTS(SELECT 1 FROM scan_jobs j WHERE j.tenant_id=a.tenant_id AND j.assessment_id=a.id AND j.status IN ('waiting','running')) RETURNING id`, tenant)
		if err != nil {
			return err
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		return s.freeze(ctx, tx, tenant)
	})
}
func (s *Service) Run(ctx context.Context, tenant uuid.UUID) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if err := s.Tick(ctx, tenant); err != nil {
			slog.Error("scan worker failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Service) Cancel(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, req string) error {
	if err := require(p, authz.AssessmentRun, authz.SimulationRun, authz.OrganizationRead, authz.AssetRead); err != nil {
		return err
	}
	return s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM assessments WHERE tenant_id=$1 AND id=$2 AND is_scan FOR UPDATE`, p.TenantID, id).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Scan")
		}
		if err != nil {
			return err
		}
		if status == "canceled" {
			return nil
		}
		if status != "running" {
			return httpx.Conflict("Only running scans can be canceled.")
		}
		if _, err = tx.Exec(ctx, `UPDATE assessments SET status='canceled',ended_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE executions SET state='ended',outcome='canceled',error='Scan canceled',ended_at=now(),lease_until=NULL,version=version+1,updated_at=now() WHERE tenant_id=$1 AND simulation_id IN(SELECT simulation_id FROM scan_jobs WHERE tenant_id=$1 AND assessment_id=$2) AND state IN ('ready','running')`, p.TenantID, id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE simulations SET status='canceled',ended_at=now(),version=version+1,updated_at=now() WHERE tenant_id=$1 AND id IN(SELECT simulation_id FROM scan_jobs WHERE tenant_id=$1 AND assessment_id=$2) AND status IN ('running','paused')`, p.TenantID, id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE scan_jobs SET status='canceled',error='Scan canceled',ended_at=now() WHERE tenant_id=$1 AND assessment_id=$2 AND status IN ('waiting','running')`, p.TenantID, id); err != nil {
			return err
		}
		if err = s.freeze(ctx, tx, p.TenantID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "scan.cancel", ResourceType: "scan", ResourceID: &id, IP: ip, RequestID: req})
	})
}
func (s *Service) freeze(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT `+scanCols+` FROM assessments a WHERE tenant_id=$1 AND is_scan AND status<>'running' AND NOT EXISTS(SELECT 1 FROM reports r WHERE r.tenant_id=a.tenant_id AND r.assessment_id=a.id) ORDER BY id LIMIT 10 FOR UPDATE OF a SKIP LOCKED`, tenant)
	if err != nil {
		return err
	}
	list := []Scan{}
	for rows.Next() {
		a, e := scanRow(rows)
		if e != nil {
			rows.Close()
			return e
		}
		list = append(list, a)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, a := range list {
		snap := reporting.Snapshot{SchemaVersion: "orchis.report.v1", ScanID: &a.ID, AssessmentID: a.ID, Organization: a.Name, Profile: a.Options.Preset, Status: a.Status, CreatedAt: a.CreatedAt.UTC(), Assets: []reporting.Asset{}, BySeverity: map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}, Comparison: "No comparable previous scan. Failed or skipped checks cannot establish resolved findings.", Remediation: "Review the evidence for each finding and validate remediation before retesting."}
		type item struct {
			id                              uuid.UUID
			asset                           *uuid.UUID
			name, target, tool, status, why string
			version                         int
			params                          []byte
			sim                             *uuid.UUID
		}
		jobs := []item{}
		rows, err = tx.Query(ctx, `SELECT id,asset_id,name,target,tool,status,error,contract_version,params,simulation_id FROM scan_jobs WHERE tenant_id=$1 AND assessment_id=$2 ORDER BY name,target,tool,id`, tenant, a.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var j item
			if err = rows.Scan(&j.id, &j.asset, &j.name, &j.target, &j.tool, &j.status, &j.why, &j.version, &j.params, &j.sim); err != nil {
				rows.Close()
				return err
			}
			jobs = append(jobs, j)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		index := map[string]int{}
		seen := map[string]bool{}
		for _, j := range jobs {
			group := contract.HostOf(j.target)
			if j.asset != nil {
				group = j.asset.String()
			}
			idx, ok := index[group]
			if !ok {
				idx = len(snap.Assets)
				index[group] = idx
				id := uuid.NewSHA1(a.OrganizationID, []byte(group))
				if j.asset != nil {
					id = *j.asset
				}
				snap.Assets = append(snap.Assets, reporting.Asset{Discovered: j.asset == nil, ID: id, Name: reporting.Redact(j.name, 200), Status: "finished", Findings: []reporting.Finding{}, Methods: []reporting.Method{}})
			}
			x := &snap.Assets[idx]
			outcome := "success"
			if j.status != "finished" {
				x.Status = "incomplete"
				outcome = j.status
			}
			m := reporting.Method{Action: map[string]string{"nmap": contract.KeyNmapTCP, "nuclei": contract.KeyNucleiVS}[j.tool], Version: j.version, Params: json.RawMessage(j.params), Target: reporting.Redact(j.target, 1000), Outcome: outcome, Error: reporting.Redact(j.why, 1000)}
			if j.sim != nil {
				e := tx.QueryRow(ctx, `SELECT started_at,ended_at,COALESCE(outcome,''),COALESCE(error,'') FROM executions WHERE tenant_id=$1 AND simulation_id=$2 ORDER BY id LIMIT 1`, tenant, *j.sim).Scan(&m.StartedAt, &m.EndedAt, &m.Outcome, &m.Error)
				if e != nil && !errors.Is(e, pgx.ErrNoRows) {
					return e
				}
				m.Error = reporting.Redact(m.Error, 1000)
			}
			x.Methods = append(x.Methods, m)
			if j.sim != nil {
				findings, e := jobFindings(ctx, tx, tenant, *j.sim)
				if e != nil {
					return e
				}
				for _, f := range findings {
					if seen[group+f.ID] {
						continue
					}
					seen[group+f.ID] = true
					x.Findings = append(x.Findings, f)
					if f.Type == "vulnerability" {
						snap.BySeverity[f.Severity]++
					}
				}
			}
		}
		if err = s.Reports.FreezeScanSnapshot(ctx, tx, tenant, a.OrganizationID, a.ID, snap); err != nil {
			return err
		}
	}
	return nil
}
