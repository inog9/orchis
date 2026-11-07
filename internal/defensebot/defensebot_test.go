package defensebot

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		eco, a, b string
		want      int
	}{
		{"Ubuntu:24.04:LTS", "3.0.13-0ubuntu3.4", "3.0.13-0ubuntu3.5", -1},
		{"Ubuntu:24.04:LTS", "1:9.6p1-3", "9.6p1-4", 1}, // epoch wins
		{"Debian:12", "1.0~rc1-1", "1.0-1", -1},         // tilde sorts before everything
		{"Debian:12", "2.10-1", "2.9-1", 1},
		{"Debian:12", "6.8.0-45.45", "6.8.0-50.51", -1},
		{"npm", "1.0.9", "1.0.10", -1},
		{"npm", "2.0.0-beta.1", "2.0.0", -1},
		{"PyPI", "2.31.0", "2.31.0", 0},
	}
	for _, c := range cases {
		if got := CompareVersions(c.eco, c.a, c.b); got != c.want {
			t.Errorf("%s %q vs %q = %d, want %d", c.eco, c.a, c.b, got, c.want)
		}
		if got := CompareVersions(c.eco, c.b, c.a); got != -c.want {
			t.Errorf("%s reversed %q vs %q = %d, want %d", c.eco, c.b, c.a, got, -c.want)
		}
	}
}

func TestCVSS31Score(t *testing.T) {
	cases := map[string]float64{
		"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H": 9.8,
		"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H": 10,
		"CVSS:3.1/AV:L/AC:L/PR:L/UI:N/S:U/C:H/I:N/A:N": 5.5,
		"CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N": 6.1,
	}
	for vec, want := range cases {
		got, ok := CVSS31Score(vec)
		if !ok || got != want {
			t.Errorf("%s = %v ok=%v, want %v", vec, got, ok, want)
		}
	}
	if _, ok := CVSS31Score("CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"); ok {
		t.Error("CVSS 4.0 must not be scored by the 3.x calculator")
	}
	if _, ok := CVSS31Score("garbage"); ok {
		t.Error("garbage must not score")
	}
	for score, want := range map[float64]string{9.8: "critical", 7.0: "high", 5.5: "medium", 3.1: "low", 0: "info"} {
		if got := SeverityFromScore(score); got != want {
			t.Errorf("severity(%v) = %s, want %s", score, got, want)
		}
	}
}

func TestParseAdvisorySeverityPrecedence(t *testing.T) {
	ubuntu := `{"id":"UBUNTU-CVE-2024-1","upstream":["CVE-2024-1"],"summary":"s","severity":[{"type":"Ubuntu","score":"negligible"},{"type":"CVSS_V3","score":"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"}],
	 "affected":[{"package":{"name":"openssl","ecosystem":"Ubuntu:24.04:LTS"},"ranges":[{"type":"ECOSYSTEM","events":[{"introduced":"0"},{"fixed":"3.0.13-0ubuntu3.5"}]}],"ecosystem_specific":{"availability":"No subscription required","binaries":[{"binary_name":"x","binary_version":"1"}]}}]}`
	a, err := ParseAdvisory([]byte(ubuntu))
	if err != nil {
		t.Fatal(err)
	}
	if a.Severity != "info" || a.SeveritySource != "distribution priority" || a.PrimaryID() != "CVE-2024-1" {
		t.Fatalf("distribution priority must win over the CVSS vector: %+v", a)
	}
	info := a.Affected[affectedKey("Ubuntu:24.04:LTS", "openssl")]
	if len(info.Fixed) != 1 || info.Fixed[0] != "3.0.13-0ubuntu3.5" {
		t.Fatalf("fixed version lost: %+v", info)
	}
	ghsa := `{"id":"GHSA-aaaa-bbbb-cccc","aliases":["CVE-2025-5"],"database_specific":{"severity":"MODERATE"},"severity":[{"type":"CVSS_V3","score":"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"}],"affected":[{"package":{"name":"x","ecosystem":"npm"},"ranges":[{"type":"SEMVER","events":[{"introduced":"0"},{"fixed":"1.2.3"}]}]}]}`
	g, _ := ParseAdvisory([]byte(ghsa))
	if g.Severity != "medium" || g.SeveritySource != "GitHub advisory rating" || g.CVSS != 9.8 {
		t.Fatalf("GitHub rating must win over the vector: %+v", g)
	}
	cvssOnly, _ := ParseAdvisory([]byte(`{"id":"GHSA-x","aliases":["CVE-2025-6"],"severity":[{"type":"CVSS_V3","score":"CVSS:3.1/AV:L/AC:L/PR:L/UI:N/S:U/C:H/I:N/A:N"}]}`))
	if cvssOnly.Severity != "medium" || cvssOnly.SeveritySource != "CVSS v3 base score" {
		t.Fatalf("vector fallback: %+v", cvssOnly)
	}
	none, _ := ParseAdvisory([]byte(`{"id":"OSV-1","aliases":["CVE-2025-7"]}`))
	if sev, src := none.SeverityFor("k"); sev != "medium" || src != "unrated" {
		t.Fatalf("unrated advisories are treated as medium and marked: %s %s", sev, src)
	}
}

