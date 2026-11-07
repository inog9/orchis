package defensebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// CPE-based lookup for software that has no OSV ecosystem (for example Cursor). The NVD API matches the version
// against its own configuration ranges, so no version-range logic is needed here. The mapping from what is installed to
// a CPE is a small reviewed dictionary: nothing is guessed, and software that is not in it is reported as unmapped.

const DefaultNVDURL = "https://services.nvd.nist.gov"

type cpeRule struct {
	Vendor, Product string
	Names           []string // lower-case package or application names
	Bundles         []string // macOS bundle identifiers
}

var cpeDictionary = []cpeRule{
	{Vendor: "anysphere", Product: "cursor", Names: []string{"cursor", "cursor.app"}, Bundles: []string{"com.todesktop.230313mzl4w4u92"}},
	{Vendor: "microsoft", Product: "visual_studio_code", Names: []string{"code", "visual studio code.app"}, Bundles: []string{"com.microsoft.vscode"}},
}

var versionRe = regexp.MustCompile(`^[0-9][0-9A-Za-z._+-]{0,40}$`)

// cpeFor maps an installed item to a CPE vendor and product, or reports that there is no reviewed mapping.
func cpeFor(name, bundle string) (vendor, product string, ok bool) {
	n, b := strings.ToLower(strings.TrimSpace(name)), strings.ToLower(strings.TrimSpace(bundle))
	for _, r := range cpeDictionary {
		for _, x := range r.Names {
			if n == x {
				return r.Vendor, r.Product, true
			}
		}
		for _, x := range r.Bundles {
			if b != "" && b == x {
				return r.Vendor, r.Product, true
			}
		}
	}
	return "", "", false
}

// upstreamVersion removes a Debian epoch and revision: 1:0.45.14-1700 -> 0.45.14.
func upstreamVersion(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.Index(v, ":"); i >= 0 {
		v = v[i+1:]
	}
	if i := strings.LastIndex(v, "-"); i > 0 {
		v = v[:i]
	}
	return v
}

type NVDVuln struct {
	CVE      string
	Summary  string
	Severity string
	Score    float64
	Source   string
	Fixed    string
	URL      string
}

// NVD is a client for the NVD CVE API 2.0 with spacing that respects the public rate limit.
type NVD struct {
	Base   string
	APIKey string
	HTTP   *http.Client
	Sleep  func(time.Duration)
	Now    func() time.Time

	mu       sync.Mutex
	last     time.Time
	cache    map[string][]NVDVuln
	failures int
}

func (n *NVD) base() string {
	if n.Base != "" {
		return n.Base
	}
	return DefaultNVDURL
}

// spacing: 5 requests per 30 s without a key, 50 with one.
func (n *NVD) spacing() time.Duration {
	if n.APIKey != "" {
		return 700 * time.Millisecond
	}
	return 6500 * time.Millisecond
}

func (n *NVD) wait() {
	n.mu.Lock()
	defer n.mu.Unlock()
	now := time.Now
	if n.Now != nil {
		now = n.Now
	}
	if gap := n.spacing() - now().Sub(n.last); gap > 0 && !n.last.IsZero() {
		if n.Sleep != nil {
			n.Sleep(gap)
		} else {
			time.Sleep(gap)
		}
	}
	n.last = now()
}

type nvdResponse struct {
	Vulnerabilities []struct {
		CVE struct {
			ID           string `json:"id"`
			Descriptions []struct {
				Lang  string `json:"lang"`
				Value string `json:"value"`
			} `json:"descriptions"`
			Metrics struct {
				V31 []struct {
					Data struct {
						BaseScore    float64 `json:"baseScore"`
						BaseSeverity string  `json:"baseSeverity"`
					} `json:"cvssData"`
				} `json:"cvssMetricV31"`
				V30 []struct {
					Data struct {
						BaseScore    float64 `json:"baseScore"`
						BaseSeverity string  `json:"baseSeverity"`
					} `json:"cvssData"`
				} `json:"cvssMetricV30"`
			} `json:"metrics"`
			References []struct {
				URL string `json:"url"`
			} `json:"references"`
			Configurations []struct {
				Nodes []struct {
					CPEMatch []struct {
						Vulnerable bool   `json:"vulnerable"`
						Criteria   string `json:"criteria"`
						EndExcl    string `json:"versionEndExcluding"`
					} `json:"cpeMatch"`
				} `json:"nodes"`
			} `json:"configurations"`
		} `json:"cve"`
	} `json:"vulnerabilities"`
}

