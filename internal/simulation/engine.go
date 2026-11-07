// Package simulation: peluncuran, mesin chain, dan pengendalian simulasi (jeda, lanjut, batal).
package simulation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"orchis-go/internal/scancontrol"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/alerts"
	"orchis-go/internal/chain"
	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
	"orchis-go/internal/ids"
	"orchis-go/internal/scanscope"
)

// Engine memajukan chain sebuah simulasi. Semua langkahnya terjadi dalam transaksi pemanggil.
type Engine struct {
	Now   func() time.Time
	Guard contract.Guard
}

type stepRow struct {
	ID          uuid.UUID
	Key         string
	Enabled     bool
	Mode        string
	Trigger     *chain.Trigger
	Delay       int
	ContractID  uuid.UUID
	ContractVer int
	ContractKey string
	Integration uuid.UUID
	Params      json.RawMessage
	Title       string
}

type graph struct {
	steps []stepRow
	edges []chain.Edge
}

func (g graph) chainSteps() []chain.Step {
	out := make([]chain.Step, len(g.steps))
	for i, s := range g.steps {
		out[i] = chain.Step{Key: s.Key, Enabled: s.Enabled, Mode: s.Mode, Trigger: s.Trigger, DelaySeconds: s.Delay}
	}
	return out
}

