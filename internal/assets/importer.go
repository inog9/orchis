package assets

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/domain"
	"orchis-go/internal/groups"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/tags"
)

const (
	maxCSVBytes   = 1 << 20
	maxCSVRows    = 1000
	maxCSVColumns = 50
	maxRowTags    = 10
)

// Target pemetaan kolom CSV.
var importTargets = []string{"host", "name", "address", "category", "platform", "arch", "criticality", "tags", "ignore"}

type ImportRequest struct {
	CSV         string            `json:"csv"`
	Mapping     map[string]string `json:"mapping"`
	SkipInvalid bool              `json:"skip_invalid"`
}

type Column struct {
	Name   string   `json:"name"`
	Sample []string `json:"sample"`
}

type RowIssue struct {
	Row     int    `json:"row"` // nomor baris di berkas (header = baris 1)
	Message string `json:"message"`
}

type ImportPreview struct {
	Columns          []Column          `json:"columns"`
	SuggestedMapping map[string]string `json:"suggested_mapping"`
	Rows             int               `json:"rows"`
	Validated        bool              `json:"validated"`
	Valid            int               `json:"valid"`
	Invalid          []RowIssue        `json:"invalid"`
	Duplicates       []RowIssue        `json:"duplicates"`
}

type parsedRow struct {
	row       int
	name      string
	category  string
	platform  *string
	arch      *string
	crit      string
	addresses []Address
	tagNames  []string
}

func parseCSV(raw string) (header []string, records [][]string, err error) {
	if len(raw) > maxCSVBytes {
		return nil, nil, httpx.Validation(httpx.FieldError{Name: "csv", Message: "The file is too large (max 1 MB)."})
	}
	raw = strings.TrimPrefix(raw, "\xef\xbb\xbf")
	if strings.TrimSpace(raw) == "" {
		return nil, nil, httpx.Validation(httpx.FieldError{Name: "csv", Message: "The file is empty."})
	}
	// Pemisah: titik koma (umum pada Excel regional) atau koma, ditebak dari baris pertama.
	first := raw
	if i := strings.IndexAny(raw, "\r\n"); i >= 0 {
		first = raw[:i]
	}
	r := csv.NewReader(strings.NewReader(raw))
	if strings.Count(first, ";") > strings.Count(first, ",") {
		r.Comma = ';'
	}
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, httpx.Validation(httpx.FieldError{Name: "csv", Message: "The file is not valid CSV: " + err.Error()})
		}
		blank := true
		for _, c := range rec {
			if strings.TrimSpace(c) != "" {
				blank = false
			}
		}
		if blank {
			continue
		}
		if header == nil {
			header = rec
			continue
		}
		records = append(records, rec)
		if len(records) > maxCSVRows {
			return nil, nil, httpx.Validation(httpx.FieldError{Name: "csv", Message: fmt.Sprintf("Too many rows (max %d per import).", maxCSVRows)})
		}
	}
	if len(header) == 0 || len(header) > maxCSVColumns {
		return nil, nil, httpx.Validation(httpx.FieldError{Name: "csv", Message: "The first row must be a header with 1 to 50 columns."})
	}
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
		if header[i] == "" {
			return nil, nil, httpx.Validation(httpx.FieldError{Name: "csv", Message: fmt.Sprintf("Column %d has no header name.", i+1)})
		}
	}
	if len(records) == 0 {
		return nil, nil, httpx.Validation(httpx.FieldError{Name: "csv", Message: "The file has a header but no rows."})
	}
	return header, records, nil
}

func suggest(col string) string {
	c := strings.ToLower(strings.ReplaceAll(col, "_", " "))
	switch {
	case strings.Contains(c, "tag"):
		return "tags"
	case strings.Contains(c, "critical"):
		return "criticality"
	case strings.Contains(c, "arch"):
		return "arch"
	case c == "os" || strings.Contains(c, "platform") || strings.Contains(c, "operating"):
		return "platform"
	case strings.Contains(c, "categor") || c == "type":
		return "category"
	case strings.Contains(c, "address") || c == "ip" || strings.HasPrefix(c, "ip "):
		return "address"
	case strings.Contains(c, "host") || c == "name" || strings.Contains(c, "asset") || strings.Contains(c, "endpoint"):
		return "host"
	}
	return "ignore"
}

