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
	"strconv"
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

// cpeDictionary is the reviewed fast path: vendor:product pairs verified to exist in the
// NVD CPE dictionary. Anything not listed here can still be mapped through the optional
// AI AppMapper, but only after NVD confirms the proposed CPE (see CPEExists).
var cpeDictionary = []cpeRule{
	{Vendor: "anysphere", Product: "cursor", Names: []string{"cursor", "cursor.app"}, Bundles: []string{"com.todesktop.230313mzl4w4u92"}},
	{Vendor: "microsoft", Product: "visual_studio_code", Names: []string{"code", "visual studio code.app", "microsoft visual studio code"}, Bundles: []string{"com.microsoft.vscode"}},
	{Vendor: "brave", Product: "browser", Names: []string{"brave", "brave browser", "brave browser.app"}, Bundles: []string{"com.brave.browser"}},
	{Vendor: "google", Product: "chrome", Names: []string{"google chrome", "google chrome.app"}, Bundles: []string{"com.google.chrome"}},
	{Vendor: "mozilla", Product: "firefox", Names: []string{"firefox", "firefox.app", "mozilla firefox"}, Bundles: []string{"org.mozilla.firefox"}},
	{Vendor: "telegram", Product: "telegram_desktop", Names: []string{"telegram", "telegram.app", "telegram desktop"}, Bundles: []string{"ru.keepcoder.telegram"}},
	{Vendor: "docker", Product: "desktop", Names: []string{"docker", "docker.app", "docker desktop", "docker desktop.app"}, Bundles: []string{"com.docker.docker"}},
	{Vendor: "zoom", Product: "zoom", Names: []string{"zoom", "zoom.us.app", "zoom workplace"}, Bundles: []string{"us.zoom.xos"}},
	{Vendor: "discord", Product: "discord", Names: []string{"discord", "discord.app"}, Bundles: []string{"com.hnc.discord"}},
	{Vendor: "spotify", Product: "spotify", Names: []string{"spotify", "spotify.app"}, Bundles: []string{"com.spotify.client"}},
	// Windows (nama program sudah dinormalisasi tanpa versi/arsitektur). Tiap pasangan diverifikasi ada di kamus CPE NVD.
	{Vendor: "7-zip", Product: "7-zip", Names: []string{"7-zip"}},
	{Vendor: "git-scm", Product: "git", Names: []string{"git"}},
	{Vendor: "wireshark", Product: "wireshark", Names: []string{"wireshark"}},
	{Vendor: "nodejs", Product: "node.js", Names: []string{"node.js", "nodejs"}},
	{Vendor: "apache", Product: "netbeans", Names: []string{"apache netbeans ide", "apache netbeans"}},
	{Vendor: "microsoft", Product: "edge_chromium", Names: []string{"microsoft edge"}},
	{Vendor: "anydesk", Product: "anydesk", Names: []string{"anydesk"}},
	{Vendor: "oracle", Product: "vm_virtualbox", Names: []string{"oracle vm virtualbox", "virtualbox"}},
	{Vendor: "postman", Product: "postman", Names: []string{"postman"}},
	{Vendor: "portswigger", Product: "burp_suite", Names: []string{"burp suite", "burp suite community edition", "burp suite professional"}},
	{Vendor: "videolan", Product: "vlc_media_player", Names: []string{"vlc media player", "vlc"}},
	{Vendor: "putty", Product: "putty", Names: []string{"putty", "putty release"}},
	{Vendor: "rarlab", Product: "winrar", Names: []string{"winrar"}},
	{Vendor: "filezilla-project", Product: "filezilla_client", Names: []string{"filezilla", "filezilla client"}},
	{Vendor: "python", Product: "python", Names: []string{"python"}},
	{Vendor: "audacityteam", Product: "audacity", Names: []string{"audacity"}},
	{Vendor: "teamviewer", Product: "teamviewer", Names: []string{"teamviewer"}},
	{Vendor: "valvesoftware", Product: "steam_client", Names: []string{"steam"}},
	{Vendor: "openvpn", Product: "openvpn", Names: []string{"openvpn"}},
	{Vendor: "oracle", Product: "jdk", Names: []string{"java(tm) se development kit"}},
	{Vendor: "adobe", Product: "flash_player", Names: []string{"adobe flash player", "adobe flash player 11 plugin"}},
	{Vendor: "microsoft", Product: "silverlight", Names: []string{"microsoft silverlight"}},
	{Vendor: "openssl", Product: "openssl", Names: []string{"openssl", "openssl light"}},
	{Vendor: "fortinet", Product: "forticlient", Names: []string{"forticlient", "forticlient vpn"}},
	{Vendor: "whatsapp", Product: "whatsapp", Names: []string{"whatsapp"}},
	{Vendor: "microsoft", Product: "teams", Names: []string{"microsoft teams", "microsoft teams classic"}},
	{Vendor: "microsoft", Product: "onedrive", Names: []string{"microsoft onedrive"}},
	{Vendor: "nmap", Product: "npcap", Names: []string{"npcap"}},
	{Vendor: "wazuh", Product: "wazuh", Names: []string{"wazuh agent"}},
	{Vendor: "rustdesk", Product: "rustdesk", Names: []string{"rustdesk"}},
	{Vendor: "keepassxc", Product: "keepassxc", Names: []string{"keepassxc"}},
	{Vendor: "apachefriends", Product: "xampp", Names: []string{"xampp"}},
	{Vendor: "dropbox", Product: "dropbox", Names: []string{"dropbox"}},
	{Vendor: "winscp", Product: "winscp", Names: []string{"winscp"}},
	{Vendor: "r-project", Product: "r", Names: []string{"r for windows"}},
	{Vendor: "apple", Product: "bonjour", Names: []string{"bonjour"}},
	{Vendor: "linuxfoundation", Product: "osquery", Names: []string{"osquery"}},
	{Vendor: "sleuthkit", Product: "autopsy", Names: []string{"autopsy"}},
	{Vendor: "avg", Product: "secure_browser", Names: []string{"avg secure browser"}},
	{Vendor: "graphviz", Product: "graphviz", Names: []string{"graphviz"}},
	{Vendor: "tonec", Product: "internet_download_manager", Names: []string{"internet download manager"}},
	{Vendor: "expressvpn", Product: "expressvpn", Names: []string{"expressvpn"}},
	{Vendor: "anthropic", Product: "claude_desktop", Names: []string{"claude"}},
}

