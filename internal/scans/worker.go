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
	"orchis-go/internal/defensebot"
	"orchis-go/internal/hackbot"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/reporting"
	"orchis-go/internal/scancontrol"
	"orchis-go/internal/scanscope"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Tick menyimpan reservasi slot dan dispatch atomik; restart tidak menduplikasi pasangan target/tool.
func (s *Service) Tick(ctx context.Context, tenant uuid.UUID) error {
	return s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, tenant); err != nil {
			return err
		}
		if err := scancontrol.Enforce(ctx, tx, tenant, scancontrol.Now(s.Now), s.Engine.Advance); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "scan-dispatch:"+tenant.String()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE scan_jobs j SET status=CASE WHEN x.hackbot_result->>'disposition' IN ('limited','input_required') OR x.defense_result->>'disposition'='limited' THEN 'incomplete' WHEN x.outcome='success' THEN 'finished' WHEN x.outcome='canceled' THEN 'canceled' ELSE 'failed' END,error=COALESCE(x.error,''),ended_at=x.ended_at FROM executions x WHERE j.tenant_id=$1 AND x.tenant_id=j.tenant_id AND x.simulation_id=j.simulation_id AND x.state='ended' AND j.status='running'`, tenant); err != nil {
			return err
		}
		// A canceled preliminary check must not leave its dependent test waiting forever.
		if _, err := tx.Exec(ctx, `UPDATE scan_jobs j SET status='canceled',error='Preliminary scanner canceled; orchis-redteam-agent was not started.',ended_at=now() WHERE j.tenant_id=$1 AND j.tool='hackbot' AND j.status='waiting' AND EXISTS(SELECT 1 FROM scan_jobs p WHERE p.tenant_id=j.tenant_id AND p.assessment_id=j.assessment_id AND p.control_asset_id=j.control_asset_id AND p.tool IN ('nmap','nuclei') AND p.status='canceled')`, tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE scan_jobs j SET status='canceled',error='Preliminary scanner canceled; orchis-defense-agent was not started.',ended_at=now() WHERE j.tenant_id=$1 AND j.tool='defensebot' AND j.status='waiting' AND EXISTS(SELECT 1 FROM scan_jobs p WHERE p.tenant_id=j.tenant_id AND p.assessment_id=j.assessment_id AND p.control_asset_id=j.control_asset_id AND p.tool='nmap' AND p.status='canceled')`, tenant); err != nil {
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
		rows, err := tx.Query(ctx, `SELECT j.id,j.assessment_id,j.control_asset_id,j.contract_id,j.target,j.name,j.tool,j.contract_version,j.params,j.addresses
 FROM scan_jobs j JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id
 WHERE j.tenant_id=$1 AND j.status='waiting' AND a.status='running'
 AND (j.tool<>'defensebot' OR (
   (a.created_at < now() - interval '10 minutes' OR NOT EXISTS(SELECT 1 FROM executions c JOIN contracts co ON co.tenant_id=c.tenant_id AND co.id=c.contract_id WHERE c.tenant_id=j.tenant_id AND c.target_asset_id=j.asset_id AND c.executor_kind='agent' AND co.key LIKE 'osquery.%' AND c.state IN ('ready','running') AND c.created_at >= a.created_at))
   AND NOT EXISTS(SELECT 1 FROM scan_jobs p WHERE p.tenant_id=j.tenant_id AND p.assessment_id=j.assessment_id AND p.control_asset_id=j.control_asset_id AND p.tool='nmap' AND p.status IN ('waiting','running','canceled'))
 ))
 AND (j.tool<>'hackbot' OR (
   EXISTS(SELECT 1 FROM scan_jobs p WHERE p.tenant_id=j.tenant_id AND p.assessment_id=j.assessment_id AND p.control_asset_id=j.control_asset_id AND p.tool='nmap')
   AND EXISTS(SELECT 1 FROM scan_jobs p WHERE p.tenant_id=j.tenant_id AND p.assessment_id=j.assessment_id AND p.control_asset_id=j.control_asset_id AND p.tool='nuclei')
   AND NOT EXISTS(SELECT 1 FROM scan_jobs p WHERE p.tenant_id=j.tenant_id AND p.assessment_id=j.assessment_id AND p.control_asset_id=j.control_asset_id AND p.tool IN ('nmap','nuclei') AND p.status IN ('waiting','running','canceled'))
 ))
 ORDER BY j.id LIMIT $2 FOR UPDATE OF j SKIP LOCKED`, tenant, available)
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
		rows, err = tx.Query(ctx, `UPDATE assessments a SET status=CASE WHEN EXISTS(SELECT 1 FROM scan_jobs stopped WHERE stopped.tenant_id=a.tenant_id AND stopped.assessment_id=a.id AND stopped.status='canceled') THEN 'canceled' WHEN NOT EXISTS(SELECT 1 FROM scan_jobs ok WHERE ok.tenant_id=a.tenant_id AND ok.assessment_id=a.id AND ok.status IN ('finished','incomplete')) THEN 'failed' WHEN EXISTS(SELECT 1 FROM scan_jobs partial WHERE partial.tenant_id=a.tenant_id AND partial.assessment_id=a.id AND partial.status IN ('incomplete','failed')) THEN 'incomplete' WHEN NOT EXISTS(SELECT 1 FROM scan_jobs completed WHERE completed.tenant_id=a.tenant_id AND completed.assessment_id=a.id AND completed.status='finished') THEN 'failed' ELSE 'finished' END,ended_at=now() WHERE a.tenant_id=$1 AND a.is_scan AND a.status='running' AND NOT EXISTS(SELECT 1 FROM scan_jobs j WHERE j.tenant_id=a.tenant_id AND j.assessment_id=a.id AND j.status IN ('waiting','running')) RETURNING id`, tenant)
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
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
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
	rows, err := tx.Query(ctx, `SELECT `+scanCols+` FROM assessments a WHERE tenant_id=$1 AND is_scan AND status<>'running' AND a.results_expired_at IS NULL AND a.report_expired_at IS NULL AND NOT EXISTS(SELECT 1 FROM reports r WHERE r.tenant_id=a.tenant_id AND r.assessment_id=a.id) ORDER BY id LIMIT 10 FOR UPDATE OF a SKIP LOCKED`, tenant)
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
		covActions := map[string]string{}
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
			m := reporting.Method{Action: map[string]string{"hackbot": contract.KeyHackbot, "nmap": contract.KeyNmapTCP, "nuclei": contract.KeyNucleiVS, "defensebot": contract.KeyDefenseBot}[j.tool], Version: j.version, Params: json.RawMessage(j.params), Target: reporting.Redact(j.target, 1000), Outcome: outcome, Error: reporting.Redact(j.why, 1000)}
			if j.sim != nil {
				e := tx.QueryRow(ctx, `SELECT started_at,ended_at,COALESCE(outcome,''),COALESCE(error,'') FROM executions WHERE tenant_id=$1 AND simulation_id=$2 ORDER BY id LIMIT 1`, tenant, *j.sim).Scan(&m.StartedAt, &m.EndedAt, &m.Outcome, &m.Error)
				if e != nil && !errors.Is(e, pgx.ErrNoRows) {
					return e
				}
				m.Error = reporting.Redact(m.Error, 1000)
			}
			if j.status != "finished" {
				m.Outcome = j.status
			}
			if j.tool == "hackbot" && j.sim != nil {
				var summary string
				var phases, blockers []string
				e := tx.QueryRow(ctx, `SELECT COALESCE(hackbot_result->>'summary',''),COALESCE(hackbot_result->'phases','[]'::jsonb),COALESCE(hackbot_result->'blockers','[]'::jsonb) FROM executions WHERE tenant_id=$1 AND simulation_id=$2 ORDER BY id LIMIT 1`, tenant, *j.sim).Scan(&summary, &phases, &blockers)
				if e != nil && !errors.Is(e, pgx.ErrNoRows) {
					return e
				}
				if summary == "" {
					summary = "orchis-redteam-agent did not complete this pentest. Coverage: " + j.status + ". See the execution details for the blocker."
				}
				x.Summary = hackbot.LimitSummary(reporting.Redact(summary, 32000))
				x.CompletedPhases = phases
				for _, blocker := range blockers {
					x.CoverageLimitations = append(x.CoverageLimitations, reporting.Redact(blocker, 1000))
				}
				snap.Summary = hackbot.LimitSummary(snap.Summary + " " + x.Summary)
			}
			if j.tool == "defensebot" && j.sim != nil {
				var raw []byte
				e := tx.QueryRow(ctx, `SELECT defense_result FROM executions WHERE tenant_id=$1 AND simulation_id=$2 ORDER BY id LIMIT 1`, tenant, *j.sim).Scan(&raw)
				if e != nil && !errors.Is(e, pgx.ErrNoRows) {
					return e
				}
				if len(raw) > 0 {
					var sum defensebot.Summary
					if json.Unmarshal(raw, &sum) == nil {
						x.Defense = &sum
						if prov, e := json.Marshal(sum.Provenance); e == nil {
							m.Params = prov
						}
					}
				}
			}
			x.Methods = append(x.Methods, m)
			if j.sim != nil {
				findings, e := jobFindings(ctx, tx, tenant, *j.sim)
				if e != nil {
					return e
				}
				for _, f := range findings {
					cov := reporting.CoverageKey(m)
					f.Coverage = []string{cov}
					if cov != "" {
						covActions[cov] = m.Action
					}
					x.Findings = append(x.Findings, f)
				}
			}
		}
		for i := range snap.Assets {
			linkOutsideConfirmation(&snap.Assets[i], covActions)
			snap.Assets[i].Findings = reporting.MergeFindings(snap.Assets[i].Findings)
			for _, f := range snap.Assets[i].Findings {
				if f.Type == "vulnerability" {
					snap.BySeverity[f.Severity]++
				}
			}
		}
		if err = s.Reports.FreezeScanSnapshot(ctx, tx, tenant, a.OrganizationID, a.ID, snap); err != nil {
			return err
		}
	}
	return nil
}