// InferKind menebak jenis alamat dari teks; NormalizeAddress tetap memvalidasi.
func InferKind(v string) string {
	s := strings.TrimSpace(v)
	switch {
	case strings.HasPrefix(strings.ToLower(s), "http://") || strings.HasPrefix(strings.ToLower(s), "https://"):
		return "url"
	case macRe.MatchString(strings.ToLower(strings.ReplaceAll(s, "-", ":"))):
		return "mac"
	case strings.Contains(s, "/"):
		return "cidr"
	}
	if _, msg := NormalizeAddress(Address{Kind: "ip", Value: s}); msg == "" {
		return "ip"
	}
	if strings.Contains(s, ".") {
		return "domain"
	}
	return "hostname"
}

var platformAlias = map[string]string{"windows": "windows", "win": "windows", "linux": "linux", "macos": "macos", "mac": "macos", "osx": "macos", "darwin": "macos"}
var archAlias = map[string]string{"x86_64": "x86_64", "x64": "x86_64", "amd64": "x86_64", "arm64": "arm64", "aarch64": "arm64"}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '|' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func validateMapping(m map[string]string, header []string) error {
	cols := map[string]bool{}
	for _, h := range header {
		cols[h] = true
	}
	hasName := false
	used := map[string]string{}
	for col, tgt := range m {
		if !cols[col] {
			return httpx.Validation(httpx.FieldError{Name: "mapping", Message: fmt.Sprintf("Column %q is not in the file.", col)})
		}
		if !domain.OneOf(tgt, importTargets) {
			return httpx.Validation(httpx.FieldError{Name: "mapping", Message: fmt.Sprintf("Unknown target %q.", tgt)})
		}
		if tgt == "host" || tgt == "name" {
			hasName = true
		}
		if tgt != "ignore" && tgt != "address" {
			if prev, dup := used[tgt]; dup {
				return httpx.Validation(httpx.FieldError{Name: "mapping", Message: fmt.Sprintf("Columns %q and %q both map to %q.", prev, col, tgt)})
			}
			used[tgt] = col
		}
	}
	if !hasName {
		return httpx.Validation(httpx.FieldError{Name: "mapping", Message: "Map one column to “Hostname or IP” (or “Name”)."})
	}
	return nil
}

func buildRows(header []string, records [][]string, m map[string]string) (rows []parsedRow, invalid []RowIssue) {
	idx := map[string][]int{}
	for i, h := range header {
		if t := m[h]; t != "" && t != "ignore" {
			idx[t] = append(idx[t], i)
		}
	}
	get := func(rec []string, i int) string {
		if i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	for n, rec := range records {
		rowNo := n + 2
		fail := func(msg string) { invalid = append(invalid, RowIssue{Row: rowNo, Message: msg}) }
		pr := parsedRow{row: rowNo, category: "host", crit: "medium"}
		var hostVal, nameVal string
		if i := idx["host"]; len(i) > 0 {
			hostVal = get(rec, i[0])
		}
		if i := idx["name"]; len(i) > 0 {
			nameVal = get(rec, i[0])
		}
		pr.name = nameVal
		if pr.name == "" {
			pr.name = hostVal
		}
		if pr.name == "" {
			fail("The name is empty.")
			continue
		}
		if len(pr.name) > maxNameLength {
			fail("The name is longer than 200 characters.")
			continue
		}
		raws := []string{}
		if hostVal != "" {
			raws = append(raws, hostVal)
		}
		for _, i := range idx["address"] {
			raws = append(raws, splitList(get(rec, i))...)
		}
		seen := map[string]bool{}
		bad := ""
		for _, raw := range raws {
			na, msg := NormalizeAddress(Address{Kind: InferKind(raw), Value: raw})
			if msg != "" {
				// O nama bebas (mis. "srv app 01") bukan alamat: hanya galat bila kolom alamat eksplisit.
				if raw == hostVal && nameVal == "" && !strings.ContainsAny(raw, ".:/") {
					continue
				}
				bad = fmt.Sprintf("%q: %s", raw, msg)
				break
			}
			if k := na.Kind + "|" + na.Value; !seen[k] {
				seen[k] = true
				pr.addresses = append(pr.addresses, na)
			}
		}
		if bad != "" {
			fail("Address " + bad)
			continue
		}
		if len(pr.addresses) > maxAddresses {
			fail("More than 20 addresses.")
			continue
		}
		if i := idx["category"]; len(i) > 0 && get(rec, i[0]) != "" {
			v := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(get(rec, i[0]), " ", "_"), "-", "_"))
			if !domain.OneOf(v, domain.Categories) {
				fail(fmt.Sprintf("Category %q isn't supported.", get(rec, i[0])))
				continue
			}
			pr.category = v
		}
		if i := idx["platform"]; len(i) > 0 && get(rec, i[0]) != "" {
			v, ok := platformAlias[strings.ToLower(get(rec, i[0]))]
			if !ok {
				fail(fmt.Sprintf("platform %q isn't supported. Use windows, linux or macos.", get(rec, i[0])))
				continue
			}
			pr.platform = &v
		}
		if i := idx["arch"]; len(i) > 0 && get(rec, i[0]) != "" {
			v, ok := archAlias[strings.ToLower(get(rec, i[0]))]
			if !ok {
				fail(fmt.Sprintf("architecture %q isn't supported. Use x86_64 or arm64.", get(rec, i[0])))
				continue
			}
			pr.arch = &v
		}
		if i := idx["criticality"]; len(i) > 0 && get(rec, i[0]) != "" {
			v := strings.ToLower(get(rec, i[0]))
			if !domain.OneOf(v, domain.Criticalities) {
				fail(fmt.Sprintf("criticality %q isn't supported. Use low, medium, high or critical.", get(rec, i[0])))
				continue
			}
			pr.crit = v
		}
		if i := idx["tags"]; len(i) > 0 {
			pr.tagNames = splitList(get(rec, i[0]))
			if len(pr.tagNames) > maxRowTags {
				fail("More than 10 tags.")
				continue
			}
			tooLong := false
			for _, t := range pr.tagNames {
				if len(t) > 50 {
					tooLong = true
				}
			}
			if tooLong {
				fail("A tag is longer than 50 characters.")
				continue
			}
		}
		rows = append(rows, pr)
	}
	return rows, invalid
}

