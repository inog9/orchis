package execution

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"orchis-go/internal/contract"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/alerts"
	"orchis-go/internal/auth"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
)

const (
	severityRank    = `array_position(ARRAY['info','low','medium','high','critical'], f.severity)`
	maxFindings     = 2000
	maxValueLen     = 500
	maxTitleLen     = 300
	maxDetailsBytes = 8 << 10
)

var severities = map[string]bool{"info": true, "low": true, "medium": true, "high": true, "critical": true}

// NewFinding adalah satu temuan yang dilaporkan injector. Aset tidak ada di sini: server memakai aset sasaran
// eksekusi, sehingga injector tidak bisa menempelkan temuan ke aset lain.
type NewFinding struct {
	Type     string          `json:"type"`
	Value    string          `json:"value"`
	Title    string          `json:"title"`
	Severity string          `json:"severity"`
	Details  json.RawMessage `json:"details"`
}

func normalizeValue(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

func valueHash(typ, value string) []byte {
	h := sha256.Sum256([]byte(typ + "\x00" + normalizeValue(value)))
	return h[:]
}

// ingest menyimpan finding satu eksekusi: satu baris findings per (aset, jenis, nilai) dan satu occurrence per eksekusi.
// Mengembalikan jumlah yang diterima serta alasan untuk yang ditolak.
func ingest(ctx context.Context, tx pgx.Tx, tenant, execID, asset uuid.UUID, simulation *uuid.UUID, allowed []string, in []NewFinding) (accepted int, rejected []string, err error) {
	storedAsset := any(asset)
	var organization *uuid.UUID
	var subject *string
	if simulation != nil {
		var org uuid.UUID
		var assigned *uuid.UUID
		var target string
		e := tx.QueryRow(ctx, `SELECT a.organization_id,j.asset_id,j.target FROM scan_jobs j JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id WHERE j.tenant_id=$1 AND j.simulation_id=$2`, tenant, *simulation).Scan(&org, &assigned, &target)
		if e == nil {
			organization = &org
			storedAsset = assigned
			host := contract.HostOf(target)
			subject = &host
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return 0, nil, e
		}
	}
	allow := map[string]bool{}
	for _, t := range allowed {
		allow[t] = true
	}
	seen := map[string]bool{}
	for i, f := range in {
		if i >= maxFindings {
			rejected = append(rejected, "more than "+strconv.Itoa(maxFindings)+" findings; the rest were dropped")
			break
		}
		f.Value = strings.TrimSpace(f.Value)
		f.Title = strings.TrimSpace(f.Title)
		switch {
		case !allow[f.Type]:
			rejected = append(rejected, "finding type "+strconv.Quote(f.Type)+" is not allowed for this contract")
			continue
		case f.Value == "" || len(f.Value) > maxValueLen || f.Title == "" || len(f.Title) > maxTitleLen:
			rejected = append(rejected, "a finding had an empty or oversized value/title")
			continue
		}
		if !severities[f.Severity] {
			f.Severity = "info"
		}
		details := f.Details
		if len(details) == 0 || len(details) > maxDetailsBytes || !json.Valid(details) {
			details = json.RawMessage(`{}`)
		}
		hash := valueHash(f.Type, f.Value)
		key := f.Type + string(hash)
		if seen[key] { // duplikat di batch yang sama dihitung sekali
			continue
		}
		seen[key] = true
		var fid uuid.UUID
		conflict := `(tenant_id, asset_id, type, value_hash) WHERE asset_id IS NOT NULL`
		if organization != nil {
			if ptr, ok := storedAsset.(*uuid.UUID); ok && ptr == nil {
				conflict = `(tenant_id,organization_id,subject_address,type,value_hash) WHERE asset_id IS NULL AND organization_id IS NOT NULL`
			}
		}
		if err = tx.QueryRow(ctx, `INSERT INTO findings(id,tenant_id,asset_id,type,value,value_hash,title,severity,details,organization_id,subject_address)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT `+conflict+`
 DO UPDATE SET last_seen_at=now(),occurrence_count=findings.occurrence_count+1,title=EXCLUDED.title,severity=EXCLUDED.severity,details=EXCLUDED.details
 RETURNING id`, ids.New(), tenant, storedAsset, f.Type, normalizeValue(f.Value), hash, f.Title, f.Severity, []byte(details), organization, subject).Scan(&fid); err != nil {
			return 0, nil, err
		}

		if f.Severity == "critical" {
			var assetName string
			if err = tx.QueryRow(ctx, `SELECT name FROM assets WHERE tenant_id=$1 AND id=$2`, tenant, asset).Scan(&assetName); err != nil {
				return 0, nil, err
			}
			if err = alerts.Raise(ctx, tx, tenant, alerts.New{Kind: "new_critical_finding", Severity: "critical", Title: "Critical finding on " + assetName,
				Body: f.Title, ResourceType: "finding", ResourceID: &fid, DedupeKey: "finding:" + fid.String()}); err != nil {
				return 0, nil, err
			}
		}
		snapshot, _ := json.Marshal(f)
		if _, err = tx.Exec(ctx, `INSERT INTO finding_occurrences (id, tenant_id, finding_id, execution_id, simulation_id, details,finding_snapshot)
VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (tenant_id, finding_id, execution_id) DO NOTHING`, ids.New(), tenant, fid, execID, simulation, []byte(details), snapshot); err != nil {
			return 0, nil, err
		}
		if err = Enqueue(ctx, tx, tenant, TopicIndexFinder, TopicIndexFinder, "finding", fid, map[string]any{"finding_id": fid, "tenant_id": tenant}); err != nil {
			return 0, nil, err
		}
		accepted++
	}
	return accepted, rejected, nil
}

const findingCols = `f.id, f.asset_id, COALESCE(a.name,f.subject_address), f.type, f.value, f.title, f.severity, f.details, f.first_seen_at, f.last_seen_at, f.occurrence_count`

// queryFindings menjalankan SELECT finding. joinWhere harus diawali JOIN/WHERE yang membatasi tenant (f.tenant_id=$1).
func queryFindings(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, joinWhere string, args []any, tail string) ([]Finding, error) {
	rows, err := tx.Query(ctx, `SELECT `+findingCols+` FROM findings f
LEFT JOIN assets a ON a.tenant_id=f.tenant_id AND a.id=f.asset_id `+joinWhere+` `+tail, args...)
	if err != nil {
		return nil, err
	}
	return scanFindings(rows)
}

func scanFindings(rows pgx.Rows) ([]Finding, error) {
	defer rows.Close()
	out := []Finding{}
	for rows.Next() {
		var f Finding
		if err := rows.Scan(&f.ID, &f.AssetID, &f.AssetName, &f.Type, &f.Value, &f.Title, &f.Severity, &f.Details, &f.FirstSeenAt, &f.LastSeenAt, &f.OccurrenceCount); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// A materialized ID match prevents a rare substring from walking the entire
// timestamp index in random heap order. Fetch full finding rows only after the
// filtered page is selected; broad matches do not materialize evidence/details.
func querySubstringPage(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, joinWhere string, args []any, tail string) ([]Finding, error) {
	rows, err := tx.Query(ctx, `WITH matched AS MATERIALIZED(
 SELECT f.id,f.last_seen_at FROM findings f LEFT JOIN assets a ON a.tenant_id=f.tenant_id AND a.id=f.asset_id `+joinWhere+`),
 page AS(SELECT id FROM matched ORDER BY last_seen_at DESC,id DESC LIMIT $`+strconv.Itoa(len(args))+`)
 SELECT `+findingCols+` FROM page p JOIN findings f ON f.id=p.id
 LEFT JOIN assets a ON a.tenant_id=f.tenant_id AND a.id=f.asset_id WHERE f.tenant_id=$1
 ORDER BY f.last_seen_at DESC,f.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	return scanFindings(rows)
}

type FindingFilter struct {
	Type, Severity, Query, Cursor string
	AssetID                       *uuid.UUID
	OrganizationID                *uuid.UUID
	SimulationID                  *uuid.UUID
	Limit                         int
}

type findingCursor struct {
	T time.Time `json:"t"`
	I uuid.UUID `json:"i"`
}

func (s *Service) ListFindings(ctx context.Context, p *auth.Principal, f FindingFilter, search Searcher) (*FindingPage, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 25
	}
	if f.Type != "" && !isFindingType(f.Type) {
		return nil, httpx.Validation(httpx.FieldError{Name: "type", Message: "Unknown finding type."})
	}
	if f.Severity != "" && !severities[f.Severity] {
		return nil, httpx.Validation(httpx.FieldError{Name: "severity", Message: "Unknown severity."})
	}
	var cur *findingCursor
	if f.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(f.Cursor)
		var c findingCursor
		if err != nil || json.Unmarshal(raw, &c) != nil {
			return nil, httpx.Validation(httpx.FieldError{Name: "cursor", Message: "Invalid cursor."})
		}
		cur = &c
	}
	page := &FindingPage{Search: "none"}
	q := strings.TrimSpace(f.Query)
	var ids []uuid.UUID
	if q != "" {
		page.Search = "database"
		if search != nil {
			hits, err := search.SearchFindings(ctx, p.TenantID, q, 200)
			if err == nil {
				ids, page.Search = hits, "elasticsearch"
			} else {
				slog.Warn("elasticsearch tidak menjawab, pencarian memakai database", "err", err)
			}
		}
	}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		// Finding milik aset yang sudah dihapus tidak ditampilkan (aset dihapus lunak; datanya tetap ada di database).
		where := []string{"f.tenant_id=$1", "(f.asset_id IS NULL OR a.deleted_at IS NULL)"}
		args := []any{p.TenantID}
		add := func(cond string, v any) {
			args = append(args, v)
			where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
		}
		if f.Type != "" {
			add("f.type=?", f.Type)
		}
		if f.Severity != "" {
			add("f.severity=?", f.Severity)
		}
		if f.OrganizationID != nil {
			add("COALESCE(f.organization_id,a.organization_id)=?", *f.OrganizationID)
		}
		if f.AssetID != nil {
			add("f.asset_id=?", *f.AssetID)
		}
		if f.SimulationID != nil {
			add("EXISTS (SELECT 1 FROM finding_occurrences o WHERE o.tenant_id=f.tenant_id AND o.finding_id=f.id AND o.simulation_id=?)", *f.SimulationID)
		}
		if q != "" {
			if page.Search == "elasticsearch" {
				add("f.id = ANY(?)", ids)
			} else {
				add("(f.title ILIKE ? OR f.value ILIKE ?)", "%"+strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)+"%")
			}
		}
		if cur != nil {
			args = append(args, cur.T, cur.I)
			where = append(where, "(f.last_seen_at, f.id) < ($"+strconv.Itoa(len(args)-1)+", $"+strconv.Itoa(len(args))+")")
		}
		args = append(args, f.Limit+1)
		queryPage := queryFindings
		if q != "" && page.Search == "database" {
			queryPage = querySubstringPage
		}
		items, err := queryPage(ctx, tx, p.TenantID, "WHERE "+strings.Join(where, " AND "), args,
			"ORDER BY f.last_seen_at DESC, f.id DESC LIMIT $"+strconv.Itoa(len(args)))
		if err != nil {
			return err
		}
		if len(items) > f.Limit {
			items = items[:f.Limit]
			last := items[f.Limit-1]
			raw, _ := json.Marshal(findingCursor{T: last.LastSeenAt, I: last.ID})
			c := base64.RawURLEncoding.EncodeToString(raw)
			page.NextCursor = &c
		}
		page.Items = items
		return nil
	})
	return page, err
}

func isFindingType(t string) bool {
	switch t {
	case "open_port", "port_scan", "service", "vulnerability", "url", "text":
		return true
	}
	return false
}

func (s *Service) GetFinding(ctx context.Context, p *auth.Principal, id uuid.UUID) (*Finding, error) {
	var out *Finding
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		items, err := queryFindings(ctx, tx, p.TenantID, "WHERE f.tenant_id=$1 AND f.id=$2", []any{p.TenantID, id}, "")
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return httpx.NotFound("Finding")
		}
		out = &items[0]
		return nil
	})
	return out, err
}
