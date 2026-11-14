// Package scans menyajikan alur organisasi -> target -> tool, memakai engine injector yang sama.
package scans

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/netip"
	"net/url"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/contract"
	"orchis-go/internal/db"
	"orchis-go/internal/defensebot"
	"orchis-go/internal/hackbot"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/reporting"
	"orchis-go/internal/scancontrol"
	"orchis-go/internal/scanscope"
	"orchis-go/internal/simulation"
	"sort"
	"time"
)

type Service struct {
	BeforeLaunch func(context.Context, pgx.Tx, *auth.Principal, Input, string) error
	Now          func() time.Time
	DB           *db.DB
	Guard        contract.Guard
	Engine       *simulation.Engine
	Reports      *reporting.Service
	MaxHosts     int
	Concurrency  int
	Collector    DefenseCollector
}

func (s *Service) hostLimit() int {
	if s.MaxHosts == 0 {
		return contract.DefaultScanMaxHosts
	}
	return s.MaxHosts
}
func (s *Service) slots() int {
	if s.Concurrency == 0 {
		return 4
	}
	return s.Concurrency
}
func require(p *auth.Principal, perms ...string) error {
	for _, perm := range perms {
		if !authz.Has(p.Permissions, perm) {
			return httpx.Forbidden(perm)
		}
	}
	return nil
}
func invalid(field, msg string) error {
	return httpx.Validation(httpx.FieldError{Name: field, Message: msg})
}

type Input struct {
	Language       string      `json:"language,omitempty"`
	OrganizationID uuid.UUID   `json:"organization_id"`
	AssetIDs       []uuid.UUID `json:"asset_ids"`
	RangeIDs       []uuid.UUID `json:"range_ids"`
	Tools          []string    `json:"tools"`
	Preset         string      `json:"preset"`
}
type Scan struct {
	CompletionStatus  string                `json:"completion_status,omitempty"`
	LimitedCoverage   bool                  `json:"limited_coverage"`
	ReportID          *uuid.UUID            `json:"report_id,omitempty"`
	ReportStatus      string                `json:"report_status,omitempty"`
	Summary           string                `json:"summary,omitempty"`
	ResultsExpiredAt  *time.Time            `json:"results_expired_at,omitempty"`
	ComparisonDetails *reporting.Comparison `json:"comparison_details,omitempty"`
	ID                uuid.UUID             `json:"id"`
	OrganizationID    uuid.UUID             `json:"organization_id"`
	Name              string                `json:"name"`
	Status            string                `json:"status"`
	Runner            string                `json:"runner"`
	Options           Input                 `json:"options"`
	CreatedAt         time.Time             `json:"created_at"`
	EndedAt           *time.Time            `json:"ended_at"`
	Jobs              []Job                 `json:"jobs,omitempty"`
}
type Job struct {
	CompletionStatus string              `json:"completion_status,omitempty"`
	Limitations      []string            `json:"limitations,omitempty"`
	ID               uuid.UUID           `json:"id"`
	AssetID          *uuid.UUID          `json:"asset_id"`
	RangeID          *uuid.UUID          `json:"range_id"`
	Name             string              `json:"name"`
	Target           string              `json:"target"`
	Tool             string              `json:"tool"`
	Status           string              `json:"status"`
	Error            string              `json:"error"`
	Summary          string              `json:"summary,omitempty"`
	Disposition      string              `json:"disposition,omitempty"`
	Defense          *defensebot.Summary `json:"defense,omitempty"`
	Phase            string              `json:"phase,omitempty"`
	SimulationID     *uuid.UUID          `json:"simulation_id"`
	Findings         []reporting.Finding `json:"findings"`
}

const scanCols = `a.id,a.organization_id,a.name,a.status,a.runner,a.scan_options,a.created_at,a.ended_at,a.results_expired_at,
 EXISTS(SELECT 1 FROM scan_jobs limited WHERE limited.tenant_id=a.tenant_id AND limited.assessment_id=a.id AND limited.status IN ('incomplete','failed','skipped'))` + `,` + scanCompletionSQL

func scanRow(row pgx.Row) (Scan, error) {
	var out Scan
	err := row.Scan(&out.ID, &out.OrganizationID, &out.Name, &out.Status, &out.Runner, &out.Options, &out.CreatedAt, &out.EndedAt, &out.ResultsExpiredAt, &out.LimitedCoverage, &out.CompletionStatus)
	return out, err
}

