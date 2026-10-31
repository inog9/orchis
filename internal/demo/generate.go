package demo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/contract"
)

// demoNamespace: namespace tetap untuk UUIDv5 turunan (tenant+seed+jalur). Nilai ini arbitrer
// tapi harus tetap sama selamanya agar seed yang sama selalu menghasilkan ID yang sama (idempotent).
var demoNamespace = uuid.MustParse("5a3f9c10-0dd0-4a0a-8e7c-0f0d1e2a3b4c")

var industries = []string{"Manufacturing", "Healthcare", "Finance", "Retail", "Logistics", "Education", "Energy", "Technology"}
var orgWords = []string{"Nimbus", "Harborlight", "Ferrotech", "Cobalt Peak", "Driftwood", "Meridian", "Solace", "Vantage"}

const orgCount = 5

type assetBlueprint struct {
	category string
	platform string // kosong bila bukan host
	arch     string
	name     string
	addr     string // "ip" | "hostname" | ""
}

// 10 aset beragam per organisasi: server Linux/Windows/macOS, web app, database, jaringan, dll.
var assetBlueprints = []assetBlueprint{
	{category: "host", platform: "linux", arch: "x86_64", name: "linux-server", addr: "ip"},
	{category: "host", platform: "windows", arch: "x86_64", name: "windows-server", addr: "ip"},
	{category: "host", platform: "macos", arch: "arm64", name: "macos-workstation", addr: "ip"},
	{category: "web_application", name: "customer-portal", addr: "hostname"},
	{category: "network_device", name: "edge-firewall", addr: "ip"},
	{category: "cloud_resource", name: "primary-database", addr: "hostname"},
	{category: "container", name: "api-service-pod", addr: ""},
	{category: "identity", name: "directory-service-account", addr: ""},
	{category: "saas_application", name: "crm-saas", addr: "hostname"},
	{category: "generic", name: "misc-inventory-item", addr: ""},
}

var criticalities = []string{"low", "medium", "high", "critical"}
var findingTypes = []string{"open_port", "service", "vulnerability", "text"}
var severities = []string{"info", "low", "medium", "high", "critical"}
var agentStatuses = []string{"online", "offline", "revoked"}
var agentVersions = []string{"1.0.0", "1.4.2"}

type generator struct {
	tenant    uuid.UUID
	seed      int
	now       time.Time
	createdBy uuid.UUID
}

// id turunan deterministik dari tenant+seed+jalur: seed yang sama -> ID yang sama (idempotent).
func (g *generator) id(parts ...string) uuid.UUID {
	key := fmt.Sprintf("%s|%d|%s", g.tenant, g.seed, strings.Join(parts, "|"))
	return uuid.NewSHA1(demoNamespace, []byte(key))
}

