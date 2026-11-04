package scancontrol

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/audit"
	"time"
)

func Now(clock func() time.Time) time.Time {
	if clock != nil {
		return clock()
	}
	return time.Now()
}
func Outcome(err error) string {
	var b *Blocked
	if errors.As(err, &b) {
		return "canceled"
	}
	return "error"
}

type Advance func(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) error

// Enforce checks durable policy on restart, queue dispatch, claim/heartbeat and
// completion. Callers acquire Lock before other scan row locks. Completed work
// is never rewritten and late results cannot turn an interrupted scan into safe.
func Enforce(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, now time.Time, advance Advance) error {
	if err := Lock(ctx, tx, tenant); err != nil {
		return err
	}
	type assessment struct{ id, org uuid.UUID }
	var assessments []assessment
	rows, err := tx.Query(ctx, `SELECT id,organization_id FROM assessments WHERE tenant_id=$1 AND status='running' ORDER BY id`, tenant)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a assessment
		if err = rows.Scan(&a.id, &a.org); err != nil {
			rows.Close()
			return err
		}
		assessments = append(assessments, a)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, a := range assessments {
		err = Check(ctx, tx, tenant, &a.org, now)
		if err == nil {
			continue
		}
		var b *Blocked
		if !errors.As(err, &b) {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE scan_jobs SET status='canceled',error=$3,ended_at=now() WHERE tenant_id=$1 AND assessment_id=$2 AND status IN ('waiting','running')`, tenant, a.id, b.Reason); err != nil {
			return err
		}
		// Cancel jobs and child simulations immediately. The scan worker closes the
		// assessment and freezes its snapshot in the same transaction; stopping must
		// not expose a terminal scan before that snapshot exists.
		if _, err = tx.Exec(ctx, `UPDATE simulations SET status='canceled',ended_at=now(),version=version+1,updated_at=now() WHERE tenant_id=$1 AND status IN ('running','paused') AND (id IN(SELECT simulation_id FROM scan_jobs WHERE tenant_id=$1 AND assessment_id=$2) OR id IN(SELECT simulation_id FROM assessment_targets WHERE tenant_id=$1 AND assessment_id=$2))`, tenant, a.id); err != nil {
			return err
		}
	}
	type execution struct {
		id, asset uuid.UUID
		org, sim  *uuid.UUID
	}
	var executions []execution
	rows, err = tx.Query(ctx, `SELECT id,target_asset_id,scan_organization_id,simulation_id FROM executions WHERE tenant_id=$1 AND executor_kind='injector' AND NOT is_demo AND state IN ('ready','running') ORDER BY id`, tenant)
	if err != nil {
		return err
	}
	for rows.Next() {
		var e execution
		if err = rows.Scan(&e.id, &e.asset, &e.org, &e.sim); err != nil {
			rows.Close()
			return err
		}
		executions = append(executions, e)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	sims := map[uuid.UUID]bool{}
	for _, e := range executions {
		err = CheckAsset(ctx, tx, tenant, e.asset, e.org, now)
		if err == nil {
			continue
		}
		var b *Blocked
		if !errors.As(err, &b) {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE executions SET state='ended',outcome='canceled',error=$3,scan_stop_reason=$4,ended_at=now(),lease_until=NULL,version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND state IN ('ready','running')`, tenant, e.id, b.Reason, b.Code); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO execution_logs(tenant_id,execution_id,level,message) VALUES($1,$2,'warn',$3)`, tenant, e.id, b.Reason); err != nil {
			return err
		}
		if err = audit.Write(ctx, tx, audit.Entry{Tenant: tenant, Action: "scan.policy_stop", ResourceType: "execution", ResourceID: &e.id, Detail: map[string]any{"reason": b.Code}}); err != nil {
			return err
		}
		if e.sim != nil {
			sims[*e.sim] = true
		}
	}
	// Sort for deterministic advancement and lock order across workers.
	if advance != nil {
		for _, e := range executions {
			if e.sim != nil && sims[*e.sim] {
				delete(sims, *e.sim)
				if err = advance(ctx, tx, tenant, *e.sim); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func Code(err error) string {
	var b *Blocked
	if errors.As(err, &b) {
		return b.Code
	}
	return ""
}
