package simulation

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/chain"
	"orchis-go/internal/contract"
	"orchis-go/internal/httpx"
)

func (e *Engine) advanceAdvanced(ctx context.Context, tx pgx.Tx, tenant, sim uuid.UUID, g graph, targets []target, execs []chain.Exec, found []chain.Found) (bool, int, error) {
	finished, count := true, 0
	by := map[string]stepRow{}
	for _, s := range g.steps {
		by[s.Key] = s
	}
	for _, t := range targets {
		xs, fs := scoped(execs, found, t.ID.String())
		res := chain.Evaluate(g.chainSteps(), g.edges, xs, fs)
		finished = finished && res.Finished
		for key, status := range res.Status {
			if status == chain.StWaiting || status == chain.StQueued || status == chain.StRunning || status == chain.StDone || status == chain.StFailed {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO simulation_decisions(tenant_id,simulation_id,step_id,asset_id,status,reason) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, tenant, sim, by[key].ID, t.ID, status, res.Reasons[key]); err != nil {
				return false, count, err
			}
		}
		for _, l := range res.Launch {
			st := by[l.Step]
			inputs, err := mappedInputs(st, t, g.edges, fs)
			reason := res.Reasons[l.Step]
			preview := map[string]any{}
			if err != nil {
				reason = "Mapping refused: " + err.Error()
				bad := t
				bad.Addrs = nil
				if err = e.launchStep(ctx, tx, tenant, sim, st, []target{bad}, l.Triggers); err != nil {
					return false, count, err
				}
				preview["error"] = reason
			} else {
				values := []map[string]any{}
				for _, in := range inputs {
					addr, gerr := mappedTarget(e, in.step, in.target)
					if gerr != nil {
						reason = "Mapped target refused: " + gerr.Error()
					}
					values = append(values, map[string]any{"params": json.RawMessage(in.step.Params), "target": addr})
					if err = e.launchStep(ctx, tx, tenant, sim, in.step, []target{in.target}, l.Triggers); err != nil {
						return false, count, err
					}
				}
				preview["inputs"] = values
			}
			raw, _ := json.Marshal(preview)
			if _, err = tx.Exec(ctx, `UPDATE simulation_decisions SET reason=$5,preview=$6 WHERE tenant_id=$1 AND simulation_id=$2 AND step_id=$3 AND asset_id=$4`, tenant, sim, st.ID, t.ID, reason, raw); err != nil {
				return false, count, err
			}
			count++
		}
	}
	return finished, count, nil
}

// advancedSummary mempertahankan tampilan agregat sambil menunggu semua aset terminal.
func advancedSummary(g graph, execs []chain.Exec, found []chain.Found, targets []target) chain.Result {
	out := chain.Result{Status: map[string]string{}}
	rank := map[string]int{chain.StRunning: 8, chain.StQueued: 7, chain.StWaiting: 6, chain.StEligible: 5, chain.StFailed: 4, chain.StBlocked: 3, chain.StDone: 2, chain.StSkipped: 1}
	for _, t := range targets {
		xs, fs := scoped(execs, found, t.ID.String())
		res := chain.Evaluate(g.chainSteps(), g.edges, xs, fs)
		for key, status := range res.Status {
			if rank[status] > rank[out.Status[key]] {
				out.Status[key] = status
			}
		}
	}
	return out
}
func (h *Handlers) registerDryRun(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/simulations/{id}/dry-run", auth.Require(authz.SimulationRead, func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		if !authz.Has(p.Permissions, authz.FindingRead) || !authz.Has(p.Permissions, authz.AssetRead) {
			httpx.Write(w, r, httpx.Forbidden(authz.FindingRead))
			return
		}
		id, err := pid(r, "Simulation")
		if err != nil {
			httpx.Write(w, r, err)
			return
		}
		items := []map[string]any{}
		err = h.Svc.DB.InTenantRead(r.Context(), p.TenantID, func(tx pgx.Tx) error {
			var exists bool
			if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM simulations WHERE tenant_id=$1 AND id=$2)`, p.TenantID, id).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return httpx.NotFound("Simulation")
			}
			g, err := loadGraph(r.Context(), tx, p.TenantID, id)
			if err != nil {
				return err
			}
			targets, err := loadTargets(r.Context(), tx, p.TenantID, id)
			if err != nil {
				return err
			}
			execs, found, err := loadState(r.Context(), tx, p.TenantID, id)
			if err != nil {
				return err
			}
			recorded := map[string]string{}
			dr, err := tx.Query(r.Context(), `SELECT d.asset_id::text,s.key,d.reason FROM simulation_decisions d JOIN simulation_steps s ON s.tenant_id=d.tenant_id AND s.id=d.step_id WHERE d.tenant_id=$1 AND d.simulation_id=$2`, p.TenantID, id)
			if err != nil {
				return err
			}
			for dr.Next() {
				var asset, key, reason string
				if err = dr.Scan(&asset, &key, &reason); err != nil {
					dr.Close()
					return err
				}
				recorded[asset+"|"+key] = reason
			}
			dr.Close()
			if err = dr.Err(); err != nil {
				return err
			}
			for _, t := range targets {
				xs, fs := scoped(execs, found, t.ID.String())
				res := chain.Evaluate(g.chainSteps(), g.edges, xs, fs)
				for _, st := range g.steps {
					row := map[string]any{"asset_id": t.ID, "step": st.Key, "status": res.Status[st.Key], "reason": res.Reasons[st.Key], "recorded_reason": recorded[t.ID.String()+"|"+st.Key]}
					inputs, e := mappedInputs(st, t, g.edges, fs)
					if e != nil {
						row["mapping_error"] = e.Error()
					} else {
						preview := []map[string]any{}
						for _, in := range inputs {
							addr, e := mappedTarget(h.Svc.Engine, in.step, in.target)
							if e != nil {
								row["mapping_error"] = e.Error()
								break
							}
							preview = append(preview, map[string]any{"params": json.RawMessage(in.step.Params), "target": addr})
						}
						row["inputs"] = preview
					}
					items = append(items, row)
				}
			}
			return nil
		})
		reply(w, r, 200, map[string]any{"items": items, "executed": false}, err)
	}))
}

func mappedTarget(e *Engine, s stepRow, t target) (string, error) {
	if d := contract.Find(s.ContractKey); d != nil && d.NeedsAgent {
		return "local", nil
	}
	return e.Guard.Target(s.ContractKey, t.Addrs)
}
