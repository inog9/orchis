package simulation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/alerts"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/chain"
	"orchis-go/internal/contract"
	"orchis-go/internal/db"
	"orchis-go/internal/execution"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
)

type Service struct {
	DB     *db.DB
	Engine *Engine
}

type Simulation struct {
	ID           uuid.UUID  `json:"id"`
	Number       int64      `json:"number"`
	Code         string     `json:"code"` // OP-0142
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	ScenarioID   *uuid.UUID `json:"scenario_id"`
	ScenarioName *string    `json:"scenario_name"`
	SafeMode     bool       `json:"safe_mode"`
	TimeoutSecs  int        `json:"timeout_seconds"`
	CancelReason *string    `json:"cancel_reason"`
	StartedAt    *time.Time `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at"`
	CreatedAt    time.Time  `json:"created_at"`
	TargetCount  int        `json:"target_count"`
	StepCount    int        `json:"step_count"`
	StepsDone    int        `json:"steps_done"` // langkah berstatus akhir
	FindingCount int        `json:"finding_count"`
}

type StepView struct {
	Key          string         `json:"key"`
	Title        string         `json:"title"`
	ContractKey  string         `json:"contract_key"`
	ContractName string         `json:"contract_name"`
	Status       string         `json:"status"`
	TriggerMode  string         `json:"trigger_mode"`
	Trigger      *chain.Trigger `json:"trigger"`
	DelaySeconds int            `json:"delay_seconds"`
	IsEnabled    bool           `json:"is_enabled"`
	Executions   map[string]int `json:"executions"` // total, running, queued, success, failed
	FindingCount int            `json:"finding_count"`
}

type TargetView struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Detail struct {
	Simulation
	Steps   []StepView   `json:"steps"`
	Edges   []chain.Edge `json:"edges"`
	Targets []TargetView `json:"targets"`
}

func terminalStatus(s string) bool {
	switch s {
	case chain.StDone, chain.StFailed, chain.StSkipped, chain.StBlocked, "canceled":
		return true
	}
	return false
}

func code(n int64) string { return fmt.Sprintf("OP-%04d", n) }

const simCols = `s.id, s.number, s.name, s.status, s.scenario_id, sc.name, s.safe_mode, s.timeout_seconds, s.cancel_reason, s.started_at, s.ended_at, s.created_at,
 (SELECT count(*) FROM simulation_targets t WHERE t.tenant_id=s.tenant_id AND t.simulation_id=s.id),
 (SELECT count(*) FROM simulation_steps t WHERE t.tenant_id=s.tenant_id AND t.simulation_id=s.id),
 (SELECT count(DISTINCT o.finding_id) FROM finding_occurrences o WHERE o.tenant_id=s.tenant_id AND o.simulation_id=s.id)`
const simFrom = `FROM simulations s LEFT JOIN scenarios sc ON sc.tenant_id=s.tenant_id AND sc.id=s.scenario_id`

func scanSim(row pgx.Row) (Simulation, error) {
	var s Simulation
	err := row.Scan(&s.ID, &s.Number, &s.Name, &s.Status, &s.ScenarioID, &s.ScenarioName, &s.SafeMode, &s.TimeoutSecs, &s.CancelReason, &s.StartedAt, &s.EndedAt, &s.CreatedAt,
		&s.TargetCount, &s.StepCount, &s.FindingCount)
	s.Code = code(s.Number)
	return s, err
}

// ---------- peluncuran ----------

type LaunchInput struct {
	Name string `json:"name"`
}

