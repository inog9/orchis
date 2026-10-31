// Package groups: grup aset dinamis. Keanggotaan dihitung dari aturan dan disimpan (asset_group_members).
package groups

import (
	"fmt"
	"strconv"
	"strings"

	"orchis-go/internal/domain"
	"orchis-go/internal/httpx"
)

type Filter struct {
	Field    string   `json:"field"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
}

type Rule struct {
	Mode    string   `json:"mode"`
	Filters []Filter `json:"filters"`
}

const (
	maxFilters  = 10
	maxValues   = 20
	maxValueLen = 253
)

var enumFields = map[string][]string{
	"category":    domain.Categories,
	"platform":    domain.Platforms,
	"arch":        domain.Arches,
	"criticality": domain.Criticalities,
}

var columnOf = map[string]string{
	"category": "a.category", "platform": "a.platform", "arch": "a.arch", "criticality": "a.criticality",
}

// operatorsFor menentukan operator yang sah untuk tiap kolom.
func operatorsFor(field string) []string {
	switch field {
	case "category", "platform", "arch", "criticality":
		return []string{"eq", "neq", "in", "not_in", "is_empty", "is_not_empty"}
	case "tag":
		return []string{"eq", "neq", "in", "not_in", "is_empty", "is_not_empty"}
	case "is_internet_facing":
		return []string{"eq", "neq"}
	case "hostname":
		return []string{"eq", "contains", "is_empty", "is_not_empty"}
	}
	return nil
}

// Fields = kolom yang boleh dipakai aturan (untuk antarmuka).
var Fields = []string{"category", "platform", "arch", "criticality", "tag", "is_internet_facing", "hostname"}

// Normalize memeriksa dan menormalkan aturan. Kesalahan dikembalikan sebagai 422 pada kolom "rule".
func (r *Rule) Normalize() error {
	bad := func(msg string) error { return httpx.Validation(httpx.FieldError{Name: "rule", Message: msg}) }
	if r.Mode == "" {
		r.Mode = "and"
	}
	if r.Mode != "and" && r.Mode != "or" {
		return bad("Mode must be \"and\" or \"or\".")
	}
	if len(r.Filters) > maxFilters {
		return bad(fmt.Sprintf("At most %d filters.", maxFilters))
	}
	for i := range r.Filters {
		f := &r.Filters[i]
		ops := operatorsFor(f.Field)
		if ops == nil {
			return bad("Unknown field: " + f.Field)
		}
		if !domain.OneOf(f.Operator, ops) {
			return bad(fmt.Sprintf("Operator %q is not valid for %s.", f.Operator, f.Field))
		}
		switch f.Operator {
		case "is_empty", "is_not_empty":
			if len(f.Values) != 0 {
				return bad(fmt.Sprintf("%s %s takes no value.", f.Field, f.Operator))
			}
		case "eq", "neq", "contains":
			if len(f.Values) != 1 {
				return bad(fmt.Sprintf("%s %s needs exactly one value.", f.Field, f.Operator))
			}
		case "in", "not_in":
			if len(f.Values) < 1 || len(f.Values) > maxValues {
				return bad(fmt.Sprintf("%s %s needs 1 to %d values.", f.Field, f.Operator, maxValues))
			}
		}
		for j, v := range f.Values {
			v = strings.TrimSpace(v)
			if v == "" || len(v) > maxValueLen {
				return bad("Values must be non-empty and at most 253 characters.")
			}
			if set, ok := enumFields[f.Field]; ok && !domain.OneOf(v, set) {
				return bad(fmt.Sprintf("%q is not a valid %s.", v, f.Field))
			}
			if f.Field == "is_internet_facing" && v != "true" && v != "false" {
				return bad("is_internet_facing must be true or false.")
			}
			if f.Field == "hostname" {
				v = strings.ToLower(v)
			}
			f.Values[j] = v
		}
	}
	return nil
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Where menghasilkan klausa WHERE terparameter untuk tabel assets (alias "a") dan argumennya.
// Placeholder dimulai dari $start. Aturan harus sudah lolos Normalize.
func (r Rule) Where(start int) (string, []any) {
	if len(r.Filters) == 0 {
		return "TRUE", nil
	}
	var parts []string
	var args []any
	n := start
	arg := func(v any) string { args = append(args, v); p := "$" + strconv.Itoa(n); n++; return p }

	anyTag := `EXISTS (SELECT 1 FROM asset_tags at JOIN tags t ON t.tenant_id = at.tenant_id AND t.id = at.tag_id
 WHERE at.tenant_id = a.tenant_id AND at.asset_id = a.id%s)`
	anyHost := `EXISTS (SELECT 1 FROM asset_addresses ad WHERE ad.tenant_id = a.tenant_id AND ad.asset_id = a.id
 AND ad.kind IN ('hostname','domain')%s)`

	for _, f := range r.Filters {
		switch f.Field {
		case "category", "platform", "arch", "criticality":
			col := columnOf[f.Field]
			switch f.Operator {
			case "eq":
				parts = append(parts, col+" = "+arg(f.Values[0]))
			case "neq":
				parts = append(parts, col+" IS DISTINCT FROM "+arg(f.Values[0]))
			case "in":
				parts = append(parts, col+" = ANY("+arg(f.Values)+")")
			case "not_in":
				parts = append(parts, "("+col+" IS NULL OR "+col+" <> ALL("+arg(f.Values)+"))")
			case "is_empty":
				parts = append(parts, col+" IS NULL")
			case "is_not_empty":
				parts = append(parts, col+" IS NOT NULL")
			}
		case "is_internet_facing":
			want := f.Values[0] == "true"
			if f.Operator == "neq" {
				want = !want
			}
			parts = append(parts, "a.is_internet_facing = "+arg(want))
		case "tag":
			switch f.Operator {
			case "eq":
				parts = append(parts, fmt.Sprintf(anyTag, " AND t.name = "+arg(f.Values[0])))
			case "neq":
				parts = append(parts, "NOT "+fmt.Sprintf(anyTag, " AND t.name = "+arg(f.Values[0])))
			case "in":
				parts = append(parts, fmt.Sprintf(anyTag, " AND t.name = ANY("+arg(f.Values)+")"))
			case "not_in":
				parts = append(parts, "NOT "+fmt.Sprintf(anyTag, " AND t.name = ANY("+arg(f.Values)+")"))
			case "is_empty":
				parts = append(parts, "NOT "+fmt.Sprintf(anyTag, ""))
			case "is_not_empty":
				parts = append(parts, fmt.Sprintf(anyTag, ""))
			}
		case "hostname":
			switch f.Operator {
			case "eq":
				parts = append(parts, fmt.Sprintf(anyHost, " AND ad.value = "+arg(f.Values[0])))
			case "contains":
				parts = append(parts, fmt.Sprintf(anyHost, " AND ad.value LIKE "+arg("%"+likeEscape(f.Values[0])+"%")+` ESCAPE '\'`))
			case "is_empty":
				parts = append(parts, "NOT "+fmt.Sprintf(anyHost, ""))
			case "is_not_empty":
				parts = append(parts, fmt.Sprintf(anyHost, ""))
			}
		}
	}
	join := " AND "
	if r.Mode == "or" {
		join = " OR "
	}
	return "(" + strings.Join(parts, join) + ")", args
}
