package chain

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// Condition adalah AST terbatas, tanpa eval, path bebas, atau eksekusi kode.
type Condition struct {
	Op    string      `json:"op"`
	Field string      `json:"field,omitempty"`
	Value any         `json:"value,omitempty"`
	Args  []Condition `json:"args,omitempty"`
}

const softwarePrefix = "osquery:package:"

// InstalledSoftware mengembalikan nama paket dari finding osquery (tipe service, nilai "osquery:package:<nama>"), atau "".
// Hanya finding hasil aturan osquery yang dibekukan yang berbentuk ini; teks lain tidak pernah dianggap paket.
func InstalledSoftware(f Found) string {
	if f.Type != "service" || !strings.HasPrefix(f.Value, softwarePrefix) {
		return ""
	}
	name := strings.ToLower(strings.TrimPrefix(f.Value, softwarePrefix))
	if name == "" || len(name) > 100 {
		return ""
	}
	return name
}

// SoftwareTags adalah tabel beku paket -> tag Nuclei. Nama paket dari endpoint tidak pernah menjadi argumen: hanya tag di tabel ini yang lolos.
var SoftwareTags = map[string]string{"nginx": "nginx", "apache2": "apache", "httpd": "apache"}

type Mapper struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

func (c Condition) Validate() error {
	nodes := 0
	var walk func(Condition, int) error
	walk = func(c Condition, depth int) error {
		nodes++
		if nodes > 128 || depth > 12 {
			return fmt.Errorf("condition exceeds complexity limit")
		}
		switch c.Op {
		case "AND", "OR":
			if len(c.Args) < 1 || len(c.Args) > 20 {
				return fmt.Errorf("AND/OR need 1 to 20 operands")
			}
			for _, a := range c.Args {
				if e := walk(a, depth+1); e != nil {
					return e
				}
			}
			return nil
		case "DEPEND_ON":
			if c.Field != "" || len(c.Args) > 0 {
				return fmt.Errorf("DEPEND_ON takes no operands")
			}
			return nil
		case "EQ", "NEQ", "IS_NULL", "IS_NOT_NULL", "GT", "GTE", "LT", "LTE", "IN", "NIN":
		default:
			return fmt.Errorf("unknown condition operator")
		}
		if len(c.Args) > 0 {
			return fmt.Errorf("comparison does not accept nested arguments")
		}
		switch c.Field {
		case "finding_count", "max_severity", "finding_types", "open_ports", "installed_software", "outcome":
		default:
			return fmt.Errorf("unknown condition field")
		}
		data, _ := json.Marshal(c.Value)
		if len(data) > 4096 {
			return fmt.Errorf("condition value too large")
		}
		if c.Op == "IN" || c.Op == "NIN" {
			v, ok := c.Value.([]any)
			if !ok || len(v) > 100 {
				return fmt.Errorf("IN/NIN need an array of at most 100 values")
			}
		}
		return nil
	}
	return walk(c, 0)
}
func parentValues(status string, found []Found) map[string]any {
	var max any
	types := []any{}
	ports := []any{}
	software := []any{}
	seenType, seenPort, seenSoft := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, f := range found {
		if !seenType[f.Type] {
			types = append(types, f.Type)
			seenType[f.Type] = true
		}
		rank := float64(severityRank[f.Severity])
		if max == nil || rank > max.(float64) {
			max = rank
		}
		if name := InstalledSoftware(f); name != "" && !seenSoft[name] {
			software = append(software, name)
			seenSoft[name] = true
		}
		if f.Type == "open_port" {
			raw := strings.TrimPrefix(strings.ToLower(f.Value), "tcp/")
			if n, e := strconv.Atoi(raw); e == nil && n > 0 && n <= 65535 && !seenPort[raw] {
				ports = append(ports, float64(n))
				seenPort[raw] = true
			}
		}
	}
	sort.Slice(types, func(i, j int) bool { return types[i].(string) < types[j].(string) })
	sort.Slice(software, func(i, j int) bool { return software[i].(string) < software[j].(string) })
	sort.Slice(ports, func(i, j int) bool { return ports[i].(float64) < ports[j].(float64) })
	outcome := "success"
	if status == StFailed {
		outcome = "error"
	} else if status != StDone {
		outcome = status
	}
	return map[string]any{"finding_count": float64(len(found)), "max_severity": max, "finding_types": types, "open_ports": ports, "installed_software": software, "outcome": outcome}
}
func (c Condition) Evaluate(status string, found []Found) bool {
	if c.Validate() != nil {
		return false
	}
	switch c.Op {
	case "AND":
		for _, a := range c.Args {
			if !a.Evaluate(status, found) {
				return false
			}
		}
		return true
	case "OR":
		for _, a := range c.Args {
			if a.Evaluate(status, found) {
				return true
			}
		}
		return false
	case "DEPEND_ON":
		return status == StDone || status == "success"
	}
	actual := parentValues(status, found)[c.Field]
	switch c.Op {
	case "IS_NULL":
		return actual == nil
	case "IS_NOT_NULL":
		return actual != nil
	case "EQ":
		return reflect.DeepEqual(actual, c.Value)
	case "NEQ":
		return !reflect.DeepEqual(actual, c.Value)
	case "IN", "NIN":
		values := c.Value.([]any)
		hit := false
		items, ok := actual.([]any)
		if !ok {
			items = []any{actual}
		}
		for _, a := range items {
			for _, v := range values {
				if reflect.DeepEqual(a, v) {
					hit = true
				}
			}
		}
		if c.Op == "NIN" {
			return !hit
		}
		return hit
	default:
		a, ok := actual.(float64)
		b, ok2 := c.Value.(float64)
		if !ok || !ok2 {
			return false
		}
		switch c.Op {
		case "GT":
			return a > b
		case "GTE":
			return a >= b
		case "LT":
			return a < b
		case "LTE":
			return a <= b
		}
	}
	return false
}
func HasAdvanced(edges []Edge) bool {
	for _, e := range edges {
		if e.Condition != nil || e.Branch != "" || e.Mapper != nil {
			return true
		}
	}
	return false
}

