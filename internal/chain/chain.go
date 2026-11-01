// Package chain: aturan murni (tanpa database) untuk graf chain: validasi DAG dan evaluasi langkah mana yang boleh jalan.
// Dipisah dari penyimpanan agar semua aturan join/trigger bisa diuji tuntas dan deterministik.
package chain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	MaxSteps = 100
	MaxEdges = 400

	ModeAfterParents = "after_parents"
	ModeOnFinding    = "on_finding"
)

var keyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

var severityRank = map[string]int{"info": 0, "low": 1, "medium": 2, "high": 3, "critical": 4}

// Trigger adalah aturan "On finding": langkah jalan bila salah satu induknya menemukan temuan yang cocok.
type Trigger struct {
	Type        string   `json:"type"`                   // jenis finding: open_port | service | vulnerability ...
	Values      []string `json:"values,omitempty"`       // nilai persis (setelah normalisasi huruf kecil); kosong = nilai apa pun
	MinSeverity string   `json:"min_severity,omitempty"` // info..critical
}

// Step adalah satu langkah chain pada template.
type Step struct {
	Key          string
	Enabled      bool
	Mode         string
	Trigger      *Trigger
	DelaySeconds int
}

type Edge struct {
	Condition *Condition `json:"condition,omitempty"`
	Branch    string     `json:"branch,omitempty"`
	Mapper    *Mapper    `json:"mapper,omitempty"`
	Parent    string     `json:"parent"`
	Child     string     `json:"child"`
}

// FieldError menunjuk bagian graf yang salah (untuk 422 yang bisa ditampilkan di UI).
type FieldError struct{ Field, Message string }

func (e FieldError) Error() string { return e.Field + ": " + e.Message }

// Validate memeriksa graf template. Mengembalikan semua masalah sekaligus agar pengguna tidak memperbaikinya satu-satu.
func Validate(steps []Step, edges []Edge, validType func(string) bool) []FieldError {
	var errs []FieldError
	add := func(f, m string, a ...any) { errs = append(errs, FieldError{f, fmt.Sprintf(m, a...)}) }
	if len(steps) > MaxSteps {
		add("steps", "A chain can have at most %d steps.", MaxSteps)
		return errs
	}
	if len(edges) > MaxEdges {
		add("edges", "A chain can have at most %d dependencies.", MaxEdges)
		return errs
	}
	idx := map[string]int{}
	for i, s := range steps {
		f := fmt.Sprintf("steps[%d]", i)
		switch {
		case !keyRe.MatchString(s.Key):
			add(f+".key", "Use 1 to 40 lowercase letters, digits, - or _.")
		case idx[s.Key] != 0:
			add(f+".key", "Step key %q is used twice.", s.Key)
		default:
			idx[s.Key] = i + 1
		}
		if s.DelaySeconds < 0 || s.DelaySeconds > 86400 {
			add(f+".delay_seconds", "Delay must be between 0 and 86400 seconds.")
		}
		switch s.Mode {
		case ModeAfterParents:
			if s.Trigger != nil {
				add(f+".trigger", "Only steps triggered by a finding take a rule.")
			}
		case ModeOnFinding:
			if s.Trigger == nil {
				add(f+".trigger", "Choose which finding starts this step.")
			} else {
				add2 := func(m string) { add(f+".trigger", "%s", m) }
				if validType != nil && !validType(s.Trigger.Type) {
					add2("Unknown finding type.")
				}
				if s.Trigger.MinSeverity != "" {
					if _, ok := severityRank[s.Trigger.MinSeverity]; !ok {
						add2("Unknown minimum severity.")
					}
				}
				if len(s.Trigger.Values) > 20 {
					add2("At most 20 values.")
				}
				for _, v := range s.Trigger.Values {
					if v == "" || len(v) > 200 {
						add2("Values must be 1 to 200 characters.")
						break
					}
				}
			}
		case "manual":
			add(f+".trigger_mode", "Manual steps are not available yet.")
		default:
			add(f+".trigger_mode", "Choose after_parents or on_finding.")
		}
	}
	parents := map[string][]string{}
	seen := map[[2]string]bool{}
	for i, e := range edges {
		f := fmt.Sprintf("edges[%d]", i)
		if e.Condition != nil {
			if err := e.Condition.Validate(); err != nil {
				add(f+".condition", "%s", err.Error())
			}
		}
		if e.Branch != "" && e.Branch != "match" && e.Branch != "else" && e.Branch != "failure" {
			add(f+".branch", "Choose match, else or failure.")
		}
		if e.Branch == "else" && e.Condition == nil {
			add(f+".condition", "An else branch needs a condition.")
		}
		if e.Mapper != nil && !validMapper(*e.Mapper) {
			add(f+".mapper", "Choose an allowed mapper source and destination.")
		}

		if idx[e.Parent] == 0 || idx[e.Child] == 0 {
			add(f, "A dependency points to a step that does not exist.")
			continue
		}
		if e.Parent == e.Child {
			add(f, "A step cannot depend on itself.")
			continue
		}
		if seen[[2]string{e.Parent, e.Child}] {
			add(f, "Duplicate dependency.")
			continue
		}
		seen[[2]string{e.Parent, e.Child}] = true
		parents[e.Child] = append(parents[e.Child], e.Parent)
	}
	for i, s := range steps {
		if s.Mode == ModeOnFinding && len(parents[s.Key]) == 0 && idx[s.Key] != 0 {
			add(fmt.Sprintf("steps[%d].trigger_mode", i), "A step triggered by a finding needs at least one parent step to take findings from.")
		}
	}
	if len(errs) == 0 {
		if cyc := findCycle(steps, edges); len(cyc) > 0 {
			add("edges", "Dependencies form a loop: %s.", strings.Join(cyc, " → "))
		}
	}
	return errs
}

