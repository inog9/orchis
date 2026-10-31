package chain

import (
	"encoding/json"
	"reflect"
	"testing"
	"testing/quick"
)

func TestConditionOperatorsAndMissingData(t *testing.T) {
	found := []Found{{ID: "p", Type: "open_port", Value: "tcp/80", Severity: "info"}, {ID: "v", Type: "vulnerability", Value: "cve", Severity: "high"}}
	cases := []struct {
		raw  string
		want bool
	}{
		{`{"op":"EQ","field":"finding_count","value":2}`, true}, {`{"op":"NEQ","field":"finding_count","value":2}`, false},
		{`{"op":"GT","field":"finding_count","value":1}`, true}, {`{"op":"GTE","field":"finding_count","value":2}`, true},
		{`{"op":"LT","field":"finding_count","value":3}`, true}, {`{"op":"LTE","field":"finding_count","value":1}`, false},
		{`{"op":"IN","field":"open_ports","value":[80,443]}`, true}, {`{"op":"NIN","field":"finding_types","value":["text"]}`, true},
		{`{"op":"IS_NOT_NULL","field":"max_severity"}`, true}, {`{"op":"IS_NULL","field":"max_severity"}`, false},
		{`{"op":"EQ","field":"outcome","value":"success"}`, true}, {`{"op":"DEPEND_ON"}`, true},
		{`{"op":"AND","args":[{"op":"DEPEND_ON"},{"op":"GT","field":"finding_count","value":0}]}`, true},
		{`{"op":"OR","args":[{"op":"EQ","field":"finding_count","value":0},{"op":"DEPEND_ON"}]}`, true},
	}
	for _, tc := range cases {
		var c Condition
		if e := json.Unmarshal([]byte(tc.raw), &c); e != nil {
			t.Fatal(e)
		}
		if e := c.Validate(); e != nil {
			t.Fatal(e)
		}
		if got := c.Evaluate(StDone, found); got != tc.want {
			t.Errorf("%s got %v", tc.raw, got)
		}
	}
	c := Condition{Op: "IS_NULL", Field: "max_severity"}
	if !c.Evaluate(StDone, nil) {
		t.Fatal("missing severity is not null")
	}
	for _, c := range []Condition{{Op: "EVAL", Field: "outcome"}, {Op: "EQ", Field: "arbitrary.path"}, {Op: "IN", Field: "finding_types", Value: "text"}, {Op: "AND"}} {
		if c.Validate() == nil {
			t.Errorf("invalid condition accepted %+v", c)
		}
	}
}
func TestAdvancedElseFailureJoinAndDeterminism(t *testing.T) {
	steps := []Step{s("scan", ModeAfterParents), s("yes", ModeAfterParents), s("no", ModeAfterParents), s("failed", ModeAfterParents)}
	c := &Condition{Op: "GT", Field: "finding_count", Value: float64(0)}
	edges := []Edge{{Parent: "scan", Child: "yes", Condition: c}, {Parent: "scan", Child: "no", Condition: c, Branch: "else"}, {Parent: "scan", Child: "failed", Branch: "failure"}}
	for _, tc := range []struct {
		outcome string
		found   []Found
		want    string
	}{{"success", []Found{{Step: "scan", Type: "open_port", Value: "tcp/80"}}, "yes"}, {"success", nil, "no"}, {"timeout", nil, "failed"}} {
		res := Evaluate(steps, edges, []Exec{ex("scan", "ended", tc.outcome)}, tc.found)
		if len(res.Launch) != 1 || res.Launch[0].Step != tc.want {
			t.Fatalf("%s: %+v", tc.want, res)
		}
		if !reflect.DeepEqual(res, Evaluate(steps, edges, []Exec{ex("scan", "ended", tc.outcome)}, tc.found)) {
			t.Fatal("not deterministic")
		}
	}
	edges = append(edges, Edge{Parent: "failed", Child: "yes"})
	res := Evaluate(steps, edges, []Exec{ex("scan", "ended", "success"), ex("failed", "running", "")}, nil)
	if res.Status["yes"] != StWaiting {
		t.Fatal("join did not wait for every parent")
	}
}
func TestConditionCountMonotonicProperty(t *testing.T) {
	if err := quick.Check(func(count uint8) bool {
		found := make([]Found, int(count))
		for i := range found {
			found[i] = Found{Type: "text"}
		}
		c := Condition{Op: "GTE", Field: "finding_count", Value: float64(count)}
		return c.Evaluate(StDone, found) && c.Evaluate(StDone, append(found, Found{Type: "text"}))
	}, nil); err != nil {
		t.Fatal(err)
	}
}