func loadGraph(ctx context.Context, tx pgx.Tx, tenant, sim uuid.UUID) (graph, error) {
	var g graph
	rows, err := tx.Query(ctx, `
SELECT s.id, s.key, s.title, s.is_enabled, s.trigger_mode, s.trigger, s.delay_seconds, s.contract_id, s.contract_version, c.key, c.integration_id, s.params
FROM simulation_steps s JOIN contracts c ON c.tenant_id=s.tenant_id AND c.id=s.contract_id
WHERE s.tenant_id=$1 AND s.simulation_id=$2 ORDER BY s.key`, tenant, sim)
	if err != nil {
		return g, err
	}
	for rows.Next() {
		var s stepRow
		var trig []byte
		if err := rows.Scan(&s.ID, &s.Key, &s.Title, &s.Enabled, &s.Mode, &trig, &s.Delay, &s.ContractID, &s.ContractVer, &s.ContractKey, &s.Integration, &s.Params); err != nil {
			rows.Close()
			return g, err
		}
		if len(trig) > 0 && string(trig) != "null" {
			s.Trigger = &chain.Trigger{}
			if err := json.Unmarshal(trig, s.Trigger); err != nil {
				rows.Close()
				return g, err
			}
		}
		g.steps = append(g.steps, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return g, err
	}
	er, err := tx.Query(ctx, `
SELECT p.key, c.key,e.rule FROM simulation_step_edges e
JOIN simulation_steps p ON p.tenant_id=e.tenant_id AND p.id=e.parent_step_id
JOIN simulation_steps c ON c.tenant_id=e.tenant_id AND c.id=e.child_step_id
WHERE e.tenant_id=$1 AND e.simulation_id=$2`, tenant, sim)
	if err != nil {
		return g, err
	}
	defer er.Close()
	for er.Next() {
		var e chain.Edge
		var rule []byte
		if err := er.Scan(&e.Parent, &e.Child, &rule); err != nil {
			return g, err
		}
		if err := json.Unmarshal(rule, &e); err != nil {
			return g, err
		}
		g.edges = append(g.edges, e)
	}
	return g, er.Err()
}

// state memuat eksekusi dan finding simulasi dalam bentuk yang dimengerti chain.Evaluate.
func loadState(ctx context.Context, tx pgx.Tx, tenant, sim uuid.UUID) ([]chain.Exec, []chain.Found, error) {
	rows, err := tx.Query(ctx, `
SELECT s.key, e.state, COALESCE(e.outcome,''),e.target_asset_id FROM executions e
JOIN simulation_steps s ON s.tenant_id=e.tenant_id AND s.id=e.simulation_step_id
WHERE e.tenant_id=$1 AND e.simulation_id=$2`, tenant, sim)
	if err != nil {
		return nil, nil, err
	}
	var execs []chain.Exec
	for rows.Next() {
		var x chain.Exec
		if err := rows.Scan(&x.Step, &x.State, &x.Outcome, &x.Asset); err != nil {
			rows.Close()
			return nil, nil, err
		}
		execs = append(execs, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	fr, err := tx.Query(ctx, `
SELECT DISTINCT f.id, s.key, COALESCE(o.finding_snapshot->>'type',f.type), COALESCE(o.finding_snapshot->>'value',f.value), COALESCE(o.finding_snapshot->>'severity',f.severity),e.target_asset_id,e.target_address
FROM finding_occurrences o
JOIN executions e ON e.tenant_id=o.tenant_id AND e.id=o.execution_id
JOIN simulation_steps s ON s.tenant_id=e.tenant_id AND s.id=e.simulation_step_id
JOIN findings f ON f.tenant_id=o.tenant_id AND f.id=o.finding_id
WHERE o.tenant_id=$1 AND o.simulation_id=$2`, tenant, sim)
	if err != nil {
		return nil, nil, err
	}
	defer fr.Close()
	var found []chain.Found
	for fr.Next() {
		var f chain.Found
		var id uuid.UUID
		if err := fr.Scan(&id, &f.Step, &f.Type, &f.Value, &f.Severity, &f.Asset, &f.Target); err != nil {
			return nil, nil, err
		}
		f.ID = id.String()
		found = append(found, f)
	}
	return execs, found, fr.Err()
}

// Advance meluncurkan langkah yang kini boleh jalan dan menutup simulasi bila semuanya selesai. Idempoten: memanggilnya
// berulang tidak membuat eksekusi ganda (kunci baris + indeks idempotensi). Tidak berbuat apa-apa bila status bukan running.
func (e *Engine) Advance(ctx context.Context, tx pgx.Tx, tenant, sim uuid.UUID) error {
	if err := scancontrol.Lock(ctx, tx, tenant); err != nil {
		return err
	}
	var status string
	var synthetic bool
	err := tx.QueryRow(ctx, `SELECT status,is_demo FROM simulations WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenant, sim).Scan(&status, &synthetic)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if synthetic || status != "running" {
		return nil
	}
	g, err := loadGraph(ctx, tx, tenant, sim)
	if err != nil {
		return err
	}
	byKey := map[string]stepRow{}
	for _, s := range g.steps {
		byKey[s.Key] = s
	}
	targets, err := loadTargets(ctx, tx, tenant, sim)
	if err != nil {
		return err
	}
	// Setiap putaran meluncurkan satu "lapisan" langkah; eksekusi yang langsung gagal bisa membuka lapisan berikutnya.
	for round := 0; round <= chain.MaxSteps+1; round++ {
		execs, found, err := loadState(ctx, tx, tenant, sim)
		if err != nil {
			return err
		}
		res := chain.Evaluate(g.chainSteps(), g.edges, execs, found)
		if chain.HasAdvanced(g.edges) {
			finished, launched, err := e.advanceAdvanced(ctx, tx, tenant, sim, g, targets, execs, found)
			if err != nil {
				return err
			}
			res = chain.Result{Finished: finished}
			if launched > 0 {
				continue
			}
		}
		if len(res.Launch) == 0 {
			if res.Finished {
				if _, err := tx.Exec(ctx, `UPDATE simulations SET status=CASE WHEN EXISTS(SELECT 1 FROM executions x WHERE x.tenant_id=$1 AND x.simulation_id=$2 AND x.outcome='canceled' AND x.scan_stop_reason<>'') THEN 'canceled' ELSE 'finished' END, ended_at=now(), version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status='running'`, tenant, sim); err != nil {
					return err
				}
				var name string
				var number int64
				if err := tx.QueryRow(ctx, `SELECT name, number FROM simulations WHERE tenant_id=$1 AND id=$2`, tenant, sim).Scan(&name, &number); err != nil {
					return err
				}
				return alerts.Raise(ctx, tx, tenant, alerts.New{Kind: "simulation_finished", Severity: "info", Title: fmt.Sprintf("Simulation OP-%04d finished", number), Body: name,
					ResourceType: "simulation", ResourceID: &sim, DedupeKey: "sim-finished:" + sim.String()})
			}
			return nil
		}
		for _, l := range res.Launch {
			if err := e.launchStep(ctx, tx, tenant, sim, byKey[l.Step], targets, l.Triggers); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("simulasi %s tidak berhenti meluncurkan langkah", sim)
}

func execKindFor(needsAgent bool) string {
	if needsAgent {
		return "agent"
	}
	return "injector"
}

type target struct {
	OrganizationID *uuid.UUID
	ID             uuid.UUID
	Name           string
	Addrs          []contract.Address
}

func loadTargets(ctx context.Context, tx pgx.Tx, tenant, sim uuid.UUID) ([]target, error) {
	rows, err := tx.Query(ctx, `
SELECT a.id, a.name,t.scan_organization_id FROM simulation_targets t JOIN assets a ON a.tenant_id=t.tenant_id AND a.id=t.asset_id
WHERE t.tenant_id=$1 AND t.simulation_id=$2 ORDER BY a.name, a.id`, tenant, sim)
	if err != nil {
		return nil, err
	}
	var out []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.ID, &t.Name, &t.OrganizationID); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		var frozen []byte
		err := tx.QueryRow(ctx, `SELECT COALESCE(a.addresses,t.addresses) FROM simulation_targets t LEFT JOIN assessment_targets a ON a.tenant_id=t.tenant_id AND a.simulation_id=t.simulation_id AND a.asset_id=t.asset_id WHERE t.tenant_id=$1 AND t.simulation_id=$2 AND t.asset_id=$3`, tenant, sim, out[i].ID).Scan(&frozen)
		if err == nil && len(frozen) > 0 {
			if err = json.Unmarshal(frozen, &out[i].Addrs); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		ar, err := tx.Query(ctx, `SELECT kind, value, is_primary FROM asset_addresses WHERE tenant_id=$1 AND asset_id=$2 AND scan_approved ORDER BY is_primary DESC, created_at`, tenant, out[i].ID)
		if err != nil {
			return nil, err
		}
		for ar.Next() {
			var a contract.Address
			if err := ar.Scan(&a.Kind, &a.Value, &a.IsPrimary); err != nil {
				ar.Close()
				return nil, err
			}
			out[i].Addrs = append(out[i].Addrs, a)
		}
		ar.Close()
		if err := ar.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// launchStep membuat satu eksekusi per aset sasaran. Satu aset yang tidak bisa dipindai (alamat hilang, ditolak penjaga)
// menghasilkan eksekusi yang langsung berakhir error, supaya kegagalannya terlihat dan chain tetap bisa maju.
func (e *Engine) launchStep(ctx context.Context, tx pgx.Tx, tenant, sim uuid.UUID, s stepRow, targets []target, triggers []string) error {
	var integKey string
	if err := tx.QueryRow(ctx, `SELECT key FROM integrations WHERE tenant_id=$1 AND id=$2`, tenant, s.Integration).Scan(&integKey); err != nil {
		return err
	}
	notBefore := time.Now().Add(time.Duration(s.Delay) * time.Second)
	def := contract.FindVersion(s.ContractKey, s.ContractVer)
	needsAgent := def != nil && def.NeedsAgent
	for _, t := range targets {
		var addr string
		var gerr error
		var agentID *uuid.UUID
		if needsAgent {
			// Aksi agent berjalan di endpoint: tujuannya agent milik aset, bukan alamat jaringan.
			addr = "local"
			var aid uuid.UUID
			err := tx.QueryRow(ctx, `SELECT id FROM agents WHERE tenant_id=$1 AND asset_id=$2 AND NOT is_demo AND status<>'revoked' ORDER BY last_heartbeat_at DESC NULLS LAST LIMIT 1`, tenant, t.ID).Scan(&aid)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				gerr = errors.New("no agent is installed on this asset")
			case err != nil:
				return err
			default:
				agentID = &aid
			}
		} else if contract.NoNetworkTarget(def) {
			// DefenseBot works on data already stored for the asset: no host or URL, but authorization still applies.
			addr = "local"
			if _, err := scanscope.CheckJob(ctx, tx, tenant, sim, addr, e.Guard, 4096); err != nil {
				gerr = err
			}
		} else {
			addr, gerr = e.Guard.Target(s.ContractKey, t.Addrs)
			if gerr == nil {
				// Batas khusus deployment telah diperiksa worker sebelum memanggil engine; 4096 adalah maksimum konfigurasi.
				proof, err := scanscope.CheckJob(ctx, tx, tenant, sim, addr, e.Guard, 4096)
				if err != nil {
					gerr = err
				} else if proof == nil || proof.CIDR == "" {
					approved, err := execution.AgentTargetConfirmed(ctx, tx, tenant, t.ID, addr)
					if err != nil {
						return err
					}
					if !approved {
						gerr = errors.New("agent-reported target requires administrator confirmation")
					}
				}
			}
		}
		if !needsAgent {
			if err := scancontrol.CheckAsset(ctx, tx, tenant, t.ID, t.OrganizationID, scancontrol.Now(e.Now)); err != nil {
				gerr = err
			}
		}
		sum := sha256.Sum256([]byte(s.ID.String() + "|" + t.ID.String() + "|" + addr + "|" + string(s.Params)))
		execID := ids.New()
		var inserted uuid.UUID
		var err error
		if gerr == nil {
			err = tx.QueryRow(ctx, `
INSERT INTO executions (id, tenant_id, simulation_id, simulation_step_id, contract_id, contract_version, target_asset_id, target_address,
  executor_kind, integration_id, params, input_hash, not_before, agent_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$13,$9,$10,$11,$12,$14)
ON CONFLICT (tenant_id, simulation_step_id, target_asset_id, input_hash) WHERE simulation_step_id IS NOT NULL DO NOTHING
RETURNING id`, execID, tenant, sim, s.ID, s.ContractID, s.ContractVer, t.ID, addr, s.Integration, []byte(s.Params), sum[:], notBefore, execKindFor(needsAgent), agentID).Scan(&inserted)
		} else {
			err = tx.QueryRow(ctx, `
INSERT INTO executions (id, tenant_id, simulation_id, simulation_step_id, contract_id, contract_version, target_asset_id, target_address,
  executor_kind, integration_id, params, input_hash, state, outcome, error, started_at, ended_at,scan_stop_reason)
VALUES ($1,$2,$3,$4,$5,$6,$7,'',  $12,$8,$9,$10,'ended',$13,$11,now(),now(),$14)
ON CONFLICT (tenant_id, simulation_step_id, target_asset_id, input_hash) WHERE simulation_step_id IS NOT NULL DO NOTHING
RETURNING id`, execID, tenant, sim, s.ID, s.ContractID, s.ContractVer, t.ID, s.Integration, []byte(s.Params), sum[:], gerr.Error(), execKindFor(needsAgent), scancontrol.Outcome(gerr), scancontrol.Code(gerr)).Scan(&inserted)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			continue // sudah ada (idempoten)
		}
		if err != nil {
			return err
		}
		msg := fmt.Sprintf("Queued by chain step %q on %s.", s.Key, t.Name)
		if gerr != nil {
			msg = fmt.Sprintf("Chain step %q could not target %s: %v.", s.Key, t.Name, gerr)
		}
		lvl := "info"
		if gerr != nil {
			lvl = "error"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO execution_logs (tenant_id, execution_id, level, message) VALUES ($1,$2,$3,$4)`, tenant, inserted, lvl, msg); err != nil {
			return err
		}
		for _, f := range triggers {
			fid, perr := uuid.Parse(f)
			if perr != nil {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO finding_triggers (tenant_id, finding_id, execution_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, tenant, fid, inserted); err != nil {
				return err
			}
		}
		if gerr != nil || needsAgent {
			continue // gagal sebelum jalan, atau agent mengambil sendiri tugasnya (tanpa pesan antrean)
		}
		if err := execution.EnqueueAt(ctx, tx, tenant, execution.TopicDispatch, "exec."+integKey, "execution", inserted,
			map[string]any{"execution_id": inserted, "tenant_id": tenant, "integration": integKey}, notBefore); err != nil {
			return err
		}
	}
	return nil
}
