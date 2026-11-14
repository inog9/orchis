package defensebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"orchis-go/internal/osquery"
)

const (
	maxEmitted = 1500
	// Findings below this priority are counted but not emitted: negligible advisories with no available fix.
	statusFix       = "fix_available"
	statusNoFix     = "no_fix_yet"
	statusSub       = "fix_requires_subscription"
	statusReboot    = "fix_pending_reboot"
	exposureSeen    = "exposed"
	exposureListen  = "listening"
	exposureLocal   = "local"
	exposureUnknown = "unknown"
)

var sevRank = map[string]int{"info": 0, "low": 1, "medium": 2, "high": 3, "critical": 4}

// Deps are the external collaborators of a run. Tests replace them with fakes.
type Deps struct {
	OSV    *OSV
	NVD    *NVD
	Enrich *Enricher
	// Mapper is optional: when set (injector with AI configured), it proposes CPEs for apps
	// outside the reviewed dictionary; every proposal is validated against NVD first.
	Mapper AppMapper
	Now    func() time.Time
}

type candidate struct {
	comp      *component
	adv       *Advisory
	key       string
	cve       string
	fixed     string
	status    string
	method    string
	collapsed int
	severity  string
	source    string
	ids       []string
}

type exposure struct {
	state string
	ports []string
}

