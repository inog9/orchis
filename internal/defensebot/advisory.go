package defensebot

import (
	"encoding/json"
	"sort"
	"strings"
)

// SeverityPolicy is shown in every report so a reader knows where each severity comes from.
const SeverityPolicy = "Advisory severity label first (distribution priority or GitHub advisory rating), then the CVSS v3 base score computed from the advisory vector, then the distribution urgency; advisories without any rating are treated as medium and marked unrated."

// AffectedInfo is what one advisory says about one package in one ecosystem.
type AffectedInfo struct {
	Fixed        []string `json:"fixed,omitempty"`
	Introduced   []string `json:"introduced,omitempty"`
	Availability string   `json:"availability,omitempty"`
	Urgency      string   `json:"urgency,omitempty"`
}

// Advisory is the small part of an OSV record that orchis-defense-agent keeps. OSV records can be very large (hundreds of affected
// releases), so everything not needed for a finding is dropped while parsing.
type Advisory struct {
	ID             string                  `json:"id"`
	CVEs           []string                `json:"cves"`
	Summary        string                  `json:"summary"`
	Severity       string                  `json:"severity"`
	SeveritySource string                  `json:"severity_source"`
	CVSS           float64                 `json:"cvss,omitempty"`
	URL            string                  `json:"url,omitempty"`
	Published      string                  `json:"published,omitempty"`
	Modified       string                  `json:"modified,omitempty"`
	Affected       map[string]AffectedInfo `json:"affected"`
}

type osvRecord struct {
	ID               string   `json:"id"`
	Aliases          []string `json:"aliases"`
	Upstream         []string `json:"upstream"`
	Related          []string `json:"related"`
	Summary          string   `json:"summary"`
	Details          string   `json:"details"`
	Modified         string   `json:"modified"`
	Published        string   `json:"published"`
	DatabaseSpecific struct {
		Severity string `json:"severity"`
	} `json:"database_specific"`
	Severity []struct {
		Type  string `json:"type"`
		Score string `json:"score"`
	} `json:"severity"`
	References []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"references"`
	Affected []struct {
		Package struct {
			Name      string `json:"name"`
			Ecosystem string `json:"ecosystem"`
		} `json:"package"`
		Ranges []struct {
			Type   string              `json:"type"`
			Events []map[string]string `json:"events"`
		} `json:"ranges"`
		EcosystemSpecific struct {
			Availability string `json:"availability"`
			Urgency      string `json:"urgency"`
			Priority     string `json:"priority"`
		} `json:"ecosystem_specific"`
	} `json:"affected"`
}

func affectedKey(ecosystem, name string) string { return ecosystem + "\x00" + name }

// supportedEcosystemPrefix limits what is kept from a record.
func keepEcosystem(e string) bool {
	return strings.HasPrefix(e, "Ubuntu:") || strings.HasPrefix(e, "Debian:") || e == "npm" || e == "PyPI"
}

// ParseAdvisory reduces an OSV record. It never fails on unknown fields.
func ParseAdvisory(raw []byte) (*Advisory, error) {
	var r osvRecord
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	a := &Advisory{ID: r.ID, Modified: r.Modified, Published: r.Published, Affected: map[string]AffectedInfo{}}
	seen := map[string]bool{}
	add := func(id string) {
		id = strings.ToUpper(strings.TrimSpace(id))
		if strings.HasPrefix(id, "UBUNTU-CVE-") {
			id = strings.TrimPrefix(id, "UBUNTU-")
		}
		if strings.HasPrefix(id, "DEBIAN-CVE-") {
			id = strings.TrimPrefix(id, "DEBIAN-")
		}
		if strings.HasPrefix(id, "CVE-") && !seen[id] {
			seen[id] = true
			a.CVEs = append(a.CVEs, id)
		}
	}
	add(r.ID)
	for _, list := range [][]string{r.Aliases, r.Upstream, r.Related} {
		for _, id := range list {
			add(id)
		}
	}
	sort.Strings(a.CVEs)
	a.Summary = strings.TrimSpace(r.Summary)
	if a.Summary == "" {
		a.Summary = strings.TrimSpace(strings.SplitN(r.Details, "\n", 2)[0])
	}
	if len([]rune(a.Summary)) > 200 {
		a.Summary = string([]rune(a.Summary)[:200]) + "…"
	}
	for _, ref := range r.References {
		if strings.HasPrefix(ref.URL, "https://") && (ref.Type == "ADVISORY" || ref.Type == "WEB" || ref.Type == "REPORT") {
			a.URL = ref.URL
			break
		}
	}
	for _, af := range r.Affected {
		if !keepEcosystem(af.Package.Ecosystem) {
			continue
		}
		info := AffectedInfo{Availability: af.EcosystemSpecific.Availability, Urgency: strings.ToLower(firstNonEmpty(af.EcosystemSpecific.Urgency, af.EcosystemSpecific.Priority))}
		for _, rg := range af.Ranges {
			for _, ev := range rg.Events {
				if v := ev["fixed"]; v != "" {
					info.Fixed = append(info.Fixed, v)
				}
				if v := ev["introduced"]; v != "" && v != "0" {
					info.Introduced = append(info.Introduced, v)
				}
			}
		}
		a.Affected[affectedKey(af.Package.Ecosystem, af.Package.Name)] = info
	}
	a.Severity, a.SeveritySource, a.CVSS = recordSeverity(&r)
	return a, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

var labelSeverity = map[string]string{
	"negligible": "info", "unimportant": "info", "low": "low", "moderate": "medium", "medium": "medium",
	"high": "high", "important": "high", "critical": "critical",
}

func recordSeverity(r *osvRecord) (severity, source string, score float64) {
	for _, s := range r.Severity {
		if strings.EqualFold(s.Type, "Ubuntu") {
			if sev, ok := labelSeverity[strings.ToLower(strings.TrimSpace(s.Score))]; ok {
				return sev, "distribution priority", 0
			}
		}
	}
	if sev, ok := labelSeverity[strings.ToLower(strings.TrimSpace(r.DatabaseSpecific.Severity))]; ok {
		cvss := 0.0
		for _, s := range r.Severity {
			if v, ok := CVSS31Score(s.Score); ok {
				cvss = v
			}
		}
		return sev, "GitHub advisory rating", cvss
	}
	for _, s := range r.Severity {
		if v, ok := CVSS31Score(s.Score); ok {
			return SeverityFromScore(v), "CVSS v3 base score", v
		}
	}
	return "", "", 0
}

// SeverityFor returns the severity and its source for one affected package, falling back to the distribution urgency.
func (a *Advisory) SeverityFor(key string) (string, string) {
	if a.Severity != "" {
		return a.Severity, a.SeveritySource
	}
	if info, ok := a.Affected[key]; ok {
		if sev, ok := labelSeverity[info.Urgency]; ok {
			return sev, "distribution urgency"
		}
	}
	return "medium", "unrated"
}

// PrimaryID is the identifier shown to people: the CVE when there is one.
func (a *Advisory) PrimaryID() string {
	if len(a.CVEs) > 0 {
		return a.CVEs[0]
	}
	return a.ID
}
