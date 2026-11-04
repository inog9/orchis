package osquery

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Status hasil satu query.
const (
	StatusOK      = "ok"
	StatusSkipped = "skipped"
	StatusFailed  = "failed"
)

// Alasan dilewati (ditampilkan apa adanya di UI; bukan galat diam-diam).
const (
	ReasonNeedsRoot   = "needs more privileges (administrator rights)"
	ReasonSensitive   = "skipped: sensitive data"
	ReasonUnsupported = "not supported on this operating system"
	ReasonDisabled    = "not approved for this organisation"
)

const (
	MaxDatasetBytes = 1 << 20 // batas JSON tersanitasi, termasuk metadata
	maxCell         = 512
	maxResult       = 8 << 20 // byte keluaran osquery yang dibaca
)

// Dataset adalah hasil satu query bernama dari satu endpoint. Dari agent ia TIDAK TEPERCAYA: server selalu memanggil
// Sanitize sebelum menyimpan atau memetakan.
type Dataset struct {
	Query       string              `json:"query"`
	Status      string              `json:"status"`
	Reason      string              `json:"reason,omitempty"`
	Rows        []map[string]string `json:"rows"`
	Truncated   bool                `json:"truncated,omitempty"`
	CollectedAt time.Time           `json:"collected_at"`
	Version     string              `json:"osquery_version,omitempty"`
}

var (
	authorizationRe = regexp.MustCompile(`(?i)authorization\s*[=:]\s*(?:basic|digest|bearer)\s+[^\r\n]+`)
	bearerRe        = regexp.MustCompile(`(?i)\bbearer\s+[^\s,;"']+`)
	redactRe        = regexp.MustCompile(`(?i)\b(password|passwd|secret|token|api[_-]?key|authorization)\s*[=:]\s*(?:"[^"\r\n]*(?:"|$)|'[^'\r\n]*(?:'|$)|[^\s,;]+)`)
	userinfoRe      = regexp.MustCompile(`(?i)(https?://)[^/\s@]+@`)
	privateKeyRe    = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`)
	versionRe       = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}(?:[.-][0-9]{1,5})?$`)
)

// Redact menutup kredensial, termasuk bearer, userinfo URL, dan blok kunci privat.
func Redact(s string) string {
	s = privateKeyRe.ReplaceAllString(s, "[redacted private key]")
	s = authorizationRe.ReplaceAllString(s, "Authorization=[redacted]")
	s = bearerRe.ReplaceAllString(s, "Bearer [redacted]")
	s = redactRe.ReplaceAllString(s, "$1=[redacted]")
	return userinfoRe.ReplaceAllString(s, "${1}[redacted]@")
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// Sanitize memvalidasi dataset yang tidak tepercaya terhadap definisi query: kolom di luar skema dibuang, sel dipotong dan
// diredaksi, baris dibatasi, status/alasan dinormalkan. Mengembalikan dataset BARU; masukan tidak diubah.
func Sanitize(q *Query, in *Dataset) (*Dataset, error) {
	if q == nil || in == nil || in.Query != q.Key {
		return nil, fmt.Errorf("dataset tidak cocok dengan query bernama")
	}
	out := &Dataset{Query: q.Key, Status: in.Status, Reason: clip(Redact(in.Reason), 200), Truncated: in.Truncated, CollectedAt: in.CollectedAt, Version: "", Rows: []map[string]string{}}
	if versionRe.MatchString(in.Version) {
		out.Version = in.Version
	}
	switch in.Status {
	case StatusOK, StatusSkipped, StatusFailed:
	default:
		return nil, fmt.Errorf("status %q tidak dikenal", in.Status)
	}
	if out.CollectedAt.IsZero() || out.CollectedAt.After(time.Now().Add(5*time.Minute)) {
		out.CollectedAt = time.Now().UTC()
	}
	if in.Status != StatusOK {
		return out, nil
	}
	allowed := map[string]bool{}
	for _, c := range q.Columns {
		allowed[c] = true
	}
	// Hitung metadata beserta escaping JSON dan penanda pemotongan sebelum menambah baris.
	budgetMeta := *out
	budgetMeta.Truncated = true
	metadata, err := json.Marshal(budgetMeta)
	if err != nil {
		return nil, err
	}
	remaining := MaxDatasetBytes - len(metadata)
	for i, row := range in.Rows {
		if i >= q.MaxRows {
			out.Truncated = true
			break
		}
		clean := make(map[string]string, len(q.Columns))
		for col, val := range row {
			if allowed[col] {
				clean[col] = clip(Redact(val), maxCell)
			}
		}
		encoded, err := json.Marshal(clean)
		if err != nil {
			return nil, err
		}
		if len(encoded)+1 > remaining {
			out.Truncated = true
			break
		}
		remaining -= len(encoded) + 1
		out.Rows = append(out.Rows, clean)
	}
	return out, nil
}

// ---------- pemetaan deklaratif ----------

// Finding adalah hasil pemetaan; server mengubahnya menjadi execution.NewFinding.
type Finding struct {
	Type     string
	Severity string
	Title    string
	Value    string
	Rule     string
	Explain  string
}

// Mapped adalah keluaran pemetaan satu dataset yang sudah disanitasi.
type Mapped struct {
	Attributes map[string]any
	Findings   []Finding
}

func num(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f, err == nil
}

func list(v any) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, fmt.Sprint(e))
		}
		return out
	case string:
		return []string{x}
	}
	return nil
}