// Run executes one orchis-defense-agent matching run against already collected data.
func Run(ctx context.Context, in Input, d Deps) Output {
	now := time.Now
	if d.Now != nil {
		now = d.Now
	}
	started := now().UTC()
	aiCPE, aiNote := proposeAppCPEs(ctx, in, d)
	p := buildPlan(in, aiCPE)
	sum := Summary{Schema: 1, Disposition: "complete", Platform: p.Platform, OSName: p.OSName, OSVersion: p.OSVersion,
		BySeverity: map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}, Notes: []Note{}}
	sum.Provenance = Provenance{Engine: EngineVersion, Ecosystem: p.Ecosystem, QueriedAt: started, SeverityPolicy: SeverityPolicy,
		Sources: []string{"OSV (osv.dev)", "CISA Known Exploited Vulnerabilities", "FIRST EPSS"}}
	var out Output
	addNote := func(n Note) { sum.Notes = append(sum.Notes, n) }
	for _, n := range p.Notes {
		addNote(n)
	}
	if aiNote != nil {
		addNote(*aiNote)
	}
	if len(aiCPE) > 0 {
		sum.Provenance.Sources = append(sum.Provenance.Sources, "AI CPE proposals (NVD-validated)")
	}
	// Collection coverage.
	platform := p.Platform
	if platform == "macos" {
		platform = "darwin"
	}
	for _, group := range requiredGroups(platform) {
		present := false
		for _, key := range group {
			if _, _, ok := okRows(in, key); ok {
				present = true
				break
			}
		}
		if present {
			continue
		}
		// A required check that does not apply to this platform (deb/rpm packages on macOS)
		// is not a coverage gap and must not mark the result incomplete.
		if platform != "" && !groupSupportsPlatform(group, platform) {
			continue
		}
		d := in.Datasets[group[0]]
		addNote(Note{Code: "dataset_missing", Detail: strings.Join(group, " / ") + dsReason(d), Limiting: true})
	}
	var oldest *time.Time
	for _, key := range Queries {
		d, ok := in.Datasets[key]
		if !ok || d.Status != "ok" || d.CollectedAt == nil {
			continue
		}
		if oldest == nil || d.CollectedAt.Before(*oldest) {
			t := *d.CollectedAt
			oldest = &t
		}
		if d.Truncated && key != QueryPackages {
			addNote(Note{Code: "dataset_truncated", Detail: key, Limiting: true})
		}
	}
	sum.Provenance.CollectedAt = oldest
	for _, key := range []string{QueryNPM, QueryPip, QueryExposure, QueryKernel} {
		if d, ok := in.Datasets[key]; ok && (d.Status == "skipped" || d.Status == "failed") {
			addNote(Note{Code: "dataset_unavailable", Detail: key + dsReason(d)})
		}
	}

	// Hardening findings come straight from the frozen osquery rules; no network is needed.
	for _, key := range []string{QueryHardening, QueryFirewall} {
		rows, ds, ok := okRows(in, key)
		q := osquery.Find(key)
		if !ok || q == nil {
			continue
		}
		set := &osquery.Dataset{Query: key, Status: osquery.StatusOK, Rows: rows, Truncated: ds.Truncated}
		if ds.CollectedAt != nil {
			set.CollectedAt = *ds.CollectedAt
		}
		for _, f := range q.Map(set).Findings {
			details, _ := json.Marshal(map[string]any{"reason": f.Explain, "method": "osquery-rule", "rule": f.Rule})
			out.Findings = append(out.Findings, Finding{Type: "text", Value: f.Value, Title: f.Title, Severity: f.Severity, Details: details})
			sum.Hardening++
		}
	}

	unsupported := map[string]bool{}
	sum.SoftwareChecked = len(p.Components)
	sum.SoftwareUnmapped = p.Unmapped
	sum.SoftwareIgnored = p.Ignored
	var cands []*candidate
	if len(p.Components) > 0 && d.OSV != nil {
		pairs := make([]Pair, len(p.Components))
		for i, c := range p.Components {
			pairs[i] = c.Pair
		}
		sum.Provenance.OSVPairs = len(pairs)
		results, uns, err := d.OSV.QueryBatch(ctx, pairs)
		for e := range uns {
			unsupported[e] = true
		}
		if err != nil {
			addNote(Note{Code: sourceCode(err), Detail: "OSV", Limiting: true})
		}
		// Collect every advisory reference once.
		refs := map[string]VulnRef{}
		for _, list := range results {
			for _, r := range list {
				if _, seen := refs[r.ID]; !seen {
					refs[r.ID] = r
				}
			}
		}
		ordered := make([]VulnRef, 0, len(refs))
		for _, r := range refs {
			ordered = append(ordered, r)
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
		var advisories map[string]*Advisory
		var failed []string
		if len(ordered) > 0 && err == nil {
			var aerr error
			advisories, failed, aerr = d.OSV.Advisories(ctx, ordered)
			if aerr != nil {
				addNote(Note{Code: sourceCode(aerr), Detail: "OSV", Limiting: true})
			}
		}
		sum.Provenance.OSVAdvisories = len(advisories)
		if len(failed) > 0 {
			addNote(Note{Code: "advisory_unreadable", Count: len(failed), Limiting: true})
		}
		for _, c := range p.Components {
			if unsupported[c.Pair.Ecosystem] {
				continue
			}
			for _, ref := range results[c.Pair] {
				a := advisories[ref.ID]
				if a == nil {
					continue
				}
				cands = append(cands, makeCandidates(c, a, p)...)
			}
		}
		if len(unsupported) > 0 {
			names := make([]string, 0, len(unsupported))
			for e := range unsupported {
				names = append(names, e)
			}
			sort.Strings(names)
			addNote(Note{Code: "ecosystem_unsupported", Detail: strings.Join(names, ", "), Limiting: true})
		}
	} else if len(p.Components) > 0 {
		addNote(Note{Code: "source_down", Detail: "OSV", Limiting: true})
	}

	cands = append(cands, nvdCandidates(ctx, p, d, &sum)...)
	cands = mergeCandidates(cands)
	// Many advisories fixed by the same installed-but-not-running kernel are one action (reboot), so they become one candidate.
	cands = collapseReboot(cands)

	// Exploitation signals.
	cves := map[string]bool{}
	for _, c := range cands {
		if strings.HasPrefix(c.cve, "CVE-") {
			cves[c.cve] = true
		}
	}
	var kev map[string]KEVEntry
	var epss map[string]EPSSEntry
	if len(cands) > 0 && d.Enrich != nil {
		var err error
		if kev, sum.Provenance.KEVDate, err = d.Enrich.KEV(ctx); err != nil {
			addNote(Note{Code: "kev_unavailable", Detail: briefErr(err)})
			kev = nil
		}
		if epss, sum.Provenance.EPSSDate, err = d.Enrich.EPSS(ctx); err != nil {
			addNote(Note{Code: "epss_unavailable", Detail: briefErr(err)})
			epss = nil
		}
	}
	exp := computeExposure(in, p, &sum)

	type emit struct {
		c        *candidate
		kev      *KEVEntry
		epss     *EPSSEntry
		exposure exposure
		reboot   bool
	}
	var emitList []emit
	for _, c := range cands {
		e := emit{c: c, exposure: exp[c.comp.Pair.Name]}
		if e.exposure.state == "" {
			e.exposure.state = exposureUnknown
		}
		if k, ok := kev[c.cve]; ok {
			kk := k
			e.kev = &kk
		}
		if s, ok := epss[c.cve]; ok {
			ss := s
			e.epss = &ss
		}
		e.reboot = c.status == statusReboot
		// Policy: advisories without a fix are listed only when high, critical or known exploited; the rest are counted.
		keep := e.kev != nil || sevRank[c.severity] >= sevRank["high"] || (sevRank[c.severity] >= sevRank["low"] && c.status != statusNoFix)
		if !keep {
			sum.Omitted++
			continue
		}
		emitList = append(emitList, e)
	}
	sort.SliceStable(emitList, func(i, j int) bool {
		a, b := emitList[i], emitList[j]
		if (a.kev != nil) != (b.kev != nil) {
			return a.kev != nil
		}
		if (a.exposure.state == exposureSeen) != (b.exposure.state == exposureSeen) {
			return a.exposure.state == exposureSeen
		}
		if sevRank[a.c.severity] != sevRank[b.c.severity] {
			return sevRank[a.c.severity] > sevRank[b.c.severity]
		}
		as, bs := 0.0, 0.0
		if a.epss != nil {
			as = a.epss.Score
		}
		if b.epss != nil {
			bs = b.epss.Score
		}
		if as != bs {
			return as > bs
		}
		if a.c.comp.Pair.Name != b.c.comp.Pair.Name {
			return a.c.comp.Pair.Name < b.c.comp.Pair.Name
		}
		return a.c.cve < b.c.cve
	})
	if len(emitList) > maxEmitted {
		sum.Omitted += len(emitList) - maxEmitted
		emitList = emitList[:maxEmitted]
	}
	for _, e := range emitList {
		c := e.c
		method := MethodExact
		if strings.HasPrefix(c.comp.Pair.Ecosystem, "Ubuntu") || strings.HasPrefix(c.comp.Pair.Ecosystem, "Debian") {
			method = MethodDistro
		}
		if c.method != "" {
			method = c.method
		}
		ev := map[string]any{
			"cve": c.cve, "package": c.comp.Display, "ecosystem": c.comp.Pair.Ecosystem, "source_package": c.comp.Pair.Name,
			"installed_version": c.comp.Pair.Version, "fix_status": c.status, "severity_source": c.source, "method": method,
			"advisory": strings.Join(c.ids, ", "), "summary": c.adv.Summary, "exposure": e.exposure.state,
			"data_as_of": sum.Provenance.QueriedAt.Format(time.RFC3339),
		}
		if c.fixed != "" {
			ev["fixed_version"] = c.fixed
		}
		if c.adv.URL != "" {
			ev["advisory_url"] = c.adv.URL
		}
		if c.adv.CVSS > 0 {
			ev["score"] = c.adv.CVSS
		}
		if len(e.exposure.ports) > 0 {
			ev["exposure_ports"] = strings.Join(e.exposure.ports, ", ")
		}
		if e.kev != nil {
			ev["kev"] = true
			ev["kev_added"] = e.kev.Added
			sum.KnownExploited++
		}
		if e.epss != nil {
			ev["epss"] = e.epss.Score
			ev["epss_percentile"] = e.epss.Percentile
		}
		if e.reboot {
			ev["reboot_required"] = true
			n := 1
			if c.collapsed > 1 {
				n = c.collapsed
				ev["advisory_count"] = float64(n)
			}
			sum.RebootRequired += n
		}
		if e.exposure.state == exposureSeen {
			sum.Exposed++
		}
		details, _ := json.Marshal(ev)
		title := fmt.Sprintf("%s in %s %s", c.cve, c.comp.Display, c.comp.Pair.Version)
		if len([]rune(title)) > 290 {
			title = string([]rune(title)[:290]) + "…"
		}
		value := strings.ToLower("defensebot:" + c.comp.Pair.Ecosystem + ":" + c.comp.Pair.Name + ":" + c.cve)
		if len(value) > 480 {
			value = value[:480]
		}
		out.Findings = append(out.Findings, Finding{Type: "vulnerability", Value: value, Title: title, Severity: c.severity, Details: details})
		sum.BySeverity[c.severity]++
		sum.Findings++
	}
	if sum.Omitted > 0 {
		addNote(Note{Code: "advisories_omitted", Count: sum.Omitted})
	}
	for _, n := range sum.Notes {
		if n.Limiting {
			sum.Disposition = "limited"
			break
		}
	}
	if err := ctx.Err(); err != nil {
		sum.Disposition = "limited"
		addNote(Note{Code: "timeout", Limiting: true})
	}
	sort.SliceStable(sum.Notes, func(i, j int) bool { return sum.Notes[i].Code < sum.Notes[j].Code })
	out.Summary = sum
	out.Logs = append(out.Logs, fmt.Sprintf("orchis-defense-agent checked %d software component(s), sent %d lookup(s), produced %d finding(s) (%d omitted).", sum.SoftwareChecked, sum.Provenance.OSVPairs, sum.Findings, sum.Omitted))
	return out
}

