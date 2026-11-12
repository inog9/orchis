package injector

import (
	"context"
	"encoding/json"
	"orchis-go/internal/recommendations"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewRunsWithPrivateHomeAndToolsDenied(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ORCHIS_INJECTOR_TOKEN", "must-not-reach-model")
	if err := os.MkdirAll(filepath.Join(home, ".pi/agent"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".pi/agent/auth.json"), []byte(`{"opencode-go":{"key":"fixture-key"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	script := `#!/usr/bin/env python3
import json,os,sys
config=json.load(open('opencode.json'))
assert config['agents']['recommendations']['permissions']==[{'action':'*','resource':'*','effect':'deny'}]
assert config['agents']['recommendations']['steps']==2
assert os.environ['HOME']!=` + "'" + home + "'" + `
assert os.environ['OPENCODE_API_KEY']=='fixture-key'
assert 'ORCHIS_INJECTOR_TOKEN' not in os.environ
assert 'OBSERVATIONS JSON' in sys.stdin.read()
out={'summary':'Review the recorded issue.','items':[{'finding_id':'f1','priority':'now','reason':'High severity.','action':'Fix the injection boundary.','validation':'Retest it.'}]}
print(json.dumps({'type':'text','part':{'text':json.dumps(out)}}))
`
	binary := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	c := recommendations.Claim{Input: recommendations.Input{Language: "en", Coverage: "finished", Findings: []recommendations.Finding{{ID: "f1", Title: "Injection", Severity: "high"}}}}
	if _, err := runOpenCodeReview(context.Background(), c, binary); err != nil {
		t.Fatal(err)
	}
}
func TestOpenCodeReviewLive(t *testing.T) {
	if os.Getenv("ORCHIS_LIVE_REVIEW") != "1" {
		t.Skip("explicit synthetic OpenCode smoke test")
	}
	c := recommendations.Claim{Input: recommendations.Input{Language: "en", Coverage: "incomplete", Findings: []recommendations.Finding{{ID: "fixture-finding", Title: "SQL injection observed in a fixture login form", Severity: "high"}}}}
	raw, err := runOpenCodeReview(context.Background(), c, "opencode")
	if err != nil {
		t.Fatal(err)
	}
	var out recommendations.Output
	if err = json.Unmarshal(raw, &out); err != nil || len(out.Items) != 1 {
		t.Fatal("invalid live review", err)
	}
	t.Log("OpenCode returned one valid recommendation for a synthetic finding.")
}
