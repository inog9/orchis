package reporting

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The same trusted template supplies HTML and PDF in both languages. Text is
// localized before execution; organization names and evidence never become keys.
//
//go:embed report-id.json
var reportIDJSON []byte
var reportIDText = func() map[string]string {
	var v map[string]string
	if err := json.Unmarshal(reportIDJSON, &v); err != nil {
		panic(err)
	}
	return v
}()
var reportParts = regexp.MustCompile(`(?s)({{.*?}}|<[^>]*>)`)
var reportWords = regexp.MustCompile(`[A-Za-z]`)
var reportAria = regexp.MustCompile(`aria-label="([^"]+)"`)

func localizedReportSource() string {
	source := reportTemplateSource()
	var out strings.Builder
	cursor := 0
	style := false
	writeText := func(text string) {
		if style || !reportWords.MatchString(text) {
			out.WriteString(text)
			return
		}
		text = html.UnescapeString(text)
		if _, ok := reportIDText[text]; !ok {
			panic("report text lacks Indonesian translation: " + text)
		}
		out.WriteString("{{text " + strconv.Quote(text) + "}}")
	}
	for _, loc := range reportParts.FindAllStringIndex(source, -1) {
		writeText(source[cursor:loc[0]])
		part := source[loc[0]:loc[1]]
		if strings.HasPrefix(part, "<style") {
			style = true
		}
		if part == "</style>" {
			style = false
		}
		if !style && strings.HasPrefix(part, "<") {
			part = reportAria.ReplaceAllStringFunc(part, func(attr string) string {
				v := reportAria.FindStringSubmatch(attr)[1]
				if _, ok := reportIDText[v]; !ok {
					panic("report aria label lacks translation")
				}
				return `aria-label="{{text ` + strconv.Quote(v) + `}}"`
			})
		}
		out.WriteString(part)
		cursor = loc[1]
	}
	writeText(source[cursor:])
	result := strings.Replace(out.String(), `lang="en"`, `lang="{{language}}"`, 1)
	result = strings.ReplaceAll(result, `.CreatedAt.Format "02 January 2006 · 15:04 UTC"`, `date .CreatedAt`)
	for _, field := range []string{"Profile", "Status", "Risk", "SkipReason", "Severity", "Type", "Basis", "Reason", "Remediation", "Comparison", "Change", "Outcome"} {
		result = strings.ReplaceAll(result, "{{."+field+"}}", "{{label ."+field+"}}")
	}
	result = strings.ReplaceAll(result, "{{$severity}}", "{{label $severity}}")
	result = strings.ReplaceAll(result, "{{$f.Severity}}", "{{label $f.Severity}}")
	for _, field := range []string{"Total", "Tested", "Skipped", "Incomplete", "EndpointSummary.AssetsWithData", "EndpointSummary.AssetsTotal", "Totals.New", "Totals.Persistent", "Totals.Resolved", "Totals.Uncompared"} {
		result = strings.ReplaceAll(result, "{{."+field+"}}", "{{number ."+field+"}}")
	}
	result = regexp.MustCompile(`{{(index [^}]+)}}`).ReplaceAllString(result, `{{number ($1)}}`)
	for _, field := range []string{"New", "Persistent", "Resolved", "Uncompared"} {
		result = strings.ReplaceAll(result, "{{$counts."+field+"}}", "{{number $counts."+field+"}}")
	}
	return strings.ReplaceAll(result, "{{label .Comparison}}", "{{comparison .}}")
}
func reportText(language, value string) string {
	if language == "id" {
		if translated, ok := reportIDText[value]; ok {
			return translated
		}
	}
	return value
}
func reportDate(language string, t time.Time) string {
	if language != "id" {
		return t.UTC().Format("02 January 2006 · 15:04 UTC")
	}
	months := []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	t = t.UTC()
	return fmt.Sprintf("%02d %s %04d · %02d:%02d UTC", t.Day(), months[t.Month()-1], t.Year(), t.Hour(), t.Minute())
}
func reportNumber(language string, value int) string {
	s := strconv.Itoa(value)
	separator := ","
	if language == "id" {
		separator = "."
	}
	start := 0
	if strings.HasPrefix(s, "-") {
		start = 1
	}
	for i := len(s) - 3; i > start; i -= 3 {
		s = s[:i] + separator + s[i:]
	}
	return s
}
func reportLabel(language, value string) string {
	if language != "id" {
		return value
	}
	if v, ok := reportIDText[value]; ok {
		return v
	}
	if v, ok := reportLabelID[value]; ok {
		return v
	}
	return value
}