func dsReason(d DatasetInput) string {
	if d.Status == "" {
		return " (no data)"
	}
	s := " (" + d.Status
	if d.Reason != "" {
		s += ": " + d.Reason
	}
	return s + ")"
}

func briefErr(err error) string {
	s := err.Error()
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

func sourceCode(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "timeout"
	default:
		return "source_down"
	}
}

// makeCandidates builds the candidate findings of one component for one advisory.
func makeCandidates(c *component, a *Advisory, p *plan) []*candidate {
	key := affectedKey(c.Pair.Ecosystem, c.Pair.Name)
	info := a.Affected[key]
	fixed := chooseFixed(c.Pair.Ecosystem, c.Pair.Version, info.Fixed)
	status := statusFix
	switch {
	case fixed == "":
		status = statusNoFix
		if proKey := proEcosystemKey(c.Pair.Ecosystem, c.Pair.Name); proKey != "" {
			if pro, ok := a.Affected[proKey]; ok && len(pro.Fixed) > 0 {
				status = statusSub
			}
		}
	case strings.Contains(strings.ToLower(info.Availability), "subscription") && !strings.Contains(strings.ToLower(info.Availability), "no subscription"):
		status = statusSub
	}
	if c.Kernel && fixed != "" && p.NewestKern != "" && CompareVersions(c.Pair.Ecosystem, fixed, p.NewestKern) <= 0 {
		status = statusReboot
	}
	sev, src := a.SeverityFor(key)
	cves := a.CVEs
	if len(cves) == 0 {
		cves = []string{a.ID}
	}
	// One finding per CVE, even when an advisory lists several.
	var out []*candidate
	for _, cve := range cves {
		out = append(out, &candidate{comp: c, adv: a, key: key, cve: cve, fixed: fixed, status: status, severity: sev, source: src, ids: []string{a.ID}})
	}
	return out
}

