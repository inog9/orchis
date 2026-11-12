// Package blueteam: engine deteksi endpoint berbasis aturan deterministik (tanpa AI).
// Membaca data yang sudah terkumpul (dataset osquery/agent + baseline) dan menghasilkan
// finding: tamper agent, port listening baru, software baru, akun baru, koneksi C2.
package blueteam

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/alerts"
	"orchis-go/internal/authz"
	"orchis-go/internal/ids"
)

// AgentRow adalah info agent untuk satu aset.
type AgentRow struct {
	ID             uuid.UUID
	Hostname       string
	Status         string
	UninstallStatus string
	RevokedAt      *time.Time
	LastHeartbeat  *time.Time
}

// Connection adalah satu koneksi keluar yang di-agregasi per (proses, tujuan).
type Connection struct {
	Process string
	Path    string
	Proto   string
	DstIP   string
	DstPort int
	State   string
	Count   int
}

// Input adalah semua data yang dibutuhkan satu run analisis untuk satu aset.
type Input struct {
	Tenant         uuid.UUID
	Asset          uuid.UUID
	AssetName      string
	OrgID          *uuid.UUID
	Now            time.Time
	OfflineMinutes int
	Agents         []AgentRow
	Datasets       map[string][]map[string]string // query_key -> rows (status ok)
	Connections    []Connection                   // dari finding net_connection terbaru
	Baselines      map[string][]map[string]string // query_key -> rows baseline sebelumnya
}

// Finding adalah temuan blueteam.
type Finding struct {
	Type     string         `json:"type"`
	Value    string         `json:"value"`
	Title    string         `json:"title"`
	Severity string         `json:"severity"`
	Details  map[string]any `json:"details"`
}

// Port standar yang tidak dicurigai sebagai C2.
var standardPorts = map[int]bool{53: true, 80: true, 443: true, 853: true, 123: true, 993: true, 995: true}

// group admin yang menandakan hak istimewa.
var adminGroups = map[string]bool{"root": true, "admin": true, "administrators": true, "sudo": true, "wheel": true, "adm": true}

// Run menjalankan semua aturan dan mengembalikan finding baru/naik.
func Run(in Input) []Finding {
	var out []Finding
	out = append(out, ruleTamper(in)...)
	out = append(out, ruleDiff(in, "osquery.network", "port|address", "new_listening_port", "high", func(r map[string]string) (string, string) {
		return fmt.Sprintf("%s:%s", r["address"], r["port"]), fmt.Sprintf("New listening port %s on %s", r["port"], r["address"])
	})...)
	for _, key := range []string{"osquery.packages", "osquery.macos_apps", "osquery.programs"} {
		out = append(out, ruleDiff(in, key, "name|version", "new_software_installed", "high", func(r map[string]string) (string, string) {
			return r["name"] + "|" + r["version"], fmt.Sprintf("New software installed: %s %s", r["name"], r["version"])
		})...)
	}
	out = append(out, ruleAccounts(in)...)
	out = append(out, ruleConnections(in)...)
	return out
}

func ruleTamper(in Input) []Finding {
	var out []Finding
	for _, a := range in.Agents {
		if a.RevokedAt != nil {
			out = append(out, Finding{Type: "agent_tamper", Value: a.ID.String(), Severity: "critical",
				Title: "Agent credentials revoked (possible tamper)", Details: map[string]any{"agent": a.Hostname, "reason": "revoked"}})
			continue
		}
		if a.UninstallStatus != "" && a.UninstallStatus != "none" {
			out = append(out, Finding{Type: "agent_tamper", Value: a.ID.String(), Severity: "critical",
				Title: "Agent uninstall " + a.UninstallStatus, Details: map[string]any{"agent": a.Hostname, "uninstall_status": a.UninstallStatus}})
			continue
		}
		if a.LastHeartbeat != nil && in.Now.Sub(*a.LastHeartbeat) > time.Duration(2*in.OfflineMinutes)*time.Minute {
			out = append(out, Finding{Type: "agent_unresponsive", Value: a.ID.String(), Severity: "high",
				Title: "Agent unresponsive", Details: map[string]any{"agent": a.Hostname, "last_heartbeat": a.LastHeartbeat}})
		}
	}
	return out
}

// ruleDiff membandingkan rows dataset saat ini dengan baseline; baris baru jadi finding.
func ruleDiff(in Input, key, keySep, ftype, severity string, mk func(map[string]string) (string, string)) []Finding {
	cur := in.Datasets[key]
	if cur == nil {
		return nil
	}
	old := map[string]bool{}
	for _, r := range in.Baselines[key] {
		parts := []string{}
		for _, f := range splitKey(keySep) {
			parts = append(parts, r[f])
		}
		old[joinKey(parts)] = true
	}
	var out []Finding
	seen := map[string]bool{}
	for _, r := range cur {
		parts := []string{}
		for _, f := range splitKey(keySep) {
			parts = append(parts, r[f])
		}
		k := joinKey(parts)
		if old[k] || seen[k] {
			continue
		}
		seen[k] = true
		val, title := mk(r)
		out = append(out, Finding{Type: ftype, Value: val, Title: title, Severity: severity, Details: map[string]any{"row": r, "source": key}})
	}
	return out
}

func splitKey(sep string) []string {
	out := []string{}
	cur := ""
	for _, c := range sep {
		if c == '|' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(c)
		}
	}
	return append(out, cur)
}
func joinKey(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "|"
		}
		out += p
	}
	return out
}