func (s *Service) Launch(ctx context.Context, p *auth.Principal, scenarioID uuid.UUID, in LaunchInput, ip, reqID string) (*Detail, error) {
	name := strings.TrimSpace(in.Name)
	if len([]rune(name)) > 200 {
		return nil, httpx.Validation(httpx.FieldError{Name: "name", Message: "At most 200 characters."})
	}
	simID := ids.New()
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var scName string
		err := tx.QueryRow(ctx, `SELECT name FROM scenarios WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL FOR SHARE`, p.TenantID, scenarioID).Scan(&scName)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Scenario")
		}
		if err != nil {
			return err
		}
		if name == "" {
			name = scName
		}
		// aset sasaran: aset langsung + anggota grup saat ini (grup dinamis dibekukan pada saat peluncuran)
		rows, err := tx.Query(ctx, `
SELECT DISTINCT a.id, a.name, a.testing_authorized FROM assets a WHERE a.tenant_id=$1 AND a.deleted_at IS NULL AND (
  a.id IN (SELECT asset_id FROM scenario_targets WHERE tenant_id=$1 AND scenario_id=$2 AND asset_id IS NOT NULL)
  OR a.id IN (SELECT m.asset_id FROM scenario_targets t JOIN asset_group_members m ON m.tenant_id=t.tenant_id AND m.asset_group_id=t.asset_group_id
              WHERE t.tenant_id=$1 AND t.scenario_id=$2 AND t.asset_group_id IS NOT NULL))
ORDER BY a.name, a.id`, p.TenantID, scenarioID)
		if err != nil {
			return err
		}
		var targets []target
		var unauthorized []string
		for rows.Next() {
			var t target
			var ok bool
			if err := rows.Scan(&t.ID, &t.Name, &ok); err != nil {
				rows.Close()
				return err
			}
			if !ok {
				unauthorized = append(unauthorized, t.Name)
			}
			targets = append(targets, t)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		fail := func(msg string) error { return httpx.Validation(httpx.FieldError{Name: "scenario", Message: msg}) }
		if len(targets) == 0 {
			return fail("This chain has no target assets. Choose the assets or groups it targets first.")
		}
		if len(unauthorized) > 0 {
			if len(unauthorized) > 5 {
				unauthorized = append(unauthorized[:5], fmt.Sprintf("and %d more", len(unauthorized)-5))
			}
			return fail("These assets are not authorized for testing: " + strings.Join(unauthorized, ", ") + ". An administrator must authorize them first.")
		}
		steps, err := tx.Query(ctx, `
SELECT st.id, st.key, st.title, COALESCE(st.description,''), st.contract_id, st.contract_version, st.params, st.delay_seconds, st.trigger_mode, st.trigger, st.is_enabled,
  st.position_x, st.position_y, c.key, i.enabled, i.key
FROM scenario_steps st JOIN contracts c ON c.tenant_id=st.tenant_id AND c.id=st.contract_id JOIN integrations i ON i.tenant_id=c.tenant_id AND i.id=c.integration_id
WHERE st.tenant_id=$1 AND st.scenario_id=$2`, p.TenantID, scenarioID)
		if err != nil {
			return err
		}
		type srcStep struct {
			id, contractID         uuid.UUID
			key, title, desc, ckey string
			ikey                   string
			ver, delay             int
			params, trig           []byte
			mode                   string
			enabled, ienabled      bool
			px, py                 float64
		}
		var src []srcStep
		for steps.Next() {
			var x srcStep
			if err := steps.Scan(&x.id, &x.key, &x.title, &x.desc, &x.contractID, &x.ver, &x.params, &x.delay, &x.mode, &x.trig, &x.enabled, &x.px, &x.py, &x.ckey, &x.ienabled, &x.ikey); err != nil {
				steps.Close()
				return err
			}
			src = append(src, x)
		}
		steps.Close()
		if err := steps.Err(); err != nil {
			return err
		}
		enabledCount := 0
		for _, x := range src {
			if !x.enabled {
				continue
			}
			enabledCount++
			if !x.ienabled {
				return fail("The " + x.ikey + " injector is disabled, but step " + x.key + " needs it.")
			}
		}
		if enabledCount == 0 {
			return fail("This chain has no enabled steps.")
		}
		// penjaga scope di muka: setiap aset harus bisa ditargetkan oleh setiap langkah aktif, supaya kegagalan terlihat sebelum jalan
		for i := range targets {
			ar, err := tx.Query(ctx, `SELECT kind, value, is_primary FROM asset_addresses WHERE tenant_id=$1 AND asset_id=$2 ORDER BY is_primary DESC, created_at`, p.TenantID, targets[i].ID)
			if err != nil {
				return err
			}
			for ar.Next() {
				var a contract.Address
				if err := ar.Scan(&a.Kind, &a.Value, &a.IsPrimary); err != nil {
					ar.Close()
					return err
				}
				targets[i].Addrs = append(targets[i].Addrs, a)
			}
			ar.Close()
			if err := ar.Err(); err != nil {
				return err
			}
		}
		for _, x := range src {
			if !x.enabled {
				continue
			}
			needsAgent := contract.Find(x.ckey) != nil && contract.Find(x.ckey).NeedsAgent
			for _, t := range targets {
				if needsAgent {
					var has bool
					if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE tenant_id=$1 AND asset_id=$2 AND status<>'revoked')`, p.TenantID, t.ID).Scan(&has); err != nil {
						return err
					}
					if !has {
						return fail(fmt.Sprintf("Step %s runs on the endpoint, but %s has no agent. Install one from Assets → Agents first.", x.key, t.Name))
					}
					continue
				}
				if _, err := s.Engine.Guard.Target(x.ckey, t.Addrs); err != nil {
					return fail(fmt.Sprintf("Step %s cannot target %s: %v.", x.key, t.Name, err))
				}
			}
		}

		if _, err := tx.Exec(ctx, `INSERT INTO simulations (id, tenant_id, scenario_id, name, status, started_at, launched_by) VALUES ($1,$2,$3,$4,'running',now(),$5)`,
			simID, p.TenantID, scenarioID, name, p.UserID); err != nil {
			return err
		}
		newID := map[uuid.UUID]uuid.UUID{}
		for _, x := range src {
			nid := ids.New()
			newID[x.id] = nid
			if _, err := tx.Exec(ctx, `
INSERT INTO simulation_steps (id, tenant_id, simulation_id, key, title, contract_id, contract_version, params, delay_seconds, trigger_mode, trigger, is_enabled, position_x, position_y)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, nid, p.TenantID, simID, x.key, x.title, x.contractID, x.ver, x.params, x.delay, x.mode, x.trig, x.enabled, x.px, x.py); err != nil {
				return err
			}
		}
		er, err := tx.Query(ctx, `SELECT parent_step_id, child_step_id,rule FROM scenario_step_edges WHERE tenant_id=$1 AND scenario_id=$2`, p.TenantID, scenarioID)
		if err != nil {
			return err
		}
		type edgeCopy struct {
			parent, child uuid.UUID
			rule          []byte
		}
		var edges []edgeCopy
		for er.Next() {
			var a, b uuid.UUID
			var rule []byte
			if err := er.Scan(&a, &b, &rule); err != nil {
				er.Close()
				return err
			}
			edges = append(edges, edgeCopy{a, b, rule})
		}
		er.Close()
		if err := er.Err(); err != nil {
			return err
		}
		for _, e := range edges {
			if _, err := tx.Exec(ctx, `INSERT INTO simulation_step_edges (tenant_id, simulation_id, parent_step_id, child_step_id,rule) VALUES ($1,$2,$3,$4,$5)`, p.TenantID, simID, newID[e.parent], newID[e.child], e.rule); err != nil {
				return err
			}
		}
		for _, t := range targets {
			addrs, _ := json.Marshal(t.Addrs)
			if _, err := tx.Exec(ctx, `INSERT INTO simulation_targets (tenant_id, simulation_id, asset_id,addresses) VALUES ($1,$2,$3,$4)`, p.TenantID, simID, t.ID, addrs); err != nil {
				return err
			}
		}
		if err := s.Engine.Advance(ctx, tx, p.TenantID, simID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "simulation.launch", ResourceType: "simulation", ResourceID: &simID, IP: ip, RequestID: reqID,
			Detail: map[string]any{"scenario_id": scenarioID, "targets": len(targets), "steps": enabledCount}})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, simID)
}

