package execution

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/hackbot"
	"orchis-go/internal/httpx"
)

// scannerInputs binds a Hackbot job to the two preliminary jobs for its own
// tenant, assessment and control asset. Mutable global findings are never used.
func (r *Runtime) scannerInputs(ctx context.Context, tx pgx.Tx, simulation uuid.UUID) ([]hackbot.ScannerInput, error) {
	rows, err := tx.Query(ctx, `SELECT p.tool,p.status,p.target,p.error,COALESCE(x.id::text,''),p.params,
 COALESCE((SELECT jsonb_agg(o.finding_snapshot ORDER BY o.id) FROM finding_occurrences o WHERE o.tenant_id=p.tenant_id AND o.execution_id=x.id AND o.finding_snapshot IS NOT NULL),'[]'::jsonb)
 FROM scan_jobs j JOIN scan_jobs p ON p.tenant_id=j.tenant_id AND p.assessment_id=j.assessment_id AND p.control_asset_id=j.control_asset_id AND p.tool IN ('nmap','nuclei')
 LEFT JOIN executions x ON x.tenant_id=p.tenant_id AND x.simulation_id=p.simulation_id
 WHERE j.tenant_id=$1 AND j.simulation_id=$2 AND j.tool='hackbot' ORDER BY p.tool`, r.Tenant, simulation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	inputs := []hackbot.ScannerInput{}
	seen := map[string]bool{}
	for rows.Next() {
		var input hackbot.ScannerInput
		if err = rows.Scan(&input.Tool, &input.Status, &input.Target, &input.Error, &input.ExecutionID, &input.Params, &input.Findings); err != nil {
			return nil, err
		}
		if input.Status != "finished" && input.Status != "failed" && input.Status != "incomplete" {
			return nil, httpx.Conflict("orchis-redteam-agent is waiting for preliminary Nmap and Nuclei scans.")
		}
		if seen[input.Tool] {
			return nil, fmt.Errorf("duplicate preliminary scanner input")
		}
		seen[input.Tool] = true
		inputs = append(inputs, input)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(inputs) != 2 || !seen["nmap"] || !seen["nuclei"] {
		return nil, httpx.Conflict("orchis-redteam-agent requires both preliminary scanner results.")
	}
	return inputs, nil
}
