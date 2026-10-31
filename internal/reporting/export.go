package reporting

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"html/template"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
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
	for _, key := range []string{"template_id", "template", "matcher_name", "matched_at", "port", "protocol", "service", "product", "version", "cve", "cwe", "mitre"} {
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
func csvCell(s string) string {
	if trimmed := strings.TrimLeftFunc(s, unicode.IsSpace); strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "@") {
		return "'" + s
	}
	return s
}
func CSV(s Snapshot) ([]byte, error) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	if e := w.Write([]string{"asset", "type", "severity", "title", "value", "synthetic"}); e != nil {
		return nil, e
	}
	for _, a := range s.Assets {
		for _, f := range a.Findings {
			if e := w.Write([]string{csvCell(a.Name), f.Type, f.Severity, csvCell(f.Title), csvCell(f.Value), strconv.FormatBool(s.IsDemo)}); e != nil {
				return nil, e
			}
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

var printTemplate = template.Must(template.New("report").Funcs(template.FuncMap{"coverage": Coverage, "timestamp": func(t *time.Time) string {
	if t == nil {
		return "Not started"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}, "severities": func() []string { return []string{"critical", "high", "medium", "low", "info"} }, "evidence": func(v any) string { return fmt.Sprint(v) }}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:; base-uri 'none'; form-action 'none'"><title>{{.Organization}} - Assessment report</title><style>
@page{size:A4;margin:20mm}*{box-sizing:border-box}body{font-family:Arial,sans-serif;color:#172333;margin:0;font-size:13px;line-height:1.5}header{border-top:8px solid #62784e;padding:36px 0 24px}h1{font-size:36px;line-height:1.15}h2{font-size:24px;margin-top:34px;border-bottom:1px solid #dce1e6;padding-bottom:8px}h3{font-size:18px}.eyebrow{letter-spacing:3px;color:#50633f;font-size:12px}.summary{background:#f0f4ec;border-radius:12px;padding:24px}.finding{border:1px solid #dce1e6;padding:18px;margin:16px 0;break-inside:avoid}code,pre{white-space:pre-wrap;overflow-wrap:anywhere;font-size:11px}table{border-collapse:collapse;width:100%;font-size:12px}td,th{padding:8px;text-align:left;border-bottom:1px solid #dce1e6}p,td{overflow-wrap:anywhere}.muted{color:#52606e}.scope{break-inside:avoid}.scope-table{table-layout:fixed}.scope-table th:nth-child(2),.scope-table td:nth-child(2){white-space:nowrap}h1,h2,h3{overflow-wrap:anywhere;break-after:avoid}tr{break-inside:avoid}.method p,.method pre{margin:8px 0;font-size:11px}.method{padding:14px}.finding p{margin:10px 0}.demo-banner{background:#fff4d6;border:2px solid #b45309;color:#7a3a00;font-weight:bold;padding:10px 14px;margin-top:12px;text-align:center}</style></head><body>
{{if .IsDemo}}<div class="demo-banner">{{if .Label}}{{.Label}}{{else}}DEMO / SYNTHETIC DATA - not a real assessment.{{end}}</div>{{end}}<header><p class="eyebrow">ORCHIS · ORGANISATION ASSESSMENT</p><h1>{{.Organization}}</h1><p>{{.CreatedAt.Format "02 January 2006 · 15:04 UTC"}}</p><p>Assessment {{.AssessmentID}} · Profile {{.Profile}} · {{.Status}}</p></header>
<main><h2>Executive summary</h2><div class="summary"><h3>{{.Risk}}</h3><p>{{with coverage .Assets}}{{.Total}} assets in scope · {{.Tested}} fully tested · {{.Skipped}} skipped · {{.Incomplete}} incomplete{{end}}</p><p>Exposure is based on the highest observed vulnerability severity. Failed, canceled and skipped checks do not establish that an asset is safe.</p><table><thead><tr><th>Severity</th><th>Vulnerabilities</th></tr></thead><tbody>{{range $severity:=severities}}<tr><td>{{$severity}}</td><td>{{index $.BySeverity $severity}}</td></tr>{{end}}</tbody></table></div>
<section class="scope"><h2>Assessment scope</h2><table class="scope-table"><colgroup><col style="width:32%"><col style="width:18%"><col style="width:50%"></colgroup><thead><tr><th>Asset</th><th>Coverage</th><th>Reason</th></tr></thead><tbody>{{range .Assets}}<tr><td>{{.Name}}</td><td>{{.Status}}</td><td>{{if .SkipReason}}{{.SkipReason}}{{else if eq .Status "incomplete"}}Some checks did not complete; see methodology.{{end}}</td></tr>{{end}}</tbody></table></section>
<h2>Findings by asset</h2>{{range .Assets}}<section><h3>{{.Name}}</h3>{{range .Findings}}<article class="finding"><p class="eyebrow">{{.Severity}} · {{.Type}}</p><h3>{{.Title}}</h3><p>{{.Value}}</p>{{range .MITRE}}<p class="muted">MITRE ATT&amp;CK ({{.Basis}}): {{.ID}} {{.Name}}. {{.Reason}}</p>{{end}}{{range $key,$value:=.Evidence}}<p><strong>{{$key}}:</strong> {{evidence $value}}</p>{{end}}</article>{{else}}<p class="muted">No findings recorded. Coverage: {{.Status}}. {{.SkipReason}}</p>{{end}}</section>{{end}}
<h2>Remediation</h2><p>{{.Remediation}}</p><h2>Comparison with previous assessment</h2><p>{{.Comparison}}</p>
<h2>Methodology and execution appendix</h2>{{range .Assets}}{{if .Methods}}<h3>{{.Name}}</h3>{{range .Methods}}<div class="finding method"><strong>{{.Action}} · definition v{{.Version}}</strong><p>{{.Target}} · {{.Outcome}}</p>{{if .Error}}<p>{{.Error}}</p>{{end}}{{if .MITRE}}<p>MITRE ATT&amp;CK: {{range .MITRE}}{{.}} {{end}}</p>{{end}}<pre>{{printf "%s" .Params}}</pre><p>{{timestamp .StartedAt}} - {{timestamp .EndedAt}}</p></div>{{end}}{{end}}{{end}}</main></body></html>`))

func HTML(s Snapshot) ([]byte, error) {
	var b bytes.Buffer
	err := printTemplate.Execute(&b, s)
	return b.Bytes(), err
}