// Query returns the CVEs that NVD lists for exactly this vendor, product and version.
func (n *NVD) Query(ctx context.Context, vendor, product, version string) ([]NVDVuln, error) {
	if !versionRe.MatchString(version) {
		return nil, nil
	}
	cpe := fmt.Sprintf("cpe:2.3:a:%s:%s:%s:*:*:*:*:*:*:*", vendor, product, version)
	n.mu.Lock()
	if v, ok := n.cache[cpe]; ok {
		n.mu.Unlock()
		return v, nil
	}
	if n.failures >= breakerFailures {
		n.mu.Unlock()
		return nil, ErrSourceDown
	}
	n.mu.Unlock()
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n.wait()
		q := url.Values{"cpeName": {cpe}, "isVulnerable": {""}, "resultsPerPage": {"500"}}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.base()+"/rest/json/cves/2.0?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "orchis-defensebot/1")
		if n.APIKey != "" {
			req.Header.Set("apiKey", n.APIKey)
		}
		cl := n.HTTP
		if cl == nil {
			cl = &http.Client{Timeout: 60 * time.Second}
		}
		resp, err := cl.Do(req)
		if err != nil {
			last = err
			continue
		}
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if rerr != nil {
			last = rerr
			continue
		}
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			last = fmt.Errorf("nvd http %d", resp.StatusCode)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("nvd http %d", resp.StatusCode)
		}
		var doc nvdResponse
		if err = json.Unmarshal(body, &doc); err != nil {
			return nil, fmt.Errorf("unreadable NVD response: %w", err)
		}
		out := parseNVD(doc, vendor, product)
		n.mu.Lock()
		if n.cache == nil {
			n.cache = map[string][]NVDVuln{}
		}
		n.cache[cpe] = out
		n.failures = 0
		n.mu.Unlock()
		return out, nil
	}
	n.mu.Lock()
	n.failures++
	n.mu.Unlock()
	if last == nil {
		last = errors.New("unknown error")
	}
	return nil, fmt.Errorf("%w: %v", ErrSourceDown, last)
}

func parseNVD(doc nvdResponse, vendor, product string) []NVDVuln {
	var out []NVDVuln
	prefix := strings.ToLower(fmt.Sprintf("cpe:2.3:a:%s:%s:", vendor, product))
	for _, v := range doc.Vulnerabilities {
		c := v.CVE
		item := NVDVuln{CVE: strings.ToUpper(c.ID), Severity: "medium", Source: "unrated"}
		for _, d := range c.Descriptions {
			if d.Lang == "en" {
				item.Summary = d.Value
				break
			}
		}
		if len([]rune(item.Summary)) > 200 {
			item.Summary = string([]rune(item.Summary)[:200]) + "…"
		}
		switch {
		case len(c.Metrics.V31) > 0:
			item.Score = c.Metrics.V31[0].Data.BaseScore
			item.Severity, item.Source = SeverityFromScore(item.Score), "CVSS v3 base score (NVD)"
		case len(c.Metrics.V30) > 0:
			item.Score = c.Metrics.V30[0].Data.BaseScore
			item.Severity, item.Source = SeverityFromScore(item.Score), "CVSS v3 base score (NVD)"
		}
		for _, r := range c.References {
			if strings.HasPrefix(r.URL, "https://") {
				item.URL = r.URL
				break
			}
		}
		for _, cfg := range c.Configurations {
			for _, node := range cfg.Nodes {
				for _, m := range node.CPEMatch {
					if m.Vulnerable && m.EndExcl != "" && strings.HasPrefix(strings.ToLower(m.Criteria), prefix) {
						if item.Fixed == "" || CompareVersions("npm", m.EndExcl, item.Fixed) < 0 {
							item.Fixed = m.EndExcl
						}
					}
				}
			}
		}
		if item.CVE != "" {
			out = append(out, item)
		}
	}
	return out
}
