package reporting

import (
	"fmt"
	"html"
	"html/template"
	"sort"
	"strings"

	"orchis-go/internal/defensebot"
)

// The DefenseBot part of a report is produced here, in both languages, from the frozen snapshot only. Its structure,
// ordering and numbers are deterministic: the same snapshot always renders the same document. Advisory text stays in
// English as published; every label and sentence around it is localized.

func tr(lang, en, id string) string {
	if lang == "id" {
		return id
	}
	return en
}

func esc(s string) string { return html.EscapeString(s) }

var sevOrder = map[string]int{"critical": 4, "high": 3, "medium": 2, "low": 1, "info": 0}

func hasDefense(assets []Asset) bool {
	for _, a := range assets {
		if a.Defense != nil {
			return true
		}
	}
	return false
}

type defItem struct {
	CVE, Severity, Source, Status, URL, Summary, Confirmed string
	KEV, Reboot                                            bool
	EPSS                                                   float64
	HasEPSS                                                bool
}

type defGroup struct {
	Software, Installed, Fixed, Ecosystem string
	Severity, Exposure, Ports             string
	KEV                                   bool
	Items                                 []defItem
}

func str(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

func isDefenseFinding(f Finding) bool {
	m := str(f.Evidence, "method")
	return f.Type == "vulnerability" && (m == defensebot.MethodDistro || m == defensebot.MethodExact || m == defensebot.MethodCPE)
}

var exposureOrder = map[string]int{"exposed": 3, "listening": 2, "local": 1, "unknown": 0, "": 0}

func defGroups(a Asset) []defGroup {
	idx := map[string]*defGroup{}
	var order []string
	for _, f := range a.Findings {
		if !isDefenseFinding(f) {
			continue
		}
		e := f.Evidence
		key := str(e, "package") + "\x00" + str(e, "installed_version") + "\x00" + str(e, "source_package")
		g := idx[key]
		if g == nil {
			g = &defGroup{Software: str(e, "package"), Installed: str(e, "installed_version"), Ecosystem: str(e, "ecosystem"), Severity: "info", Exposure: "unknown"}
			idx[key] = g
			order = append(order, key)
		}
		it := defItem{CVE: str(e, "cve"), Severity: f.Severity, Source: str(e, "severity_source"), Status: str(e, "fix_status"), URL: str(e, "advisory_url"),
			Summary: str(e, "summary"), Confirmed: str(e, "confirmed_by")}
		it.KEV, _ = e["kev"].(bool)
		it.Reboot, _ = e["reboot_required"].(bool)
		if v, ok := e["epss"].(float64); ok {
			it.EPSS, it.HasEPSS = v, true
		}
		g.Items = append(g.Items, it)
		if sevOrder[f.Severity] > sevOrder[g.Severity] {
			g.Severity = f.Severity
		}
		if it.KEV {
			g.KEV = true
		}
		if x := str(e, "exposure"); exposureOrder[x] > exposureOrder[g.Exposure] {
			g.Exposure = x
			g.Ports = str(e, "exposure_ports")
		}
		if fx := str(e, "fixed_version"); fx != "" && (g.Fixed == "" || defensebot.CompareVersions(g.Ecosystem, fx, g.Fixed) > 0) {
			g.Fixed = fx
		}
	}
	out := make([]defGroup, 0, len(order))
	for _, k := range order {
		g := idx[k]
		sort.SliceStable(g.Items, func(i, j int) bool {
			a, b := g.Items[i], g.Items[j]
			if a.KEV != b.KEV {
				return a.KEV
			}
			if sevOrder[a.Severity] != sevOrder[b.Severity] {
				return sevOrder[a.Severity] > sevOrder[b.Severity]
			}
			if a.EPSS != b.EPSS {
				return a.EPSS > b.EPSS
			}
			return a.CVE < b.CVE
		})
		out = append(out, *g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.KEV != b.KEV {
			return a.KEV
		}
		if exposureOrder[a.Exposure] != exposureOrder[b.Exposure] {
			return exposureOrder[a.Exposure] > exposureOrder[b.Exposure]
		}
		if sevOrder[a.Severity] != sevOrder[b.Severity] {
			return sevOrder[a.Severity] > sevOrder[b.Severity]
		}
		if a.Software != b.Software {
			return a.Software < b.Software
		}
		return a.Installed < b.Installed
	})
	return out
}

type defHardening struct{ Severity, Title, Why string }

func defHardeningOf(a Asset) []defHardening {
	var out []defHardening
	for _, f := range a.Findings {
		if f.Type == "text" && str(f.Evidence, "method") == "osquery-rule" {
			out = append(out, defHardening{Severity: f.Severity, Title: f.Title, Why: str(f.Evidence, "reason")})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if sevOrder[out[i].Severity] != sevOrder[out[j].Severity] {
			return sevOrder[out[i].Severity] > sevOrder[out[j].Severity]
		}
		return out[i].Title < out[j].Title
	})
	return out
}

type defRow struct {
	Name         string
	Counts       map[string]int
	KEV, Exposed int
	Complete     bool
	Groups       []defGroup
}

func defRows(assets []Asset) []defRow {
	var rows []defRow
	for _, a := range assets {
		if a.Defense == nil {
			continue
		}
		r := defRow{Name: a.Name, Counts: map[string]int{}, Complete: a.Defense.Disposition == "complete" && a.Status == "finished", Groups: defGroups(a)}
		for _, g := range r.Groups {
			for _, it := range g.Items {
				r.Counts[it.Severity]++
				if it.KEV {
					r.KEV++
				}
			}
			if g.Exposure == "exposed" {
				r.Exposed += len(g.Items)
			}
		}
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		for _, s := range []string{"critical", "high"} {
			if a.Counts[s] != b.Counts[s] {
				return a.Counts[s] > b.Counts[s]
			}
		}
		if a.KEV != b.KEV {
			return a.KEV > b.KEV
		}
		if a.Counts["medium"] != b.Counts["medium"] {
			return a.Counts["medium"] > b.Counts["medium"]
		}
		return a.Name < b.Name
	})
	return rows
}