func (s *Service) Launch(ctx context.Context, p *auth.Principal, in Input, key, ip, req string) (*Scan, error) {
	if in.Language != "" && in.Language != "en" && in.Language != "id" {
		return nil, httpx.Validation(httpx.FieldError{Name: "language", Message: "Choose English or Indonesia."})
	}
	language := in.Language
	if language == "" {
		language = p.UILanguage
	}
	if language == "" {
		language = "en"
	}

	if err := require(p, authz.OrganizationRead, authz.AssetRead, authz.AssessmentRun, authz.SimulationRun, authz.FindingRead); err != nil {
		return nil, err
	}
	if len(in.RangeIDs) > 0 {
		if err := require(p, authz.AgentRead); err != nil {
			return nil, err
		}
	}
	if len(in.Tools) == 1 && in.Tools[0] == "defensebot" {
		return s.launchDefense(ctx, p, in, language, key, ip, req)
	}
	if in.OrganizationID == uuid.Nil {
		return nil, invalid("organization_id", "Choose an organisation.")
	}
	if len(key) < 1 || len(key) > 200 {
		return nil, invalid("idempotency_key", "Provide an Idempotency-Key of 1 to 200 characters.")
	}
	if len(in.RangeIDs) > 0 {
		return nil, invalid("range_ids", "Choose authorized web assets with HTTP(S) URLs; network range scans are no longer supported.")
	}
	full := len(in.Tools) == 4 && containsFullTools(in.Tools)
	if len(in.Tools) > 0 && !full && !(len(in.Tools) == 3 && containsPipelineTools(in.Tools)) {
		return nil, invalid("tools", "Web pentests run the orchis-redteam-agent pipeline; a full assessment adds the orchis-defense-agent engine.")
	}
	in.Tools = []string{"nmap", "nuclei", "hackbot"}
	if full {
		in.Tools = append(in.Tools, "defensebot")
	}
	in.Preset = "standard"
	if len(in.AssetIDs)+len(in.RangeIDs) == 0 || len(in.AssetIDs) > s.hostLimit() || len(in.RangeIDs) > 32 {
		return nil, invalid("targets", "Select authorized assets or at most 32 approved ranges.")
	}
	for _, list := range [][]uuid.UUID{in.AssetIDs, in.RangeIDs} {
		sort.Slice(list, func(i, j int) bool { return list[i].String() < list[j].String() })
		for i, id := range list {
			if id == uuid.Nil || i > 0 && list[i-1] == id {
				return nil, invalid("targets", "Select each target once.")
			}
		}
	}
	body, _ := json.Marshal(in)
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	id := ids.New()
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := scancontrol.Lock(ctx, tx, p.TenantID); err != nil {
			return err
		}
		var name string
		var demo bool
		err := tx.QueryRow(ctx, `SELECT name,is_demo FROM organizations WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, in.OrganizationID).Scan(&name, &demo)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Organisation")
		}
		if err != nil {
			return err
		}
		if demo {
			return httpx.Conflict("Synthetic organisations cannot run scans.")
		}
		var oldHash string
		err = tx.QueryRow(ctx, `SELECT id,request_hash FROM assessments WHERE tenant_id=$1 AND organization_id=$2 AND idempotency_key=$3`, p.TenantID, in.OrganizationID, key).Scan(&id, &oldHash)
		if err == nil {
			if oldHash != hash {
				return httpx.Conflict("This idempotency key belongs to different scan options.")
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if s.BeforeLaunch != nil {
			if err := s.BeforeLaunch(ctx, tx, p, in, key); err != nil {
				return err
			}
		}
		if err := preventOverlap(ctx, tx, p.TenantID, in.AssetIDs); err != nil {
			return err
		}
		if err := scancontrol.Check(ctx, tx, p.TenantID, &in.OrganizationID, scancontrol.Now(s.Now)); err != nil {
			return err
		}
		type target struct {
			control      uuid.UUID
			asset        *uuid.UUID
			rangeID      *uuid.UUID
			rangeVersion *int
			name         string
			addrs        []contract.Address
			cidr         string
			public       bool
			agent        bool // agent terdaftar: sisi dalam (orchis-defense-agent) tersedia
		}
		targets := []target{}
		for _, asset := range in.AssetIDs {
			t := target{control: asset, asset: &asset}
			var authorized bool
			err = tx.QueryRow(ctx, `SELECT name,testing_authorized,is_internet_facing FROM assets WHERE tenant_id=$1 AND id=$2 AND organization_id=$3 AND deleted_at IS NULL AND NOT is_demo FOR SHARE`, p.TenantID, asset, in.OrganizationID).Scan(&t.name, &authorized, &t.public)
			if errors.Is(err, pgx.ErrNoRows) {
				return invalid("asset_ids", "An asset is unavailable or belongs to another organisation.")
			}
			if err != nil {
				return err
			}
			if !authorized {
				return invalid("asset_ids", "Every selected asset requires testing authorization.")
			}
			// Hackbot berat (satu engagement per worker, lama): tolak run kedua yang tumpang tindih di aset yang sama.
			var busy bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scan_jobs j JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id WHERE j.tenant_id=$1 AND j.asset_id=$2 AND j.tool='hackbot' AND a.status='running' AND j.status IN ('waiting','running'))`, p.TenantID, asset).Scan(&busy); err != nil {
				return err
			}
			if busy {
				return httpx.Conflict("orchis-redteam-agent is already testing " + t.name + "; wait for that run to finish before starting another.")
			}
			// Full assessment juga menjalankan orchis-defense-agent: aset butuh agent terdaftar, bukan hanya URL web.
			// Penilaian penuh boleh campuran: aset ber-agent menjalankan orchis-defense-agent, aset ber-URL menjalankan Hackbot,
			// aset dengan keduanya menjalankan keduanya. Aset tanpa satu pun ditolak di bawah.
			if full {
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE tenant_id=$1 AND asset_id=$2 AND NOT is_demo AND status<>'revoked')`, p.TenantID, asset).Scan(&t.agent); err != nil {
					return err
				}
			}
			rows, e := tx.Query(ctx, `SELECT kind,value,is_primary FROM asset_addresses WHERE tenant_id=$1 AND asset_id=$2 AND scan_approved ORDER BY is_primary DESC,created_at,id`, p.TenantID, asset)
			if e != nil {
				return e
			}
			for rows.Next() {
				var a contract.Address
				if e = rows.Scan(&a.Kind, &a.Value, &a.IsPrimary); e != nil {
					rows.Close()
					return e
				}
				t.addrs = append(t.addrs, a)
			}
			rows.Close()
			if e = rows.Err(); e != nil {
				return e
			}
			targets = append(targets, t)
		}
		var selectedAgent *uuid.UUID
		for _, rangeID := range in.RangeIDs {
			var control, agent uuid.UUID
			var cidr string
			var version int
			var consent int64
			err = tx.QueryRow(ctx, `SELECT control_asset_id,agent_id,cidr,version,consent_revision FROM network_range_authorizations WHERE tenant_id=$1 AND organization_id=$2 AND id=$3 AND authorized FOR SHARE`, p.TenantID, in.OrganizationID, rangeID).Scan(&control, &agent, &cidr, &version, &consent)
			if errors.Is(err, pgx.ErrNoRows) {
				return invalid("range_ids", "A range is unavailable or no longer approved.")
			}
			if err != nil {
				return err
			}
			if selectedAgent != nil && *selectedAgent != agent {
				return invalid("range_ids", "Select ranges from one agent per scan.")
			}
			selectedAgent = &agent
			source, e := scanscope.SourceForAgent(ctx, tx, p.TenantID, in.OrganizationID, agent)
			if e != nil {
				return invalid("range_ids", e.Error())
			}
			if source.Asset != control || source.Revision != consent || !scanscope.ContainsReported(cidr, source.Rows, s.Guard) {
				return invalid("range_ids", "The network or consent changed; request fresh approval.")
			}
			prefix, e := contract.BoundedNetwork(cidr, s.hostLimit(), s.Guard)
			if e != nil {
				return invalid("range_ids", e.Error())
			}
			for addr := prefix.Addr(); prefix.Contains(addr); addr = addr.Next() {
				a := addr.String()
				targets = append(targets, target{control: control, rangeID: &rangeID, rangeVersion: &version, name: a, addrs: []contract.Address{{Kind: "ip", Value: a, IsPrimary: true}}, cidr: cidr, public: source.InternetFacing})
				if len(targets) > s.hostLimit() {
					return invalid("targets", fmt.Sprintf("A scan is limited to %d addresses.", s.hostLimit()))
				}
				if !addr.Next().IsValid() {
					break
				}
			}
		}
		if len(targets) > s.hostLimit() {
			return invalid("targets", "The total selected scope exceeds the address limit.")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO assessments(id,tenant_id,organization_id,name,profile,idempotency_key,request_hash,created_by,is_scan,runner,scan_options,report_language) VALUES($1,$2,$3,$4,$5,$6,$7,$8,true,'injector',$9,$10)`, id, p.TenantID, in.OrganizationID, name, in.Preset, key, hash, p.UserID, body, language); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, t := range targets {
			webTarget, e := s.Guard.Target(contract.KeyHackbot, t.addrs)
			hasWeb := e == nil
			if !hasWeb && !(full && t.agent) {
				if full {
					return invalid("asset_ids", t.name+" needs an approved web URL for orchis-redteam-agent or an enrolled agent for orchis-defense-agent.")
				}
				return invalid("asset_ids", "Every web pentest requires an approved HTTP(S) URL.")
			}
			var nmapTarget string
			var nmapParams []byte
			if hasWeb {
				nmapTarget, nmapParams, e = webDiscovery(webTarget)
				if e != nil {
					return invalid("asset_ids", "The approved web URL has an invalid service port.")
				}
			}
			for _, tool := range in.Tools {
				if tool == "defensebot" || !hasWeb {
					continue // job orchis-defense-agent dibuat di blok full assessment; aset tanpa URL melewati sisi web.
				}
				ckey := map[string]string{"hackbot": contract.KeyHackbot, "nmap": contract.KeyNmapTCP, "nuclei": contract.KeyNucleiVS}[tool]
				var cid uuid.UUID
				var version int
				err = tx.QueryRow(ctx, `SELECT c.id,c.version FROM contracts c JOIN integrations i ON i.tenant_id=c.tenant_id AND i.id=c.integration_id WHERE c.tenant_id=$1 AND c.key=$2 AND i.enabled`, p.TenantID, ckey).Scan(&cid, &version)
				if errors.Is(err, pgx.ErrNoRows) {
					return invalid("tools", "The selected tool integration is disabled.")
				}
				if err != nil {
					return err
				}
				params, _ := json.Marshal(map[string]string{"language": language})
				if tool == "nmap" {
					params = nmapParams
				}
				if tool == "nuclei" {
					params = json.RawMessage(`{"severity":["info","low","medium","high","critical"]}`)
				}
				params, e := contract.NormalizeVersion(ckey, version, params)
				if e != nil {
					return e
				}
				target := webTarget
				if tool == "nmap" {
					target = nmapTarget
				}
				reason := ""
				scope := contract.ScanScope{Target: target, CIDR: t.cidr, TestingAuthorized: true, InternetFacing: t.public, MaxHosts: s.hostLimit()}
				if reason == "" {
					if e = scope.Validate(target, s.Guard); e != nil {
						reason = e.Error()
					}
				}
				dedupe := "network|" + tool + "|" + target
				if t.asset != nil {
					dedupe = t.asset.String() + "|" + tool + "|" + target
				}
				if reason == "" && seen[dedupe] {
					continue
				}
				seen[dedupe] = true
				state := "waiting"
				if reason != "" {
					state = "failed"
				}
				addresses, _ := json.Marshal(t.addrs)
				scopeJSON, _ := json.Marshal(scope)
				if _, e = tx.Exec(ctx, `INSERT INTO scan_jobs(id,tenant_id,assessment_id,control_asset_id,asset_id,range_id,range_version,name,target,addresses,tool,contract_id,contract_version,params,scope,status,error) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, ids.New(), p.TenantID, id, t.control, t.asset, t.rangeID, t.rangeVersion, t.name, target, addresses, tool, cid, version, []byte(params), scopeJSON, state, reason); e != nil {
					return e
				}
			}
			if full && t.agent {
				// Engine orchis-defense-agent untuk sisi dalam (aplikasi terpasang): kumpulan baru diantrikan di transaksi yang
				// sama sehingga agent offline atau persetujuan yang belum ada menggagalkan launch dengan alasan jelas.
				// Worker menahan job ini sampai Nmap aset yang sama selesai agar tag "seen by scanner" terisi.
				if s.Collector == nil {
					return httpx.Conflict("orchis-defense-agent collection is not available.")
				}
				if _, err = s.Collector.QueueDefenseCollection(ctx, tx, p.TenantID, t.control, defensebot.Queries); err != nil {
					return err
				}
				var cid uuid.UUID
				var version int
				err = tx.QueryRow(ctx, `SELECT c.id,c.version FROM contracts c JOIN integrations i ON i.tenant_id=c.tenant_id AND i.id=c.integration_id WHERE c.tenant_id=$1 AND c.key=$2 AND c.is_active AND i.enabled ORDER BY c.version DESC LIMIT 1`, p.TenantID, contract.KeyDefenseBot).Scan(&cid, &version)
				if errors.Is(err, pgx.ErrNoRows) {
					return invalid("tools", "The orchis-defense-agent integration is disabled.")
				}
				if err != nil {
					return err
				}
				dparams, _ := json.Marshal(map[string]string{"language": language})
				dparams, err = contract.NormalizeVersion(contract.KeyDefenseBot, version, dparams)
				if err != nil {
					return err
				}
				dscope, _ := json.Marshal(contract.ScanScope{Target: "local", TestingAuthorized: true, MaxHosts: s.hostLimit()})
				if _, err = tx.Exec(ctx, `INSERT INTO scan_jobs(id,tenant_id,assessment_id,control_asset_id,asset_id,range_id,range_version,name,target,addresses,tool,contract_id,contract_version,params,scope,status,error) VALUES($1,$2,$3,$4,$4,NULL,NULL,$5,'local','[]','defensebot',$6,$7,$8,$9,'waiting','')`, ids.New(), p.TenantID, id, t.control, t.name, cid, version, []byte(dparams), dscope); err != nil {
					return err
				}
			}
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "scan.launch", ResourceType: "scan", ResourceID: &id, IP: ip, RequestID: req, Detail: map[string]any{"organization_id": in.OrganizationID, "asset_ids": in.AssetIDs, "range_ids": in.RangeIDs, "tools": in.Tools, "preset": in.Preset, "runner": "injector"}})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

func (s *Service) Get(ctx context.Context, p *auth.Principal, id uuid.UUID) (*Scan, error) {
	if err := require(p, authz.OrganizationRead, authz.AssessmentRead, authz.SimulationRead, authz.AssetRead, authz.FindingRead); err != nil {
		return nil, err
	}
	var out Scan
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		var e error
		out, e = scanRow(tx.QueryRow(ctx, `SELECT `+scanCols+` FROM assessments a WHERE a.tenant_id=$1 AND a.id=$2 AND a.is_scan`, p.TenantID, id))
		if errors.Is(e, pgx.ErrNoRows) {
			return httpx.NotFound("Scan")
		}
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT id,asset_id,range_id,name,target,tool,status,error,simulation_id FROM scan_jobs WHERE tenant_id=$1 AND assessment_id=$2 ORDER BY name,target,tool,id`, p.TenantID, id)
		if e != nil {
			return e
		}
		out.Jobs = []Job{}
		for rows.Next() {
			var j Job
			j.Findings = []reporting.Finding{}
			if e = rows.Scan(&j.ID, &j.AssetID, &j.RangeID, &j.Name, &j.Target, &j.Tool, &j.Status, &j.Error, &j.SimulationID); e != nil {
				rows.Close()
				return e
			}
			out.Jobs = append(out.Jobs, j)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		out.Status = effectiveStatus(out.Status, out.Jobs)
		for i := range out.Jobs {
			j := &out.Jobs[i]
			if j.SimulationID == nil {
				continue
			}
			var reviewCompleted bool
			e = tx.QueryRow(ctx, `SELECT COALESCE(hackbot_result->>'summary',''),COALESCE(hackbot_result->>'disposition',''),COALESCE(hackbot_progress->>'phase',''),COALESCE(hackbot_result->'blockers','[]'::jsonb),COALESCE(state='ended' AND hackbot_result->>'phase'='report' AND COALESCE(hackbot_result->>'report','')<>'' AND COALESCE(hackbot_result->>'validation','')<>'' AND outcome NOT IN ('canceled','timeout'),false) FROM executions WHERE tenant_id=$1 AND simulation_id=$2 ORDER BY id LIMIT 1`, p.TenantID, *j.SimulationID).Scan(&j.Summary, &j.Disposition, &j.Phase, &j.Limitations, &reviewCompleted)
			if e != nil && !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			if j.Tool == "defensebot" {
				var raw []byte
				if e = tx.QueryRow(ctx, `SELECT defense_result FROM executions WHERE tenant_id=$1 AND simulation_id=$2 ORDER BY id LIMIT 1`, p.TenantID, *j.SimulationID).Scan(&raw); e != nil && !errors.Is(e, pgx.ErrNoRows) {
					return e
				}
				if len(raw) > 0 {
					var sum defensebot.Summary
					if json.Unmarshal(raw, &sum) == nil {
						j.Defense = &sum
					}
				}
			}
			if j.Tool == "hackbot" && j.Summary == "" && j.Status != "running" && j.Status != "waiting" {
				j.Summary = "orchis-redteam-agent did not complete this pentest. Coverage: " + j.Status + ". See the execution details for the blocker."
			}
			if j.Tool == "hackbot" && j.Status == "incomplete" && reviewCompleted && (j.Disposition == "limited" || j.Disposition == "input_required") {
				j.CompletionStatus = "finished"
			}
			for n := range j.Limitations {
				j.Limitations[n] = reporting.Redact(j.Limitations[n], 1000)
			}
			j.Summary = hackbot.LimitSummary(reporting.Redact(j.Summary, 32000))
			if j.Summary != "" {
				out.Summary = hackbot.LimitSummary(out.Summary + " " + j.Summary)
			}
			j.Findings, e = jobFindings(ctx, tx, p.TenantID, *j.SimulationID)
			if e != nil {
				return e
			}
		}
		if out.Status != "running" && authz.Has(p.Permissions, authz.ReportRead) {
			e = tx.QueryRow(ctx, `SELECT id,status FROM reports WHERE tenant_id=$1 AND assessment_id=$2`, p.TenantID, id).Scan(&out.ReportID, &out.ReportStatus)
			if e != nil && !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			var raw []byte
			e = tx.QueryRow(ctx, `SELECT snapshot->'comparison_details' FROM reports WHERE tenant_id=$1 AND assessment_id=$2`, p.TenantID, id).Scan(&raw)
			if e != nil && !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			if len(raw) > 0 {
				if e = json.Unmarshal(raw, &out.ComparisonDetails); e != nil {
					return e
				}
			}
		}
		return nil
	})
	return &out, err
}

