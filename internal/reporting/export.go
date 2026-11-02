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
var printTemplate = template.Must(template.New("report").Funcs(template.FuncMap{"params": func(raw json.RawMessage) string {
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return "[invalid parameters]"
	}
	return out.String()
}, "coverage": Coverage, "timestamp": func(t *time.Time) string {
	if t == nil {
		return "Not started"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}, "severities": func() []string { return []string{"critical", "high", "medium", "low", "info"} }, "evidence": func(v any) string { return fmt.Sprint(v) }, "ports": osquery.Ints, "more": func(n, limit int) int { return n - limit }, "attr": func(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		return fmt.Sprint(v)
	}
	return ""
}}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:; base-uri 'none'; form-action 'none'"><title>{{.Organization}} - Orchis security report</title>{{if .IsDemo}}<meta name="orchis-document-type" content="demo-assessment">{{end}}<style>
@page{size:A4;margin:18mm 20mm 22mm}*{box-sizing:border-box}body{font-family:Arial,sans-serif;color:#202329;margin:0;font-size:10pt;line-height:1.5}h1,h2,h3{overflow-wrap:anywhere;break-after:avoid}h1{font-size:32pt;line-height:1.12;letter-spacing:-1pt;margin:8mm 0}h2{font-size:18pt;line-height:1.2;margin:10mm 0 5mm;border-bottom:1pt solid #d9dce1;padding-bottom:3mm}h3{font-size:12pt;margin:5mm 0 2mm}.eyebrow{text-transform:uppercase;letter-spacing:1.5pt;color:#805a2b;font-size:8pt;font-weight:bold}.report-cover{min-height:238mm;padding:10mm 8mm;border:1pt solid #e4daca;border-left:5pt solid #c19a5b;background:linear-gradient(135deg,#fbf8f2 82%,#f1e7d5 82%);display:flex;flex-direction:column;justify-content:space-between;break-after:page}.brand{display:flex;align-items:center;gap:3mm;font-size:17pt;letter-spacing:3pt;font-weight:bold}.brand-dot{height:3mm;width:3mm;background:#c19a5b;border-radius:50%;display:inline-block}.cover-title{font-size:15pt;font-weight:normal;color:#50565f}.cover-meta{display:grid;grid-template-columns:1fr 1fr;gap:5mm 7mm;margin-top:12mm}.cover-meta dt{font-size:8pt;text-transform:uppercase;letter-spacing:1pt;color:#626973}.cover-meta dd{margin:1mm 0 0;font-size:10pt;overflow-wrap:anywhere}.cover-note{font-size:9pt;color:#50565f;margin:0}.summary{border:1pt solid #d9dce1;border-top:4pt solid #c19a5b;background:#fbf8f2;padding:6mm}.summary h3{font-size:23pt;line-height:1.2;margin:0 0 4mm}.summary table{margin-top:4mm;background:white}.finding{border:1pt solid #d9dce1;padding:4mm 5mm;margin:4mm 0;break-inside:avoid;box-decoration-break:clone}.finding h3{margin-top:1mm}.finding p{margin:2mm 0}code,pre{white-space:pre-wrap;overflow-wrap:anywhere;font-size:8pt}table{border-collapse:collapse;width:100%;font-size:9pt}thead{display:table-header-group}tr{break-inside:avoid}th{background:#f2f3f5;color:#343a43;font-size:8pt;text-transform:uppercase;letter-spacing:.3pt}td,th{padding:2.5mm 2mm;text-align:left;border-bottom:1pt solid #d9dce1;vertical-align:top}p,td,dd{overflow-wrap:anywhere}ul{margin:0;padding-left:4mm}.muted{color:#59616b}.scope{break-inside:auto}.scope-table,.endpoint-table{table-layout:fixed}.scope-table th:nth-child(2),.scope-table td:nth-child(2){white-space:nowrap}.finding.method{break-inside:avoid;padding:3mm;margin:3mm 0;background:#fafafa}.finding.method p,.finding.method pre{margin:1mm 0;font-size:8pt;line-height:1.4}.demo-banner{background:#fff4d6;border:1pt solid #a56b1c;color:#754813;font-weight:bold;padding:3mm 4mm;margin:5mm 0;text-align:center}.section-note{border-left:3pt solid #c19a5b;padding-left:4mm;color:#50565f}</style></head><body>
<section class="report-cover" aria-label="Orchis report cover"><div><div class="brand" role="img" aria-label="Orchis"><span class="brand-dot"></span><span>ORCHIS</span></div>{{if .IsDemo}}<div class="demo-banner">{{if .Label}}{{.Label}}{{else}}DEMO / SYNTHETIC DATA - not a real assessment.{{end}}</div>{{end}}</div><div><p class="eyebrow">Organisation security report</p><h1>{{.Organization}}</h1><p class="cover-title">Observed exposure, scan coverage<br>and endpoint posture</p><dl class="cover-meta"><div><dt>Launched</dt><dd>{{.CreatedAt.Format "02 January 2006 · 15:04 UTC"}}</dd></div><div><dt>Profile and outcome</dt><dd>{{.Profile}} · {{.Status}}</dd></div><div><dt>{{if .ScanID}}Scan{{else}}Assessment{{end}} reference</dt><dd>{{if .ScanID}}{{.ScanID}}{{else}}{{.AssessmentID}}{{end}}</dd></div><div><dt>Provenance</dt><dd>{{if .IsDemo}}Synthetic demonstration{{else}}Frozen results snapshot{{end}}</dd></div></dl></div><p class="cover-note">Prepared from recorded results for this organisation. Failed, skipped or canceled checks do not establish safety. Share according to your organisation's access and retention policy.</p></section>
<main><h2>Executive summary</h2><div class="summary"><h3>{{.Risk}}</h3><p>{{with coverage .Assets}}{{.Total}} assets in scope · {{.Tested}} fully tested · {{.Skipped}} skipped · {{.Incomplete}} incomplete{{end}}</p><p>Exposure is based on the highest observed vulnerability severity. Failed, canceled and skipped checks do not establish that an asset is safe.</p><table><thead><tr><th>Severity</th><th>Vulnerabilities</th></tr></thead><tbody>{{range $severity:=severities}}<tr><td>{{$severity}}</td><td>{{index $.BySeverity $severity}}</td></tr>{{end}}</tbody></table></div>
<section class="scope"><h2>Scope and coverage</h2><table class="scope-table"><colgroup><col style="width:32%"><col style="width:18%"><col style="width:50%"></colgroup><thead><tr><th>Asset</th><th>Coverage</th><th>Reason</th></tr></thead><tbody>{{range .Assets}}<tr><td>{{.Name}}</td><td>{{.Status}}</td><td>{{if .SkipReason}}{{.SkipReason}}{{else if eq .Status "incomplete"}}Some checks did not complete; see methodology.{{end}}</td></tr>{{end}}</tbody></table></section>
{{if .EndpointSummary}}<section class="scope"><h2>Endpoint posture</h2><p>{{.EndpointSummary.AssetsWithData}} of {{.EndpointSummary.AssetsTotal}} assets reported data from inside the machine (agent and osquery). This is what the machine says about itself; a scan only sees it from outside. Findings from endpoint data: critical {{index .EndpointSummary.BySeverity "critical"}}, high {{index .EndpointSummary.BySeverity "high"}}, medium {{index .EndpointSummary.BySeverity "medium"}}, low {{index .EndpointSummary.BySeverity "low"}}, info {{index .EndpointSummary.BySeverity "info"}}.</p><table class="endpoint-table"><colgroup><col style="width:22%"><col style="width:26%"><col style="width:26%"><col style="width:26%"></colgroup><thead><tr><th>Asset</th><th>System</th><th>Endpoint findings</th><th>Ports: outside vs inside</th></tr></thead><tbody>{{range .Assets}}{{if .Endpoint}}<tr><td>{{.Name}}</td><td>{{attr .Endpoint.Attributes "os_name"}} {{attr .Endpoint.Attributes "os_version"}}{{with attr .Endpoint.Attributes "kernel"}}, kernel {{.}}{{end}}</td><td>{{if .Endpoint.Findings}}<ul>{{range $i, $f := .Endpoint.Findings}}{{if lt $i 8}}<li>{{$f.Severity}}: {{$f.Title}}</li>{{end}}{{end}}{{if gt (len .Endpoint.Findings) 8}}<li>and {{more (len .Endpoint.Findings) 8}} more (see the asset's Endpoint data)</li>{{end}}</ul>{{else}}None.{{end}}</td><td>{{with .Endpoint.Ports}}Seen by both: {{ports .Both}}. Scan only: {{ports .ScanOnly}}. Inside only: {{ports .HostOnly}}.{{else}}No port data.{{end}}</td></tr>{{end}}{{end}}</tbody></table><p class="muted">Differences between the scan and the inside view are expected (firewalls, NAT, proxies, a limited scan profile). They are a prompt to check, not proof of a problem.</p></section>{{end}}
<h2>Findings by target</h2><p class="section-note">Nmap reports observed ports and services. Nuclei reports matches from curated vulnerability templates. Evidence below is limited and redacted; raw tool output is excluded.</p>{{range .Assets}}<section><h3>{{.Name}}</h3>{{range .Findings}}<article class="finding"><p class="eyebrow">{{.Severity}} · {{.Type}}</p><h3>{{.Title}}</h3><p>{{.Value}}</p>{{range .MITRE}}<p class="muted">MITRE ATT&amp;CK ({{.Basis}}): {{.ID}} {{.Name}}. {{.Reason}}</p>{{end}}{{range $key,$value:=.Evidence}}<p><strong>{{$key}}:</strong> {{evidence $value}}</p>{{end}}</article>{{else}}<p class="muted">No findings recorded. Coverage: {{.Status}}. {{.SkipReason}}</p>{{end}}</section>{{end}}
<h2>Remediation</h2><p>{{.Remediation}}</p><h2>Comparable previous coverage</h2><p>{{.Comparison}}</p>
<h2>Methodology and execution appendix</h2>{{range .Assets}}{{if .Methods}}<h3>{{.Name}}</h3>{{range .Methods}}<div class="finding method"><strong>{{.Action}} · definition v{{.Version}}</strong><p>{{.Target}} · {{.Outcome}}</p>{{if .Error}}<p>{{.Error}}</p>{{end}}{{if .MITRE}}<p>MITRE ATT&amp;CK: {{range .MITRE}}{{.}} {{end}}</p>{{end}}<pre>{{params .Params}}</pre><p>{{timestamp .StartedAt}} - {{timestamp .EndedAt}}</p></div>{{end}}{{end}}{{end}}</main></body></html>`))

func HTML(s Snapshot) ([]byte, error) {
	var b bytes.Buffer
	err := printTemplate.Execute(&b, s)
	return b.Bytes(), err
}