// evaluateAdvanced mengasumsikan pemanggil sudah menyaring data ke satu aset dan satu run.
func evaluateAdvanced(steps []Step, edges []Edge, execs []Exec, found []Found) Result {
	incoming := map[string][]Edge{}
	for _, e := range edges {
		incoming[e.Child] = append(incoming[e.Child], e)
	}
	by := map[string]Step{}
	xs := map[string][]Exec{}
	fs := map[string][]Found{}
	for _, s := range steps {
		by[s.Key] = s
	}
	for _, x := range execs {
		xs[x.Step] = append(xs[x.Step], x)
	}
	for _, f := range found {
		fs[f.Step] = append(fs[f.Step], f)
	}
	res := Result{Status: map[string]string{}, Reasons: map[string]string{}}
	visiting := map[string]bool{}
	var eval func(string) string
	eval = func(key string) string {
		if v, ok := res.Status[key]; ok {
			return v
		}
		if visiting[key] {
			return StBlocked
		}
		visiting[key] = true
		defer delete(visiting, key)
		s := by[key]
		state, why := StEligible, "All parent dependencies matched."
		switch {
		case !s.Enabled:
			state, why = StSkipped, "Step is disabled."
		case len(xs[key]) > 0:
			state, why = strictExecStatus(xs[key]), "Execution state."
		default:
			parents := incoming[key]
			for _, e := range parents {
				if !terminal(eval(e.Parent)) {
					state, why = StWaiting, "Waiting for all parents to finish."
					break
				}
			}
			if state == StEligible {
				for _, e := range parents {
					ps := eval(e.Parent)
					match := true
					if e.Condition != nil {
						match = e.Condition.Evaluate(parentOutcome(xs[e.Parent], ps), fs[e.Parent])
					}
					branch := e.Branch
					if branch == "" {
						branch = "match"
					}
					allowed := false
					switch branch {
					case "failure":
						allowed = ps == StFailed
					case "else":
						allowed = ps == StDone && !match
					case "match":
						allowed = (ps == StDone || !by[e.Parent].Enabled) && match
					}
					if !allowed {
						state, why = StSkipped, "Dependency "+e.Parent+" did not select the "+branch+" branch."
						break
					}
				}
			}
			if state == StEligible {
				hits := []string{}
				if s.Mode == ModeOnFinding {
					for _, e := range parents {
						for _, f := range fs[e.Parent] {
							if s.Trigger != nil && s.Trigger.matches(f) {
								hits = append(hits, f.ID)
							}
						}
					}
					if len(hits) == 0 {
						state, why = StSkipped, "No parent finding matched the trigger."
					}
				}
				if state == StEligible {
					sort.Strings(hits)
					res.Launch = append(res.Launch, Launch{Step: key, Triggers: hits})
				}
			}
		}
		res.Status[key] = state
		res.Reasons[key] = why
		return state
	}
	res.Finished = true
	for _, s := range steps {
		if !terminal(eval(s.Key)) {
			res.Finished = false
		}
	}
	return res
}

func strictExecStatus(execs []Exec) string {
	state := execStatus(execs)
	if state == StDone {
		for _, x := range execs {
			if x.Outcome != "success" {
				return StFailed
			}
		}
	}
	return state
}

func parentOutcome(xs []Exec, fallback string) string {
	for _, outcome := range []string{"error", "timeout", "canceled", "skipped", "success"} {
		for _, x := range xs {
			if x.Outcome == outcome {
				return outcome
			}
		}
	}
	return fallback
}