func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func hash(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

func titleCase(s string) string {
	words := strings.Split(s, "_")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

type findingSummary struct {
	Evidence map[string]any `json:"evidence"`
	Value    string         `json:"value"`
	Title    string         `json:"title"`
	Type     string         `json:"type"`
	Severity string         `json:"severity"`
}

type assetInfo struct {
	id         uuid.UUID
	name       string
	skipReason string
	findings   []findingSummary
}

// run cria 5 organisasi x 10 aset beragam, agent dengan status campuran, findings beragam
// severity/tipe, dan 3 assessment+report historis per organisasi (tren) — semua ditandai is_demo.
func (g *generator) run(ctx context.Context, tx pgx.Tx) error {
	authorizer := g.id("authorizer") // akun semu (bukan baris users) sekadar nilai bukan-nol untuk authorized_by
	demoTagIDs := []uuid.UUID{g.id("tag", "sample-demo-asset"), g.id("tag", "demo-critical-path")}
	demoTagNames := []string{"demo-sample", "demo-critical-path"}
	for i, tid := range demoTagIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO tags (id, tenant_id, name, color, is_demo) VALUES ($1,$2,$3,'#B45309',true) ON CONFLICT (id) DO NOTHING`,
			tid, g.tenant, demoTagNames[i]); err != nil {
			return err
		}
	}

	for oi := 0; oi < orgCount; oi++ {
		idx := (g.seed + oi) % len(orgWords)
		industry := industries[(g.seed+oi*3)%len(industries)]
		orgID := g.id("org", fmt.Sprint(oi))
		orgName := fmt.Sprintf("%s %s (Demo)", orgWords[idx], industry)
		orgSlug := fmt.Sprintf("demo-%s-%d", slugify(orgWords[idx]), oi)
		if _, err := tx.Exec(ctx, `INSERT INTO organizations (id, tenant_id, name, slug, description, industry, contact, is_demo)
VALUES ($1,$2,$3,$4,$5,$6,$7,true) ON CONFLICT (id) DO NOTHING`,
			orgID, g.tenant, orgName, orgSlug,
			"Synthetic demo organization for review/testing. Not a real customer.", industry, "demo@example.invalid"); err != nil {
			return err
		}

		assets := make([]assetInfo, len(assetBlueprints))
		vulnSeverity := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}

		for ai, bp := range assetBlueprints {
			assetID := g.id("org", fmt.Sprint(oi), "asset", fmt.Sprint(ai))
			name := fmt.Sprintf("demo-%s-%02d", bp.name, oi+1)
			crit := criticalities[(oi+ai)%len(criticalities)]
			authorized := (oi+ai)%2 == 0
			internetFacing := (oi+ai)%3 == 0
			var authorizedBy *uuid.UUID
			var authorizedAt *time.Time
			if authorized {
				a := authorizer
				authorizedBy = &a
				at := g.now.AddDate(0, 0, -(10 + ai))
				authorizedAt = &at
			}
			if _, err := tx.Exec(ctx, `INSERT INTO assets (id, tenant_id, organization_id, category, name, description, platform, arch, criticality,
is_internet_facing, testing_authorized, authorized_by, authorized_at, source, is_demo)
VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9,$10,$11,$12,$13,'manual',true) ON CONFLICT (id) DO NOTHING`,
				assetID, g.tenant, orgID, bp.category, name, "Synthetic demo asset. Not a real target; do not scan over the network.",
				bp.platform, bp.arch, crit, internetFacing, authorized, authorizedBy, authorizedAt); err != nil {
				return err
			}
			skipReason := "Demo/synthetic organization: no real scan was executed."
			switch bp.addr {
			case "ip":
				addrID := g.id("org", fmt.Sprint(oi), "addr", fmt.Sprint(ai))
				// Rentang dokumentasi RFC 5737 (TEST-NET-3): dijamin tidak pernah dipindai sungguhan.
				value := fmt.Sprintf("203.0.113.%d", 10+(oi*10+ai)%240)
				if _, err := tx.Exec(ctx, `INSERT INTO asset_addresses (id, tenant_id, asset_id, kind, value, is_primary)
VALUES ($1,$2,$3,'ip',$4,true) ON CONFLICT (id) DO NOTHING`, addrID, g.tenant, assetID, value); err != nil {
					return err
				}
			case "hostname":
				addrID := g.id("org", fmt.Sprint(oi), "addr", fmt.Sprint(ai))
				value := fmt.Sprintf("demo-%s-%s.example.invalid", orgSlug, slugify(bp.name))
				if _, err := tx.Exec(ctx, `INSERT INTO asset_addresses (id, tenant_id, asset_id, kind, value, is_primary)
VALUES ($1,$2,$3,'hostname',$4,true) ON CONFLICT (id) DO NOTHING`, addrID, g.tenant, assetID, value); err != nil {
					return err
				}
			default:
				skipReason = "Demo/synthetic organization: asset has no configured address."
			}
			if (oi+ai)%4 == 0 {
				tagID := demoTagIDs[(oi+ai)%len(demoTagIDs)]
				if _, err := tx.Exec(ctx, `INSERT INTO asset_tags (tenant_id, asset_id, tag_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
					g.tenant, assetID, tagID); err != nil {
					return err
				}
			}

			var summaries []findingSummary
			nFindings := 1 + ai%3
			for fi := 0; fi < nFindings; fi++ {
				ftype := findingTypes[(ai+fi)%len(findingTypes)]
				sev := severities[(ai+fi*2)%len(severities)]
				value := fmt.Sprintf("demo-%s-%d-%d", ftype, ai, fi)
				title := fmt.Sprintf("%s (demo)", titleCase(ftype))
				occurrence := 1 + (ai+fi)%5
				firstSeen := g.now.AddDate(0, 0, -(30 + (ai+fi)*11))
				lastSeen := firstSeen.AddDate(0, 0, (ai+fi)%20)
				details := map[string]any{"is_demo": true, "note": "Synthetic/demo finding - not a real scan result."}
				switch ftype {
				case "vulnerability":
					details["cve"] = fmt.Sprintf("CVE-2023-%04d", 1000+ai*7+fi)
					details["mitre_technique"] = "T1190"
					vulnSeverity[sev]++
				case "open_port":
					details["port"] = 1000 + ai*3 + fi
					details["protocol"] = "tcp"
				case "service":
					details["service"] = "demo-service"
				}
				raw, err := json.Marshal(details)
				if err != nil {
					return err
				}
				findingID := g.id("org", fmt.Sprint(oi), "asset", fmt.Sprint(ai), "finding", fmt.Sprint(fi))
				if _, err := tx.Exec(ctx, `INSERT INTO findings (id, tenant_id, asset_id, type, value, value_hash, title, severity, details,
first_seen_at, last_seen_at, occurrence_count, is_demo)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,true) ON CONFLICT (id) DO NOTHING`,
					findingID, g.tenant, assetID, ftype, value, hash(value), title, sev, raw, firstSeen, lastSeen, occurrence); err != nil {
					return err
				}
				summaries = append(summaries, findingSummary{Evidence: map[string]any{"note": "Synthetic demo evidence"}, Value: value, Title: title, Type: ftype, Severity: sev})
			}
			assets[ai] = assetInfo{id: assetID, name: name, skipReason: skipReason, findings: summaries}
		}

		if len(assets) >= 3 {
			for i := 0; i < 3; i++ {
				bp := assetBlueprints[i]
				agentID := g.id("org", fmt.Sprint(oi), "agent", fmt.Sprint(i))
				status := agentStatuses[(oi+i)%len(agentStatuses)]
				version := agentVersions[(oi+i)%len(agentVersions)]
				var lastHB *time.Time
				var revokedAt *time.Time
				var revokedBy *uuid.UUID
				switch status {
				case "online":
					t := g.now.Add(-1 * time.Minute)
					lastHB = &t
				case "offline":
					t := g.now.Add(-2 * time.Hour)
					lastHB = &t
				case "revoked":
					t := g.now.AddDate(0, 0, -10)
					lastHB = &t
					rt := g.now.AddDate(0, 0, -9)
					revokedAt = &rt
					rb := g.createdBy
					revokedBy = &rb
				}
				secret := hash(fmt.Sprintf("demo-agent-secret|%s|%d|%d|%d", g.tenant, g.seed, oi, i))
				enrolledAt := g.now.AddDate(0, 0, -60)
				if _, err := tx.Exec(ctx, `INSERT INTO agents (id, tenant_id, asset_id, organization_id, instance_name, hostname, os, arch, mode, version,
protocol_version, status, secret_hash, last_heartbeat_at, enrolled_at, revoked_at, revoked_by, is_demo)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'service',$9,1,$10,$11,$12,$13,$14,$15,true) ON CONFLICT (id) DO NOTHING`,
					agentID, g.tenant, assets[i].id, orgID, fmt.Sprintf("demo-agent-%d-%d", oi, i), bp.name+"-host",
					bp.platform, bp.arch, version, status, secret, lastHB, enrolledAt, revokedAt, revokedBy); err != nil {
					return err
				}
			}
		}

		if err := g.seedAssessmentsAndReports(ctx, tx, oi, orgID, orgName, assets, vulnSeverity); err != nil {
			return err
		}
	}
	if err := g.seedScenarios(ctx, tx); err != nil {
		return err
	}
	return g.seedAlerts(ctx, tx)
}