// effectiveStatus: scan yang selesai tetapi tidak satu pun job-nya berhasil dilaporkan gagal, bukan selesai.
func effectiveStatus(status string, jobs []Job) string {
	if status != "finished" || len(jobs) == 0 {
		return status
	}
	for _, j := range jobs {
		if j.Status != "failed" {
			return status
		}
	}
	return "failed"
}

func jobFindings(ctx context.Context, tx pgx.Tx, tenant, sim uuid.UUID) ([]reporting.Finding, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT ON(f.id) f.id,COALESCE(o.finding_snapshot->>'type',f.type),COALESCE(o.finding_snapshot->>'value',f.value),COALESCE(o.finding_snapshot->>'title',f.title),COALESCE(o.finding_snapshot->>'severity',f.severity),o.details FROM finding_occurrences o JOIN findings f ON f.tenant_id=o.tenant_id AND f.id=o.finding_id WHERE o.tenant_id=$1 AND o.simulation_id=$2 ORDER BY f.id,o.seen_at DESC`, tenant, sim)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []reporting.Finding{}
	for rows.Next() {
		var f reporting.Finding
		var details map[string]any
		if err = rows.Scan(&f.ID, &f.Type, &f.Value, &f.Title, &f.Severity, &details); err != nil {
			return nil, err
		}
		f.Title = reporting.Redact(f.Title, 500)
		f.Value = reporting.Redact(f.Value, 1000)
		f.Evidence = reporting.SafeEvidence(details)
		out = append(out, f)
	}
	return out, rows.Err()
}
func (s *Service) List(ctx context.Context, p *auth.Principal, org *uuid.UUID, cursor string) ([]Scan, *string, error) {
	if err := require(p, authz.OrganizationRead, authz.AssessmentRead, authz.SimulationRead, authz.AssetRead); err != nil {
		return nil, nil, err
	}
	var before *uuid.UUID
	if cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(cursor)
		if e != nil {
			return nil, nil, httpx.BadRequest("Invalid cursor.")
		}
		id, e := uuid.Parse(string(b))
		if e != nil {
			return nil, nil, httpx.BadRequest("Invalid cursor.")
		}
		before = &id
	}
	items := []Scan{}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT `+scanCols+` FROM assessments a WHERE a.tenant_id=$1 AND a.is_scan AND ($2::uuid IS NULL OR a.organization_id=$2) AND ($3::uuid IS NULL OR a.id<$3) ORDER BY a.id DESC LIMIT 26`, p.TenantID, org, before)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			a, e := scanRow(rows)
			if e != nil {
				return e
			}
			items = append(items, a)
		}
		return rows.Err()
	})
	var next *string
	if len(items) > 25 {
		items = items[:25]
		v := base64.RawURLEncoding.EncodeToString([]byte(items[24].ID.String()))
		next = &v
	}
	return items, next, err
}