// ---------- baca ----------

type ListFilter struct{ Status string }

func (s *Service) List(ctx context.Context, p *auth.Principal, f ListFilter) ([]Simulation, error) {
	out := []Simulation{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		q := `SELECT ` + simCols + ` ` + simFrom + ` WHERE s.tenant_id=$1`
		args := []any{p.TenantID}
		if f.Status != "" {
			q += ` AND s.status=$2`
			args = append(args, f.Status)
		}
		rows, err := tx.Query(ctx, q+` ORDER BY s.created_at DESC, s.id LIMIT 200`, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			sim, err := scanSim(rows)
			if err != nil {
				rows.Close()
				return err
			}
			out = append(out, sim)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range out {
			done, err := stepsDone(ctx, tx, p.TenantID, out[i].ID, out[i].Status)
			if err != nil {
				return err
			}
			out[i].StepsDone = done
		}
		return nil
	})
	return out, err
}

// stepsDone menghitung langkah berstatus akhir lewat evaluasi yang sama dengan mesin.
func stepsDone(ctx context.Context, tx pgx.Tx, tenant, sim uuid.UUID, simStatus string) (int, error) {
	views, _, err := stepViews(ctx, tx, tenant, sim, simStatus)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, v := range views {
		if terminalStatus(v.Status) {
			n++
		}
	}
	return n, nil
}