func proEcosystemKey(eco, name string) string {
	if strings.HasPrefix(eco, "Ubuntu:") && !strings.HasPrefix(eco, "Ubuntu:Pro:") {
		return affectedKey("Ubuntu:Pro:"+strings.TrimPrefix(eco, "Ubuntu:"), name)
	}
	return ""
}

func chooseFixed(eco, installed string, fixed []string) string {
	best := ""
	for _, f := range fixed {
		if CompareVersions(eco, f, installed) > 0 && (best == "" || CompareVersions(eco, f, best) < 0) {
			best = f
		}
	}
	if best == "" && len(fixed) > 0 {
		return fixed[0]
	}
	return best
}

// mergeCandidates keeps one candidate per (component, CVE): the highest severity wins and advisory ids are combined.
func mergeCandidates(in []*candidate) []*candidate {
	type k struct {
		pair Pair
		cve  string
	}
	idx := map[k]*candidate{}
	var out []*candidate
	for _, c := range in {
		key := k{c.comp.Pair, c.cve}
		if prev, ok := idx[key]; ok {
			prev.ids = appendUnique(prev.ids, c.ids...)
			if sevRank[c.severity] > sevRank[prev.severity] {
				prev.severity, prev.source = c.severity, c.source
			}
			if prev.fixed == "" && c.fixed != "" {
				prev.fixed, prev.status = c.fixed, c.status
			}
			continue
		}
		idx[key] = c
		out = append(out, c)
	}
	for _, c := range out {
		sort.Strings(c.ids)
	}
	return out
}