func sevLabel(lang, s string) string { return reportLabel(lang, s) }

func sevBadge(lang, s string) string {
	return `<span class="def-sev def-` + esc(s) + `">` + esc(sevLabel(lang, s)) + `</span>`
}

func statusLabel(lang, s string) string {
	switch s {
	case "fix_available":
		return tr(lang, "Fix available", "Perbaikan tersedia")
	case "no_fix_yet":
		return tr(lang, "No fix yet", "Belum ada perbaikan")
	case "fix_requires_subscription":
		return tr(lang, "Fix needs a subscription", "Perbaikan butuh langganan")
	case "fix_pending_reboot":
		return tr(lang, "Fixed - reboot required", "Sudah diperbaiki - perlu reboot")
	}
	return ""
}

func exposureLabel(lang, s string) string {
	switch s {
	case "exposed":
		return tr(lang, "Seen by scanner", "Terlihat oleh scanner")
	case "listening":
		return tr(lang, "Listening", "Mendengarkan")
	case "local":
		return tr(lang, "Local only", "Hanya lokal")
	}
	return tr(lang, "Unknown", "Tidak diketahui")
}

func noteText(lang string, n defensebot.Note) string {
	c := reportNumber(lang, n.Count)
	switch n.Code {
	case "packages_truncated":
		return tr(lang, "The software list was cut at "+c+" entries; software beyond that was not checked.", "Daftar software dipotong pada "+c+" entri; software di luar itu tidak diperiksa.")
	case "rpm_unsupported":
		return tr(lang, c+" RPM packages were not checked: RPM-based distributions are not supported yet.", c+" paket RPM tidak diperiksa: distribusi berbasis RPM belum didukung.")
	case "system_packages_skipped":
		return tr(lang, c+" system packages (libraries, kernel, development) were skipped: DefenseBot checks applications and services.", c+" paket sistem (pustaka, kernel, pengembangan) dilewati: DefenseBot memeriksa aplikasi dan layanan.")
	case "old_kernels_skipped":
		return tr(lang, c+" installed kernel packages that are not running were skipped.", c+" paket kernel terpasang yang tidak sedang berjalan dilewati.")
	case "ecosystem_unsupported":
		return tr(lang, "Vulnerability data is not available for this distribution ("+n.Detail+"); its packages were not checked.", "Data kerentanan belum tersedia untuk distribusi ini ("+n.Detail+"); paketnya tidak diperiksa.")
	case "apps_unmapped":
		return tr(lang, c+" applications could not be mapped to vulnerability data and were not checked.", c+" aplikasi tidak dapat dipetakan ke data kerentanan dan tidak diperiksa.")
	case "dataset_missing":
		return tr(lang, "Required endpoint data is missing ("+n.Detail+"); the result is incomplete.", "Data endpoint yang diperlukan tidak ada ("+n.Detail+"); hasilnya tidak lengkap.")
	case "dataset_truncated":
		return tr(lang, "Endpoint data was truncated ("+n.Detail+").", "Data endpoint terpotong ("+n.Detail+").")
	case "dataset_unavailable":
		return tr(lang, "A check did not run ("+n.Detail+").", "Satu pemeriksaan tidak berjalan ("+n.Detail+").")
	case "source_down":
		return tr(lang, "The vulnerability service could not be reached; no result can be shown as clean.", "Layanan kerentanan tidak dapat dihubungi; tidak ada hasil yang boleh dianggap bersih.")
	case "timeout":
		return tr(lang, "The run stopped before it finished; results may be partial.", "Proses berhenti sebelum selesai; hasil mungkin parsial.")
	case "advisory_unreadable":
		return tr(lang, c+" advisories could not be read and are missing from this report.", c+" advisori tidak dapat dibaca dan tidak ada di laporan ini.")
	case "kev_unavailable":
		return tr(lang, "The CISA known-exploited catalogue was unavailable; exploitation status is not shown.", "Katalog CISA known-exploited tidak tersedia; status eksploitasi tidak ditampilkan.")
	case "epss_unavailable":
		return tr(lang, "EPSS scores were unavailable; they are not shown.", "Skor EPSS tidak tersedia; tidak ditampilkan.")
	case "advisories_omitted":
		return tr(lang, c+" low-priority advisories (negligible severity without a fix) were counted but not listed.", c+" advisori prioritas rendah (tingkat diabaikan tanpa perbaikan) dihitung tetapi tidak dicantumkan.")
	case "ports_unattributed":
		return tr(lang, c+" listening ports belong to container proxies and could not be attributed to a package.", c+" port yang mendengarkan milik proxy kontainer dan tidak dapat dikaitkan ke paket.")
	case "exposure_needs_scan":
		return tr(lang, "No recent Nmap scan of this asset exists, so exposure is shown as listening or unknown. Run a web scan, then DefenseBot again.", "Belum ada scan Nmap terbaru untuk aset ini, sehingga paparan hanya ditampilkan sebagai mendengarkan atau tidak diketahui. Jalankan scan web, lalu DefenseBot lagi.")
	}
	return n.Code
}

