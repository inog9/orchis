package chain

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func okType(t string) bool { return t == "open_port" || t == "vulnerability" || t == "service" }

func s(key, mode string) Step { return Step{Key: key, Enabled: true, Mode: mode} }
func onFinding(key string, tr Trigger) Step {
	return Step{Key: key, Enabled: true, Mode: ModeOnFinding, Trigger: &tr}
}

func hasErr(errs []FieldError, field, contains string) bool {
	for _, e := range errs {
		if strings.HasPrefix(e.Field, field) && strings.Contains(e.Message, contains) {
			return true
		}
	}
	return false
}

func TestValidateRejectsBadGraphs(t *testing.T) {
	cases := []struct {
		name       string
		steps      []Step
		edges      []Edge
		field, msg string
	}{
		{"kunci ganda", []Step{s("a", ModeAfterParents), s("a", ModeAfterParents)}, nil, "steps[1].key", "twice"},
		{"kunci buruk", []Step{s("A B", ModeAfterParents)}, nil, "steps[0].key", "lowercase"},
		{"siklus", []Step{s("a", ModeAfterParents), s("b", ModeAfterParents), s("c", ModeAfterParents)},
			[]Edge{{Parent: "a", Child: "b"}, {Parent: "b", Child: "c"}, {Parent: "c", Child: "a"}}, "edges", "loop"},
		{"self loop", []Step{s("a", ModeAfterParents)}, []Edge{{Parent: "a", Child: "a"}}, "edges[0]", "itself"},
		{"tepi ke langkah tak ada", []Step{s("a", ModeAfterParents)}, []Edge{{Parent: "a", Child: "zzz"}}, "edges[0]", "does not exist"},
		{"tepi ganda", []Step{s("a", ModeAfterParents), s("b", ModeAfterParents)}, []Edge{{Parent: "a", Child: "b"}, {Parent: "a", Child: "b"}}, "edges[1]", "Duplicate"},
		{"on_finding tanpa aturan", []Step{s("a", ModeAfterParents), {Key: "b", Enabled: true, Mode: ModeOnFinding}}, []Edge{{Parent: "a", Child: "b"}}, "steps[1].trigger", "Choose"},
		{"on_finding tanpa induk", []Step{onFinding("a", Trigger{Type: "open_port"})}, nil, "steps[0].trigger_mode", "parent"},
		{"jenis finding tak dikenal", []Step{s("a", ModeAfterParents), onFinding("b", Trigger{Type: "bogus"})}, []Edge{{Parent: "a", Child: "b"}}, "steps[1].trigger", "Unknown finding type"},
		{"severity tak dikenal", []Step{s("a", ModeAfterParents), onFinding("b", Trigger{Type: "open_port", MinSeverity: "urgent"})}, []Edge{{Parent: "a", Child: "b"}}, "steps[1].trigger", "severity"},
		{"after_parents dengan aturan", []Step{{Key: "a", Enabled: true, Mode: ModeAfterParents, Trigger: &Trigger{Type: "open_port"}}}, nil, "steps[0].trigger", "Only steps"},
		{"manual belum ada", []Step{s("a", "manual")}, nil, "steps[0].trigger_mode", "not available"},
		{"mode kosong", []Step{s("a", "")}, nil, "steps[0].trigger_mode", "Choose"},
		{"delay negatif", []Step{{Key: "a", Enabled: true, Mode: ModeAfterParents, DelaySeconds: -1}}, nil, "steps[0].delay_seconds", "between"},
	}
	for _, c := range cases {
		errs := Validate(c.steps, c.edges, okType)
		if !hasErr(errs, c.field, c.msg) {
			t.Errorf("%s: harus ada galat %q ~ %q, dapat %+v", c.name, c.field, c.msg, errs)
		}
	}
	many := make([]Step, MaxSteps+1)
	for i := range many {
		many[i] = s(fmt.Sprintf("s%d", i), ModeAfterParents)
	}
	if errs := Validate(many, nil, okType); !hasErr(errs, "steps", "at most") {
		t.Errorf("lebih dari %d langkah harus ditolak: %+v", MaxSteps, errs)
	}
}

func TestValidateAcceptsDiamondAndEmpty(t *testing.T) {
	steps := []Step{s("a", ModeAfterParents), s("b", ModeAfterParents), s("c", ModeAfterParents), s("d", ModeAfterParents)}
	edges := []Edge{{Parent: "a", Child: "b"}, {Parent: "a", Child: "c"}, {Parent: "b", Child: "d"}, {Parent: "c", Child: "d"}}
	if errs := Validate(steps, edges, okType); len(errs) != 0 {
		t.Fatalf("berlian harus valid: %+v", errs)
	}
	if errs := Validate(nil, nil, okType); len(errs) != 0 {
		t.Fatalf("chain kosong (draf) valid: %+v", errs)
	}
}

func ex(step, state, outcome string) Exec { return Exec{Step: step, State: state, Outcome: outcome} }

func launched(r Result) []string {
	var out []string
	for _, l := range r.Launch {
		out = append(out, l.Step)
	}
	return out
}