var versionRe = regexp.MustCompile(`^[0-9][0-9A-Za-z._+-]{0,40}$`)

var cpeNamePartRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

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
			// Helper processes share the main bundle id as a prefix (com.brave.Browser.helper).
			if b != "" && (b == x || strings.HasPrefix(b, x+".")) {
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
	// CacheDir (opsional) menyimpan hasil per produk dan versi di disk supaya bertahan antar scan dan restart worker.
	CacheDir string
	CacheTTL time.Duration

	mu       sync.Mutex
	last     time.Time
	cache    map[string][]NVDVuln
	cpeOK    map[string]bool
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
					CPEMatch []cpeMatch `json:"cpeMatch"`
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
	n.mu.Unlock()
	if v, ok := n.diskGet(cpe); ok {
		n.mu.Lock()
		if n.cache == nil {
			n.cache = map[string][]NVDVuln{}
		}
		n.cache[cpe] = v
		n.mu.Unlock()
		return v, nil
	}
	n.mu.Lock()
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
		// NVD menyimpan konfigurasi CVE sebagai RANGE versi; cpeName dengan versi eksak
		// selalu 0 hasil. Query per produk (tanpa versi), lalu filter range di parseNVD.
		match := fmt.Sprintf("cpe:2.3:a:%s:%s:*:*:*:*:*:*:*:*", vendor, product)
		// Jangan kirim isVulnerable: NVD membalas 404 untuk parameter itu (nilai kosong), dan 404 dulu dibaca "tidak ada CVE".
		// Filter rentan/tidak ditangani parseNVD lewat cpeMatch.Vulnerable.
		q := url.Values{"virtualMatchString": {match}, "resultsPerPage": {"500"}}
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
			// NVD sering membalas 503/429 sesaat: beri jeda yang makin panjang sebelum mencoba lagi.
			backoff := time.Duration(attempt+1) * 3 * time.Second
			if n.Sleep != nil {
				n.Sleep(backoff)
			} else {
				time.Sleep(backoff)
			}
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			// NVD membalas 200 dengan totalResults=0 bila memang tidak ada CVE. 404 berarti permintaan ditolak/tidak dikenal:
			// jangan dibaca "aman" (negatif palsu) dan jangan disimpan di cache.
			last = fmt.Errorf("nvd http 404")
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("nvd http %d", resp.StatusCode)
		}
		var doc nvdResponse
		if err = json.Unmarshal(body, &doc); err != nil {
			return nil, fmt.Errorf("unreadable NVD response: %w", err)
		}
		out := parseNVD(doc, vendor, product, version)
		n.mu.Lock()
		if n.cache == nil {
			n.cache = map[string][]NVDVuln{}
		}
		n.cache[cpe] = out
		n.failures = 0
		n.mu.Unlock()
		n.diskPut(cpe, out)
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

