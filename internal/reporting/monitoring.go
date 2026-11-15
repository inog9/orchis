package reporting

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// monitoringWindowDays: deteksi blue team yang masih terlihat dalam jendela ini ikut dibekukan ke laporan.
const monitoringWindowDays = 30

// monitoringTypes adalah tipe temuan engine blue team (deteksi berdiri sendiri, bukan hasil satu scan).
func monitoringTypes() []string {
	out := make([]string, 0, len(typeTechniques))
	for k := range typeTechniques {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func hasMonitoring(assets []Asset) bool {
	for _, a := range assets {
		if len(a.Monitoring) > 0 {
			return true
		}
	}
	return false
}

// monitoringFindings membekukan deteksi blue team terbaru pada satu aset: tipe tetap, bukti dibatasi dan disunting.
// Isi koneksi tidak pernah disimpan; hanya tujuan, port, jumlah, dan nama proses.
func monitoringFindings(ctx context.Context, tx pgx.Tx, tenant, asset uuid.UUID) ([]Finding, error) {
	rows, err := tx.Query(ctx, `SELECT id,type,value,title,severity,details,first_seen_at,last_seen_at,occurrence_count FROM findings WHERE tenant_id=$1 AND asset_id=$2 AND type = ANY($3) AND last_seen_at > now() - make_interval(days => $4) ORDER BY last_seen_at DESC LIMIT 100`, tenant, asset, monitoringTypes(), monitoringWindowDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		var f Finding
		var id uuid.UUID
		var details map[string]any
		var first, last any
		var count int
		if err := rows.Scan(&id, &f.Type, &f.Value, &f.Title, &f.Severity, &details, &first, &last, &count); err != nil {
			return nil, err
		}
		f.ID = id.String()
		f.Value = Redact(f.Value, 300)
		f.Title = Redact(f.Title, 500)
		f.MITRE = EstimatedTechniques(f)
		f.Evidence = monitoringEvidence(details)
		f.Evidence["seen_count"] = count
		f.Evidence["first_seen"] = fmt.Sprint(first)
		f.Evidence["last_seen"] = fmt.Sprint(last)
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return sevOrder[out[i].Severity] > sevOrder[out[j].Severity] })
	return out, rows.Err()
}

func monitoringEvidence(d map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"dst", "connections", "ports", "processes", "non_standard_port", "beacon_like"} {
		if v, ok := d[k]; ok {
			switch x := v.(type) {
			case string:
				out[k] = Redact(x, 300)
			case []any:
				parts := make([]string, 0, len(x))
				for _, p := range x {
					parts = append(parts, Redact(fmt.Sprint(p), 100))
				}
				out[k] = strings.Join(parts, ", ")
			default:
				out[k] = v
			}
		}
	}
	if row, ok := d["row"].(map[string]any); ok {
		var parts []string
		for _, k := range []string{"port", "address", "process", "name", "version", "username", "uid"} {
			if v, ok := row[k]; ok && fmt.Sprint(v) != "" {
				parts = append(parts, k+"="+Redact(fmt.Sprint(v), 100))
			}
		}
		if len(parts) > 0 {
			out["observed"] = strings.Join(parts, " ")
		}
	}
	return out
}