// findCycle mengembalikan salah satu simpul pada siklus (urutan alfabet agar pesan stabil), atau nil bila DAG.
func findCycle(steps []Step, edges []Edge) []string {
	indeg := map[string]int{}
	out := map[string][]string{}
	for _, s := range steps {
		indeg[s.Key] = 0
	}
	for _, e := range edges {
		indeg[e.Child]++
		out[e.Parent] = append(out[e.Parent], e.Child)
	}
	var queue []string
	for k, d := range indeg {
		if d == 0 {
			queue = append(queue, k)
		}
	}
	removed := 0
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		removed++
		for _, c := range out[k] {
			if indeg[c]--; indeg[c] == 0 {
				queue = append(queue, c)
			}
		}
	}
	if removed == len(steps) {
		return nil
	}
	var left []string
	for k, d := range indeg {
		if d > 0 {
			left = append(left, k)
		}
	}
	sort.Strings(left)
	return left
}

// ---------- evaluasi ----------

const (
	StWaiting  = "waiting" // ada induk yang belum selesai
	StQueued   = "queued"
	StRunning  = "running"
	StDone     = "done"
	StFailed   = "failed"
	StSkipped  = "skipped"  // dinonaktifkan, atau aturan trigger tidak terpenuhi
	StBlocked  = "blocked"  // induk gagal/dilewati sehingga langkah ini tidak pernah tercapai
	StEligible = "eligible" // bisa diluncurkan; tidak pernah tersimpan, hanya hasil hitung
)

// Exec adalah ringkasan satu eksekusi milik sebuah langkah.
type Exec struct {
	Asset   string
	Step    string
	State   string // ready | running | ended
	Outcome string // success | error | canceled | timeout | skipped (bila ended)
}

// Found adalah satu finding yang ditemukan oleh eksekusi sebuah langkah (untuk mengevaluasi trigger anak-anaknya).
type Found struct {
	Target   string
	Asset    string
	ID       string
	Step     string
	Type     string
	Value    string
	Severity string
}

type Launch struct {
	Step     string
	Triggers []string // ID finding yang memicu (kosong untuk after_parents)
}

type Result struct {
	Reasons  map[string]string `json:"reasons,omitempty"`
	Status   map[string]string
	Launch   []Launch // langkah yang kini boleh dijalankan dan belum punya eksekusi
	Finished bool     // semua langkah berstatus akhir dan tidak ada yang perlu diluncurkan
}

func terminal(s string) bool { return s == StDone || s == StFailed || s == StSkipped || s == StBlocked }