func stepViews(ctx context.Context, tx pgx.Tx, tenant, sim uuid.UUID, simStatus string) ([]StepView, []chain.Edge, error) {
	g, err := loadGraph(ctx, tx, tenant, sim)
	if err != nil {
		return nil, nil, err
	}
	execs, found, err := loadState(ctx, tx, tenant, sim)
	if err != nil {
		return nil, nil, err
	}
	res := chain.Evaluate(g.chainSteps(), g.edges, execs, found)
	if chain.HasAdvanced(g.edges) {
		targets, e := loadTargets(ctx, tx, tenant, sim)
		if e != nil {
			return nil, nil, e
		}
		res = advancedSummary(g, execs, found, targets)
	}
	counts := map[string]map[string]int{}
	for _, x := range execs {
		c := counts[x.Step]
		if c == nil {
			c = map[string]int{"total": 0, "running": 0, "queued": 0, "success": 0, "failed": 0, "canceled": 0}
			counts[x.Step] = c
		}
		c["total"]++
		switch {
		case x.State == "running":
			c["running"]++
		case x.State == "ready":
			c["queued"]++
		case x.Outcome == "success":
			c["success"]++
		case x.Outcome == "canceled":
			c["canceled"]++
		default:
			c["failed"]++
		}
	}
	fc := map[string]map[string]bool{}
	for _, f := range found {
		if fc[f.Step] == nil {
			fc[f.Step] = map[string]bool{}
		}
		fc[f.Step][f.ID] = true
	}
	names := map[string]string{}
	rows, err := tx.Query(ctx, `SELECT DISTINCT c.key, c.name FROM simulation_steps s JOIN contracts c ON c.tenant_id=s.tenant_id AND c.id=s.contract_id WHERE s.tenant_id=$1 AND s.simulation_id=$2`, tenant, sim)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var k, n string
		if err := rows.Scan(&k, &n); err != nil {
			rows.Close()
			return nil, nil, err
		}
		names[k] = n
	}
	rows.Close()
	out := make([]StepView, 0, len(g.steps))
	for _, st := range g.steps {
		status := res.Status[st.Key]
		// Langkah yang tak pernah jalan pada simulasi yang dihentikan ditandai dilewati, bukan "menunggu" selamanya.
		if status == chain.StEligible || status == chain.StWaiting {
			if simStatus == "canceled" || simStatus == "failed" {
				status = chain.StSkipped
			} else {
				status = chain.StWaiting
			}
		}
		c := counts[st.Key]
		if c == nil {
			c = map[string]int{"total": 0, "running": 0, "queued": 0, "success": 0, "failed": 0, "canceled": 0}
		}
		if status == chain.StFailed && c["total"] > 0 && c["canceled"] == c["total"] {
			status = "canceled" // dihentikan pengguna atau batas waktu, bukan gagal sendiri
		}
		out = append(out, StepView{Key: st.Key, Title: st.Title, ContractKey: st.ContractKey, ContractName: names[st.ContractKey], Status: status,
			TriggerMode: st.Mode, Trigger: st.Trigger, DelaySeconds: st.Delay, IsEnabled: st.Enabled, Executions: c, FindingCount: len(fc[st.Key])})
	}
	return out, g.edges, nil
}