const maxGroupsShown = 40
const maxItemsShown = 12

func flags(lang string, it defItem) string {
	var parts []string
	if it.KEV {
		parts = append(parts, `<span class="def-flag def-kev">`+esc(tr(lang, "Known exploited", "Sudah dieksploitasi"))+`</span>`)
	}
	if it.HasEPSS {
		parts = append(parts, esc(fmt.Sprintf("EPSS %.1f%%", it.EPSS*100)))
	}
	if it.Confirmed != "" {
		parts = append(parts, esc(tr(lang, "Also found by ", "Juga ditemukan oleh ")+it.Confirmed))
	}
	return strings.Join(parts, " · ")
}

func fixText(lang string, g defGroup) string {
	if g.Fixed != "" {
		return esc(tr(lang, "Upgrade to ", "Perbarui ke ") + g.Fixed + " " + tr(lang, "or later", "atau lebih baru"))
	}
	return esc(tr(lang, "No fix available yet", "Belum ada perbaikan"))
}

// defenseHTML renders the DefenseBot chapter of a report.
func defenseHTML(lang string, s Snapshot) template.HTML {
	if !hasDefense(s.Assets) {
		return ""
	}
	rows := defRows(s.Assets)
	var b strings.Builder
	b.WriteString(`<section class="defense"><h2 class="module-title" id="defensebot">` + esc(tr(lang, "Software vulnerabilities (DefenseBot)", "Kerentanan perangkat lunak (DefenseBot)")) + `</h2>`)
	b.WriteString(`<p class="section-note">` + esc(tr(lang,
		"DefenseBot maps the software installed on enrolled endpoints to known vulnerabilities and lists hardening gaps. It reads data collected by the agent and contacts public vulnerability services with software names and versions only. A version match is not proof that a weakness can be exploited, and distributions may fix issues without changing the version number.",
		"DefenseBot memetakan perangkat lunak terpasang di endpoint terdaftar ke kerentanan yang sudah dikenal dan mencantumkan celah hardening. Datanya berasal dari agent dan hanya nama serta versi perangkat lunak yang dikirim ke layanan kerentanan publik. Kecocokan versi bukan bukti kelemahan dapat dieksploitasi, dan distribusi dapat memperbaiki masalah tanpa mengubah nomor versi.")) + `</p>`)

	if len(rows) > 1 {
		b.WriteString(`<h3>` + esc(tr(lang, "Fleet overview", "Ringkasan armada")) + `</h3><table class="def-table"><thead><tr><th>` + esc(tr(lang, "Host", "Host")) + `</th>`)
		for _, sv := range []string{"critical", "high", "medium", "low"} {
			b.WriteString(`<th>` + esc(sevLabel(lang, sv)) + `</th>`)
		}
		b.WriteString(`<th>` + esc(tr(lang, "Known exploited", "Sudah dieksploitasi")) + `</th><th>` + esc(tr(lang, "Seen by scanner", "Terlihat scanner")) + `</th><th>` + esc(tr(lang, "Coverage", "Cakupan")) + `</th></tr></thead><tbody>`)
		for _, r := range rows {
			b.WriteString(`<tr><td>` + esc(r.Name) + `</td>`)
			for _, sv := range []string{"critical", "high", "medium", "low"} {
				b.WriteString(`<td>` + reportNumber(lang, r.Counts[sv]) + `</td>`)
			}
			cov := tr(lang, "Complete", "Lengkap")
			if !r.Complete {
				cov = tr(lang, "Incomplete", "Tidak lengkap")
			}
			b.WriteString(`<td>` + reportNumber(lang, r.KEV) + `</td><td>` + reportNumber(lang, r.Exposed) + `</td><td>` + esc(cov) + `</td></tr>`)
		}
		b.WriteString(`</tbody></table>`)
	}

	type fix struct {
		host string
		g    defGroup
	}
	var fixes []fix
	for _, r := range rows {
		for _, g := range r.Groups {
			fixes = append(fixes, fix{r.Name, g})
		}
	}
	sort.SliceStable(fixes, func(i, j int) bool {
		a, c := fixes[i].g, fixes[j].g
		if a.KEV != c.KEV {
			return a.KEV
		}
		if exposureOrder[a.Exposure] != exposureOrder[c.Exposure] {
			return exposureOrder[a.Exposure] > exposureOrder[c.Exposure]
		}
		if sevOrder[a.Severity] != sevOrder[c.Severity] {
			return sevOrder[a.Severity] > sevOrder[c.Severity]
		}
		if fixes[i].host != fixes[j].host {
			return fixes[i].host < fixes[j].host
		}
		return a.Software < c.Software
	})
	if len(fixes) > 0 {
		b.WriteString(`<h3>` + esc(tr(lang, "Fix first", "Perbaiki lebih dulu")) + `</h3><ol class="def-fix">`)
		for i, f := range fixes {
			if i >= 5 {
				break
			}
			b.WriteString(`<li><strong>` + esc(f.g.Software) + ` ` + esc(f.g.Installed) + `</strong> · ` + esc(f.host) + ` · ` + sevBadge(lang, f.g.Severity) + ` · ` + fixText(lang, f.g))
			var extra []string
			if f.g.KEV {
				extra = append(extra, esc(tr(lang, "known exploited", "sudah dieksploitasi")))
			}
			if f.g.Exposure == "exposed" {
				extra = append(extra, esc(tr(lang, "seen by scanner", "terlihat oleh scanner")))
			}
			word := tr(lang, "advisories", "advisori")
			if len(f.g.Items) == 1 {
				word = tr(lang, "advisory", "advisori")
			}
			extra = append(extra, reportNumber(lang, len(f.g.Items))+" "+esc(word))
			b.WriteString(` (` + strings.Join(extra, ", ") + `)</li>`)
		}
		b.WriteString(`</ol>`)
	}

	if len(rows) > 1 {
		type agg struct {
			name         string
			hosts, items int
		}
		m := map[string]*agg{}
		for _, r := range rows {
			seen := map[string]bool{}
			for _, g := range r.Groups {
				a := m[g.Software]
				if a == nil {
					a = &agg{name: g.Software}
					m[g.Software] = a
				}
				a.items += len(g.Items)
				if !seen[g.Software] {
					seen[g.Software] = true
					a.hosts++
				}
			}
		}
		list := make([]*agg, 0, len(m))
		for _, a := range m {
			list = append(list, a)
		}
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].hosts != list[j].hosts {
				return list[i].hosts > list[j].hosts
			}
			if list[i].items != list[j].items {
				return list[i].items > list[j].items
			}
			return list[i].name < list[j].name
		})
		if len(list) > 0 {
			b.WriteString(`<h3>` + esc(tr(lang, "Most affected software", "Perangkat lunak paling terdampak")) + `</h3><table class="def-table"><thead><tr><th>` + esc(tr(lang, "Software", "Perangkat lunak")) + `</th><th>` + esc(tr(lang, "Hosts", "Host")) + `</th><th>` + esc(tr(lang, "Advisories", "Advisori")) + `</th></tr></thead><tbody>`)
			for i, a := range list {
				if i >= 8 {
					break
				}
				b.WriteString(`<tr><td>` + esc(a.name) + `</td><td>` + reportNumber(lang, a.hosts) + `</td><td>` + reportNumber(lang, a.items) + `</td></tr>`)
			}
			b.WriteString(`</tbody></table>`)
		}
	}
	b.WriteString(`</section>`)

	for _, a := range s.Assets {
		if a.Defense == nil {
			continue
		}
		groups := defGroups(a)
		sum := a.Defense
		counts := map[string]int{}
		kev, exposed, reboot, total := 0, 0, 0, 0
		for _, g := range groups {
			for _, it := range g.Items {
				counts[it.Severity]++
				total++
				if it.KEV {
					kev++
				}
				if g.Exposure == "exposed" {
					exposed++
				}
				if it.Reboot {
					reboot++
				}
			}
		}
		b.WriteString(`<section class="defense-host"><h3 class="def-host">` + esc(a.Name) + `</h3>`)
		platform := strings.TrimSpace(sum.OSName + " " + sum.OSVersion)
		if platform != "" {
			b.WriteString(`<p class="muted">` + esc(platform) + `</p>`)
		}
		kpi := func(label string, value int) string {
			return `<div class="kpi"><span class="kpi-label">` + esc(label) + `</span><strong class="kpi-value">` + reportNumber(lang, value) + `</strong></div>`
		}
		b.WriteString(`<div class="kpi-grid">` + kpi(tr(lang, "Advisories", "Advisori"), total) + kpi(sevLabel(lang, "critical"), counts["critical"]) + kpi(sevLabel(lang, "high"), counts["high"]) +
			kpi(tr(lang, "Known exploited", "Dieksploitasi"), kev) + kpi(tr(lang, "Seen by scanner", "Terlihat scanner"), exposed) + kpi(tr(lang, "Software checked", "Software diperiksa"), sum.SoftwareChecked) + `</div>`)
		if sum.Disposition != "complete" || a.Status != "finished" {
			b.WriteString(`<div class="def-warn"><strong>` + esc(tr(lang, "Incomplete result.", "Hasil tidak lengkap.")) + `</strong> ` + esc(tr(lang, "Some data or sources were unavailable. Absence of a finding does not mean the software is safe. See the coverage notes below.", "Sebagian data atau sumber tidak tersedia. Tidak adanya temuan bukan berarti perangkat lunak aman. Lihat catatan cakupan di bawah.")) + `</div>`)
		}
		if reboot > 0 {
			b.WriteString(`<p>` + esc(tr(lang, "A newer kernel is installed but not running; "+reportNumber(lang, reboot)+" advisories are fixed there and need a reboot.", "Kernel yang lebih baru sudah terpasang tetapi belum berjalan; "+reportNumber(lang, reboot)+" advisori sudah diperbaiki di sana dan perlu reboot.")) + `</p>`)
		}
		b.WriteString(`<h4>` + esc(tr(lang, "Vulnerable software", "Perangkat lunak rentan")) + `</h4>`)
		if len(groups) == 0 {
			b.WriteString(`<p class="muted">` + esc(tr(lang, "No known vulnerabilities were found in the software that could be checked.", "Tidak ada kerentanan yang dikenal pada perangkat lunak yang dapat diperiksa.")) + `</p>`)
		}
		for gi, g := range groups {
			if gi >= maxGroupsShown {
				b.WriteString(`<p class="muted">` + esc(tr(lang, reportNumber(lang, len(groups)-gi)+" more software items are in the JSON export.", reportNumber(lang, len(groups)-gi)+" perangkat lunak lain ada di ekspor JSON.")) + `</p>`)
				break
			}
			b.WriteString(`<article class="def-group"><p class="def-title"><strong>` + esc(g.Software) + `</strong> ` + esc(g.Installed) + ` · ` + sevBadge(lang, g.Severity) + `</p>`)
			meta := tr(lang, "Exposure", "Paparan") + ": " + exposureLabel(lang, g.Exposure)
			if g.Ports != "" {
				meta += " (" + g.Ports + ")"
			}
			b.WriteString(`<p class="def-meta">` + fixText(lang, g) + ` · ` + esc(meta) + `</p><table class="def-table"><thead><tr><th>CVE</th><th>` + esc(tr(lang, "Severity", "Tingkat")) + `</th><th>` + esc(tr(lang, "Status", "Status")) + `</th><th>` + esc(tr(lang, "Signals", "Sinyal")) + `</th><th>` + esc(tr(lang, "Advisory (English)", "Advisori (Inggris)")) + `</th></tr></thead><tbody>`)
			for ii, it := range g.Items {
				if ii >= maxItemsShown {
					b.WriteString(`<tr><td colspan="5" class="muted">` + esc(tr(lang, "and "+reportNumber(lang, len(g.Items)-ii)+" more", "dan "+reportNumber(lang, len(g.Items)-ii)+" lainnya")) + `</td></tr>`)
					break
				}
				id := esc(it.CVE)
				if it.URL != "" {
					id = `<a href="` + esc(it.URL) + `">` + id + `</a>`
				}
				b.WriteString(`<tr><td>` + id + `</td><td>` + sevBadge(lang, it.Severity) + `</td><td>` + esc(statusLabel(lang, it.Status)) + `</td><td>` + flags(lang, it) + `</td><td>` + esc(it.Summary) + `</td></tr>`)
			}
			b.WriteString(`</tbody></table></article>`)
		}
		if hard := defHardeningOf(a); len(hard) > 0 {
			b.WriteString(`<h4>` + esc(tr(lang, "Hardening gaps", "Celah hardening")) + `</h4><ul class="def-hard">`)
			for _, h := range hard {
				b.WriteString(`<li>` + sevBadge(lang, h.Severity) + ` <strong>` + esc(h.Title) + `</strong>`)
				if h.Why != "" {
					b.WriteString(` - ` + esc(h.Why))
				}
				b.WriteString(`</li>`)
			}
			b.WriteString(`</ul>`)
		}
		if len(sum.Notes) > 0 {
			b.WriteString(`<h4>` + esc(tr(lang, "Coverage notes", "Catatan cakupan")) + `</h4><ul>`)
			for _, n := range sum.Notes {
				b.WriteString(`<li>` + esc(noteText(lang, n)) + `</li>`)
			}
			b.WriteString(`</ul>`)
		}
		p := sum.Provenance
		var data []string
		if p.Ecosystem != "" {
			data = append(data, esc(p.Ecosystem))
		}
		if p.CollectedAt != nil {
			data = append(data, esc(tr(lang, "collected ", "dikumpulkan ")+p.CollectedAt.UTC().Format("2006-01-02 15:04 UTC")))
		}
		data = append(data, esc(tr(lang, "advisories queried ", "advisori ditanyakan ")+p.QueriedAt.UTC().Format("2006-01-02 15:04 UTC")))
		if p.KEVDate != "" {
			data = append(data, esc("KEV "+p.KEVDate))
		}
		if p.EPSSDate != "" {
			data = append(data, esc("EPSS "+p.EPSSDate))
		}
		b.WriteString(`<p class="muted def-prov">` + esc(tr(lang, "Data: ", "Data: ")) + strings.Join(data, " · ") + `</p></section>`)
	}

	b.WriteString(`<section class="defense-notes"><h4>` + esc(tr(lang, "DefenseBot sources and method", "Sumber dan metode DefenseBot")) + `</h4><p>` + esc(tr(lang,
		"Vulnerabilities come from OSV (osv.dev), which aggregates the Ubuntu and Debian security trackers and the GitHub Advisory Database; some of this data is published under CC-BY 4.0. Exploitation status uses the CISA Known Exploited Vulnerabilities catalogue. Exploit probability uses EPSS from FIRST (first.org/epss). Advisory text is shown in English as published.",
		"Kerentanan berasal dari OSV (osv.dev) yang menggabungkan pelacak keamanan Ubuntu dan Debian serta GitHub Advisory Database; sebagian data ini dipublikasikan dengan lisensi CC-BY 4.0. Status eksploitasi memakai katalog CISA Known Exploited Vulnerabilities. Probabilitas eksploitasi memakai EPSS dari FIRST (first.org/epss). Teks advisori ditampilkan dalam bahasa Inggris seperti yang dipublikasikan.")) + `</p><p>` + esc(tr(lang, "Severity policy: ", "Kebijakan tingkat keparahan: ")+defensebot.SeverityPolicy) + `</p>`)
	if lang == "id" {
		b.WriteString(`<p class="muted">` + esc("Kebijakan di atas ditulis dalam bahasa Inggris: label peringkat advisori dipakai lebih dulu, lalu skor dasar CVSS v3 dari vektor advisori, lalu urgensi distribusi; advisori tanpa peringkat dianggap sedang dan ditandai tidak diberi peringkat.") + `</p>`)
	}
	b.WriteString(`</section>`)
	return template.HTML(b.String())
}