var cveRe = regexp.MustCompile(`CVE-\d{4}-\d{4,}`)

// linkOutsideConfirmation marks a orchis-defense-agent finding when another method (Nuclei, Hackbot) reported the same CVE on the same
// asset: the weakness was seen from inside and from outside. Findings are never merged or dropped, only annotated.
func linkOutsideConfirmation(a *reporting.Asset, covActions map[string]string) {
	outside := map[string]map[string]bool{}
	for _, f := range a.Findings {
		if m := f.Evidence["method"]; m == "osv-distro" || m == "osv-exact" || m == "nvd-cpe" {
			continue
		}
		raw, _ := f.Evidence["cve"].(string)
		if tpl, _ := f.Evidence["template_id"].(string); tpl != "" {
			raw += " " + tpl // Nuclei template ids are usually the CVE id
		}
		for _, cve := range cveRe.FindAllString(strings.ToUpper(raw), -1) {
			if outside[cve] == nil {
				outside[cve] = map[string]bool{}
			}
			// Coverage disimpan sebagai hash; tindakan asal metode dipetakan saat freeze agar
			// konfirmasi luar tetap bisa mengenali Nuclei dan Hackbot.
			for _, c := range f.Coverage {
				switch action := covActions[c]; {
				case strings.Contains(action, contract.KeyNucleiVS):
					outside[cve]["Nuclei"] = true
				case strings.Contains(action, contract.KeyHackbot):
					outside[cve]["orchis-redteam-agent"] = true
				}
			}
		}
	}
	for i := range a.Findings {
		f := &a.Findings[i]
		method, _ := f.Evidence["method"].(string)
		if method != "osv-distro" && method != "osv-exact" && method != "nvd-cpe" {
			continue
		}
		cve, _ := f.Evidence["cve"].(string)
		seen := outside[strings.ToUpper(cve)]
		if len(seen) == 0 {
			continue
		}
		names := make([]string, 0, len(seen))
		for n := range seen {
			names = append(names, n)
		}
		sort.Strings(names)
		f.Evidence["confirmed_by"] = strings.Join(names, ", ")
	}
}
