package server_test

import (
	"orchis-go/internal/osquery"
	"sync"
	"testing"
)

func (f *agentFixture) collectionChoice(t *testing.T, org, key string, enabled bool) map[string]any {
	t.Helper()
	var data struct {
		Items []struct {
			Key     string
			Version int64
		}
	}
	r := f.a.do("GET", "/api/v1/organizations/"+org+"/osquery", nil, f.admin)
	if r.Status != 200 {
		t.Fatalf("read choices: %d %s", r.Status, r.Body)
	}
	r.JSON(&data)
	versions := map[string]int64{}
	for _, item := range data.Items {
		versions[item.Key] = item.Version
	}
	body := map[string]any{"enabled": enabled, "consent": osquery.Consent().ConsentRef}
	if pack := osquery.FindPack(key); pack != nil {
		expected := map[string]int64{}
		for _, k := range pack.RunnableKeys() {
			expected[k] = versions[k]
		}
		body["versions"] = expected
	} else {
		body["version"] = versions[key]
	}
	return body
}

func TestOsquerySettingCASRejectsStaleApproval(t *testing.T) {
	f := newAgentFixture(t)
	_, asset := f.authorizedAgent(t, "cas")
	org := f.joinOrg(t, asset)
	path := "/api/v1/organizations/" + org + "/osquery/osquery.users"
	put := func(version any, enabled bool) int {
		body := map[string]any{"enabled": enabled, "consent": osquery.Consent().ConsentRef}
		if version != nil {
			body["version"] = version
		}
		return f.a.do("PUT", path, body, f.admin).Status
	}
	for _, v := range []any{nil, -1} {
		if got := put(v, true); got != 422 {
			t.Fatalf("missing/negative version accepted: %d", got)
		}
	}
	if got := put(0, true); got != 204 {
		t.Fatalf("initial approval: %d", got)
	}
	if got := put(0, false); got != 409 {
		t.Fatalf("stale update accepted: %d", got)
	}
	if got := put(1, false); got != 204 {
		t.Fatalf("revoke: %d", got)
	}
	if got := put(1, true); got != 409 {
		t.Fatalf("stale approval overwrote revocation: %d", got)
	}
	body := f.collectionChoice(t, org, "osquery.users", true)
	if body["version"] != int64(2) {
		t.Fatalf("rejected writes changed version: %+v", body)
	}
	if n := rowCount(t, f, `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1 AND enabled`, org); n != 0 {
		t.Fatal("revocation lost")
	}
	if n := rowCount(t, f, `SELECT count(*) FROM audit_log WHERE action='osquery.setting' AND resource_id=$1`, org); n != 2 {
		t.Fatalf("rejected writes audited as changes: %d", n)
	}
	// Both callers reviewed version 2; serialization must permit exactly one update.
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- put(2, true) }()
	}
	wg.Wait()
	close(results)
	counts := map[int]int{}
	for status := range results {
		counts[status]++
	}
	if counts[204] != 1 || counts[409] != 1 {
		t.Fatalf("concurrent overwrite: %v", counts)
	}
}

func TestOsqueryPackCASIsAtomicAndRequiresEveryVersion(t *testing.T) {
	f := newAgentFixture(t)
	_, asset := f.authorizedAgent(t, "pack-cas")
	org := f.joinOrg(t, asset)
	path := "/api/v1/organizations/" + org + "/osquery-packs/it-compliance"
	keys := osquery.FindPack("it-compliance").RunnableKeys()
	stale := f.collectionChoice(t, org, "it-compliance", true)
	for _, kind := range []string{"missing", "negative", "extra", "wrong-key"} {
		body := f.collectionChoice(t, org, "it-compliance", true)
		vs := body["versions"].(map[string]int64)
		switch kind {
		case "missing":
			delete(vs, keys[0])
		case "negative":
			vs[keys[0]] = -1
		case "extra":
			vs["unknown"] = 0
		case "wrong-key":
			delete(vs, keys[0])
			vs["unknown"] = 0
		}
		if r := f.a.do("PUT", path, body, f.admin); r.Status != 422 {
			t.Fatalf("invalid %s versions accepted: %d", kind, r.Status)
		}
	}
	single := "/api/v1/organizations/" + org + "/osquery/" + keys[len(keys)-1]
	if r := f.a.do("PUT", single, f.collectionChoice(t, org, keys[len(keys)-1], false), f.admin); r.Status != 204 {
		t.Fatalf("single revoke: %d", r.Status)
	}
	if r := f.a.do("PUT", path, stale, f.admin); r.Status != 409 {
		t.Fatalf("stale pack accepted: %d", r.Status)
	}
	if n := rowCount(t, f, `SELECT count(*) FROM osquery_org_settings WHERE organization_id=$1`, org); n != 1 {
		t.Fatalf("pack partially wrote: %d", n)
	}
	if n := rowCount(t, f, `SELECT count(*) FROM audit_log WHERE action='osquery.pack' AND resource_id=$1`, org); n != 0 {
		t.Fatal("rejected pack recorded as success")
	}
	if r := f.a.do("PUT", path, f.collectionChoice(t, org, "it-compliance", true), f.admin); r.Status != 204 {
		t.Fatalf("current pack: %d %s", r.Status, r.Body)
	}
	if r := f.a.do("PUT", single, map[string]any{"enabled": false, "version": 1}, f.admin); r.Status != 409 {
		t.Fatalf("pack did not invalidate single choice: %d", r.Status)
	}
}

func TestOsquerySettingCASAndBulkShareTheSameVersions(t *testing.T) {
	f := newAgentFixture(t)
	_, asset := f.authorizedAgent(t, "bulk-cas")
	org := f.joinOrg(t, asset)
	base := "/api/v1/organizations/" + org
	single := f.collectionChoice(t, org, "osquery.users", true)
	pack := f.collectionChoice(t, org, "it-compliance", true)
	var data struct {
		Items []struct {
			Key     string
			Version int64
		}
	}
	f.a.do("GET", base+"/osquery", nil, f.admin).JSON(&data)
	versions := map[string]int64{}
	for _, it := range data.Items {
		versions[it.Key] = it.Version
	}
	bulk := map[string]any{"platform": "linux", "keys": []string{}, "versions": versions, "consent": osquery.Consent().ConsentRef}
	if r := f.a.do("PUT", base+"/endpoint-collection", bulk, f.admin); r.Status != 204 {
		t.Fatalf("bulk consent: %d %s", r.Status, r.Body)
	}
	if r := f.a.do("PUT", base+"/osquery/osquery.users", single, f.admin); r.Status != 409 {
		t.Fatalf("single ignored bulk revocation: %d", r.Status)
	}
	if r := f.a.do("PUT", base+"/osquery-packs/it-compliance", pack, f.admin); r.Status != 409 {
		t.Fatalf("pack ignored bulk revocation: %d", r.Status)
	}
	if r := f.a.do("PUT", base+"/osquery/osquery.users", f.collectionChoice(t, org, "osquery.users", true), f.admin); r.Status != 204 {
		t.Fatalf("current single: %d", r.Status)
	}
	for key := range versions {
		versions[key] = 1
	}
	if r := f.a.do("PUT", base+"/endpoint-collection", bulk, f.admin); r.Status != 409 {
		t.Fatalf("bulk overwrote single choice: %d", r.Status)
	}
}
