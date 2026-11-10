package schedules

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/alerts"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/reporting"
	"orchis-go/internal/scans"
)

func (s *Service) Run(ctx context.Context, tenant uuid.UUID) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		run, cancel := context.WithTimeout(ctx, 90*time.Second)
		e := s.Tick(run, tenant)
		cancel()
		if e != nil && ctx.Err() == nil {
			slog.Error("audit scheduler tick failed", "err", e)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// The advisory transaction serializes scheduler instances across processes. Each
// occurrence is committed before launch and uses a stable scan idempotency key,
// so a crash on either side of Launch is recovered without repeating an audit.
func (s *Service) Tick(ctx context.Context, tenant uuid.UUID) error {
	return s.DB.InTenant(ctx, tenant, func(lock pgx.Tx) error {
		var held bool
		if e := lock.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, "audit-scheduler:"+tenant.String()).Scan(&held); e != nil {
			return e
		}
		if !held {
			return nil
		}
		if e := s.claim(ctx, tenant); e != nil {
			return e
		}
		if e := s.dispatch(ctx, tenant); e != nil {
			return e
		}
		if e := s.reconcile(ctx, tenant); e != nil {
			return e
		}
		return s.digests(ctx, tenant)
	})
}
func (s *Service) claim(ctx context.Context, tenant uuid.UUID) error {
	now := s.now()
	return s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT id,organization_id,name,config,next_run_at,run_as FROM audit_schedules WHERE tenant_id=$1 AND NOT paused AND deleted_at IS NULL AND next_run_at<=$2 ORDER BY next_run_at,id LIMIT 20 FOR UPDATE SKIP LOCKED`, tenant, now)
		if e != nil {
			return e
		}
		type due struct {
			id, org, actor uuid.UUID
			name           string
			c              Config
			at             time.Time
		}
		items := []due{}
		for rows.Next() {
			var d due
			if e = rows.Scan(&d.id, &d.org, &d.name, &d.c, &d.at, &d.actor); e != nil {
				rows.Close()
				return e
			}
			items = append(items, d)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		for _, d := range items {
			next, e := d.c.Rule.Next(now)
			if e != nil {
				return e
			}
			status, reason := "queued", ""
			if now.Sub(d.at) > 15*time.Minute {
				status = "missed"
				reason = "This audit missed its 15-minute start window. Past occurrences were skipped; the next scheduled audit remains active."
			}
			input := scans.Input{Language: d.c.Language, OrganizationID: d.org, AssetIDs: append([]uuid.UUID{}, d.c.AssetIDs...), Tools: tools(d.c.Kind)}
			if d.c.AllAssets {
				r, e := tx.Query(ctx, `SELECT id FROM assets WHERE tenant_id=$1 AND organization_id=$2 AND testing_authorized AND deleted_at IS NULL AND NOT is_demo ORDER BY id`, tenant, d.org)
				if e != nil {
					return e
				}
				for r.Next() {
					var id uuid.UUID
					if e = r.Scan(&id); e != nil {
						r.Close()
						return e
					}
					input.AssetIDs = append(input.AssetIDs, id)
				}
				r.Close()
				if e = r.Err(); e != nil {
					return e
				}
			}
			id := ids.New()
			var ended *time.Time
			if status == "missed" {
				ended = &now
			}
			if _, e = tx.Exec(ctx, `INSERT INTO audit_schedule_runs(id,tenant_id,schedule_id,scheduled_at,status,run_as,input,max_runtime_minutes,notify_changes,reason,ended_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(tenant_id,schedule_id,scheduled_at) DO NOTHING`, id, tenant, d.id, d.at, status, d.actor, input, d.c.MaxRuntimeMinutes, d.c.NotifyChanges, reason, ended); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE audit_schedules SET next_run_at=$3 WHERE tenant_id=$1 AND id=$2`, tenant, d.id, next); e != nil {
				return e
			}
			if status == "missed" && d.c.NotifyGaps {
				if e = s.gap(ctx, tx, tenant, d.org, d.id, id, d.name, reason); e != nil {
					return e
				}
			}
		}
		return nil
	})
}
func tools(kind string) []string {
	if kind == "defense" {
		return []string{"defensebot"}
	}
	out := []string{"nmap", "nuclei", "hackbot"}
	if kind == "full" {
		out = append(out, "defensebot")
	}
	return out
}
func (s *Service) principal(ctx context.Context, tenant, user uuid.UUID) (*auth.Principal, error) {
	p := &auth.Principal{TenantID: tenant, UserID: user}
	e := s.DB.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT r.permissions,COALESCE(u.ui_language,'') FROM users u JOIN roles r ON r.tenant_id=u.tenant_id AND r.id=u.role_id WHERE u.tenant_id=$1 AND u.id=$2 AND u.status='active' AND u.deleted_at IS NULL AND u.kind='human' AND NOT u.is_demo`, tenant, user).Scan(&p.Permissions, &p.UILanguage)
	})
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, httpx.Conflict("The schedule's responsible user is no longer active. Save or resume the schedule with an active account.")
	}
	if e != nil {
		return nil, e
	}
	if e = access(p, true); e != nil {
		return nil, e
	}
	return p, nil
}
func (s *Service) dispatch(ctx context.Context, tenant uuid.UUID) error {
	type pending struct {
		id, schedule, actor uuid.UUID
		at                  time.Time
		in                  scans.Input
		paused              bool
		deleted             *time.Time
	}
	items := []pending{}
	e := s.DB.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT r.id,r.schedule_id,r.run_as,r.scheduled_at,r.input,s.paused,s.deleted_at FROM audit_schedule_runs r JOIN audit_schedules s ON s.tenant_id=r.tenant_id AND s.id=r.schedule_id WHERE r.tenant_id=$1 AND r.status='queued' ORDER BY r.scheduled_at,r.id LIMIT 20`, tenant)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v pending
			if e = rows.Scan(&v.id, &v.schedule, &v.actor, &v.at, &v.in, &v.paused, &v.deleted); e != nil {
				return e
			}
			items = append(items, v)
		}
		return rows.Err()
	})
	if e != nil {
		return e
	}
	for _, v := range items {
		key := "schedule:" + v.id.String()
		var scanID uuid.UUID
		// Recover a committed launch even if the process died before linking it.
		e = s.DB.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT id FROM assessments WHERE tenant_id=$1 AND organization_id=$2 AND idempotency_key=$3 AND is_scan`, tenant, v.in.OrganizationID, key).Scan(&scanID)
		})
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if errors.Is(e, pgx.ErrNoRows) {
			var p *auth.Principal
			switch {
			case v.paused || v.deleted != nil:
				e = httpx.Conflict("The schedule was paused or deleted before dispatch.")
			case s.now().Sub(v.at) > 15*time.Minute:
				e = httpx.Conflict("The audit could not start within its 15-minute start window. It will resume at the next scheduled time.")
			default:
				p, e = s.principal(ctx, tenant, v.actor)
			}
			if e == nil {
				var scan *scans.Scan
				scan, e = s.Scans.Launch(ctx, p, v.in, key, "", "scheduler")
				if e == nil {
					scanID = scan.ID
				}
			}
			if e != nil {
				var problem *httpx.Problem
				if !errors.As(e, &problem) || problem.Status >= 500 {
					return e
				} // transient failure: retain occurrence for idempotent recovery
				reason := problem.Detail
				if len(problem.Fields) > 0 {
					reason = problem.Fields[0].Message
				}
				if reason == "" {
					reason = problem.Title
				}
				if e = s.block(ctx, tenant, v.schedule, v.id, reason); e != nil {
					return e
				}
				continue
			}
		}
		e = s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `UPDATE audit_schedule_runs SET scan_id=$3,status='running',started_at=(SELECT created_at FROM assessments WHERE tenant_id=$1 AND id=$3) WHERE tenant_id=$1 AND id=$2 AND status='queued'`, tenant, v.id, scanID)
			return e
		})
		if e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) block(ctx context.Context, tenant, schedule, run uuid.UUID, reason string) error {
	return s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `UPDATE audit_schedule_runs SET status='blocked',reason=$3,ended_at=$4 WHERE tenant_id=$1 AND id=$2 AND status='queued'`, tenant, run, reporting.Redact(displayReason(reason), 600), s.now()); e != nil {
			return e
		}
		var org uuid.UUID
		var name string
		var c Config
		if e := tx.QueryRow(ctx, `SELECT organization_id,name,config FROM audit_schedules WHERE tenant_id=$1 AND id=$2`, tenant, schedule).Scan(&org, &name, &c); e != nil {
			return e
		}
		if !c.NotifyGaps {
			return nil
		}
		return s.gap(ctx, tx, tenant, org, schedule, run, name, reason)
	})
}
func (s *Service) gap(ctx context.Context, tx pgx.Tx, tenant, org, schedule, run uuid.UUID, name, reason string) error {
	// One unresolved coverage-gap alert per schedule. Successful coverage clears
	// the episode key, allowing a later independent failure to notify again.
	return alerts.Raise(ctx, tx, tenant, alerts.New{Kind: "scheduled_audit_gap", Severity: "warning", Title: "Scheduled audit needs attention", Body: reporting.Redact(name+": "+displayReason(reason), 700), ResourceType: "audit_schedule", ResourceID: &schedule, Audience: authz.AssessmentRead, DedupeKey: "schedule-gap:" + schedule.String()})
}
func (s *Service) reconcile(ctx context.Context, tenant uuid.UUID) error {
	// Retention may remove a terminal scan before the scheduler links its result.
	// A detached run is a visible coverage failure, never a reason to stall all runs.
	if e := s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE audit_schedule_runs SET status='failed',reason='The scan record was removed before its result could be summarised.',ended_at=$2 WHERE tenant_id=$1 AND status='running' AND scan_id IS NULL`, tenant, s.now())
		return e
	}); e != nil {
		return e
	}
	type active struct {
		id, scan, schedule, actor uuid.UUID
		started                   time.Time
		limit                     int
		notify                    bool
	}
	items := []active{}
	e := s.DB.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT id,scan_id,schedule_id,run_as,started_at,max_runtime_minutes,notify_changes FROM audit_schedule_runs WHERE tenant_id=$1 AND status='running' ORDER BY scheduled_at LIMIT 100`, tenant)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v active
			if e = rows.Scan(&v.id, &v.scan, &v.schedule, &v.actor, &v.started, &v.limit, &v.notify); e != nil {
				return e
			}
			items = append(items, v)
		}
		return rows.Err()
	})
	if e != nil {
		return e
	}
	for _, v := range items {
		var status string
		e = s.DB.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT status FROM assessments WHERE tenant_id=$1 AND id=$2`, tenant, v.scan).Scan(&status)
		})
		if e != nil {
			return e
		}
		timedOut := status == "running" && s.now().Sub(v.started) >= time.Duration(v.limit)*time.Minute
		if timedOut {
			// Cancellation is a scheduler-owned cleanup even if its creator lost permissions.
			p := &auth.Principal{TenantID: tenant, UserID: v.actor, Permissions: []string{authz.AssessmentRun, authz.SimulationRun, authz.OrganizationRead, authz.AssetRead, authz.AssessmentRead, authz.SimulationRead}}
			if e = s.Scans.Cancel(ctx, p, v.scan, "", "scheduler-runtime-limit"); e != nil {
				return e
			}
			status = "canceled"
		}
		if status == "running" {
			continue
		}
		e = s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
			var snap reporting.Snapshot
			e := tx.QueryRow(ctx, `SELECT snapshot FROM reports WHERE tenant_id=$1 AND assessment_id=$2`, tenant, v.scan).Scan(&snap)
			expired := false
			if errors.Is(e, pgx.ErrNoRows) {
				if e = tx.QueryRow(ctx, `SELECT results_expired_at IS NOT NULL OR report_expired_at IS NOT NULL FROM assessments WHERE tenant_id=$1 AND id=$2`, tenant, v.scan).Scan(&expired); e != nil {
					return e
				}
				if !expired {
					return nil
				}
			} else if e != nil {
				return e
			} // wait for a frozen comparison unless retention expired it

			summary := summarize(snap)
			if e = markReappeared(ctx, tx, tenant, &snap, &summary); e != nil {
				return e
			}
			reason := ""
			if expired {
				status = "incomplete"
				reason = "Results expired under the retention policy before a comparison was available."
			}
			if timedOut {
				reason = "The audit reached its configured runtime limit and was stopped."
			}
			if _, e = tx.Exec(ctx, `UPDATE audit_schedule_runs SET status=$3,summary=$4,reason=$5,ended_at=$6 WHERE tenant_id=$1 AND id=$2 AND status='running'`, tenant, v.id, status, summary, reason, s.now()); e != nil {
				return e
			}
			var org uuid.UUID
			var name string
			var c Config
			if e = tx.QueryRow(ctx, `SELECT organization_id,name,config FROM audit_schedules WHERE tenant_id=$1 AND id=$2`, tenant, v.schedule).Scan(&org, &name, &c); e != nil {
				return e
			}
			if summary.Total == 0 || summary.Completed < summary.Total || status != "finished" {
				if c.NotifyGaps {
					if e = s.gap(ctx, tx, tenant, org, v.schedule, v.id, name, "Some checks did not complete. Review the run for coverage gaps."); e != nil {
						return e
					}
				}
			} else {
				// Preserve the old alert while releasing its episode deduplication key.
				if _, e = tx.Exec(ctx, `UPDATE alerts SET dedupe_key=dedupe_key||':'||$3 WHERE tenant_id=$1 AND dedupe_key=$2`, tenant, "schedule-gap:"+v.schedule.String(), v.id.String()); e != nil {
					return e
				}
			}
			// Notify first-run high findings as well as comparable new/returned findings.
			if v.notify && summary.Attention > 0 {
				if e = alerts.Raise(ctx, tx, tenant, alerts.New{Kind: "scheduled_audit_changes", Severity: "critical", Title: "Scheduled audit found changes to review", Body: fmt.Sprintf("%s: %d critical or high findings need review. Open the scan for evidence, priorities, and coverage.", reporting.Redact(name, 120), summary.Attention), ResourceType: "scan", ResourceID: &v.scan, Audience: authz.FindingRead, DedupeKey: "schedule-changes:" + v.id.String()}); e != nil {
					return e
				}
			}
			return audit.Write(ctx, tx, audit.Entry{Tenant: tenant, ActorUser: &v.actor, Action: "schedule.run_completed", ResourceType: "audit_schedule", ResourceID: &v.schedule, RequestID: "scheduler", Detail: map[string]any{"scan_id": v.scan, "status": status}})
		})
		if e != nil {
			return e
		}
	}
	return nil
}
func summarize(snap reporting.Snapshot) Summary {
	out := Summary{}
	for _, a := range snap.Assets {
		for _, m := range a.Methods {
			out.Total++
			if m.Outcome == "success" {
				out.Completed++
			}
		}
	}
	if c := snap.ComparisonDetails; c != nil {
		out.New = c.Totals.New
		out.Persistent = c.Totals.Persistent
		out.Resolved = c.Totals.Resolved
		out.Uncompared = c.Totals.Uncompared
		for _, r := range c.Rows {
			if (r.Severity == "critical" || r.Severity == "high") && (r.Change == "new" || (r.Change == "uncompared" && r.Reason == "no_comparable_baseline")) {
				out.Attention++
			}
		}
	}
	return out
}
func markReappeared(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, snap *reporting.Snapshot, out *Summary) error {
	if snap.ComparisonDetails == nil {
		return nil
	}
	candidates := map[string]bool{}
	assets := []string{}
	findings := []string{}
	for _, r := range snap.ComparisonDetails.Rows {
		if r.Change == "new" && r.Reason == "" {
			candidates[r.AssetID.String()+"/"+r.FindingID] = true
			assets = append(assets, r.AssetID.String())
			findings = append(findings, r.FindingID)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	// One bounded identity lookup for the entire report. Legacy/null arrays do not
	// abort reconciliation; only matching tenant/org/epoch history is evidence.
	rows, e := tx.Query(ctx, `SELECT DISTINCT a->>'id',f->>'id'
 FROM reports p JOIN assessments h ON h.tenant_id=p.tenant_id AND h.id=p.assessment_id
 CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(p.snapshot->'assets')='array' THEN p.snapshot->'assets' ELSE '[]'::jsonb END) a
 CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(a->'findings')='array' THEN a->'findings' ELSE '[]'::jsonb END) f
 WHERE p.tenant_id=$1 AND p.organization_id=(SELECT organization_id FROM assessments WHERE tenant_id=$1 AND id=$2)
 AND p.assessment_id<>$2 AND p.snapshot->>'findings_epoch'=$3 AND h.created_at<$4
 AND a->>'id'=ANY($5::text[]) AND f->>'id'=ANY($6::text[])
 AND h.results_expired_at IS NULL AND h.report_expired_at IS NULL`, tenant, snap.AssessmentID, fmt.Sprint(snap.FindingsEpoch), snap.CreatedAt, assets, findings)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var asset, finding string
		if e = rows.Scan(&asset, &finding); e != nil {
			return e
		}
		key := asset + "/" + finding
		if candidates[key] {
			out.New--
			out.Reappeared++
			delete(candidates, key)
		}
	}
	return rows.Err()
}
func (s *Service) digests(ctx context.Context, tenant uuid.UUID) error {
	now := s.now()
	return s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT id,name,next_digest_at FROM audit_schedules WHERE tenant_id=$1 AND deleted_at IS NULL AND NOT paused AND (config->>'weekly_digest')::boolean AND next_digest_at<=$2 LIMIT 20 FOR UPDATE SKIP LOCKED`, tenant, now)
		if e != nil {
			return e
		}
		type item struct {
			id   uuid.UUID
			name string
			at   time.Time
		}
		items := []item{}
		for rows.Next() {
			var v item
			if e = rows.Scan(&v.id, &v.name, &v.at); e != nil {
				rows.Close()
				return e
			}
			items = append(items, v)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		for _, v := range items {
			var total, gaps, changes int
			if e = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status IN ('missed','blocked','failed','incomplete','canceled')),COALESCE(sum(COALESCE((summary->>'attention')::integer,0)),0) FROM audit_schedule_runs WHERE tenant_id=$1 AND schedule_id=$2 AND scheduled_at>=$3 AND scheduled_at<$4`, tenant, v.id, now.Add(-7*24*time.Hour), now).Scan(&total, &gaps, &changes); e != nil {
				return e
			}
			body := fmt.Sprintf("%s: %d scheduled audits, %d coverage gaps, %d critical/high findings flagged in the past 7 days. Open the schedule to review results.", reporting.Redact(v.name, 120), total, gaps, changes)
			if e = alerts.Raise(ctx, tx, tenant, alerts.New{Kind: "scheduled_audit_digest", Severity: "info", Title: "Your weekly audit summary is ready", Body: body, ResourceType: "audit_schedule", ResourceID: &v.id, Audience: authz.AssessmentRead, DedupeKey: "schedule-digest:" + v.id.String() + ":" + v.at.UTC().Format(time.RFC3339)}); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE audit_schedules SET next_digest_at=$3 WHERE tenant_id=$1 AND id=$2`, tenant, v.id, now.Add(7*24*time.Hour)); e != nil {
				return e
			}
		}
		return nil
	})
}

// Keep returned reasons readable when old engine names appear in server errors.
func displayReason(s string) string { return strings.ReplaceAll(s, "Hackbot", "orchis-redteam-agent") }
