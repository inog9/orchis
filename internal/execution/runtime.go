package execution

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"orchis-go/internal/scancontrol"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/alerts"
	"orchis-go/internal/audit"
	"orchis-go/internal/contract"
	"orchis-go/internal/db"
	"orchis-go/internal/defensebot"
	"orchis-go/internal/hackbot"
	"orchis-go/internal/httpx"
	"orchis-go/internal/osquery"
	"orchis-go/internal/scanscope"
)

const (
	leaseGrace     = 60 * time.Second // tambahan di atas timeout kontrak
	heartbeatLease = 60 * time.Second // perpanjangan lease tiap heartbeat eksekusi
	maxRawOutput   = 4 << 20
	maxLogLines    = 200
)

// Runtime adalah sisi server untuk injector: autentikasi token, claim, heartbeat, hasil, dan reaper.
type Runtime struct {
	Now          func() time.Time
	DB           *db.DB
	Tenant       uuid.UUID
	Guard        contract.Guard
	ScanMaxHosts int
	Store        ObjectStore
	// AfterEnd dipanggil DALAM transaksi yang sama setiap kali eksekusi milik simulasi berakhir, agar chain maju atomik
	// bersama hasilnya. Boleh nil (atomic test tidak memerlukannya).
	AfterEnd func(ctx context.Context, tx pgx.Tx, tenant, simulation uuid.UUID) error
}

func (r *Runtime) afterEnd(ctx context.Context, tx pgx.Tx, sim *uuid.UUID) error {
	if sim == nil || r.AfterEnd == nil {
		return nil
	}
	return r.AfterEnd(ctx, tx, r.Tenant, *sim)
}

// Injector adalah pemanggil terautentikasi di sisi pekerja: injector (hanya integrasi) atau agent (integrasi "agent" + AgentID).
// Untuk agent, setiap kueri juga membatasi pada eksekusi yang ditujukan kepada agent itu.
type Injector struct {
	ID      uuid.UUID
	Key     string
	AgentID *uuid.UUID
}

const InjectorTokenPrefix = "oij_"

// ValidInjectorToken validates provisioned credentials before a worker is started.
func ValidInjectorToken(token string) bool {
	if !strings.HasPrefix(token, InjectorTokenPrefix) {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, InjectorTokenPrefix))
	return err == nil && len(raw) == 32 && len(token) == 47
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// IssueToken membuat token baru untuk sebuah injector dan menyimpan hash-nya (token lama langsung tidak berlaku).
// Token hanya dikembalikan sekali.
func IssueToken(ctx context.Context, d *db.DB, tenant uuid.UUID, key string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	tok := InjectorTokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	err := d.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE integrations SET config = config || jsonb_build_object('token_sha256', $3::text), updated_at=now()
WHERE tenant_id=$1 AND key=$2 AND kind='injector' AND NOT is_demo`, tenant, key, hashToken(tok))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("injector %q tidak dikenal", key)
		}
		return nil
	})
	return tok, err
}

// Authenticate mencocokkan token bearer dengan hash yang tersimpan.
func (r *Runtime) Authenticate(ctx context.Context, token string) (*Injector, error) {
	if !strings.HasPrefix(token, InjectorTokenPrefix) || len(token) > 200 {
		return nil, httpx.Unauthorized()
	}
	var in Injector
	err := r.DB.InTenantRead(ctx, r.Tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id, key FROM integrations WHERE tenant_id=$1 AND kind='injector' AND NOT is_demo AND enabled AND config->>'token_sha256'=$2`,
			r.Tenant, hashToken(token)).Scan(&in.ID, &in.Key)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.Unauthorized()
	}
	return &in, err
}