// AddDiscoveredAsset mengambil alamat dari job berhasil; input tidak dapat mengganti host atau mengotorisasi testing.
func (s *Service) AddDiscoveredAsset(ctx context.Context, p *auth.Principal, job uuid.UUID, ip, req string) (uuid.UUID, error) {
	if err := require(p, authz.OrganizationWrite, authz.AssetWrite, authz.AssetRead, authz.AssessmentRead, authz.FindingRead); err != nil {
		return uuid.Nil, err
	}
	id := ids.New()
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var org uuid.UUID
		var target, name string
		err := tx.QueryRow(ctx, `SELECT a.organization_id,j.target,j.name FROM scan_jobs j JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id WHERE j.tenant_id=$1 AND j.id=$2 AND j.range_id IS NOT NULL AND j.status='finished' FOR UPDATE OF a,j`, p.TenantID, job).Scan(&org, &target, &name)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Successful discovered host")
		}
		if err != nil {
			return err
		}
		host := contract.HostOf(target)
		if _, err = netip.ParseAddr(host); err != nil {
			return invalid("job_id", "The job does not contain a valid discovered address.")
		}
		var locked uuid.UUID
		if err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, org).Scan(&locked); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `SELECT a.id FROM assets a JOIN asset_addresses d ON d.tenant_id=a.tenant_id AND d.asset_id=a.id WHERE a.tenant_id=$1 AND a.organization_id=$2 AND a.deleted_at IS NULL AND d.kind='ip' AND d.value=$3 ORDER BY a.id LIMIT 1`, p.TenantID, org, host).Scan(&id)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO assets(id,tenant_id,organization_id,name,category,criticality,testing_authorized,is_internet_facing) VALUES($1,$2,$3,$4,'host','medium',false,false)`, id, p.TenantID, org, name); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO asset_addresses(id,tenant_id,asset_id,kind,value,is_primary) VALUES($1,$2,$3,'ip',$4,true)`, ids.New(), p.TenantID, id, host); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "scan.host.add_asset", ResourceType: "asset", ResourceID: &id, IP: ip, RequestID: req, Detail: map[string]any{"scan_job_id": job, "organization_id": org, "testing_authorized": false}})
	})
	return id, err
}