func (s *Service) Get(ctx context.Context, p *auth.Principal, id uuid.UUID) (*Detail, error) {
	var out *Detail
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		sim, err := scanSim(tx.QueryRow(ctx, `SELECT `+simCols+` `+simFrom+` WHERE s.tenant_id=$1 AND s.id=$2`, p.TenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Simulation")
		}
		if err != nil {
			return err
		}
		views, edges, err := stepViews(ctx, tx, p.TenantID, id, sim.Status)
		if err != nil {
			return err
		}
		for _, v := range views {
			if terminalStatus(v.Status) {
				sim.StepsDone++
			}
		}
		d := &Detail{Simulation: sim, Steps: views, Edges: edges, Targets: []TargetView{}}
		if d.Edges == nil {
			d.Edges = []chain.Edge{}
		}
		rows, err := tx.Query(ctx, `SELECT a.id, a.name FROM simulation_targets t JOIN assets a ON a.tenant_id=t.tenant_id AND a.id=t.asset_id WHERE t.tenant_id=$1 AND t.simulation_id=$2 ORDER BY a.name`, p.TenantID, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t TargetView
			if err := rows.Scan(&t.ID, &t.Name); err != nil {
				return err
			}
			d.Targets = append(d.Targets, t)
		}
		out = d
		return rows.Err()
	})
	return out, err
}

// ---------- kendali ----------

func (s *Service) transition(ctx context.Context, p *auth.Principal, id uuid.UUID, from []string, to, action, ip, reqID string, extra func(ctx context.Context, tx pgx.Tx) error) (*Detail, error) {
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var cur string
		err := tx.QueryRow(ctx, `SELECT status FROM simulations WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, id).Scan(&cur)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Simulation")
		}
		if err != nil {
			return err
		}
		ok := false
		for _, f := range from {
			ok = ok || f == cur
		}
		if !ok {
			return httpx.Conflict("A " + cur + " simulation cannot be " + action + ".")
		}
		if _, err := tx.Exec(ctx, `UPDATE simulations SET status=$3, version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id, to); err != nil {
			return err
		}
		if extra != nil {
			if err := extra(ctx, tx); err != nil {
				return err
			}
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "simulation." + action, ResourceType: "simulation", ResourceID: &id, IP: ip, RequestID: reqID})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

func (s *Service) Pause(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) (*Detail, error) {
	return s.transition(ctx, p, id, []string{"running"}, "paused", "pause", ip, reqID, nil)
}