type demoStep struct {
	key, title, contract, trigger string
	params                        string
}

var demoScenarios = []struct {
	name, desc, severity, category, status string
	steps                                  []demoStep
}{
	{"Demo: perimeter exposure", "Synthetic scenario: port discovery followed by a vulnerability scan. Demo only.", "high", "network", "ready",
		[]demoStep{{"discover", "Discover open ports", "nmap.tcp_connect", "after_parents", `{"profile":"top100"}`}, {"scan", "Scan for known exposures", "nuclei.scan", "after_parents", `{"severity":["high","critical"]}`}}},
	{"Demo: endpoint baseline", "Synthetic scenario: read-only endpoint checks via the agent. Demo only.", "low", "endpoint", "ready",
		[]demoStep{{"info", "Collect system information", "agent.system_info", "after_parents", `{}`}, {"ports", "List listening ports", "agent.listening_ports", "after_parents", `{}`}}},
	{"Demo: draft web check", "Synthetic draft scenario that is not ready to run. Demo only.", "medium", "web", "draft",
		[]demoStep{{"web", "Check web ports", "nmap.tcp_connect", "manual", `{"profile":"web"}`}}},
}

// seedScenarios membuat skenario demo yang memakai kontrak sungguhan (divalidasi lewat contract.Normalize);
// target-nya hanya aset demo. Skenario tidak dijalankan: tidak ada simulasi/eksekusi dibuat.
func (g *generator) seedScenarios(ctx context.Context, tx pgx.Tx) error {
	for si, sc := range demoScenarios {
		scID := g.id("scenario", fmt.Sprint(si))
		if _, err := tx.Exec(ctx, `INSERT INTO scenarios (id, tenant_id, name, description, severity, category, status, created_by, is_demo)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,true) ON CONFLICT (id) DO NOTHING`, scID, g.tenant, sc.name, sc.desc, sc.severity, sc.category, sc.status, g.createdBy); err != nil {
			return err
		}
		var prev uuid.UUID
		for i, st := range sc.steps {
			def := contract.Find(st.contract)
			if def == nil {
				return fmt.Errorf("demo: kontrak %s tidak dikenal", st.contract)
			}
			params, err := contract.Normalize(st.contract, json.RawMessage(st.params))
			if err != nil {
				return fmt.Errorf("demo: parameter %s: %w", st.contract, err)
			}
			stepID := g.id("scenario", fmt.Sprint(si), "step", st.key)
			tag, err := tx.Exec(ctx, `INSERT INTO scenario_steps (id, tenant_id, scenario_id, key, title, contract_id, contract_version, params, trigger_mode, position_x, position_y)
SELECT $1,$2,$3,$4,$5,c.id,c.version,$6,$7,$8,0 FROM contracts c WHERE c.tenant_id=$2 AND c.key=$9 AND c.version=$10
ON CONFLICT (id) DO NOTHING`, stepID, g.tenant, scID, st.key, st.title, params, st.trigger, i*240, st.contract, def.Version)
			if err != nil {
				return err
			}
			_ = tag
			if i > 0 && st.trigger == "after_parents" {
				if _, err := tx.Exec(ctx, `INSERT INTO scenario_step_edges (tenant_id, scenario_id, parent_step_id, child_step_id) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
					g.tenant, scID, prev, stepID); err != nil {
					return err
				}
			}
			prev = stepID
		}
		// Target: aset terotorisasi pertama dari tiap organisasi (hanya aset demo).
		targetID := g.id("scenario", fmt.Sprint(si), "target")
		assetID := g.id("org", fmt.Sprint(si%orgCount), "asset", "0")
		if _, err := tx.Exec(ctx, `INSERT INTO scenario_targets (id, tenant_id, scenario_id, asset_id) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`,
			targetID, g.tenant, scID, assetID); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) seedAlerts(ctx context.Context, tx pgx.Tx) error {
	items := []struct{ kind, severity, title, body string }{
		{"demo.notice", "info", "Demo data loaded", "Synthetic demo data is present in this workspace. It is not evidence of a real issue."},
		{"demo.report", "warning", "Demo report failed to render", "Synthetic failure used to exercise the report retry flow."},
		{"demo.coverage", "critical", "Demo: coverage gap", "Synthetic alert: several demo assets were skipped. No real scan was run."},
	}
	for i, it := range items {
		if _, err := tx.Exec(ctx, `INSERT INTO alerts (id, tenant_id, kind, severity, title, body, audience_permission, dedupe_key, is_demo)
VALUES ($1,$2,$3,$4,$5,$6,'alert.read',$7,true) ON CONFLICT DO NOTHING`, g.id("alert", fmt.Sprint(i)), g.tenant, it.kind, it.severity, it.title, it.body,
			fmt.Sprintf("demo-%d-%d", g.seed, i)); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) seedAssessmentsAndReports(ctx context.Context, tx pgx.Tx, oi int, orgID uuid.UUID, orgName string, assets []assetInfo, vulnSeverity map[string]int) error {
	runs := []struct {
		offsetDays int
		status     string
		factor     float64
	}{
		{90, "ready", 1.0},
		{30, "failed", 0.6},
		{0, "ready", 0.3},
	}
	for ri, run := range runs {
		assessID := g.id("org", fmt.Sprint(oi), "assessment", fmt.Sprint(ri))
		createdAt := g.now.AddDate(0, 0, -run.offsetDays)
		endedAt := createdAt.Add(25 * time.Minute)
		name := fmt.Sprintf("%s demo run %d", orgName, ri+1)
		idemKey := fmt.Sprintf("demo-seed-%d-org-%d-run-%d", g.seed, oi, ri)
		reqHash := hex.EncodeToString(hash(idemKey))
		if _, err := tx.Exec(ctx, `INSERT INTO assessments (id, tenant_id, organization_id, name, profile, idempotency_key, request_hash,
status, created_by, created_at, ended_at, is_demo)
VALUES ($1,$2,$3,$4,'quick',$5,$6,'finished',$7,$8,$9,true) ON CONFLICT (id) DO NOTHING`,
			assessID, g.tenant, orgID, name, idemKey, reqHash, g.createdBy, createdAt, endedAt); err != nil {
			return err
		}

		snapAssets := make([]map[string]any, 0, len(assets))
		for _, a := range assets {
			findings, _ := json.Marshal(a.findings)
			addrs := []byte(`[]`)
			if _, err := tx.Exec(ctx, `INSERT INTO assessment_targets (tenant_id, assessment_id, asset_id, asset_name, addresses, simulation_id, skip_reason)
VALUES ($1,$2,$3,$4,$5,NULL,$6) ON CONFLICT DO NOTHING`, g.tenant, assessID, a.id, a.name, addrs, a.skipReason); err != nil {
				return err
			}
			snapAssets = append(snapAssets, map[string]any{
				"id": a.id, "name": a.name, "status": "skipped", "skip_reason": a.skipReason,
				"findings": json.RawMessage(findings), "methods": []any{},
			})
		}
		bySeverity := map[string]int{}
		for sev, n := range vulnSeverity {
			bySeverity[sev] = int(math.Round(float64(n) * run.factor))
		}
		snapshot := map[string]any{
			"is_demo":       true,
			"label":         "DEMO / SYNTHETIC - not a real assessment. Do not use for real security decisions.",
			"organization":  orgName,
			"assessment_id": assessID,
			"profile":       "quick",
			"created_at":    createdAt,
			"status":        "finished",
			"risk":          "Synthetic demo data - risk not evaluated.",
			"by_severity":   bySeverity,
			"assets":        snapAssets,
			"remediation":   "This is synthetic demo data; no remediation advice applies.",
			"comparison":    "Comparison unavailable: synthetic demo data.",
		}
		raw, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		reportID := g.id("org", fmt.Sprint(oi), "report", fmt.Sprint(ri))
		status, reportErr, attempts := "ready", "", 1
		if run.status == "failed" {
			status, reportErr, attempts = "failed", "Renderer unreachable (synthetic demo failure for trend testing).", 2
		}
		if _, err := tx.Exec(ctx, `INSERT INTO reports (id, tenant_id, organization_id, assessment_id, version, snapshot, status, pdf_key, error,
attempts, created_at, updated_at, is_demo)
VALUES ($1,$2,$3,$4,1,$5,$6,'',$7,$8,$9,$9,true) ON CONFLICT (id) DO NOTHING`,
			reportID, g.tenant, orgID, assessID, raw, status, reportErr, attempts, endedAt); err != nil {
			return err
		}
	}
	return nil
}