func ruleAccounts(in Input) []Finding {
	var out []Finding
	users := in.Datasets["osquery.users"]
	oldUsers := map[string]bool{}
	for _, r := range in.Baselines["osquery.users"] {
		oldUsers[r["uid"]+"|"+r["username"]] = true
	}
	for _, r := range users {
		k := r["uid"] + "|" + r["username"]
		if oldUsers[k] {
			continue
		}
		sev := "high"
		if r["uid"] == "0" {
			sev = "critical"
		}
		out = append(out, Finding{Type: "new_local_account", Value: k, Severity: sev,
			Title: fmt.Sprintf("New local account %s (uid %s)", r["username"], r["uid"]),
			Details: map[string]any{"row": r, "uid0": r["uid"] == "0"}})
	}
	// group admin baru
	oldG := map[string]bool{}
	for _, r := range in.Baselines["osquery.user_groups"] {
		oldG[r["user_id"]+"|"+r["group_id"]] = true
	}
	groupNames := map[string]string{}
	for _, r := range in.Datasets["osquery.groups"] {
		groupNames[r["gid"]] = r["group_name"]
	}
	userNames := map[string]string{}
	for _, r := range users {
		userNames[r["uid"]] = r["username"]
	}
	for _, r := range in.Datasets["osquery.user_groups"] {
		k := r["user_id"] + "|" + r["group_id"]
		if oldG[k] {
			continue
		}
		gname := groupNames[r["group_id"]]
		if !adminGroups[gname] {
			continue
		}
		out = append(out, Finding{Type: "new_local_account", Value: "admin:" + userNames[r["user_id"]], Severity: "critical",
			Title: fmt.Sprintf("Account %s added to privileged group %s", userNames[r["user_id"]], gname),
			Details: map[string]any{"user": userNames[r["user_id"]], "group": gname}})
	}
	return out
}

// ruleConnections mendeteksi pola C2 dari koneksi keluar yang ter-agregasi.
func ruleConnections(in Input) []Finding {
	byDst := map[string][]Connection{}
	for _, c := range in.Connections {
		byDst[c.DstIP] = append(byDst[c.DstIP], c)
	}
	var out []Finding
	for dst, conns := range byDst {
		total := 0
		ports := map[int]bool{}
		procs := map[string]bool{}
		nonStd := false
		for _, c := range conns {
			total += c.Count
			ports[c.DstPort] = true
			procs[c.Process] = true
			if !standardPorts[c.DstPort] {
				nonStd = true
			}
		}
		if len(conns) == 0 {
			continue
		}
		if nonStd || total >= 10 {
			sev := "medium"
			if nonStd {
				sev = "high"
			}
			plist := make([]string, 0, len(procs))
			for p := range procs {
				plist = append(plist, p)
			}
			sort.Strings(plist)
			portList := make([]int, 0, len(ports))
			for p := range ports {
				portList = append(portList, p)
			}
			sort.Ints(portList)
			out = append(out, Finding{
				Type: "c2_suspect_connection", Value: dst, Severity: sev,
				Title: fmt.Sprintf("Suspicious outbound connections to %s (%d conns, ports %v)", dst, total, portList),
				Details: map[string]any{"dst": dst, "connections": total, "ports": portList, "processes": plist, "non_standard_port": nonStd, "beacon_like": total >= 10},
			})
		}
	}
	return out
}

// Write menyimpan findings (upsert seperti execution) dan memicu alert untuk yang critical.
func Write(ctx context.Context, tx pgx.Tx, in Input, findings []Finding) error {
	for _, f := range findings {
		h := sha256.Sum256([]byte(f.Type + "\x00" + f.Value))
		details, _ := json.Marshal(f.Details)
		var fid uuid.UUID
		err := tx.QueryRow(ctx, `INSERT INTO findings(id,tenant_id,asset_id,type,value,value_hash,title,severity,details,organization_id,subject_address)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULL)
 ON CONFLICT (tenant_id, asset_id, type, value_hash) WHERE asset_id IS NOT NULL
 DO UPDATE SET last_seen_at=now(),occurrence_count=findings.occurrence_count+1,title=EXCLUDED.title,severity=EXCLUDED.severity,details=EXCLUDED.details
 RETURNING id`, ids.New(), in.Tenant, in.Asset, f.Type, f.Value, hex.EncodeToString(h[:]), f.Title, f.Severity, details, in.OrgID).Scan(&fid)
		if err != nil {
			return err
		}
		if f.Severity == "critical" {
			if err := alerts.Raise(ctx, tx, in.Tenant, alerts.New{
				Kind: "blueteam_critical", Severity: "critical",
				Title: "Critical endpoint detection on " + in.AssetName,
				Body:  f.Title, ResourceType: "finding", ResourceID: &fid,
				Audience: authz.FindingRead, DedupeKey: "blueteam:" + fid.String(),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// UpdateBaselines menyimpan rows dataset sebagai baseline berikutnya.
func UpdateBaselines(ctx context.Context, tx pgx.Tx, tenant, asset uuid.UUID, datasets map[string][]map[string]string, keys []string) error {
	for _, k := range keys {
		rows := datasets[k]
		raw, _ := json.Marshal(rows)
		h := sha256.Sum256(raw)
		if _, err := tx.Exec(ctx, `INSERT INTO endpoint_baselines(tenant_id,asset_id,query_key,rows,rows_hash,updated_at)
 VALUES($1,$2,$3,$4,$5,now()) ON CONFLICT (tenant_id,asset_id,query_key) DO UPDATE SET rows=EXCLUDED.rows, rows_hash=EXCLUDED.rows_hash, updated_at=now()`,
			tenant, asset, k, raw, h[:]); err != nil {
			return err
		}
	}
	return nil
}