// Resume melanjutkan simulasi: pesan dispatch eksekusi ready diterbitkan ulang (injector membuang yang diterima saat jeda), lalu chain dimajukan.
func (s *Service) Resume(ctx context.Context, p *auth.Principal, id uuid.UUID, ip, reqID string) (*Detail, error) {
	return s.transition(ctx, p, id, []string{"paused"}, "running", "resume", ip, reqID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT e.id, i.key, e.not_before FROM executions e JOIN integrations i ON i.tenant_id=e.tenant_id AND i.id=e.integration_id
WHERE e.tenant_id=$1 AND e.simulation_id=$2 AND e.state='ready' AND e.executor_kind='injector'`, p.TenantID, id)
		if err != nil {
			return err
		}
		type ready struct {
			id  uuid.UUID
			key string
			at  time.Time
		}
		var rs []ready
		for rows.Next() {
			var r ready
			if err := rows.Scan(&r.id, &r.key, &r.at); err != nil {
				rows.Close()
				return err
			}
			rs = append(rs, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, r := range rs {
			if err := execution.EnqueueAt(ctx, tx, p.TenantID, execution.TopicDispatch, "exec."+r.key, "execution", r.id,
				map[string]any{"execution_id": r.id, "tenant_id": p.TenantID, "integration": r.key}, r.at); err != nil {
				return err
			}
		}
		return s.Engine.Advance(ctx, tx, p.TenantID, id)
	})
}

type CancelInput struct {
	Reason string `json:"reason"`
}

func (s *Service) Cancel(ctx context.Context, p *auth.Principal, id uuid.UUID, in CancelInput, ip, reqID string) (*Detail, error) {
	reason := strings.TrimSpace(in.Reason)
	if len(reason) > 500 {
		return nil, httpx.Validation(httpx.FieldError{Name: "reason", Message: "At most 500 characters."})
	}
	return s.transition(ctx, p, id, []string{"running", "paused", "draft", "scheduled"}, "canceled", "cancel", ip, reqID, func(ctx context.Context, tx pgx.Tx) error {
		return stop(ctx, tx, p.TenantID, id, "canceled", &p.UserID, reason, "Canceled by "+p.Email+".")
	})
}

// stop menutup simulasi dan menghentikan semua eksekusinya. ready/running -> ended(canceled); injector yang sedang menjalankan
// mengetahuinya dari heartbeat berikutnya.
func stop(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, status string, by *uuid.UUID, reason, logMsg string) error {
	if _, err := tx.Exec(ctx, `UPDATE simulations SET status=$3, ended_at=now(), canceled_by=$4, cancel_reason=NULLIF($5,''), updated_at=now() WHERE tenant_id=$1 AND id=$2`,
		tenant, id, status, by, reason); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
UPDATE executions SET state='ended', outcome='canceled', ended_at=now(), lease_until=NULL, version=version+1, updated_at=now()
WHERE tenant_id=$1 AND simulation_id=$2 AND state IN ('ready','running') RETURNING id`, tenant, id)
	if err != nil {
		return err
	}
	var ended []uuid.UUID
	for rows.Next() {
		var x uuid.UUID
		if err := rows.Scan(&x); err != nil {
			rows.Close()
			return err
		}
		ended = append(ended, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, x := range ended {
		if _, err := tx.Exec(ctx, `INSERT INTO execution_logs (tenant_id, execution_id, level, message) VALUES ($1,$2,'warn',$3)`, tenant, x, logMsg); err != nil {
			return err
		}
	}
	return nil
}

// Tick dijalankan berkala: memajukan simulasi yang berjalan (jaring pengaman) dan menggagalkan yang melewati batas waktu.
func (s *Service) Tick(ctx context.Context, tenant uuid.UUID) (advanced, timedOut int, err error) {
	var running, expired []uuid.UUID
	err = s.DB.InTenantRead(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, (started_at + make_interval(secs => timeout_seconds) < now()) FROM simulations WHERE tenant_id=$1 AND status IN ('running','paused')`, tenant)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			var exp bool
			if err := rows.Scan(&id, &exp); err != nil {
				return err
			}
			if exp {
				expired = append(expired, id)
			} else {
				running = append(running, id)
			}
		}
		return rows.Err()
	})
	if err != nil {
		return
	}
	for _, id := range expired {
		err = s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error {
			var st string
			if err := tx.QueryRow(ctx, `SELECT status FROM simulations WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenant, id).Scan(&st); err != nil {
				return err
			}
			if st != "running" && st != "paused" {
				return nil
			}
			timedOut++
			if err := stop(ctx, tx, tenant, id, "failed", nil, "The simulation exceeded its time limit.", "Stopped: the simulation exceeded its time limit."); err != nil {
				return err
			}
			var name string
			var number int64
			if err := tx.QueryRow(ctx, `SELECT name, number FROM simulations WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&name, &number); err != nil {
				return err
			}
			return alerts.Raise(ctx, tx, tenant, alerts.New{Kind: "simulation_failed", Severity: "warning", Title: fmt.Sprintf("Simulation OP-%04d stopped: time limit", number), Body: name,
				ResourceType: "simulation", ResourceID: &id, DedupeKey: "sim-failed:" + id.String()})
		})
		if err != nil {
			return
		}
	}
	for _, id := range running {
		if err = s.DB.InTenant(ctx, tenant, func(tx pgx.Tx) error { return s.Engine.Advance(ctx, tx, tenant, id) }); err != nil {
			return
		}
		advanced++
	}
	return
}

// RunTicker menjalankan Tick berkala sampai ctx selesai.
func (s *Service) RunTicker(ctx context.Context, tenant uuid.UUID, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, _, err := s.Tick(ctx, tenant); err != nil {
				slog.Error("tick simulasi gagal", "err", err)
			}
		}
	}
}

// ---------- attack path ----------

type PathNode struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"` // step | asset | finding
	Label    string `json:"label"`
	Sub      string `json:"sub,omitempty"`
	Status   string `json:"status,omitempty"`   // langkah
	Severity string `json:"severity,omitempty"` // finding
	Column   int    `json:"column"`             // 0 aksi awal, 1 sasaran, 2 temuan, 3 aksi lanjutan
}

type PathEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // targets | found | triggers | then
}

type AttackPath struct {
	Nodes     []PathNode `json:"nodes"`
	Edges     []PathEdge `json:"edges"`
	Truncated bool       `json:"truncated"`
}

const maxPathFindings = 100

// AttackPath diturunkan (tanpa tabel sendiri) dari langkah, eksekusi, finding, dan finding_triggers.
func (s *Service) AttackPath(ctx context.Context, p *auth.Principal, id uuid.UUID) (*AttackPath, error) {
	out := &AttackPath{Nodes: []PathNode{}, Edges: []PathEdge{}}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM simulations WHERE tenant_id=$1 AND id=$2`, p.TenantID, id).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Simulation")
		} else if err != nil {
			return err
		}
		views, edges, err := stepViews(ctx, tx, p.TenantID, id, status)
		if err != nil {
			return err
		}
		hasParent := map[string]bool{}
		for _, e := range edges {
			hasParent[e.Child] = true
		}
		mode := map[string]string{}
		for _, v := range views {
			mode[v.Key] = v.TriggerMode
		}
		for _, v := range views {
			col := 0
			if hasParent[v.Key] {
				col = 3
			}
			out.Nodes = append(out.Nodes, PathNode{ID: "step:" + v.Key, Kind: "step", Label: v.Title, Sub: v.ContractName, Status: v.Status, Column: col})
		}
		// sasaran per langkah
		rows, err := tx.Query(ctx, `
SELECT DISTINCT s.key, a.id, a.name FROM executions e
JOIN simulation_steps s ON s.tenant_id=e.tenant_id AND s.id=e.simulation_step_id
JOIN assets a ON a.tenant_id=e.tenant_id AND a.id=e.target_asset_id
WHERE e.tenant_id=$1 AND e.simulation_id=$2`, p.TenantID, id)
		if err != nil {
			return err
		}
		seenAsset := map[string]bool{}
		for rows.Next() {
			var key, name string
			var aid uuid.UUID
			if err := rows.Scan(&key, &aid, &name); err != nil {
				rows.Close()
				return err
			}
			an := "asset:" + aid.String()
			if !seenAsset[an] {
				seenAsset[an] = true
				out.Nodes = append(out.Nodes, PathNode{ID: an, Kind: "asset", Label: name, Column: 1})
			}
			// Aksi lanjutan menyasar aset yang sama; menggambarnya lagi hanya menambah garis balik yang membingungkan.
			if !hasParent[key] {
				out.Edges = append(out.Edges, PathEdge{From: "step:" + key, To: an, Kind: "targets"})
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		// temuan, diurutkan dari yang paling parah
		fr, err := tx.Query(ctx, `
SELECT f.id, f.asset_id, f.type, f.title, f.severity FROM findings f
WHERE f.tenant_id=$1 AND EXISTS (SELECT 1 FROM finding_occurrences o WHERE o.tenant_id=f.tenant_id AND o.finding_id=f.id AND o.simulation_id=$2)
ORDER BY array_position(ARRAY['info','low','medium','high','critical'], f.severity) DESC, f.title LIMIT $3`, p.TenantID, id, maxPathFindings+1)
		if err != nil {
			return err
		}
		inPath := map[uuid.UUID]bool{}
		n := 0
		for fr.Next() {
			var fid uuid.UUID
			var aid *uuid.UUID
			var typ, title, sev string
			if err := fr.Scan(&fid, &aid, &typ, &title, &sev); err != nil {
				fr.Close()
				return err
			}
			if n++; n > maxPathFindings {
				out.Truncated = true
				break
			}
			inPath[fid] = true
			out.Nodes = append(out.Nodes, PathNode{ID: "finding:" + fid.String(), Kind: "finding", Label: title, Sub: typ, Severity: sev, Column: 2})
			if aid != nil && seenAsset["asset:"+aid.String()] {
				out.Edges = append(out.Edges, PathEdge{From: "asset:" + aid.String(), To: "finding:" + fid.String(), Kind: "found"})
			}
		}
		fr.Close()
		if err := fr.Err(); err != nil {
			return err
		}
		// temuan -> langkah yang dipicunya
		tr, err := tx.Query(ctx, `
SELECT DISTINCT ft.finding_id, s.key FROM finding_triggers ft
JOIN executions e ON e.tenant_id=ft.tenant_id AND e.id=ft.execution_id
JOIN simulation_steps s ON s.tenant_id=e.tenant_id AND s.id=e.simulation_step_id
WHERE ft.tenant_id=$1 AND e.simulation_id=$2`, p.TenantID, id)
		if err != nil {
			return err
		}
		triggered := map[string]bool{}
		for tr.Next() {
			var fid uuid.UUID
			var key string
			if err := tr.Scan(&fid, &key); err != nil {
				tr.Close()
				return err
			}
			if inPath[fid] {
				triggered[key] = true
				out.Edges = append(out.Edges, PathEdge{From: "finding:" + fid.String(), To: "step:" + key, Kind: "triggers"})
			}
		}
		tr.Close()
		if err := tr.Err(); err != nil {
			return err
		}
		// Ketergantungan langsung antar aksi. Aksi yang dipicu temuan sudah terhubung lewat temuan itu, jadi tidak digandakan.
		for _, e := range edges {
			if mode[e.Child] == "on_finding" && triggered[e.Child] {
				continue
			}
			out.Edges = append(out.Edges, PathEdge{From: "step:" + e.Parent, To: "step:" + e.Child, Kind: "then"})
		}
		return nil
	})
	sort.SliceStable(out.Nodes, func(i, j int) bool { return out.Nodes[i].Column < out.Nodes[j].Column })
	return out, err
}

var _ = json.Marshal