// analyze menjalankan seluruh validasi (termasuk duplikat terhadap data yang ada) tanpa menulis apa pun.
func (s *Service) analyze(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, req ImportRequest) (*ImportPreview, []parsedRow, error) {
	header, records, err := parseCSV(req.CSV)
	if err != nil {
		return nil, nil, err
	}
	pv := &ImportPreview{Rows: len(records), SuggestedMapping: map[string]string{}, Invalid: []RowIssue{}, Duplicates: []RowIssue{}}
	for i, h := range header {
		col := Column{Name: h}
		for _, rec := range records[:min(3, len(records))] {
			if i < len(rec) {
				col.Sample = append(col.Sample, strings.TrimSpace(rec[i]))
			}
		}
		pv.Columns = append(pv.Columns, col)
		pv.SuggestedMapping[h] = suggest(h)
	}
	if len(req.Mapping) == 0 {
		return pv, nil, nil
	}
	if err := validateMapping(req.Mapping, header); err != nil {
		return nil, nil, err
	}
	rows, invalid := buildRows(header, records, req.Mapping)
	pv.Validated = true
	pv.Invalid = append(pv.Invalid, invalid...)

	existing := map[string]bool{}
	rs, err := tx.Query(ctx, `SELECT lower(name) FROM assets WHERE tenant_id=$1 AND deleted_at IS NULL`, tenant)
	if err != nil {
		return nil, nil, err
	}
	for rs.Next() {
		var n string
		if err := rs.Scan(&n); err != nil {
			rs.Close()
			return nil, nil, err
		}
		existing[n] = true
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, nil, err
	}
	var ok []parsedRow
	seen := map[string]int{}
	for _, r := range rows {
		k := strings.ToLower(r.name)
		if existing[k] {
			pv.Duplicates = append(pv.Duplicates, RowIssue{Row: r.row, Message: r.name + " already exists."})
			continue
		}
		if first, dup := seen[k]; dup {
			pv.Duplicates = append(pv.Duplicates, RowIssue{Row: r.row, Message: fmt.Sprintf("%s repeats row %d.", r.name, first)})
			continue
		}
		seen[k] = r.row
		ok = append(ok, r)
	}
	pv.Valid = len(ok)
	return pv, ok, nil
}