func TestEvaluateRootsLaunchImmediatelyAndChildrenWait(t *testing.T) {
	steps := []Step{s("scan", ModeAfterParents), s("next", ModeAfterParents)}
	edges := []Edge{{Parent: "scan", Child: "next"}}
	r := Evaluate(steps, edges, nil, nil)
	if !reflect.DeepEqual(launched(r), []string{"scan"}) || r.Status["next"] != StWaiting || r.Finished {
		t.Fatalf("awal: %+v", r)
	}
	r = Evaluate(steps, edges, []Exec{ex("scan", "running", "")}, nil)
	if r.Status["scan"] != StRunning || r.Status["next"] != StWaiting || len(r.Launch) != 0 {
		t.Fatalf("saat berjalan: %+v", r)
	}
	r = Evaluate(steps, edges, []Exec{ex("scan", "ended", "success")}, nil)
	if r.Status["scan"] != StDone || !reflect.DeepEqual(launched(r), []string{"next"}) {
		t.Fatalf("setelah induk selesai anak harus diluncurkan: %+v", r)
	}
	r = Evaluate(steps, edges, []Exec{ex("scan", "ended", "success"), ex("next", "ended", "success")}, nil)
	if !r.Finished || r.Status["next"] != StDone {
		t.Fatalf("semua selesai: %+v", r)
	}
}

func TestJoinWaitsForAllParents(t *testing.T) {
	steps := []Step{s("a", ModeAfterParents), s("b", ModeAfterParents), s("join", ModeAfterParents)}
	edges := []Edge{{Parent: "a", Child: "join"}, {Parent: "b", Child: "join"}}
	r := Evaluate(steps, edges, []Exec{ex("a", "ended", "success"), ex("b", "running", "")}, nil)
	if r.Status["join"] != StWaiting || len(r.Launch) != 0 {
		t.Fatalf("join harus menunggu SEMUA induk: %+v", r)
	}
	r = Evaluate(steps, edges, []Exec{ex("a", "ended", "success"), ex("b", "ended", "success")}, nil)
	if !reflect.DeepEqual(launched(r), []string{"join"}) {
		t.Fatalf("join jalan setelah semua induk done: %+v", r)
	}
}

func TestFailedParentBlocksAfterParentsButNotOnFinding(t *testing.T) {
	steps := []Step{s("a", ModeAfterParents), s("child", ModeAfterParents), onFinding("alt", Trigger{Type: "open_port"})}
	edges := []Edge{{Parent: "a", Child: "child"}, {Parent: "a", Child: "alt"}}
	r := Evaluate(steps, edges, []Exec{ex("a", "ended", "error")}, nil)
	if r.Status["a"] != StFailed || r.Status["child"] != StBlocked {
		t.Fatalf("anak after_parents harus blocked bila induk gagal: %+v", r)
	}
	if r.Status["alt"] != StSkipped {
		t.Fatalf("on_finding tanpa temuan induk dilewati (bukan blocked): %+v", r)
	}
	if !r.Finished || len(r.Launch) != 0 {
		t.Fatalf("tidak ada lagi yang bisa jalan -> selesai: %+v", r)
	}
}

func TestOnFindingMatchesTypeValueAndSeverity(t *testing.T) {
	tr := Trigger{Type: "open_port", Values: []string{"TCP/80", "tcp/443"}, MinSeverity: "info"}
	steps := []Step{s("scan", ModeAfterParents), onFinding("web", tr)}
	edges := []Edge{{Parent: "scan", Child: "web"}}
	done := []Exec{ex("scan", "ended", "success")}
	cases := []struct {
		name  string
		found []Found
		want  string
	}{
		{"cocok persis (huruf besar tidak jadi soal)", []Found{{ID: "f1", Step: "scan", Type: "open_port", Value: "tcp/80"}}, "launch"},
		{"cocok kedua", []Found{{ID: "f1", Step: "scan", Type: "open_port", Value: "tcp/443"}}, "launch"},
		{"port lain", []Found{{ID: "f1", Step: "scan", Type: "open_port", Value: "tcp/22"}}, StSkipped},
		{"awalan saja tidak cukup", []Found{{ID: "f1", Step: "scan", Type: "open_port", Value: "tcp/8080"}}, StSkipped},
		{"jenis lain", []Found{{ID: "f1", Step: "scan", Type: "vulnerability", Value: "tcp/80"}}, StSkipped},
		{"temuan dari langkah lain tidak dihitung", []Found{{ID: "f1", Step: "other", Type: "open_port", Value: "tcp/80"}}, StSkipped},
		{"tanpa temuan", nil, StSkipped},
	}
	for _, c := range cases {
		r := Evaluate(steps, edges, done, c.found)
		if c.want == "launch" {
			if len(r.Launch) != 1 || r.Launch[0].Step != "web" || len(r.Launch[0].Triggers) != 1 || r.Launch[0].Triggers[0] != "f1" {
				t.Errorf("%s: harus diluncurkan dengan pemicu f1: %+v", c.name, r)
			}
		} else if r.Status["web"] != c.want || len(r.Launch) != 0 {
			t.Errorf("%s: status %q, harapan %q (%+v)", c.name, r.Status["web"], c.want, r)
		}
	}
	// severity minimum
	steps[1] = onFinding("web", Trigger{Type: "vulnerability", MinSeverity: "high"})
	low := Evaluate(steps, edges, done, []Found{{ID: "f", Step: "scan", Type: "vulnerability", Value: "x", Severity: "medium"}})
	high := Evaluate(steps, edges, done, []Found{{ID: "f", Step: "scan", Type: "vulnerability", Value: "x", Severity: "critical"}})
	if low.Status["web"] != StSkipped || len(high.Launch) != 1 {
		t.Fatalf("severity minimum: low=%+v high=%+v", low, high)
	}
}

