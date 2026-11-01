package demo

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/chain"
	"orchis-go/internal/contract"
	"orchis-go/internal/osquery"
	"orchis-go/internal/reporting"
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

type findingSummary = reporting.Finding

type assetInfo struct {
	id         uuid.UUID
	name       string
	skipReason string
	findings   []findingSummary
}

// run cria 5 organisasi x 10 aset beragam, agent dengan status campuran, findings beragam
// severity/tipe, dan 3 assessment+report historis per organisasi (tren) — semua ditandai is_demo.
func (g *generator) run(ctx context.Context, tx pgx.Tx) error {
	authorizer := g.createdBy // operator seed nyata; tidak memakai referensi akun yatim
	demoTagIDs := []uuid.UUID{g.id("tag", "sample-demo-asset"), g.id("tag", "demo-critical-path")}
	demoTagNames := []string{"demo-sample", "demo-critical-path"}
	for i, tid := range demoTagIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO tags (id, tenant_id, name, color, is_demo) VALUES ($1,$2,$3,'#B45309',true) ON CONFLICT (id) DO NOTHING`,
			tid, g.tenant, demoTagNames[i]); err != nil {
			return err
		}
	}

	type orgFixture struct {
		index    int
		id       uuid.UUID
		name     string
		assets   []assetInfo
		severity map[string]int
	}
	orgs := []orgFixture{}
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
			authorized := (oi+ai)%2 == 0 && bp.addr != "hostname"
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
				// Demo yang diberi izin hanya menunjuk loopback; tidak ke jaringan luar.
				value := "127.0.0.1"
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
			if ai == 1 {
				nFindings = 3
			}
			for fi := 0; fi < nFindings; fi++ {
				ftype := findingTypes[(ai+fi)%len(findingTypes)]
				if ai == 1 && fi == 2 {
					ftype = "open_port"
				}
				sev := severities[(ai+fi*2)%len(severities)]
				value := fmt.Sprintf("demo-%s-%d-%d", ftype, ai, fi)
				if ftype == "open_port" {
					value = "tcp/80"
				}
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
					details["port"] = float64(80)
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
				summaries = append(summaries, findingSummary{ID: findingID.String(), MITRE: reporting.EstimatedTechniques(reporting.Finding{Type: ftype}), Evidence: reporting.SafeEvidence(details), Value: value, Title: title, Type: ftype, Severity: sev})
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

		if err := g.seedEndpointData(ctx, tx, oi, assets[0].id); err != nil {
			return err
		}
		orgs = append(orgs, orgFixture{oi, orgID, orgName, assets, vulnSeverity})
	}
	if err := g.seedScenarios(ctx, tx); err != nil {
		return err
	}
	if err := g.seedRelated(ctx, tx); err != nil {
		return err
	}
	for _, o := range orgs {
		if err := g.seedAssessmentsAndReports(ctx, tx, o.index, o.id, o.name, o.assets, o.severity); err != nil {
			return err
		}
	}
	if err := g.seedRunStates(ctx, tx); err != nil {
		return err
	}
	if err := g.seedAtomicHistory(ctx, tx); err != nil {
		return err
	}
	if err := g.seedAI(ctx, tx); err != nil {
		return err
	}
	// Reconcile metadata only for wholly synthetic occurrences. A later real
	// observation is never overwritten by reseeding illustrative history.
	if _, err := tx.Exec(ctx, `UPDATE findings f SET first_seen_at=o.first_seen,last_seen_at=o.last_seen,occurrence_count=o.n FROM (
 SELECT x.finding_id,min(x.seen_at) AS first_seen,max(x.seen_at) AS last_seen,count(*)::integer AS n
 FROM finding_occurrences x JOIN executions e ON e.tenant_id=x.tenant_id AND e.id=x.execution_id
 WHERE x.tenant_id=$1 AND e.is_demo GROUP BY x.finding_id) o
 WHERE f.tenant_id=$1 AND f.is_demo AND f.id=o.finding_id AND NOT EXISTS(
 SELECT 1 FROM finding_occurrences x JOIN executions e ON e.tenant_id=x.tenant_id AND e.id=x.execution_id
 WHERE x.tenant_id=f.tenant_id AND x.finding_id=f.id AND NOT e.is_demo)`, g.tenant); err != nil {
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
		[]demoStep{{"discover", "Discover open ports", "nmap.tcp_connect", "after_parents", `{"profile":"web"}`},
			{"scan", "Scan discovered web services", "nuclei.scan", "after_parents", `{"severity":["info","low","medium","high","critical"]}`},
			{"no-web", "Else: confirm the local port profile", "nmap.tcp_connect", "after_parents", `{"profile":"web"}`},
			{"failure", "Failure: bounded local retry", "nmap.tcp_connect", "after_parents", `{"profile":"top100"}`}}},
	{"Demo: endpoint baseline", "Synthetic scenario: read-only endpoint checks via the agent. Demo only.", "low", "endpoint", "ready",
		[]demoStep{{"info", "Collect system information", "agent.system_info", "after_parents", `{}`}, {"ports", "List listening ports", "agent.listening_ports", "after_parents", `{}`}}},
	{"Demo: draft web check", "Synthetic draft scenario that is not ready to run. Demo only.", "medium", "web", "draft",
		[]demoStep{{"web", "Check web ports", "nmap.tcp_connect", "after_parents", `{"profile":"web"}`}}},
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
		stepIDs := map[string]uuid.UUID{}
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
			stepIDs[st.key] = stepID
			tag, err := tx.Exec(ctx, `INSERT INTO scenario_steps (id, tenant_id, scenario_id, key, title, contract_id, contract_version, params, trigger_mode, position_x, position_y)
SELECT $1,$2,$3,$4,$5,c.id,c.version,$6,$7,$8,0 FROM contracts c WHERE c.tenant_id=$2 AND c.key=$9 AND c.version=$10
ON CONFLICT (id) DO NOTHING`, stepID, g.tenant, scID, st.key, st.title, params, st.trigger, i*240, st.contract, def.Version)
			if err != nil {
				return err
			}
			_ = tag

		}
		edges := demoEdges(si)
		steps := []chain.Step{}
		for _, st := range sc.steps {
			steps = append(steps, chain.Step{Key: st.key, Enabled: true, Mode: st.trigger})
		}
		if problems := chain.Validate(steps, edges, nil); len(problems) > 0 {
			return fmt.Errorf("invalid demo graph: %v", problems)
		}
		for _, edge := range edges {
			rule, err := json.Marshal(edge)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO scenario_step_edges(tenant_id,scenario_id,parent_step_id,child_step_id,rule) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, g.tenant, scID, stepIDs[edge.Parent], stepIDs[edge.Child], rule); err != nil {
				return err
			}
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

func demoEdges(si int) []chain.Edge {
	if si == 1 {
		return []chain.Edge{{Parent: "info", Child: "ports"}}
	}
	if si != 0 {
		return nil
	}
	condition := &chain.Condition{Op: "AND", Args: []chain.Condition{{Op: "DEPEND_ON"}, {Op: "OR", Args: []chain.Condition{{Op: "GT", Field: "finding_count", Value: float64(0)}, {Op: "IN", Field: "open_ports", Value: []any{float64(80), float64(443)}}}}}}
	return []chain.Edge{
		{Parent: "discover", Child: "scan", Condition: condition, Branch: "match", Mapper: &chain.Mapper{Source: "web_urls", Destination: "target"}},
		{Parent: "discover", Child: "no-web", Condition: condition, Branch: "else"},
		{Parent: "discover", Child: "failure", Condition: &chain.Condition{Op: "DEPEND_ON"}, Branch: "failure"},
	}
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

		if i == 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO alert_states(tenant_id,alert_id,user_id,read_at) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, g.tenant, g.id("alert", fmt.Sprint(i)), g.createdBy, g.now); err != nil {
				return err
			}
		}
	}
	return nil
}

func endpointSummary(total, withData int, p *osquery.Posture) map[string]any {
	by := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}
	if p != nil {
		for _, f := range p.Findings {
			by[f.Severity]++
		}
	}
	return map[string]any{"assets_total": total, "assets_with_data": withData, "by_severity": by}
}

// syntheticEndpointSets membuat hasil osquery SINTETIS yang realistis untuk satu organisasi (bukan hasil eksekusi nyata): bentuk
// dasar dari contoh katalog, dengan variasi per organisasi supaya postur dan temuan berbeda-beda.
func syntheticEndpointSets(oi int) map[string]*osquery.Dataset {
	now := time.Now().UTC()
	sets := map[string]*osquery.Dataset{}
	for i := range osquery.Queries {
		q := &osquery.Queries[i]
		if q.Pack != "" {
			continue // pack resmi bersifat opt-in; demo tidak mengarang datanya
		}
		rows := make([]map[string]string, 0, len(q.Example))
		for _, r := range q.Example {
			c := map[string]string{}
			for k, v := range r {
				c[k] = v
			}
			rows = append(rows, c)
		}
		switch q.Key {
		case "osquery.inventory":
			rows[0]["hostname"] = fmt.Sprintf("demo-linux-%02d", oi+1)
		case "osquery.users":
			if oi == 1 {
				rows = append(rows, map[string]string{"username": "backdoor", "uid": "0", "shell": "/bin/bash", "groups": "root"})
			}
		case "osquery.hardening":
			if oi%3 == 0 {
				rows = append(rows, map[string]string{"item": "sysctl", "target": "kernel.randomize_va_space", "value": "0"})
			}
		case "osquery.network":
			rows = append(rows, map[string]string{"port": "5432", "address": "0.0.0.0", "protocol": "6", "process": "postgres"}, map[string]string{"port": "22", "address": "0.0.0.0", "protocol": "6", "process": "sshd"})
		}
		sets[q.Key] = &osquery.Dataset{Query: q.Key, Status: osquery.StatusOK, Rows: rows, CollectedAt: now, Version: osquery.Version}
	}
	return sets
}

// seedEndpointData menyimpan data osquery sintetis untuk aset Linux demo (is_demo=true; bukan eksekusi nyata, tidak dihitung di MITRE).
func (g *generator) seedEndpointData(ctx context.Context, tx pgx.Tx, oi int, asset uuid.UUID) error {
	for key, ds := range syntheticEndpointSets(oi) {
		q := osquery.Find(key)
		safe, err := osquery.Sanitize(q, ds)
		if err != nil {
			return err
		}
		m := q.Map(safe)
		rows, _ := json.Marshal(safe.Rows)
		attrs, _ := json.Marshal(m.Attributes)
		if _, err := tx.Exec(ctx, `INSERT INTO endpoint_datasets (tenant_id, asset_id, query_key, rows, attributes, truncated, collected_at, osquery_version, last_status, last_attempt_at, is_demo)
VALUES ($1,$2,$3,$4,$5,false,$6,$7,'ok',$6,true) ON CONFLICT (tenant_id, asset_id, query_key) DO NOTHING`, g.tenant, asset, key, rows, attrs, g.now, osquery.Version); err != nil {
			return err
		}
	}
	return nil
}
