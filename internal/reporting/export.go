package reporting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"orchis-go/internal/osquery"
	"regexp"
	"strings"
	"time"
)

var secretRE = regexp.MustCompile(`(?i)(bearer\s+[a-z0-9._~+/=-]+|(?:password|passwd|secret|token|api[_-]?key|authorization|cookie)\s*[=:]\s*[^\s,;&<]+|(?:oat_|oag_|oen_|sk-)[a-z0-9_/-]+)`)
var userinfoRE = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/\s@]+@`)
var accessKeyRE = regexp.MustCompile(`(?:AKIA|ASIA)[A-Z0-9]{16}|eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`)
var emailRE = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)

func Redact(s string, limit int) string {
	if strings.Contains(strings.ToLower(s), "-----begin ") {
		return "[REDACTED KEY MATERIAL]"
	}
	s = userinfoRE.ReplaceAllString(s, "${1}[REDACTED]@")
	s = accessKeyRE.ReplaceAllString(s, "[REDACTED]")
	s = secretRE.ReplaceAllString(s, "[REDACTED]")
	s = emailRE.ReplaceAllString(s, "[REDACTED EMAIL]")
	r := []rune(s)
	if len(r) > limit {
		s = string(r[:limit]) + "…"
	}
	return s
}
func SafeEvidence(in map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"template_id", "template", "matcher_name", "matched_at", "port", "protocol", "service", "product", "version", "cve", "cwe", "mitre", "verdict", "reason", "vector", "score", "remediation", "replay", "reproduction", "impact",
		"package", "ecosystem", "source_package", "installed_version", "fixed_version", "fix_status", "severity_source", "advisory", "advisory_url", "summary",
		"exposure", "exposure_ports", "kev", "kev_added", "epss", "epss_percentile", "reboot_required", "data_as_of", "rule", "method"} {
		if value, ok := in[key]; ok {
			switch v := value.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					out[key] = Redact(v, 1000)
				}
			case float64:
				out[key] = v
			case bool:
				out[key] = v
			}
		}
	}
	return out
}

func newPrintTemplate(language string) *template.Template {
	return template.Must(template.New("report").Funcs(template.FuncMap{
		"comparison": func(s Snapshot) string { return reportComparison(language, s) },
		"text":       func(v string) string { return reportText(language, v) },
		"language":   func() string { return language },
		"label":      func(v string) string { return reportLabel(language, v) },
		"date":       func(t time.Time) string { return reportDate(language, t) },
		"number":     func(v int) string { return reportNumber(language, v) },
		"params": func(raw json.RawMessage) string {
			var out bytes.Buffer
			if err := json.Compact(&out, raw); err != nil {
				return "[invalid parameters]"
			}
			return out.String()
		}, "coverage": Coverage, "timestamp": func(t *time.Time) string {
			if t == nil {
				if language == "id" {
					return "Belum dimulai"
				}
				return "Not started"
			}
			if language == "id" {
				return reportDate(language, *t)
			}
			return t.UTC().Format("2006-01-02 15:04:05 UTC")
		}, "severities": func() []string { return []string{"critical", "high", "medium", "low", "info"} }, "evidence": func(v any) string { return fmt.Sprint(v) }, "ports": osquery.Ints, "more": func(n, limit int) int { return n - limit }, "attr": func(m map[string]any, k string) string {
			if v, ok := m[k]; ok {
				return fmt.Sprint(v)
			}
			return ""
		}}).Parse(localizedReportSource()))
}

var printTemplate = newPrintTemplate("en")
var printTemplateID = newPrintTemplate("id")

func HTML(s Snapshot) ([]byte, error) {
	var b bytes.Buffer
	tmpl := printTemplate
	if s.Language == "id" {
		tmpl = printTemplateID
	}
	err := tmpl.Execute(&b, s)
	return b.Bytes(), err
}