func TestEcosystemFor(t *testing.T) {
	cases := []struct {
		row  map[string]string
		want string
	}{
		{map[string]string{"platform": "ubuntu", "major": "24", "minor": "4", "version": "24.04.1 LTS (Noble Numbat)"}, "Ubuntu:24.04:LTS"},
		{map[string]string{"platform": "ubuntu", "major": "24", "minor": "10", "version": "24.10 (Oracular Oriole)"}, "Ubuntu:24.10"},
		{map[string]string{"platform": "debian", "major": "12", "minor": "5"}, "Debian:12"},
		{map[string]string{"platform": "rhel", "major": "9"}, ""},
		{map[string]string{"platform": "darwin", "major": "14"}, ""},
	}
	for _, c := range cases {
		if got := ecosystemFor(c.row); got != c.want {
			t.Errorf("%v = %q, want %q", c.row, got, c.want)
		}
	}
}

// ---- fakes ----

type fakeOSV struct {
	srv      *httptest.Server
	vulns    map[string][]VulnRef // "eco|name|version" -> refs
	records  map[string]string
	pages    map[string][]VulnRef // token -> refs
	requests atomic.Int32
	down     atomic.Bool
}

func newFakeOSV(t *testing.T) *fakeOSV {
	f := &fakeOSV{vulns: map[string][]VulnRef{}, records: map[string]string{}, pages: map[string][]VulnRef{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		if f.down.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/querybatch":
			var req struct {
				Queries []struct {
					Package struct{ Name, Ecosystem string } `json:"package"`
					Version string                           `json:"version"`
					Token   string                           `json:"page_token"`
				} `json:"queries"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			results := []map[string]any{}
			for i, q := range req.Queries {
				if strings.HasPrefix(q.Package.Ecosystem, "Nonexistent") {
					w.WriteHeader(400)
					fmt.Fprintf(w, `{"code":3,"message":"error in query at index %d: rpc error: code = InvalidArgument desc = invalid ecosystem"}`, i)
					return
				}
				res := map[string]any{}
				if q.Token != "" {
					res["vulns"] = f.pages[q.Token]
				} else {
					list := f.vulns[q.Package.Ecosystem+"|"+q.Package.Name+"|"+q.Version]
					if len(list) > 0 {
						res["vulns"] = list
					}
					if _, ok := f.pages["next:"+q.Package.Name]; ok {
						res["next_page_token"] = "next:" + q.Package.Name
					}
				}
				results = append(results, res)
			}
			json.NewEncoder(w).Encode(map[string]any{"results": results})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/vulns/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/vulns/")
			if rec, ok := f.records[id]; ok {
				fmt.Fprint(w, rec)
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOSV) client() *OSV {
	return &OSV{Base: f.srv.URL, HTTP: f.srv.Client(), Sleep: func(time.Duration) {}, Concurrency: 4}
}

func fakeEnrich(t *testing.T, kev []string, epss map[string]float64) *Enricher {
	mux := http.NewServeMux()
	mux.HandleFunc("/kev.json", func(w http.ResponseWriter, r *http.Request) {
		type v struct {
			CVE   string `json:"cveID"`
			Added string `json:"dateAdded"`
		}
		var list []v
		for _, c := range kev {
			list = append(list, v{c, "2025-02-01"})
		}
		json.NewEncoder(w).Encode(map[string]any{"dateReleased": "2026-11-05T15:00:00.000Z", "vulnerabilities": list})
	})
	mux.HandleFunc("/epss.csv.gz", func(w http.ResponseWriter, r *http.Request) {
		gz := gzip.NewWriter(w)
		fmt.Fprintln(gz, "#model_version:v2025.03.14,score_date:2026-11-07T12:55:00Z")
		fmt.Fprintln(gz, "cve,epss,percentile")
		for c, s := range epss {
			fmt.Fprintf(gz, "%s,%.5f,%.5f\n", c, s, s/2)
		}
		gz.Close()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &Enricher{KEVURL: srv.URL + "/kev.json", EPSSURL: srv.URL + "/epss.csv.gz", HTTP: srv.Client()}
}

func ubuntuRecord(id, cve, sev, pkg, fixed string) string {
	events := `{"introduced":"0"}`
	if fixed != "" {
		events += fmt.Sprintf(`,{"fixed":%q}`, fixed)
	}
	return fmt.Sprintf(`{"id":%q,"upstream":[%q],"summary":"Advisory %s","severity":[{"type":"Ubuntu","score":%q}],"references":[{"type":"REPORT","url":"https://ubuntu.com/security/%s"}],
	"affected":[{"package":{"name":%q,"ecosystem":"Ubuntu:24.04:LTS"},"ranges":[{"type":"ECOSYSTEM","events":[%s]}],"ecosystem_specific":{"availability":"No subscription required"}}]}`, id, cve, cve, sev, cve, pkg, events)
}

func baseInput() Input {
	at := time.Date(2026, 11, 8, 1, 0, 0, 0, time.UTC)
	ok := func(rows ...map[string]string) DatasetInput {
		return DatasetInput{Status: "ok", CollectedAt: &at, Rows: rows}
	}
	row := func(kv ...string) map[string]string {
		m := map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	seen := at.Add(-time.Hour)
	return Input{
		ScannerSeenAt: &seen,
		ExternalPorts: []ExternalPort{{Port: 443, Protocol: "tcp", SeenAt: seen}},
		Datasets: map[string]DatasetInput{
			QueryOSRelease: ok(row("name", "Ubuntu", "version", "24.04.1 LTS (Noble Numbat)", "major", "24", "minor", "4", "platform", "ubuntu", "codename", "noble")),
			QueryKernel:    ok(row("version", "6.8.0-45-generic")),
			QueryPackages: ok(
				row("name", "openssh-server", "source_name", "openssl", "version", "3.0.13-0ubuntu3.4", "pkg_type", "deb", "section", "net"),
				row("name", "libssl3t64", "source_name", "openssl", "version", "3.0.13-0ubuntu3.4", "pkg_type", "deb", "section", "libs"),
				row("name", "nginx", "source_name", "nginx", "version", "1.24.0-2ubuntu7", "pkg_type", "deb", "section", "httpd"),
				row("name", "linux-image-6.8.0-45-generic", "source_name", "linux", "version", "6.8.0-45.45", "pkg_type", "deb"),
				row("name", "linux-image-6.8.0-50-generic", "source_name", "linux", "version", "6.8.0-50.51", "pkg_type", "deb"),
				row("name", "linux-image-6.8.0-30-generic", "source_name", "linux", "version", "6.8.0-30.30", "pkg_type", "deb"),
				row("name", "bash", "version", "5.2", "source", "deb"), // older agent: label column, no source_name
			),
			QueryNPM: ok(row("name", "@anthropic-ai/claude-code", "version", "1.0.10")),
			QueryExposure: ok(
				row("port", "443", "protocol", "6", "address", "0.0.0.0", "process", "nginx", "owner_package", "nginx"),
				row("port", "5432", "protocol", "6", "address", "127.0.0.1", "process", "postgres", "owner_package", "postgresql-16"),
				row("port", "15672", "protocol", "6", "address", "0.0.0.0", "process", "docker-proxy", "owner_package", "docker.io"),
			),
			QueryHardening: ok(row("item", "sysctl", "target", "kernel.randomize_va_space", "value", "0")),
			QueryFirewall:  ok(row("iptables_rules", "0", "active_firewall_units", "0")),
		},
	}
}

func TestRunMapsSoftwareToFindings(t *testing.T) {
	f := newFakeOSV(t)
	eco := "Ubuntu:24.04:LTS"
	f.vulns[eco+"|openssl|3.0.13-0ubuntu3.4"] = []VulnRef{{ID: "UBUNTU-CVE-2024-0001"}}
	f.vulns[eco+"|nginx|1.24.0-2ubuntu7"] = []VulnRef{{ID: "UBUNTU-CVE-2024-0002"}, {ID: "UBUNTU-CVE-2024-0003"}}
	f.vulns[eco+"|linux|6.8.0-45.45"] = []VulnRef{{ID: "UBUNTU-CVE-2024-0004"}}
	f.vulns["npm|@anthropic-ai/claude-code|1.0.10"] = []VulnRef{{ID: "GHSA-aaaa-bbbb-cccc"}}
	f.records["UBUNTU-CVE-2024-0001"] = ubuntuRecord("UBUNTU-CVE-2024-0001", "CVE-2024-0001", "high", "openssl", "3.0.13-0ubuntu3.5")
	f.records["UBUNTU-CVE-2024-0002"] = ubuntuRecord("UBUNTU-CVE-2024-0002", "CVE-2024-0002", "negligible", "nginx", "")
	f.records["UBUNTU-CVE-2024-0003"] = ubuntuRecord("UBUNTU-CVE-2024-0003", "CVE-2024-0003", "medium", "nginx", "1.24.0-2ubuntu7.1")
	f.records["UBUNTU-CVE-2024-0004"] = ubuntuRecord("UBUNTU-CVE-2024-0004", "CVE-2024-0004", "medium", "linux", "6.8.0-50.51")
	f.records["GHSA-aaaa-bbbb-cccc"] = `{"id":"GHSA-aaaa-bbbb-cccc","aliases":["CVE-2025-0005"],"summary":"npm issue","database_specific":{"severity":"HIGH"},"affected":[{"package":{"name":"@anthropic-ai/claude-code","ecosystem":"npm"},"ranges":[{"type":"SEMVER","events":[{"introduced":"0"},{"fixed":"1.0.20"}]}]}]}`
	f.vulns[eco+"|bash|5.2"] = nil

	enrich := fakeEnrich(t, []string{"CVE-2024-0001"}, map[string]float64{"CVE-2024-0003": 0.42})
	out := Run(context.Background(), baseInput(), Deps{OSV: f.client(), Enrich: enrich, Now: func() time.Time { return time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC) }})

	byCVE := map[string]Finding{}
	hardening := 0
	for _, fd := range out.Findings {
		var ev map[string]any
		_ = json.Unmarshal(fd.Details, &ev)
		if fd.Type == "vulnerability" {
			byCVE[ev["cve"].(string)] = fd
		} else {
			hardening++
		}
	}
	if len(byCVE) != 3 {
		t.Fatalf("expected 4 vulnerability findings (the negligible unfixed one is omitted), got %d: %v", len(byCVE), keys(byCVE))
	}
	if out.Summary.Omitted != 1 || out.Summary.Disposition != "complete" {
		t.Fatalf("omitted/limited: %+v", out.Summary)
	}
	detail := func(cve string) map[string]any {
		var ev map[string]any
		_ = json.Unmarshal(byCVE[cve].Details, &ev)
		return ev
	}
	a := detail("CVE-2024-0001")
	if byCVE["CVE-2024-0001"].Severity != "high" || a["kev"] != true || a["fixed_version"] != "3.0.13-0ubuntu3.5" || a["method"] != MethodDistro || a["fix_status"] != statusFix || a["advisory_url"] == nil {
		t.Fatalf("openssl finding: %v", a)
	}
	if a["exposure"] != exposureUnknown {
		t.Fatalf("openssl has no listening port: %v", a["exposure"])
	}
	n := detail("CVE-2024-0003")
	if n["exposure"] != exposureSeen || n["exposure_ports"] != "443/tcp" || n["epss"] != 0.42 {
		t.Fatalf("nginx is listening on all interfaces and confirmed by the scanner: %v", n)
	}
	g := detail("CVE-2025-0005")
	if byCVE["CVE-2025-0005"].Severity != "high" || g["method"] != MethodExact || g["fixed_version"] != "1.0.20" {
		t.Fatalf("npm finding: %v", g)
	}
	if hardening != 2 {
		t.Fatalf("expected ASLR and no-firewall hardening findings, got %d", hardening)
	}
	// Sorting: the known-exploited finding comes first among vulnerabilities.
	for _, fd := range out.Findings {
		if fd.Type == "vulnerability" {
			if !strings.Contains(fd.Value, "cve-2024-0001") {
				t.Fatalf("known exploited finding must be first, got %s", fd.Value)
			}
			break
		}
	}
	if out.Summary.KnownExploited != 1 || out.Summary.Exposed != 1 || out.Summary.RebootRequired != 0 {
		t.Fatalf("summary counters: %+v", out.Summary)
	}
	codes := map[string]Note{}
	for _, nn := range out.Summary.Notes {
		codes[nn.Code] = nn
	}
	if codes["system_packages_skipped"].Count != 4 || codes["ports_unattributed"].Count != 1 {
		t.Fatalf("notes: %+v", out.Summary.Notes)
	}
	if out.Summary.Provenance.KEVDate != "2026-11-05" || out.Summary.Provenance.EPSSDate != "2026-11-07" || out.Summary.Provenance.Ecosystem != eco || out.Summary.Provenance.OSVPairs == 0 {
		t.Fatalf("provenance: %+v", out.Summary.Provenance)
	}
	// An older agent's package row without source_name falls back to the package name.
	if f.requests.Load() == 0 {
		t.Fatal("no OSV requests were made")
	}
}

func keys[K comparable, V any](m map[K]V) []K {
	var out []K
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestRunNeverReportsCleanWhenTheSourceIsDown(t *testing.T) {
	f := newFakeOSV(t)
	f.down.Store(true)
	out := Run(context.Background(), baseInput(), Deps{OSV: f.client(), Enrich: fakeEnrich(t, nil, nil)})
	if out.Summary.Disposition != "limited" || out.Summary.Findings != 0 {
		t.Fatalf("a dead source must produce an incomplete run: %+v", out.Summary)
	}
	found := false
	for _, n := range out.Summary.Notes {
		if n.Code == "source_down" && n.Limiting {
			found = true
		}
	}
	if !found {
		t.Fatalf("source_down note missing: %+v", out.Summary.Notes)
	}
}

func TestCircuitBreakerStopsHammeringADeadSource(t *testing.T) {
	f := newFakeOSV(t)
	f.down.Store(true)
	c := f.client()
	for i := 0; i < 20; i++ {
		_, _, _ = c.QueryBatch(context.Background(), []Pair{{Ecosystem: "npm", Name: "x", Version: "1"}})
	}
	if got := f.requests.Load(); got > int32(breakerFailures*4+4) {
		t.Fatalf("breaker did not trip: %d requests", got)
	}
	if _, _, err := c.QueryBatch(context.Background(), []Pair{{Ecosystem: "npm", Name: "x", Version: "1"}}); err == nil {
		t.Fatal("tripped breaker must return an error")
	}
}

func TestUnknownEcosystemIsIsolatedNotFatal(t *testing.T) {
	f := newFakeOSV(t)
	f.vulns["npm|x|1"] = []VulnRef{{ID: "GHSA-1"}}
	pairs := []Pair{{Ecosystem: "Nonexistent:1", Name: "a", Version: "1"}, {Ecosystem: "npm", Name: "x", Version: "1"}}
	res, uns, err := f.client().QueryBatch(context.Background(), pairs)
	if err != nil {
		t.Fatal(err)
	}
	if !uns["Nonexistent:1"] || len(res[pairs[1]]) != 1 {
		t.Fatalf("unsupported=%v res=%v", uns, res)
	}
}

func TestUnsupportedDistributionIsALimitationNotAGuess(t *testing.T) {
	in := baseInput()
	at := time.Now()
	in.Datasets[QueryOSRelease] = DatasetInput{Status: "ok", CollectedAt: &at, Rows: []map[string]string{{"name": "Rocky Linux", "version": "9.4", "major": "9", "minor": "4", "platform": "rhel"}}}
	in.Datasets[QueryPackages] = DatasetInput{Status: "ok", CollectedAt: &at, Rows: []map[string]string{{"name": "openssl-libs", "version": "3.0.7", "pkg_type": "rpm", "arch": "x86_64"}}}
	delete(in.Datasets, QueryNPM)
	f := newFakeOSV(t)
	out := Run(context.Background(), in, Deps{OSV: f.client(), Enrich: fakeEnrich(t, nil, nil)})
	if out.Summary.Disposition != "limited" || out.Summary.Findings != 0 || f.requests.Load() != 0 {
		t.Fatalf("unsupported distribution: %+v requests=%d", out.Summary, f.requests.Load())
	}
	codes := map[string]bool{}
	for _, n := range out.Summary.Notes {
		codes[n.Code] = true
	}
	if !codes["rpm_unsupported"] {
		t.Fatalf("rpm limitation missing: %+v", out.Summary.Notes)
	}
}

func TestMissingPackageDatasetIsLimiting(t *testing.T) {
	in := baseInput()
	delete(in.Datasets, QueryPackages)
	out := Run(context.Background(), in, Deps{OSV: newFakeOSV(t).client(), Enrich: fakeEnrich(t, nil, nil)})
	if out.Summary.Disposition != "limited" {
		t.Fatalf("no package inventory must not look complete: %+v", out.Summary)
	}
}

func TestPaginationAndAdvisoryCache(t *testing.T) {
	f := newFakeOSV(t)
	eco := "Ubuntu:24.04:LTS"
	f.vulns[eco+"|openssl|3.0.13-0ubuntu3.4"] = []VulnRef{{ID: "UBUNTU-CVE-2024-0001"}}
	f.pages["next:openssl"] = []VulnRef{{ID: "UBUNTU-CVE-2024-0003"}}
	f.pages["next:openssl"] = []VulnRef{{ID: "UBUNTU-CVE-2024-0003"}}
	f.records["UBUNTU-CVE-2024-0001"] = ubuntuRecord("UBUNTU-CVE-2024-0001", "CVE-2024-0001", "high", "openssl", "3.0.13-0ubuntu3.5")
	f.records["UBUNTU-CVE-2024-0003"] = ubuntuRecord("UBUNTU-CVE-2024-0003", "CVE-2024-0003", "high", "openssl", "3.0.13-0ubuntu3.5")
	c := f.client()
	res, _, err := c.QueryBatch(context.Background(), []Pair{{Ecosystem: eco, Name: "openssl", Version: "3.0.13-0ubuntu3.4"}})
	if err != nil {
		t.Fatal(err)
	}
	refs := res[Pair{Ecosystem: eco, Name: "openssl", Version: "3.0.13-0ubuntu3.4"}]
	if len(refs) != 2 {
		t.Fatalf("pagination lost results: %v", refs)
	}
	before := f.requests.Load()
	if _, _, err = c.Advisories(context.Background(), refs); err != nil {
		t.Fatal(err)
	}
	mid := f.requests.Load()
	if _, _, err = c.Advisories(context.Background(), refs); err != nil {
		t.Fatal(err)
	}
	if mid-before != 2 || f.requests.Load() != mid {
		t.Fatalf("second fetch must be served from the cache: before=%d mid=%d after=%d", before, mid, f.requests.Load())
	}
}

func TestDeadlineMakesTheRunLimited(t *testing.T) {
	f := newFakeOSV(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := Run(ctx, baseInput(), Deps{OSV: f.client(), Enrich: fakeEnrich(t, nil, nil)})
	if out.Summary.Disposition != "limited" {
		t.Fatalf("a cancelled run must be limited: %+v", out.Summary)
	}
}

func TestExposureNeedsAScanResult(t *testing.T) {
	in := baseInput()
	in.ScannerSeenAt = nil
	in.ExternalPorts = nil
	f := newFakeOSV(t)
	eco := "Ubuntu:24.04:LTS"
	f.vulns[eco+"|nginx|1.24.0-2ubuntu7"] = []VulnRef{{ID: "UBUNTU-CVE-2024-0003"}}
	f.records["UBUNTU-CVE-2024-0003"] = ubuntuRecord("UBUNTU-CVE-2024-0003", "CVE-2024-0003", "medium", "nginx", "1.24.0-2ubuntu7.1")
	out := Run(context.Background(), in, Deps{OSV: f.client(), Enrich: fakeEnrich(t, nil, nil)})
	for _, fd := range out.Findings {
		if fd.Type != "vulnerability" {
			continue
		}
		var ev map[string]any
		_ = json.Unmarshal(fd.Details, &ev)
		if ev["exposure"] != exposureListen {
			t.Fatalf("without a scanner result a listening service can only be 'listening': %v", ev["exposure"])
		}
	}
	found := false
	for _, n := range out.Summary.Notes {
		if n.Code == "exposure_needs_scan" {
			found = true
		}
	}
	if !found {
		t.Fatal("exposure_needs_scan note missing")
	}
}

func TestEnricherCachesAndRejectsEmptyData(t *testing.T) {
	e := fakeEnrich(t, []string{"CVE-1"}, map[string]float64{"CVE-1": 0.1})
	if _, _, err := e.KEV(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.KEVURL = "http://127.0.0.1:1/unreachable"
	if m, _, err := e.KEV(context.Background()); err != nil || len(m) != 1 {
		t.Fatalf("fresh cache must serve KEV without the network: %v %v", m, err)
	}
	empty := fakeEnrich(t, nil, nil)
	if _, _, err := empty.KEV(context.Background()); err == nil {
		t.Fatal("an empty KEV catalogue must be rejected, not treated as 'nothing is exploited'")
	}
}

func fakeNVD(t *testing.T, status int) (*NVD, *[]string, *int) {
	var queries []string
	slept := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("cpeName")+"|key="+r.Header.Get("apiKey"))
		if status != http.StatusOK {
			http.Error(w, "no", status)
			return
		}
		fmt.Fprint(w, `{"vulnerabilities":[{"cve":{"id":"CVE-2025-54135","descriptions":[{"lang":"en","value":"Cursor allows remote code execution through prompt injection."}],
		"metrics":{"cvssMetricV31":[{"cvssData":{"baseScore":8.6,"baseSeverity":"HIGH"}}]},"references":[{"url":"https://github.com/cursor/cursor/security/advisories/GHSA-x"}],
		"configurations":[{"nodes":[{"cpeMatch":[{"vulnerable":true,"criteria":"cpe:2.3:a:anysphere:cursor:*:*:*:*:*:*:*:*","versionEndExcluding":"1.3.9"}]}]}]}}]}`)
	}))
	t.Cleanup(srv.Close)
	return &NVD{Base: srv.URL, HTTP: srv.Client(), APIKey: "k1", Sleep: func(time.Duration) { slept++ }}, &queries, &slept
}

func TestCPEDictionaryIsSmallAndExact(t *testing.T) {
	for _, c := range []struct {
		name, bundle, vendor, product string
		ok                            bool
	}{
		{"cursor", "", "anysphere", "cursor", true},
		{"Cursor.app", "", "anysphere", "cursor", true},
		{"Other.app", "com.todesktop.230313mzl4w4u92", "anysphere", "cursor", true},
		{"code", "", "microsoft", "visual_studio_code", true},
		{"cursor-extension-pack", "", "", "", false},
		{"unknown", "", "", "", false},
	} {
		v, p, ok := cpeFor(c.name, c.bundle)
		if ok != c.ok || v != c.vendor || p != c.product {
			t.Errorf("%q/%q = %q %q %v", c.name, c.bundle, v, p, ok)
		}
	}
	if got := upstreamVersion("1:0.45.14-1700"); got != "0.45.14" {
		t.Fatalf("upstream version: %q", got)
	}
}

func TestNVDQueryUsesTheExactCPEKeyAndParsesTheFix(t *testing.T) {
	n, queries, slept := fakeNVD(t, http.StatusOK)
	got, err := n.Query(context.Background(), "anysphere", "cursor", "1.3.0")
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	if got[0].CVE != "CVE-2025-54135" || got[0].Severity != "high" || got[0].Fixed != "1.3.9" || got[0].Score != 8.6 || !strings.HasPrefix(got[0].URL, "https://") {
		t.Fatalf("parsed: %+v", got[0])
	}
	if len(*queries) != 1 || (*queries)[0] != "cpe:2.3:a:anysphere:cursor:1.3.0:*:*:*:*:*:*:*|key=k1" {
		t.Fatalf("request: %v", *queries)
	}
	if _, err = n.Query(context.Background(), "anysphere", "cursor", "1.3.0"); err != nil || len(*queries) != 1 {
		t.Fatal("a repeated lookup must be served from the cache")
	}
	if _, err = n.Query(context.Background(), "anysphere", "cursor", "1.4.0"); err != nil || *slept == 0 {
		t.Fatalf("requests must be spaced to respect the rate limit (slept %d): %v", *slept, err)
	}
	if v, err := n.Query(context.Background(), "anysphere", "cursor", "not a version; rm -rf"); err != nil || v != nil || len(*queries) != 2 {
		t.Fatal("a malformed version must never reach the API")
	}
}

func TestRunMapsAMacApplicationThroughTheDictionary(t *testing.T) {
	n, _, _ := fakeNVD(t, http.StatusOK)
	at := time.Now()
	in := Input{Datasets: map[string]DatasetInput{
		QueryOSRelease: {Status: "ok", CollectedAt: &at, Rows: []map[string]string{{"name": "macOS", "version": "14.5", "major": "14", "minor": "5", "platform": "darwin"}}},
		QueryApps: {Status: "ok", CollectedAt: &at, Rows: []map[string]string{
			{"name": "Cursor.app", "bundle_identifier": "com.todesktop.230313mzl4w4u92", "version": "1.3.0"},
			{"name": "Other.app", "bundle_identifier": "x.y", "version": "1"}}},
	}}
	out := Run(context.Background(), in, Deps{OSV: newFakeOSV(t).client(), NVD: n, Enrich: fakeEnrich(t, []string{"CVE-2025-54135"}, nil)})
	if out.Summary.Findings != 1 || out.Summary.KnownExploited != 1 {
		t.Fatalf("summary: %+v", out.Summary)
	}
	var ev map[string]any
	_ = json.Unmarshal(out.Findings[0].Details, &ev)
	if ev["method"] != MethodCPE || ev["fixed_version"] != "1.3.9" || ev["cve"] != "CVE-2025-54135" || out.Findings[0].Severity != "high" {
		t.Fatalf("finding: %v", ev)
	}
	if out.Summary.SoftwareUnmapped != 1 {
		t.Fatalf("only the application outside the dictionary is unmapped: %d", out.Summary.SoftwareUnmapped)
	}
}

func TestADeadNVDMakesTheRunIncompleteNotClean(t *testing.T) {
	n, _, _ := fakeNVD(t, http.StatusServiceUnavailable)
	at := time.Now()
	in := Input{Datasets: map[string]DatasetInput{
		QueryOSRelease: {Status: "ok", CollectedAt: &at, Rows: []map[string]string{{"name": "macOS", "version": "14.5", "major": "14", "minor": "5", "platform": "darwin"}}},
		QueryApps:      {Status: "ok", CollectedAt: &at, Rows: []map[string]string{{"name": "Cursor.app", "bundle_identifier": "x", "version": "1.3.0"}}},
	}}
	out := Run(context.Background(), in, Deps{OSV: newFakeOSV(t).client(), NVD: n, Enrich: fakeEnrich(t, nil, nil)})
	if out.Summary.Disposition != "limited" || out.Summary.Findings != 0 {
		t.Fatalf("an unreachable NVD must not look clean: %+v", out.Summary)
	}
}

func TestOnlyApplicationsAreChecked(t *testing.T) {
	for name, section := range map[string]string{"nginx": "httpd", "openssh-server": "net", "docker.io": "admin", "git": "vcs", "cursor": "", "code": "devel", "postgresql-16": "database"} {
		if !isApplication(name, section) {
			t.Errorf("%s (%s) is an application", name, section)
		}
	}
	for name, section := range map[string]string{"libssl3t64": "libs", "libc6": "libs", "linux-image-6.8.0-45-generic": "kernel", "python3-requests": "python", "zlib1g-dev": "libdevel",
		"libfoo1": "", "linux-headers-6.8": "", "fonts-dejavu": "fonts", "base-files": "metapackages", "firmware-linux": ""} {
		if isApplication(name, section) {
			t.Errorf("%s (%s) is a system building block", name, section)
		}
	}
}