func TestOnFindingWaitsUntilAllParentsFinishedThenUsesAllTheirFindings(t *testing.T) {
	steps := []Step{s("p1", ModeAfterParents), s("p2", ModeAfterParents), onFinding("x", Trigger{Type: "open_port"})}
	edges := []Edge{{Parent: "p1", Child: "x"}, {Parent: "p2", Child: "x"}}
	found := []Found{{ID: "f", Step: "p1", Type: "open_port", Value: "tcp/1"}}
	r := Evaluate(steps, edges, []Exec{ex("p1", "ended", "success"), ex("p2", "running", "")}, found)
	if r.Status["x"] != StWaiting || len(r.Launch) != 0 {
		t.Fatalf("harus menunggu semua induk walau trigger sudah terpenuhi: %+v", r)
	}
	r = Evaluate(steps, edges, []Exec{ex("p1", "ended", "success"), ex("p2", "ended", "success")}, found)
	if len(r.Launch) != 1 {
		t.Fatalf("setelah semua induk selesai: %+v", r)
	}
}

func TestSkippedAndDisabledPropagation(t *testing.T) {
	// langkah dinonaktifkan dianggap lolos; langkah dilewati karena trigger membuat anak after_parents blocked
	off := Step{Key: "off", Enabled: false, Mode: ModeAfterParents}
	steps := []Step{s("a", ModeAfterParents), off, s("c", ModeAfterParents)}
	edges := []Edge{{Parent: "a", Child: "off"}, {Parent: "off", Child: "c"}}
	r := Evaluate(steps, edges, []Exec{ex("a", "ended", "success")}, nil)
	if r.Status["off"] != StSkipped || !reflect.DeepEqual(launched(r), []string{"c"}) {
		t.Fatalf("anak dari induk nonaktif tetap jalan: %+v", r)
	}
	steps2 := []Step{s("a", ModeAfterParents), onFinding("b", Trigger{Type: "open_port"}), s("c", ModeAfterParents)}
	edges2 := []Edge{{Parent: "a", Child: "b"}, {Parent: "b", Child: "c"}}
	r = Evaluate(steps2, edges2, []Exec{ex("a", "ended", "success")}, nil)
	if r.Status["b"] != StSkipped || r.Status["c"] != StBlocked || !r.Finished {
		t.Fatalf("anak dari langkah yang dilewati karena aturan harus blocked: %+v", r)
	}
}

func TestMultipleExecutionsPerStep(t *testing.T) {
	// satu langkah x tiga aset: running selama ada yang berjalan; done bila ada yang sukses; failed bila tak ada
	steps := []Step{s("scan", ModeAfterParents)}
	cases := []struct {
		execs []Exec
		want  string
	}{
		{[]Exec{ex("scan", "ended", "success"), ex("scan", "running", ""), ex("scan", "ended", "error")}, StRunning},
		{[]Exec{ex("scan", "ended", "success"), ex("scan", "ready", ""), ex("scan", "ended", "error")}, StQueued},
		{[]Exec{ex("scan", "ended", "success"), ex("scan", "ended", "error")}, StDone},
		{[]Exec{ex("scan", "ended", "error"), ex("scan", "ended", "timeout")}, StFailed},
		{[]Exec{ex("scan", "ended", "canceled")}, StFailed},
	}
	for _, c := range cases {
		if got := Evaluate(steps, nil, c.execs, nil).Status["scan"]; got != c.want {
			t.Errorf("%+v -> %s, harapan %s", c.execs, got, c.want)
		}
	}
}

func TestEvaluateIsDeterministicAndSurvivesCycles(t *testing.T) {
	steps := []Step{s("a", ModeAfterParents), s("b", ModeAfterParents)}
	r := Evaluate(steps, []Edge{{Parent: "a", Child: "b"}, {Parent: "b", Child: "a"}}, nil, nil) // seharusnya ditolak Validate; evaluator tidak boleh menggantung
	if r.Finished && len(r.Launch) > 0 {
		t.Fatalf("hasil tak konsisten: %+v", r)
	}
	a := Evaluate(steps, nil, nil, nil)
	b := Evaluate(steps, nil, nil, nil)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("harus deterministik")
	}
}