// Evaluate menghitung status setiap langkah dari graf, eksekusi, dan finding. Fungsi murni: hasil yang sama untuk masukan yang sama.
//
// Aturan (lihat ARSITEKTUR 9.3):
//   - join menunggu SEMUA induk berstatus akhir;
//   - after_parents: jalan bila semua induk done (induk yang dinonaktifkan dianggap lolos); bila ada induk failed/blocked/
//     dilewati karena aturan -> blocked;
//   - on_finding: setelah semua induk berstatus akhir, jalan bila ada finding induk yang cocok aturan, kalau tidak skipped.
func Evaluate(steps []Step, edges []Edge, execs []Exec, found []Found) Result {
	if HasAdvanced(edges) {
		return evaluateAdvanced(steps, edges, execs, found)
	}
	byKey := map[string]Step{}
	order := make([]string, 0, len(steps))
	for _, s := range steps {
		byKey[s.Key] = s
		order = append(order, s.Key)
	}
	parents := map[string][]string{}
	for _, e := range edges {
		parents[e.Child] = append(parents[e.Child], e.Parent)
	}
	execsOf := map[string][]Exec{}
	for _, e := range execs {
		execsOf[e.Step] = append(execsOf[e.Step], e)
	}
	foundBy := map[string][]Found{}
	for _, f := range found {
		foundBy[f.Step] = append(foundBy[f.Step], f)
	}

	res := Result{Status: map[string]string{}}
	disabled := map[string]bool{}
	var eval func(k string) string
	visiting := map[string]bool{}
	eval = func(k string) string {
		if st, ok := res.Status[k]; ok {
			return st
		}
		if visiting[k] { // graf berisi siklus: seharusnya ditolak Validate; jangan berputar selamanya
			return StBlocked
		}
		visiting[k] = true
		defer delete(visiting, k)
		s := byKey[k]
		var st string
		switch {
		case !s.Enabled:
			disabled[k] = true
			st = StSkipped
		case len(execsOf[k]) > 0:
			st = execStatus(execsOf[k])
		default:
			st = decide(s, parents[k], eval, disabled, foundBy, &res)
		}
		res.Status[k] = st
		return st
	}
	for _, k := range order {
		eval(k)
	}
	res.Finished = len(res.Launch) == 0
	for _, k := range order {
		if !terminal(res.Status[k]) {
			res.Finished = false
		}
	}
	return res
}

func execStatus(es []Exec) string {
	running, ready, success := false, false, false
	for _, e := range es {
		switch {
		case e.State == "running":
			running = true
		case e.State == "ready":
			ready = true
		case e.Outcome == "success":
			success = true
		}
	}
	switch {
	case running:
		return StRunning
	case ready:
		return StQueued
	case success:
		return StDone
	}
	return StFailed
}

func decide(s Step, parents []string, eval func(string) string, disabled map[string]bool, foundBy map[string][]Found, res *Result) string {
	if len(parents) == 0 {
		res.Launch = append(res.Launch, Launch{Step: s.Key})
		return StEligible
	}
	sts := make(map[string]string, len(parents))
	for _, p := range parents {
		st := eval(p)
		if !terminal(st) {
			return StWaiting
		}
		sts[p] = st
	}
	switch s.Mode {
	case ModeOnFinding:
		var hits []string
		for _, p := range parents {
			for _, f := range foundBy[p] {
				if s.Trigger != nil && s.Trigger.matches(f) {
					hits = append(hits, f.ID)
				}
			}
		}
		if len(hits) == 0 {
			return StSkipped
		}
		sort.Strings(hits)
		res.Launch = append(res.Launch, Launch{Step: s.Key, Triggers: hits})
		return StEligible
	default:
		for _, p := range parents {
			if disabled[p] {
				continue
			}
			if sts[p] != StDone {
				return StBlocked
			}
		}
		res.Launch = append(res.Launch, Launch{Step: s.Key})
		return StEligible
	}
}

func (t *Trigger) matches(f Found) bool {
	if f.Type != t.Type {
		return false
	}
	if t.MinSeverity != "" && severityRank[f.Severity] < severityRank[t.MinSeverity] {
		return false
	}
	if len(t.Values) == 0 {
		return true
	}
	v := strings.ToLower(strings.TrimSpace(f.Value))
	for _, want := range t.Values {
		if strings.ToLower(strings.TrimSpace(want)) == v {
			return true
		}
	}
	return false
}

func validMapper(m Mapper) bool {
	if m.Source == "installed_software" {
		return m.Destination == "tags"
	}
	return (m.Source == "open_ports" || m.Source == "web_urls") && (m.Destination == "ports" || m.Destination == "target")
}