// cpeMatchInRange mengevaluasi apakah versi terpasang masuk dalam range satu cpeMatch
// (versionStart/End Including/Excluding). cpeMatch tanpa batas versi berlaku untuk semua.
type cpeMatch struct {
	Vulnerable bool   `json:"vulnerable"`
	Criteria   string `json:"criteria"`
	StartIncl  string `json:"versionStartIncluding"`
	StartExcl  string `json:"versionStartExcluding"`
	EndIncl    string `json:"versionEndIncluding"`
	EndExcl    string `json:"versionEndExcluding"`
}

var dateVersionRe = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

func cpeMatchInRange(version string, m cpeMatch) bool {
	if !m.Vulnerable {
		return false
	}
	// Batas berupa tanggal ("before 2021-04-17", gaya Steam) tidak bisa dibandingkan dengan nomor versi/build: bukan bukti rentan.
	if !dateVersionRe.MatchString(version) {
		for _, b := range []string{m.StartIncl, m.StartExcl, m.EndIncl, m.EndExcl} {
			if dateVersionRe.MatchString(b) {
				return false
			}
		}
	}
	if m.StartIncl != "" && CompareVersions("npm", version, m.StartIncl) < 0 {
		return false
	}
	if m.StartExcl != "" && CompareVersions("npm", version, m.StartExcl) <= 0 {
		return false
	}
	if m.EndIncl != "" && CompareVersions("npm", version, m.EndIncl) > 0 {
		return false
	}
	if m.EndExcl != "" && CompareVersions("npm", version, m.EndExcl) >= 0 {
		return false
	}
	return true
}

func parseNVD(doc nvdResponse, vendor, product, version string) []NVDVuln {
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
		matched := false
		for _, cfg := range c.Configurations {
			for _, node := range cfg.Nodes {
				for _, m := range node.CPEMatch {
					if !strings.HasPrefix(strings.ToLower(m.Criteria), prefix) {
						continue
					}
					if !criteriaVersionMatches(version, m.Criteria) || !cpeMatchInRange(version, m) {
						continue
					}
					matched = true
					end := m.EndExcl
					if end == "" {
						end = m.EndIncl
					}
					if end != "" && (item.Fixed == "" || CompareVersions("npm", end, item.Fixed) < 0) {
						item.Fixed = end
					}
				}
			}
		}
		if item.CVE != "" && matched {
			out = append(out, item)
		}
	}
	return out
}