func appendUnique(list []string, add ...string) []string {
	for _, a := range add {
		found := false
		for _, l := range list {
			if l == a {
				found = true
			}
		}
		if !found {
			list = append(list, a)
		}
	}
	return list
}

// computeExposure attributes listening ports to source packages. Ports published by a container proxy are not attributed
// because the owning package would be the container runtime, not the software behind the port.
func computeExposure(in Input, p *plan, sum *Summary) map[string]exposure {
	res := map[string]exposure{}
	rows, _, ok := okRows(in, QueryExposure)
	if !ok {
		return res
	}
	ext := map[string]bool{}
	for _, e := range in.ExternalPorts {
		ext[fmt.Sprintf("%d/%s", e.Port, normalizeProto(e.Protocol))] = true
	}
	type agg struct {
		all, seen, local bool
		ports            map[string]bool
	}
	by := map[string]*agg{}
	unattributed := 0
	for _, r := range rows {
		if r["process"] == "docker-proxy" || r["process"] == "containerd-shim" || r["owner_package"] == "" {
			unattributed++
			continue
		}
		src := p.Binary2Src[r["owner_package"]]
		if src == "" {
			src = r["owner_package"]
		}
		port, err := strconv.Atoi(r["port"])
		if err != nil || port <= 0 {
			continue
		}
		a := by[src]
		if a == nil {
			a = &agg{ports: map[string]bool{}}
			by[src] = a
		}
		label := fmt.Sprintf("%d/%s", port, normalizeProto(r["protocol"]))
		addr := r["address"]
		switch {
		case addr == "0.0.0.0" || addr == "::" || addr == "":
			a.all = true
			a.ports[label] = true
			if ext[label] {
				a.seen = true
			}
		case strings.HasPrefix(addr, "127.") || addr == "::1":
			a.local = true
		default:
			a.all = true
			a.ports[label] = true
			if ext[label] {
				a.seen = true
			}
		}
	}
	for src, a := range by {
		var ports []string
		for l := range a.ports {
			ports = append(ports, l)
		}
		sort.Strings(ports)
		switch {
		case a.seen:
			res[src] = exposure{state: exposureSeen, ports: ports}
		case a.all:
			res[src] = exposure{state: exposureListen, ports: ports}
		case a.local:
			res[src] = exposure{state: exposureLocal}
		}
	}
	if unattributed > 0 {
		sum.Notes = append(sum.Notes, Note{Code: "ports_unattributed", Count: unattributed})
	}
	if in.ScannerSeenAt == nil {
		sum.Notes = append(sum.Notes, Note{Code: "exposure_needs_scan"})
	}
	return res
}

func normalizeProto(p string) string {
	switch p {
	case "6", "tcp", "TCP":
		return "tcp"
	case "17", "udp", "UDP":
		return "udp"
	}
	return p
}