func (s *Service) ImportPreview(ctx context.Context, p *auth.Principal, req ImportRequest) (*ImportPreview, error) {
	var out *ImportPreview
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		pv, _, err := s.analyze(ctx, tx, p.TenantID, req)
		out = pv
		return err
	})
	return out, err
}

type ImportResult struct {
	Created    int `json:"created"`
	Skipped    int `json:"skipped_invalid"`
	Duplicates int `json:"skipped_duplicates"`
	TagsMade   int `json:"tags_created"`
}

// Import membuat aset dalam SATU transaksi (semua atau tidak sama sekali). Aset hasil impor tidak pernah
// berstatus "authorized": otorisasi pengujian tetap keputusan terpisah per aset.
func (s *Service) Import(ctx context.Context, p *auth.Principal, req ImportRequest, ip, reqID string) (*ImportResult, error) {
	res := &ImportResult{}
	err := s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		pv, rows, err := s.analyze(ctx, tx, p.TenantID, req)
		if err != nil {
			return err
		}
		if !pv.Validated {
			return httpx.Validation(httpx.FieldError{Name: "mapping", Message: "A column mapping is required."})
		}
		if len(pv.Invalid) > 0 && !req.SkipInvalid {
			return httpx.Validation(httpx.FieldError{Name: "csv", Message: fmt.Sprintf("%d row(s) need fixing. Fix the file or choose to skip them.", len(pv.Invalid))})
		}
		if len(rows) == 0 {
			return httpx.Validation(httpx.FieldError{Name: "csv", Message: "There are no new rows to import."})
		}
		res.Skipped, res.Duplicates = len(pv.Invalid), len(pv.Duplicates)

		tagIDs := map[string]uuid.UUID{}
		trs, err := tx.Query(ctx, `SELECT lower(name), id FROM tags WHERE tenant_id=$1`, p.TenantID)
		if err != nil {
			return err
		}
		for trs.Next() {
			var n string
			var id uuid.UUID
			if err := trs.Scan(&n, &id); err != nil {
				trs.Close()
				return err
			}
			tagIDs[n] = id
		}
		trs.Close()
		if err := trs.Err(); err != nil {
			return err
		}
		for _, r := range rows {
			id := ids.New()
			if _, err := tx.Exec(ctx, `INSERT INTO assets (id, tenant_id, category, name, platform, arch, criticality, source, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,'csv',$8)`, id, p.TenantID, r.category, r.name, r.platform, r.arch, r.crit, p.UserID); err != nil {
				return err
			}
			for i, ad := range r.addresses {
				if _, err := tx.Exec(ctx, `INSERT INTO asset_addresses (id, tenant_id, asset_id, kind, value, is_primary) VALUES ($1,$2,$3,$4,$5,$6)`,
					ids.New(), p.TenantID, id, ad.Kind, ad.Value, i == 0); err != nil {
					return err
				}
			}
			for _, name := range r.tagNames {
				key := strings.ToLower(name)
				tid, ok := tagIDs[key]
				if !ok {
					tid = ids.New()
					if _, err := tx.Exec(ctx, `INSERT INTO tags (id, tenant_id, name, color) VALUES ($1,$2,$3,$4)`, tid, p.TenantID, name, tags.DefaultColor); err != nil {
						return err
					}
					tagIDs[key] = tid
					res.TagsMade++
				}
				if _, err := tx.Exec(ctx, `INSERT INTO asset_tags (tenant_id, asset_id, tag_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, p.TenantID, id, tid); err != nil {
					return err
				}
			}
			res.Created++
		}
		if err := groups.RecomputeAll(ctx, tx, p.TenantID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "asset.import", ResourceType: "asset", IP: ip, RequestID: reqID,
			Detail: map[string]any{"created": res.Created, "skipped_invalid": res.Skipped, "skipped_duplicates": res.Duplicates, "tags_created": res.TagsMade}})
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (h *Handlers) ImportPreview(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in ImportRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxCSVBytes+4096)
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	pv, err := h.Svc.ImportPreview(r.Context(), p, in)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, pv)
}

func (h *Handlers) Import(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var in ImportRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxCSVBytes+4096)
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Write(w, r, err)
		return
	}
	res, err := h.Svc.Import(r.Context(), p, in, auth.ClientIP(r), httpx.RequestIDFrom(r.Context()))
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}