// Beat mencatat bahwa injector hidup.
func (r *Runtime) Beat(ctx context.Context, in *Injector, version string) error {
	return r.DB.InTenant(ctx, r.Tenant, func(tx pgx.Tx) error {
		// Catat sambungan baru (pertama kali atau setelah lama senyap) agar halaman Integrations punya log yang berarti.
		var fresh bool
		if err := tx.QueryRow(ctx, `SELECT last_heartbeat_at IS NULL OR last_heartbeat_at < now() - interval '90 seconds' FROM integrations WHERE tenant_id=$1 AND id=$2`, r.Tenant, in.ID).Scan(&fresh); err != nil {
			return err
		}
		if fresh {
			if _, err := tx.Exec(ctx, `INSERT INTO integration_logs (tenant_id, integration_id, level, message) VALUES ($1,$2,'info',$3)`, r.Tenant, in.ID, "Injector connected"+map[bool]string{true: " (version " + version + ")", false: ""}[version != ""]+"."); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE integrations SET last_heartbeat_at=now(), status='running', software_version=NULLIF($3,'') WHERE tenant_id=$1 AND id=$2`,
			r.Tenant, in.ID, version)
		return err
	})
}

type ClaimSpec struct {
	ScannerInputs   []hackbot.ScannerInput `json:"scanner_inputs,omitempty"`
	Defense         *defensebot.Input      `json:"defense,omitempty"`
	ContractVersion int                    `json:"contract_version"`
	ScanScope       *contract.ScanScope    `json:"scan_scope,omitempty"`
	ExecutionID     uuid.UUID              `json:"execution_id"`
	ContractKey     string                 `json:"contract_key"`
	Target          string                 `json:"target"`
	Params          json.RawMessage        `json:"params"`
	TimeoutSeconds  int                    `json:"timeout_seconds"`
}

// Claim memindahkan eksekusi ready -> running dengan satu UPDATE bersyarat. Penjaga scope diperiksa ulang di sini:
// otorisasi aset bisa dicabut setelah eksekusi diantrekan.
func (r *Runtime) Claim(ctx context.Context, in *Injector, id uuid.UUID) (*ClaimSpec, error) {
	var spec *ClaimSpec
	var refusal error
	err := r.DB.InTenant(ctx, r.Tenant, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, r.Tenant); err != nil {
			return err
		}
		var (
			ckey, target, state string
			params              json.RawMessage
			asset               uuid.UUID
			timeout             int
			contractVersion     int
			integ               uuid.UUID
			notBefore           time.Time
			sim                 *uuid.UUID
			agent               *uuid.UUID
			frozen              *uuid.UUID
		)
		err := tx.QueryRow(ctx, `
SELECT c.key, e.target_address, e.state, e.params, e.target_asset_id, c.default_timeout_seconds, e.integration_id, e.not_before, e.simulation_id, e.agent_id,e.contract_version,e.scan_organization_id
FROM executions e JOIN contracts c ON c.tenant_id=e.tenant_id AND c.id=e.contract_id
WHERE e.tenant_id=$1 AND e.id=$2 AND NOT e.is_demo FOR UPDATE OF e`, r.Tenant, id).Scan(&ckey, &target, &state, &params, &asset, &timeout, &integ, &notBefore, &sim, &agent, &contractVersion, &frozen)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (integ != in.ID || (in.AgentID != nil && (agent == nil || *agent != *in.AgentID)))) {
			return httpx.NotFound("Execution")
		}
		if err != nil {
			return err
		}
		if in.AgentID != nil {
			var blocked bool
			if err := tx.QueryRow(ctx, `SELECT paused OR uninstall_status IN('queued','dispatched') OR status='revoked' FROM agents WHERE tenant_id=$1 AND id=$2 FOR SHARE`, r.Tenant, *in.AgentID).Scan(&blocked); err != nil {
				return err
			}
			if blocked {
				return httpx.Conflict("The agent is paused or awaiting uninstall.")
			}
		}
		if state != "ready" || notBefore.After(time.Now()) {
			return httpx.Conflict("Execution is not ready (state: " + state + ").")
		}
		if sim != nil {
			// Simulasi yang dijeda tidak boleh memulai eksekusi baru. Pesan dispatch dibuang injector dan diterbitkan ulang saat dilanjutkan.
			var simStatus string
			if err := tx.QueryRow(ctx, `SELECT status FROM simulations WHERE tenant_id=$1 AND id=$2`, r.Tenant, *sim).Scan(&simStatus); err != nil {
				return err
			}
			if simStatus != "running" {
				refusal = httpx.Conflict("The simulation is " + simStatus + ".")
				return nil
			}
		}
		var authorized bool
		err = tx.QueryRow(ctx, `SELECT testing_authorized FROM assets WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, r.Tenant, asset).Scan(&authorized)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		host := contract.HostOf(target)
		why := ""
		// Aksi agent berjalan di endpoint itu sendiri: tidak ada target jaringan yang perlu diperiksa penjaga.
		claimDef := contract.FindVersion(ckey, contractVersion)
		onAgent := claimDef != nil && claimDef.NeedsAgent
		// DefenseBot memakai data yang sudah tersimpan untuk aset: tidak ada host/URL, tetapi otorisasi aset dan kontrol scan tetap berlaku.
		local := onAgent || contract.NoNetworkTarget(claimDef)
		switch {
		case !authorized:
			why = "The target asset is no longer authorized for testing."
		case !local && r.Guard.CheckHost(host) != nil:
			why = "The target is refused by the scope guard: " + r.Guard.CheckHost(host).Error() + "."
		}
		var scanProof *contract.ScanScope
		if why == "" && sim != nil {
			limit := r.ScanMaxHosts
			if limit == 0 {
				limit = contract.DefaultScanMaxHosts
			}
			var e error
			scanProof, e = scanscope.CheckJob(ctx, tx, r.Tenant, *sim, target, r.Guard, limit)
			if e != nil {
				why = "Scan scope refused: " + e.Error()
			}
		}

		if why == "" && !local && (scanProof == nil || scanProof.CIDR == "") {
			approved, e := AgentTargetConfirmed(ctx, tx, r.Tenant, asset, target)
			if e != nil {
				return e
			}
			if !approved {
				why = "Agent-reported target requires administrator confirmation."
			}
		}
		refusedOutcome := "error"
		refusedCode := ""
		if !onAgent {
			if e := scancontrol.CheckAsset(ctx, tx, r.Tenant, asset, frozen, scancontrol.Now(r.Now)); e != nil {
				var b *scancontrol.Blocked
				if !errors.As(e, &b) {
					return e
				}
				why = e.Error()
				refusedOutcome = "canceled"
				refusedCode = b.Code
			}
		}
		if why != "" {
			if _, err := tx.Exec(ctx, `UPDATE executions SET state='ended', outcome=$4, error=$3,scan_stop_reason=$5, ended_at=now(), version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`,
				r.Tenant, id, why, refusedOutcome, refusedCode); err != nil {
				return err
			}
			if refusedCode != "" {
				if err := audit.Write(ctx, tx, audit.Entry{Tenant: r.Tenant, Action: "scan.policy_stop", ResourceType: "execution", ResourceID: &id, Detail: map[string]any{"reason": refusedCode}}); err != nil {
					return err
				}
			}
			if err := logLine(ctx, tx, r.Tenant, id, "error", why); err != nil {
				return err
			}
			if err := r.afterEnd(ctx, tx, sim); err != nil {
				return err
			}
			refusal = httpx.Conflict(why)
			return nil // commit agar penolakan tercatat
		}
		if onAgent && osquery.IsOsquery(ckey) {
			skipped, err := skipUnapproved(ctx, tx, r.Tenant, id, asset, ckey)
			if err != nil {
				return err
			}
			if skipped {
				if err := r.afterEnd(ctx, tx, sim); err != nil {
					return err
				}
				refusal = httpx.Conflict("Skipped: " + osquery.ReasonDisabled + ".")
				return nil
			}
		}
		lease := time.Duration(timeout)*time.Second + leaseGrace
		if ckey == contract.KeyHackbot {
			lease = heartbeatLease
		}
		if _, err := tx.Exec(ctx, `
UPDATE executions SET state='running', started_at=now(), attempt=attempt+1, lease_until=now()+make_interval(secs => $3),
  claimed_by=$4, version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`, r.Tenant, id, lease.Seconds(), in.Key); err != nil {
			return err
		}
		if err := logLine(ctx, tx, r.Tenant, id, "info", "Started by the "+in.Key+" injector."); err != nil {
			return err
		}
		var scannerInputs []hackbot.ScannerInput
		if ckey == contract.KeyHackbot && sim != nil && scanProof != nil {
			var e error
			scannerInputs, e = r.scannerInputs(ctx, tx, *sim)
			if e != nil {
				return e
			}
		}
		var defense *defensebot.Input
		if ckey == contract.KeyDefenseBot {
			var e error
			defense, e = r.defenseInput(ctx, tx, asset, params)
			if e != nil {
				return e
			}
		}
		spec = &ClaimSpec{ScannerInputs: scannerInputs, Defense: defense, ScanScope: scanProof, ContractVersion: contractVersion, ExecutionID: id, ContractKey: ckey, Target: target, Params: params, TimeoutSeconds: timeout}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if refusal != nil {
		return nil, refusal
	}
	return spec, nil
}

// Heartbeat memperpanjang lease dan memberi tahu injector bila eksekusi sudah dibatalkan.
func (r *Runtime) Heartbeat(ctx context.Context, in *Injector, id uuid.UUID) (cancel bool, err error) {
	err = r.DB.InTenant(ctx, r.Tenant, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, r.Tenant); err != nil {
			return err
		}
		if err := scancontrol.Enforce(ctx, tx, r.Tenant, scancontrol.Now(r.Now), r.AfterEnd); err != nil {
			return err
		}
		if in.Key == "hackbot" || in.Key == "nmap" || in.Key == "nuclei" || in.Key == "defensebot" {
			var sim *uuid.UUID
			var target string
			e := tx.QueryRow(ctx, `SELECT simulation_id,target_address FROM executions WHERE tenant_id=$1 AND id=$2 AND integration_id=$3 AND state='running'`, r.Tenant, id, in.ID).Scan(&sim, &target)
			if errors.Is(e, pgx.ErrNoRows) {
				cancel = true
				return nil
			}
			if e != nil {
				return e
			}
			if sim != nil {
				limit := r.ScanMaxHosts
				if limit == 0 {
					limit = contract.DefaultScanMaxHosts
				}
				if _, e = scanscope.CheckJob(ctx, tx, r.Tenant, *sim, target, r.Guard, limit); e != nil {
					if _, e = tx.Exec(ctx, `UPDATE executions SET state='ended',outcome='canceled',error='Pentest authorization changed',ended_at=now(),lease_until=NULL WHERE tenant_id=$1 AND id=$2`, r.Tenant, id); e != nil {
						return e
					}
					cancel = true
					return r.afterEnd(ctx, tx, sim)
				}
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE executions SET lease_until=now()+make_interval(secs => $4), updated_at=now()
WHERE tenant_id=$1 AND id=$2 AND integration_id=$3 AND state='running' AND ($5::uuid IS NULL OR agent_id=$5)`, r.Tenant, id, in.ID, heartbeatLease.Seconds(), in.AgentID)
		if err != nil {
			return err
		}
		cancel = tag.RowsAffected() == 0
		_, err = tx.Exec(ctx, `UPDATE integrations SET last_heartbeat_at=now(), status='running' WHERE tenant_id=$1 AND id=$2`, r.Tenant, in.ID)
		return err
	})
	return cancel, err
}

type Result struct {
	Hackbot  *hackbot.Result     `json:"hackbot,omitempty"`
	Defense  *defensebot.Summary `json:"defense,omitempty"`
	ExitCode *int                `json:"exit_code"`
	Error    string              `json:"error"`
	TimedOut bool                `json:"timed_out"`
	Output   string              `json:"output"`
	Logs     []LogLine           `json:"logs"`
	Findings []NewFinding        `json:"findings"`
	// Datasets: hasil query bernama osquery dari agent (tidak tepercaya; lihat processEndpointData).
	Datasets []osquery.Dataset `json:"datasets,omitempty"`
}

// Complete menutup eksekusi running dan menyimpan finding dalam satu transaksi. Hasil kedua untuk eksekusi yang sama
// ditolak (409), sehingga pengiriman ulang injector tidak menggandakan finding.
func (r *Runtime) Complete(ctx context.Context, in *Injector, id uuid.UUID, res Result) error {
	if len(res.Output) > maxRawOutput {
		res.Output = res.Output[:maxRawOutput]
	}
	outcome := "success"
	switch {
	case res.TimedOut:
		outcome = "timeout"
	case res.Error != "" || (res.ExitCode != nil && *res.ExitCode != 0):
		outcome = "error"
	}
	if len(res.Error) > 2000 {
		res.Error = res.Error[:2000]
	}
	var refusal error
	err := r.DB.InTenant(ctx, r.Tenant, func(tx pgx.Tx) error {
		if err := scancontrol.Enforce(ctx, tx, r.Tenant, scancontrol.Now(r.Now), r.AfterEnd); err != nil {
			return err
		}
		if err := scancontrol.Lock(ctx, tx, r.Tenant); err != nil {
			return err
		}
		var asset uuid.UUID
		var sim *uuid.UUID
		var ckey string
		var contractVersion int
		var policyJSON []byte
		var agentID *uuid.UUID
		err := tx.QueryRow(ctx, `
SELECT e.target_asset_id, e.simulation_id, c.key, e.contract_version, e.endpoint_policy, e.agent_id
FROM executions e JOIN contracts c ON c.tenant_id=e.tenant_id AND c.id=e.contract_id
WHERE e.tenant_id=$1 AND e.id=$2 AND e.integration_id=$3 AND e.state='running'
  AND ($4::uuid IS NULL OR e.agent_id=$4)
FOR UPDATE OF e`, r.Tenant, id, in.ID, in.AgentID).Scan(&asset, &sim, &ckey, &contractVersion, &policyJSON, &agentID)
		if errors.Is(err, pgx.ErrNoRows) {
			refusal = httpx.Conflict("Execution is not running (already finished, canceled or timed out).")
			return nil
		}
		if err != nil {
			return err
		}
		if sim != nil {
			var target string
			if err = tx.QueryRow(ctx, `SELECT target_address FROM executions WHERE tenant_id=$1 AND id=$2`, r.Tenant, id).Scan(&target); err != nil {
				return err
			}
			limit := r.ScanMaxHosts
			if limit == 0 {
				limit = contract.DefaultScanMaxHosts
			}
			if _, e := scanscope.CheckJob(ctx, tx, r.Tenant, *sim, target, r.Guard, limit); e != nil {
				outcome = "error"
				res = Result{Error: "Scan scope refused: " + e.Error()}
			}
		}
		endpointRefused := false
		if q := osquery.Find(ckey); q != nil {
			why, err := acceptEndpointResult(ctx, tx, r.Tenant, asset, q, policyJSON, agentID)
			if err != nil {
				return err
			}
			if why != "" {
				endpointRefused = true
				outcome = "error"
				res = Result{Error: why}
			}
			// Dataset bernama adalah satu-satunya sumber data endpoint; kanal bebas dibuang.
			res.Output, res.Findings, res.Logs = "", nil, nil
			res.Error = osquery.Redact(res.Error)
			if len(res.Error) > 2000 {
				res.Error = res.Error[:2000]
			}
		}
		if ckey == contract.KeyHackbot {
			res.Findings = nil
			if res.Hackbot == nil {
				if outcome == "success" {
					return httpx.BadRequest("Hackbot result is required.")
				}
			} else {
				if err := res.Hackbot.Validate(); err != nil {
					return httpx.BadRequest(err.Error())
				}
				if res.Hackbot.Disposition != "complete" {
					outcome = "error"
				}
				for _, f := range res.Hackbot.Findings {
					res.Findings = append(res.Findings, NewFinding{Type: "vulnerability", Value: f.Value, Title: f.Title, Severity: f.Severity, Details: f.Details})
				}
				payload, err := json.Marshal(res.Hackbot)
				if err != nil {
					return err
				}
				if _, err = tx.Exec(ctx, `UPDATE executions SET hackbot_result=$3 WHERE tenant_id=$1 AND id=$2`, r.Tenant, id, payload); err != nil {
					return err
				}
			}
		} else if res.Hackbot != nil {
			return httpx.BadRequest("Hackbot result is not valid for this contract.")
		}
		if ckey == contract.KeyDefenseBot {
			if res.Defense == nil {
				if outcome == "success" {
					return httpx.BadRequest("DefenseBot summary is required.")
				}
			} else {
				if res.Defense.Schema != 1 || (res.Defense.Disposition != "complete" && res.Defense.Disposition != "limited") {
					return httpx.BadRequest("Invalid DefenseBot summary.")
				}
				payload, err := json.Marshal(res.Defense)
				if err != nil || len(payload) > 256<<10 {
					return httpx.BadRequest("The DefenseBot summary is too large.")
				}
				if _, err = tx.Exec(ctx, `UPDATE executions SET defense_result=$3 WHERE tenant_id=$1 AND id=$2`, r.Tenant, id, payload); err != nil {
					return err
				}
			}
		} else if res.Defense != nil {
			return httpx.BadRequest("A DefenseBot summary is not valid for this contract.")
		}
		var rawKey *string
		if r.Store != nil && res.Output != "" {
			key := fmt.Sprintf("executions/%s/%s/output.txt", r.Tenant, id)
			if err := r.Store.Put(ctx, key, []byte(res.Output), "text/plain; charset=utf-8"); err != nil {
				slog.Error("keluaran mentah gagal disimpan ke penyimpanan objek", "execution", id, "err", err)
				res.Logs = append(res.Logs, LogLine{Level: "warn", Message: "Raw output could not be stored."})
			} else {
				rawKey = &key
			}
		}
		var errText *string
		if res.Error != "" {
			errText = &res.Error
		}
		if _, err := tx.Exec(ctx, `UPDATE executions SET state='ended', outcome=$3, ended_at=now(), exit_code=$4, error=$5, raw_output_key=$6,
lease_until=NULL, version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`, r.Tenant, id, outcome, res.ExitCode, errText, rawKey); err != nil {
			return err
		}
		for i, l := range res.Logs {
			if i >= maxLogLines {
				break
			}
			lvl := l.Level
			if lvl != "debug" && lvl != "info" && lvl != "warn" && lvl != "error" {
				lvl = "info"
			}
			if err := logLine(ctx, tx, r.Tenant, id, lvl, l.Message); err != nil {
				return err
			}
		}
		if !endpointRefused {
			if err := processEndpointData(ctx, tx, r.Tenant, id, asset, ckey, outcome, &res); err != nil {
				return err
			}
		}
		if outcome == "success" || (ckey == contract.KeyHackbot && res.Hackbot != nil && (res.Hackbot.Disposition == "limited" || res.Hackbot.Disposition == "input_required")) {
			def := contract.FindVersion(ckey, contractVersion)
			if def == nil {
				return fmt.Errorf("kontrak %q tidak dikenal", ckey)
			}
			n, rejected, err := ingest(ctx, tx, r.Tenant, id, asset, sim, def.OutputTypes, res.Findings)
			if err != nil {
				return err
			}
			for _, why := range rejected {
				if err := logLine(ctx, tx, r.Tenant, id, "warn", "Rejected: "+why); err != nil {
					return err
				}
			}
			if err := logLine(ctx, tx, r.Tenant, id, "info", fmt.Sprintf("Finished: %d finding(s) recorded.", n)); err != nil {
				return err
			}
			return r.afterEnd(ctx, tx, sim)
		}
		if err := logLine(ctx, tx, r.Tenant, id, "error", "Finished with outcome "+outcome+"."); err != nil {
			return err
		}
		if err := r.raiseFailed(ctx, tx, id, asset, ckey, outcome, res.Error); err != nil {
			return err
		}
		return r.afterEnd(ctx, tx, sim)
	})
	if err != nil {
		return err
	}
	return refusal
}

type pair struct {
	id  uuid.UUID
	key string
}

// raiseFailed membuat alert "test gagal" untuk eksekusi yang berakhir error/timeout (dedupe per eksekusi).
func (r *Runtime) raiseFailed(ctx context.Context, tx pgx.Tx, exec, asset uuid.UUID, ckey, outcome, errText string) error {
	var name string
	if err := tx.QueryRow(ctx, `SELECT name FROM assets WHERE tenant_id=$1 AND id=$2`, r.Tenant, asset).Scan(&name); err != nil {
		return err
	}
	title := "Scan failed on " + name
	if outcome == "timeout" {
		title = "Scan timed out on " + name
	}
	return alerts.Raise(ctx, tx, r.Tenant, alerts.New{Kind: "test_failed", Severity: "warning", Title: title, Body: ckey + ": " + errText,
		ResourceType: "execution", ResourceID: &exec, DedupeKey: "exec:" + exec.String()})
}

// Sweep menutup eksekusi running yang lease-nya habis: ulangi bila masih ada jatah percobaan, kalau tidak timeout.
// Idempoten, jadi aman dijalankan bersamaan oleh beberapa instance server.
func (r *Runtime) Sweep(ctx context.Context) (retried, timedOut int, err error) {
	err = r.DB.InTenant(ctx, r.Tenant, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, r.Tenant); err != nil {
			return err
		}
		if err := scancontrol.Enforce(ctx, tx, r.Tenant, scancontrol.Now(r.Now), r.AfterEnd); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
UPDATE executions e SET state='ready', lease_until=NULL, claimed_by=NULL, not_before=now(), version=e.version+1, updated_at=now()
FROM integrations i
WHERE e.tenant_id=$1 AND e.state='running' AND e.lease_until < now() AND e.attempt < e.max_attempts AND e.executor_kind='injector'
  AND i.tenant_id=e.tenant_id AND i.id=e.integration_id AND i.key<>'hackbot'
RETURNING e.id, i.key`, r.Tenant)
		if err != nil {
			return err
		}
		var again []pair
		for rows.Next() {
			var p pair
			if err := rows.Scan(&p.id, &p.key); err != nil {
				rows.Close()
				return err
			}
			again = append(again, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, p := range again {
			if err := logLine(ctx, tx, r.Tenant, p.id, "warn", "The injector stopped responding; trying again."); err != nil {
				return err
			}
			if err := Enqueue(ctx, tx, r.Tenant, TopicDispatch, "exec."+p.key, "execution", p.id,
				map[string]any{"execution_id": p.id, "tenant_id": r.Tenant, "integration": p.key}); err != nil {
				return err
			}
		}
		retried = len(again)
		// Pesan dispatch yang hilang (antrean dibersihkan, injector mati lama): terbitkan ulang eksekusi ready yang menggantung.
		// Aman: Claim bersifat atomik, jadi pesan ganda tidak menjalankan dua kali.
		stale, err := tx.Query(ctx, `
UPDATE executions e SET updated_at=now()
FROM integrations i
WHERE e.tenant_id=$1 AND e.state='ready' AND e.updated_at < now() - interval '5 minutes' AND e.executor_kind='injector'
  AND i.tenant_id=e.tenant_id AND i.id=e.integration_id RETURNING e.id, i.key`, r.Tenant)
		if err != nil {
			return err
		}
		var lost []pair
		for stale.Next() {
			var p pair
			if err := stale.Scan(&p.id, &p.key); err != nil {
				stale.Close()
				return err
			}
			lost = append(lost, p)
		}
		stale.Close()
		if err := stale.Err(); err != nil {
			return err
		}
		for _, p := range lost {
			if err := Enqueue(ctx, tx, r.Tenant, TopicDispatch, "exec."+p.key, "execution", p.id,
				map[string]any{"execution_id": p.id, "tenant_id": r.Tenant, "integration": p.key}); err != nil {
				return err
			}
		}
		trows, err := tx.Query(ctx, `
UPDATE executions SET state='ended', outcome='timeout', ended_at=now(), lease_until=NULL,
  error='No result arrived before the lease expired.', version=version+1, updated_at=now()
WHERE tenant_id=$1 AND state='running' AND lease_until < now() RETURNING id, simulation_id`, r.Tenant)
		if err != nil {
			return err
		}
		type deadExec struct {
			id  uuid.UUID
			sim *uuid.UUID
		}
		var dead []deadExec
		for trows.Next() {
			var d deadExec
			if err := trows.Scan(&d.id, &d.sim); err != nil {
				trows.Close()
				return err
			}
			dead = append(dead, d)
		}
		trows.Close()
		if err := trows.Err(); err != nil {
			return err
		}
		for _, d := range dead {
			if err := logLine(ctx, tx, r.Tenant, d.id, "error", "Timed out: no result arrived before the lease expired."); err != nil {
				return err
			}
			if err := r.afterEnd(ctx, tx, d.sim); err != nil {
				return err
			}
		}
		timedOut = len(dead)
		return nil
	})
	return
}

// RunSweeper menjalankan Sweep berkala sampai ctx selesai.
func (r *Runtime) RunSweeper(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if re, to, err := r.Sweep(ctx); err != nil {
				slog.Error("sweep eksekusi gagal", "err", err)
			} else if re+to > 0 {
				slog.Info("sweep eksekusi", "retried", re, "timed_out", to)
			}
		}
	}
}

// ClaimNext dipanggil agent (polling): mengambil eksekusi ready berikutnya yang ditujukan kepadanya. Mengembalikan nil bila tidak ada.
// Kandidat yang ditolak (simulasi dijeda, otorisasi dicabut, diambil bersamaan) dilewati; Claim tetap atomik per baris.
func (r *Runtime) ClaimNext(ctx context.Context, in *Injector) (*ClaimSpec, error) {
	if in.AgentID == nil {
		return nil, httpx.Unauthorized()
	}
	var cands []uuid.UUID
	err := r.DB.InTenantRead(ctx, r.Tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id FROM executions WHERE tenant_id=$1 AND agent_id=$2 AND state='ready' AND not_before <= now()
ORDER BY ready_at, id LIMIT 5`, r.Tenant, *in.AgentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			cands = append(cands, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	for _, id := range cands {
		spec, err := r.Claim(ctx, in, id)
		if err == nil {
			return spec, nil
		}
		var pr *httpx.Problem
		if errors.As(err, &pr) && (pr.Status == 409 || pr.Status == 404) {
			continue
		}
		return nil, err
	}
	return nil, nil
}

// ProvisionToken initializes a deploy-time credential without resurrecting a
// token that an administrator has subsequently rotated through the API.
func ProvisionToken(ctx context.Context, d *db.DB, tenant uuid.UUID, key, token string) error {
	if !ValidInjectorToken(token) {
		return fmt.Errorf("invalid provisioning token format")
	}
	return d.InTenant(ctx, tenant, func(tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `UPDATE integrations SET config=config || jsonb_build_object('token_sha256',$3::text)
   WHERE tenant_id=$1 AND key=$2 AND kind='injector'
   AND (config->>'token_sha256' IS NULL OR config->>'token_sha256'=$3)`, tenant, key, hashToken(token))
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return httpx.Conflict("Provisioning credential differs from the current token. Update private worker configuration before restarting.")
		}
		return nil
	})
}

// Progress accepts only bounded public metadata from the claiming Hackbot worker.
func (r *Runtime) Progress(ctx context.Context, in *Injector, id uuid.UUID, p hackbot.Progress) error {
	if in.Key != "hackbot" || !p.Valid() {
		return httpx.BadRequest("Invalid Hackbot progress.")
	}
	return r.DB.InTenant(ctx, r.Tenant, func(tx pgx.Tx) error {
		data, _ := json.Marshal(p)
		tag, err := tx.Exec(ctx, `UPDATE executions SET hackbot_progress=$4 WHERE tenant_id=$1 AND id=$2 AND integration_id=$3 AND state='running'`, r.Tenant, id, in.ID, data)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return httpx.Conflict("Execution is no longer running.")
		}
		return nil
	})
}

// RetainWorkspace lets a worker remove private evidence only after the owning
// execution has been deleted by Orchis retention. Network/API failures retain it.
func (r *Runtime) RetainWorkspace(ctx context.Context, in *Injector, id uuid.UUID) (bool, error) {
	if in.Key != "hackbot" {
		return true, httpx.Forbidden("hackbot")
	}
	retained := true
	err := r.DB.InTenantRead(ctx, r.Tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM executions WHERE tenant_id=$1 AND id=$2 AND integration_id=$3)`, r.Tenant, id, in.ID).Scan(&retained)
	})
	return retained, err
}