// collapseReboot replaces two or more "fixed, reboot required" advisories of one component by a single candidate that
// carries the highest severity, the highest fixed version and the number of advisories.
func collapseReboot(in []*candidate) []*candidate {
	groups := map[*component][]*candidate{}
	var order []*component
	var out []*candidate
	for _, c := range in {
		if c.status != statusReboot {
			out = append(out, c)
			continue
		}
		if _, ok := groups[c.comp]; !ok {
			order = append(order, c.comp)
		}
		groups[c.comp] = append(groups[c.comp], c)
	}
	for _, comp := range order {
		g := groups[comp]
		if len(g) == 1 {
			out = append(out, g[0])
			continue
		}
		sort.SliceStable(g, func(i, j int) bool {
			if sevOrderOf(g[i].severity) != sevOrderOf(g[j].severity) {
				return sevOrderOf(g[i].severity) > sevOrderOf(g[j].severity)
			}
			return g[i].cve < g[j].cve
		})
		top := *g[0]
		adv := *top.adv
		adv.Summary = fmt.Sprintf("%d advisories are fixed by the installed kernel; reboot to apply it. Most severe: %s.", len(g), top.cve)
		top.adv = &adv
		top.ids = nil
		for _, c := range g {
			if top.fixed == "" || (c.fixed != "" && CompareVersions(comp.Pair.Ecosystem, c.fixed, top.fixed) > 0) {
				top.fixed = c.fixed
			}
			top.ids = appendUnique(top.ids, c.ids...)
		}
		if len(top.ids) > 5 {
			top.ids = top.ids[:5]
		}
		top.collapsed = len(g)
		out = append(out, &top)
	}
	return out
}

func sevOrderOf(s string) int { return sevRank[s] }

// groupSupportsPlatform reports whether any check in the group can run on this platform.
// The platform here is the raw os_release value (a distro name on Linux, darwin on macOS),
// so it is normalized the same way requiredGroups treats it.
func groupSupportsPlatform(group []string, platform string) bool {
	norm := platform
	switch norm {
	case "darwin", "macos":
		norm = "macos"
	case "windows":
		// tetap "windows"
	default:
		if norm != "" {
			norm = "linux"
		}
	}
	for _, key := range group {
		for i := range osquery.Queries {
			if osquery.Queries[i].Key == key && osquery.Queries[i].SupportsOS(norm) {
				return true
			}
		}
	}
	return false
}

// proposeAppCPEs asks the optional AI mapper for CPEs of apps the reviewed dictionary does
// not know, and keeps only proposals that NVD confirms exist. Wrong guesses are dropped.
func proposeAppCPEs(ctx context.Context, in Input, d Deps) (map[string]cpeComp, *Note) {
	out := map[string]cpeComp{}
	if d.Mapper == nil || d.NVD == nil {
		return out, nil
	}
	var apps []AppRef
	seen := map[string]bool{}
	for _, key := range []string{QueryApps, QueryPrograms} {
		rows, _, ok := okRows(in, key)
		if !ok {
			continue
		}
		for _, r := range rows {
			name := strings.TrimSpace(r["name"])
			if name == "" || seen[AppKey(name)] {
				continue
			}
			if _, _, mapped := cpeFor(name, r["bundle_identifier"]); mapped {
				continue // already covered by the reviewed dictionary
			}
			seen[AppKey(name)] = true
			apps = append(apps, AppRef{Name: name, Bundle: r["bundle_identifier"], Version: r["version"]})
		}
	}
	if len(apps) == 0 {
		return out, nil
	}
	prop, err := d.Mapper.ProposeCPE(ctx, apps)
	if err != nil {
		return out, &Note{Code: "ai_mapper_failed", Detail: err.Error()}
	}
	n := 0
	for _, a := range apps {
		vp, ok := prop[AppKey(a.Name)]
		if !ok {
			continue
		}
		parts := strings.SplitN(vp, ":", 2)
		if len(parts) != 2 {
			continue
		}
		exists, e := d.NVD.CPEExists(ctx, parts[0], parts[1])
		if e != nil || !exists {
			continue
		}
		out[AppKey(a.Name)] = cpeComp{Vendor: parts[0], Product: parts[1], Name: a.Name, Version: strings.TrimSpace(a.Version)}
		n++
	}
	if n > 0 {
		return out, &Note{Code: "apps_ai_mapped", Count: n}
	}
	return out, nil
}
