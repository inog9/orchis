package demo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (g *generator) seedAtomicHistory(ctx context.Context, tx pgx.Tx) error {
	for i, name := range []string{"Quick exposure", "Full TCP + services", "Web CVEs"} {
		test := g.id("test", name)
		exec := g.id("atomic-history", name)
		outcome := []string{"success", "error", "skipped"}[i]
		reason := ""
		if outcome != "success" {
			reason = "Demo/synthetic " + outcome + " example; no scanner process was run."
		}
		target := "127.0.0.1"
		if i == 2 {
			target = "http://127.0.0.1:80"
		}
		start := g.now.Add(-30 * time.Minute)
		end := start.Add(time.Minute)
		if _, err := tx.Exec(ctx, `INSERT INTO executions(id,tenant_id,atomic_test_id,contract_id,contract_version,target_asset_id,target_address,executor_kind,integration_id,state,outcome,params,input_hash,created_at,started_at,ended_at,error,is_demo)
 SELECT $1,$2,t.id,t.contract_id,t.contract_version,t.target_asset_id,$4,'injector',c.integration_id,'ended',$5,t.params,$6,$7,$7,$8,$9,true
 FROM atomic_tests t JOIN contracts c ON c.tenant_id=t.tenant_id AND c.id=t.contract_id JOIN assets a ON a.tenant_id=t.tenant_id AND a.id=t.target_asset_id
 WHERE t.tenant_id=$2 AND t.id=$3 AND t.is_demo AND a.is_demo ON CONFLICT(id) DO NOTHING`, exec, g.tenant, test, target, outcome, hash(exec.String()), start, end, reason); err != nil {
			return err
		}
		if outcome == "success" {
			finding := g.id("org", fmt.Sprint(i), "asset", "0", "finding", "0")
			if _, err := tx.Exec(ctx, `INSERT INTO finding_occurrences(id,tenant_id,finding_id,execution_id,seen_at,details) VALUES($1,$2,$3,$4,$5,'{"is_demo":true,"label":"Synthetic saved-test history; no check was executed"}') ON CONFLICT DO NOTHING`, g.id("atomic-history-occurrence", name), g.tenant, finding, exec, start); err != nil {
				return err
			}
		}
	}
	return nil
}