var reportLabelID = map[string]string{
	"identity_changed": "Identitas temuan kedaluwarsa sesuai retensi; buat baseline baru",
	"initial_coverage": "Pemeriksaan pertama yang selesai", "coverage_changed": "Target, alat, atau parameter berubah", "profile_changed": "Profil scan berubah", "ambiguous_source": "Pemeriksaan penghasil sebelumnya tidak diketahui", "scan_not_completed": "Pemeriksaan yang diperlukan tidak selesai", "previous_not_completed": "Pemeriksaan cocok sebelumnya tidak selesai", "no_comparable_baseline": "Tidak ada pemeriksaan cocok sebelumnya",

	"No observed vulnerabilities": "Tidak ada kerentanan yang diamati", "No completed scan coverage": "Tidak ada cakupan scan yang selesai",
	"No data yet": "Belum ada data", "High exposure": "Paparan tinggi", "Elevated exposure": "Paparan meningkat", "Moderate exposure": "Paparan sedang", "Low exposure": "Paparan rendah",
	"running": "berjalan", "finished": "selesai", "canceled": "dibatalkan", "skipped": "dilewati", "failed": "gagal", "incomplete": "tidak lengkap", "success": "berhasil", "failure": "gagal", "timeout": "batas waktu habis",
	"critical": "kritis", "high": "tinggi", "medium": "sedang", "low": "rendah", "info": "informasi", "quick": "cepat", "standard": "standar", "full": "penuh", "web": "web",
	"vulnerability": "kerentanan", "open_port": "port terbuka", "service": "layanan", "estimate": "perkiraan", "new": "baru", "persistent": "masih ada", "resolved": "teratasi", "uncompared": "tidak dibandingkan",
	"Review each finding before remediation. AI advice can be requested as a separate advisory supplement; it does not modify this assessment snapshot.":                       "Tinjau setiap temuan sebelum remediasi. Saran AI dapat diminta sebagai suplemen terpisah; saran tidak mengubah snapshot assessment ini.",
	"Review the evidence for each finding and validate remediation before retesting.":                                                                                          "Tinjau bukti setiap temuan dan validasi remediasi sebelum menguji ulang.",
	"Comparison unavailable: no validated comparable assessment baseline.":                                                                                                     "Perbandingan tidak tersedia: belum ada baseline assessment sebanding yang tervalidasi.",
	"No comparable previous scan. Failed or skipped checks cannot establish resolved findings.":                                                                                "Tidak ada scan sebelumnya yang sebanding. Pemeriksaan gagal atau dilewati tidak dapat membuktikan temuan teratasi.",
	"A vulnerability found on a reachable service could be abused this way. This is an estimate from the finding type; it does not show that the vulnerability was exploited.": "Kerentanan pada layanan yang dapat dijangkau dapat disalahgunakan dengan cara ini. Ini adalah perkiraan berdasarkan jenis temuan; bukan bukti bahwa kerentanan dieksploitasi.",
}

func reportComparison(language string, s Snapshot) string {
	if language != "id" || s.ComparisonDetails == nil {
		return reportLabel(language, s.Comparison)
	}
	c := s.ComparisonDetails
	compared := 0
	for _, check := range c.Checks {
		if check.Comparable {
			compared++
		}
	}
	return fmt.Sprintf("Dibandingkan %s pemeriksaan target/alat yang selesai: %s temuan baru, %s masih ada, %s teratasi; %s tidak dibandingkan. Pemeriksaan gagal, dilewati, dihentikan, dan berbeda cakupan tidak pernah membuktikan temuan teratasi.", reportNumber(language, compared), reportNumber(language, c.Totals.New), reportNumber(language, c.Totals.Persistent), reportNumber(language, c.Totals.Resolved), reportNumber(language, c.Totals.Uncompared))
}