func (m Match) holds(row map[string]string) bool {
	cell, ok := row[m.Col]
	if !ok {
		return false
	}
	switch m.Op {
	case "eq":
		return cell == fmt.Sprint(m.Val)
	case "ne":
		return cell != fmt.Sprint(m.Val)
	case "gt", "lt", "ge":
		a, ok1 := num(cell)
		b, ok2 := num(fmt.Sprint(m.Val))
		if !ok1 || !ok2 {
			return false
		}
		return (m.Op == "gt" && a > b) || (m.Op == "lt" && a < b) || (m.Op == "ge" && a >= b)
	case "in":
		for _, v := range list(m.Val) {
			if cell == v {
				return true
			}
		}
	case "has_any":
		have := map[string]bool{}
		for _, g := range strings.Split(cell, ",") {
			have[strings.TrimSpace(g)] = true
		}
		for _, v := range list(m.Val) {
			if have[v] {
				return true
			}
		}
	case "re":
		re, err := regexp.Compile(fmt.Sprint(m.Val))
		return err == nil && re.MatchString(cell)
	}
	return false
}

func all(ms []Match, row map[string]string) bool {
	for _, m := range ms {
		if !m.holds(row) {
			return false
		}
	}
	return true
}

var tplRe = regexp.MustCompile(`\{([a-z_]+)\}`)

func fill(t string, vars map[string]string) string {
	return tplRe.ReplaceAllStringFunc(t, func(m string) string { return vars[m[1:len(m)-1]] })
}

// Map menerapkan aturan beku pada dataset yang SUDAH disanitasi. Dataset yang tidak berstatus ok tidak menghasilkan apa pun.
// Tidak ada CVE yang dikarang: aturan hanya menilai nilai yang benar-benar diukur.
func (q *Query) Map(ds *Dataset) Mapped {
	m := Mapped{Attributes: map[string]any{}}
	if ds == nil || ds.Status != StatusOK {
		return m
	}
	if len(ds.Rows) > 0 {
		for attr, col := range q.Attributes.FirstRow {
			if v, ok := ds.Rows[0][col]; ok {
				m.Attributes[attr] = v
			}
		}
	}
	for _, ag := range q.Attributes.Aggregates {
		var rows []map[string]string
		for _, r := range ds.Rows {
			if all(ag.Where, r) {
				rows = append(rows, r)
			}
		}
		switch ag.Op {
		case "count":
			m.Attributes[ag.Key] = len(rows)
		case "list":
			limit := ag.Limit
			if limit <= 0 || limit > 100 {
				limit = 50
			}
			out := []map[string]string{}
			for _, r := range rows {
				if len(out) >= limit {
					break
				}
				item := map[string]string{}
				for _, c := range ag.Cols {
					item[c] = r[c]
				}
				out = append(out, item)
			}
			m.Attributes[ag.Key] = out
		}
	}
	seen := map[string]bool{}
	add := func(r Rule, vars map[string]string) {
		f := Finding{Type: r.Finding.Type, Severity: r.Finding.Severity, Title: clip(fill(r.Finding.Title, vars), 200), Value: clip(fill(r.Finding.Value, vars), 200), Rule: r.ID, Explain: r.Explain}
		if f.Title == "" || f.Value == "" || seen[f.Type+"\x00"+f.Value] {
			return
		}
		seen[f.Type+"\x00"+f.Value] = true
		m.Findings = append(m.Findings, f)
	}
	for _, r := range q.Rules {
		switch r.Scope {
		case "row":
			for _, row := range ds.Rows {
				if all(r.Match, row) {
					add(r, row)
				}
			}
		case "agg":
			n := 0
			for _, row := range ds.Rows {
				if all(r.CountWhere, row) {
					n++
				}
			}
			if r.CountLT > 0 {
				present := false
				for _, row := range ds.Rows {
					if all(r.Require, row) {
						present = true
						break
					}
				}
				if present && n < r.CountLT {
					add(r, map[string]string{"count": strconv.Itoa(n)})
				}
			} else if n > r.CountGT {
				add(r, map[string]string{"count": strconv.Itoa(n)})
			}
		}
	}
	sort.SliceStable(m.Findings, func(i, j int) bool { return m.Findings[i].Value < m.Findings[j].Value })
	return m
}
