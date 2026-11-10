package reporting

import (
	"fmt"
	"html/template"
	"sort"
	"strings"
)

// RemediationItem adalah satu langkah perbaikan deterministik yang diturunkan dari temuan
// di snapshot beku (tanpa AI): perangkat lunak yang terpengaruh beserta versi perbaikannya.
type RemediationItem struct {
	Asset     string
	Software  string
	Installed string
	Fixed     string
	Severity  string
	CVEs      string
}

// RemediationItems mengumpulkan saran perbaikan dari seluruh aset di snapshot.
func RemediationItems(s Snapshot) []RemediationItem {
	var out []RemediationItem
	for _, a := range s.Assets {
		for _, g := range defGroups(a) {
			cves := []string{}
			for _, it := range g.Items {
				if it.CVE != "" {
					cves = append(cves, it.CVE)
				}
			}
			out = append(out, RemediationItem{Asset: a.Name, Software: g.Software, Installed: g.Installed, Fixed: g.Fixed, Severity: g.Severity, CVEs: strings.Join(cves, ", ")})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if sevOrder[a.Severity] != sevOrder[b.Severity] {
			return sevOrder[a.Severity] > sevOrder[b.Severity]
		}
		if a.Asset != b.Asset {
			return a.Asset < b.Asset
		}
		return a.Software < b.Software
	})
	return out
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// remediationHTML merender tabel perbaikan dari snapshot beku. Bagian Remediation pada
// laporan sebelumnya hanya berisi kalimat statis; kini juga memuat daftar upgrade
// konkret (paket + versi terpasang -> versi perbaikan) bila tersedia.
func remediationHTML(lang string, s Snapshot) template.HTML {
	items := RemediationItems(s)
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<table class="remediation-table"><thead><tr><th>` + esc(tr(lang, "Asset", "Aset")) + `</th><th>` + esc(tr(lang, "Software", "Perangkat lunak")) + `</th><th>` + esc(tr(lang, "Severity", "Severitas")) + `</th><th>` + esc(tr(lang, "Action", "Tindakan")) + `</th><th>` + esc(tr(lang, "Advisories", "Advisori")) + `</th></tr></thead><tbody>`)
	for _, it := range items {
		action := tr(lang, "No fixed version yet — monitor the advisory", "Belum ada versi perbaikan — pantau advisori")
		if it.Fixed != "" {
			action = fmt.Sprintf(tr(lang, "Upgrade %s from %s to %s", "Naikkan versi %s dari %s ke %s"), it.Software, orDash(it.Installed), it.Fixed)
		}
		b.WriteString("<tr><td>" + esc(it.Asset) + "</td><td>" + esc(it.Software) + "</td><td>" + esc(it.Severity) + "</td><td>" + esc(action) + "</td><td>" + esc(it.CVEs) + "</td></tr>")
	}
	b.WriteString("</tbody></table>")
	return template.HTML(b.String())
}