// CPEExists reports whether NVD lists any CVE for this vendor:product at all. It is the
// validation gate for AI-proposed mappings: a product NVD does not know can never match,
// so a hallucinated CPE is dropped instead of producing a finding.
func (n *NVD) CPEExists(ctx context.Context, vendor, product string) (bool, error) {
	vendor, product = strings.ToLower(strings.TrimSpace(vendor)), strings.ToLower(strings.TrimSpace(product))
	if !cpeNamePartRe.MatchString(vendor) || !cpeNamePartRe.MatchString(product) {
		return false, nil
	}
	cpe := fmt.Sprintf("cpe:2.3:a:%s:%s:*:*:*:*:*:*:*:*", vendor, product)
	n.mu.Lock()
	if v, ok := n.cpeOK[cpe]; ok {
		n.mu.Unlock()
		return v, nil
	}
	if n.failures >= breakerFailures {
		n.mu.Unlock()
		return false, ErrSourceDown
	}
	n.mu.Unlock()
	n.wait()
	q := url.Values{"virtualMatchString": {cpe}, "resultsPerPage": {"1"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.base()+"/rest/json/cves/2.0?"+q.Encode(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", "orchis-defensebot/1")
	if n.APIKey != "" {
		req.Header.Set("apiKey", n.APIKey)
	}
	hc := n.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		n.mu.Lock()
		n.failures++
		n.mu.Unlock()
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		n.mu.Lock()
		n.failures++
		n.mu.Unlock()
		return false, fmt.Errorf("nvd cpe check: %s", resp.Status)
	}
	var out struct {
		TotalResults int `json:"totalResults"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return false, err
	}
	ok := out.TotalResults > 0
	n.mu.Lock()
	if n.cpeOK == nil {
		n.cpeOK = map[string]bool{}
	}
	n.cpeOK[cpe] = ok
	n.mu.Unlock()
	return ok, nil
}

var (
	progArchRe         = regexp.MustCompile(`(?i)\s*\((x64|x86|x86_64|amd64|arm64|[0-9]+-bit|user|machine)[^)]*\)`)
	progVersionRe      = regexp.MustCompile(`(?i)(\s+v?[0-9]+(\.[0-9]+)*[a-z0-9._+-]*)+$`)
	progBareArchRe     = regexp.MustCompile(`(?i)\s+(x64|x86|x86_64|amd64|arm64|64-bit|32-bit)(\s|$)`)
	progAnyDottedVerRe = regexp.MustCompile(`(?i)\s+v?[0-9]+(\.[0-9]+)+[a-z0-9._+-]*`)
	progLeadVerRe      = regexp.MustCompile(`(?i)\s+(v?[0-9]+(\.[0-9]+)+[a-z0-9._+-]*)`)
)

// normalizeProgramName menghapus penanda arsitektur dan versi dari nama program Windows dan mengembalikan nama bersih (huruf kecil)
// beserta versi yang ditemukan di nama (kosong bila tidak ada). Dipakai agar "7-Zip 26.00 (x64)" cocok dengan entri kamus "7-zip".
func normalizeProgramName(name string) (clean, version string) {
	n := strings.TrimSpace(progBareArchRe.ReplaceAllString(progArchRe.ReplaceAllString(name, ""), " "))
	if m := progLeadVerRe.FindStringSubmatch(n); m != nil {
		version = strings.TrimPrefix(strings.TrimPrefix(m[1], "v"), "V")
	}
	n = progAnyDottedVerRe.ReplaceAllString(n, "") // versi bertitik di tengah nama: "OpenSSL 3.6.1 Light" -> "OpenSSL Light"
	n = strings.TrimSpace(progVersionRe.ReplaceAllString(n, ""))
	return strings.ToLower(strings.Join(strings.Fields(n), " ")), version
}

// breakerOpen melaporkan apakah terlalu banyak kegagalan beruntun sehingga sisa permintaan NVD tidak perlu dicoba.
func (n *NVD) breakerOpen() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.failures >= breakerFailures
}

// pythonCPEVersion: pemasang Windows melaporkan versi MSI ("3.10.6150.0" = 3.10.6 + kode rilis); CPE python memakai "3.10.6".
// Komponen ketiga MSI = micro*1000 + kode rilis (mis. 6150), jadi micro = bagian ketiga / 1000 bila >= 1000.
func pythonCPEVersion(v string) string {
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	if len(parts) == 3 {
		if n, err := strconv.Atoi(parts[2]); err == nil && n >= 1000 {
			parts[2] = strconv.Itoa(n / 1000)
		}
	}
	return strings.Join(parts, ".")
}

// criteriaVersionMatches memeriksa kolom versi di dalam kriteria CPE ("cpe:2.3:a:v:p:<versi>:..."). "*" berarti "versi apa pun"
// (dibatasi oleh rentang versionStart/End), "-" berarti tanpa versi (tidak bisa dibandingkan: tidak cocok), dan nilai spesifik
// hanya cocok bila versi terpasang sama persis. Tanpa ini, entri versi tunggal (mis. 3.2.8) cocok ke SEMUA versi.
func criteriaVersionMatches(installed, criteria string) bool {
	parts := strings.Split(criteria, ":")
	if len(parts) < 6 {
		return false
	}
	switch v := parts[5]; v {
	case "*":
		return true
	case "-", "":
		return false
	default:
		return CompareVersions("npm", installed, v) == 0
	}
}

var (
	guidNameRe  = regexp.MustCompile(`(?i)^\{?[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\}?$`)
	storePkgRe  = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)+$`)
	vcRuntimeRe = regexp.MustCompile(`(?i)visual c\+\+ .*(additional|minimum) runtime`)
)

// noiseProgramPrefixes: komponen sistem Windows yang bukan aplikasi untuk diperiksa kerentanannya satu per satu.
var noiseProgramPrefixes = []string{"windows driver package", "microsoft.", "windows ", "update for ", "security update for ", "hotfix for ",
	"nvidia ", "hp ", "omen ", "xbox", "vs_", "winrt intellisense", "universal crt", "sdk arm64", "microsoft .net native", "microsoft visual c++",
	"microsoft visual studio setup", "microsoft visual studio installer", "microsoft visual studio 2008", "microsoft windows sdk", "microsoft store",
	"microsoft update health", "microsoft bing", "microsoft engagement", "microsoft family", "microsoft gameinput", "microsoft web publishing",
	"microsoft sql server compact", "msdn library", "msi development tools", "kits configuration", "vc runtimes", "intel(r) c++", "application verifier",
	"epic games launcher prerequisites", "launcher prerequisites", "wildtangent", "update installer for", "widgets platform", "directx", "diagnosticshub",
	"holo", "mixedreality", "ncsiuwp", "pinningconfirmation", "capturepicker", "desktopview", "environmentsapp", "roomadjustment", "passthrough",
	"b&amp;o ", "office 16 click-to-run", "mozilla maintenance", "java auto updater", "python launcher", "avg update helper", "udk package"}

var pythonPartRe = regexp.MustCompile(`(?i)^python [0-9.]+ (add to path|core interpreter|development libraries|documentation|executables|standard library|tcl/tk support|test suite|utility scripts|pip bootstrap)`)

// isNoiseProgram menandai entri daftar program yang bukan aplikasi: kosong, GUID mentah, paket Store/AppX ("Publisher.App"),
// paket driver Windows, pembaruan sistem, dan potongan runtime Visual C++ (sudah diwakili paket redistributable-nya).
// Dipanggil hanya untuk program yang TIDAK terpetakan, jadi aplikasi bernama "Node.js" tetap diperiksa lebih dulu.
func isNoiseProgram(name string) bool {
	n := strings.TrimSpace(name)
	if n == "" || guidNameRe.MatchString(n) || vcRuntimeRe.MatchString(n) || pythonPartRe.MatchString(n) {
		return true
	}
	if !strings.Contains(n, " ") && storePkgRe.MatchString(n) {
		return true
	}
	l := strings.ToLower(n)
	for _, p := range noiseProgramPrefixes {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

// commonProgramKeywords: perangkat lunak yang sering dipasang sengaja dan relevan untuk keamanan (peramban, runtime, perkakas
// jarak jauh/VPN, kantor, pengembangan, keamanan). Program tak terpetakan yang cocok dihitung sebagai "umum" (prioritas).
var commonProgramKeywords = []string{"java", "jdk", "jre", "adobe", "flash", "silverlight", "office", "teams", "onedrive", "outlook", "zoom", "whatsapp",
	"telegram", "visual studio", "openssl", "openvpn", "forticlient", "norton", "wireshark", "postman", "xampp", "mysql", "keepass", "npcap", "wazuh",
	"rustdesk", "anydesk", "teamviewer", "slack", "dropbox", "google drive", "docker", "winscp", "filezilla", "7-zip", "vlc", "notepad++", "putty",
	"git", "node", "python", "r for windows", "expressvpn", "hotspot shield", "avg", "internet download manager", "winrar", "balena", "windirstat",
	"graphviz", "autopsy", "ida pro", "webview2", "defender", "android studio", "composer", "osquery", "kiro", "warp", "claude", "tor browser",
	"chrome", "firefox", "edge", "brave", "opera", "safari", "skype", "discord", "spotify", "steam", "epic games", "bonjour", "apple mobile"}

func isCommonProgram(name string) bool {
	l := strings.ToLower(name)
	for _, k := range commonProgramKeywords {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}