func containsFullTools(tools []string) bool {
	seen := map[string]bool{}
	for _, tool := range tools {
		seen[tool] = true
	}
	return len(seen) == 4 && seen["nmap"] && seen["nuclei"] && seen["hackbot"] && seen["defensebot"]
}
func containsPipelineTools(tools []string) bool {
	seen := map[string]bool{}
	for _, tool := range tools {
		seen[tool] = true
	}
	return len(seen) == 3 && seen["nmap"] && seen["nuclei"] && seen["hackbot"]
}

// webDiscovery limits service discovery to the approved web origin's port.
// Hosting providers may share their other ports with unrelated applications.
func webDiscovery(target string) (string, json.RawMessage, error) {
	u, err := url.Parse(target)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", nil, fmt.Errorf("invalid web origin")
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	raw, err := json.Marshal(map[string]any{"profile": "custom", "ports": port, "service_detection": true})
	if err != nil {
		return "", nil, err
	}
	normalized, err := contract.NormalizeVersion(contract.KeyNmapTCP, 1, raw)
	return u.Hostname(), normalized, err
}

// Called after idempotency recovery while holding the tenant scan-control lock.
func preventOverlap(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, assets []uuid.UUID) error {
	var busy bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scan_jobs j JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id WHERE j.tenant_id=$1 AND j.asset_id=ANY($2::uuid[]) AND a.status='running' AND j.status IN ('waiting','running'))`, tenant, assets).Scan(&busy); e != nil {
		return e
	}
	if busy {
		return httpx.Conflict("An audit is already testing a selected asset. Wait for it to finish before starting another.")
	}
	return nil
}
