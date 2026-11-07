package defensebot

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultKEVURL  = "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json"
	DefaultEPSSURL = "https://epss.cyentia.com/epss_scores-current.csv.gz"
)

type KEVEntry struct {
	Added      string `json:"added"`
	Ransomware bool   `json:"ransomware"`
}

type EPSSEntry struct {
	Score      float64 `json:"score"`
	Percentile float64 `json:"percentile"`
}

// Enricher downloads the CISA KEV catalogue and the FIRST EPSS scores and caches them in memory.
type Enricher struct {
	KEVURL, EPSSURL string
	HTTP            *http.Client
	TTL             time.Duration
	Now             func() time.Time

	mu       sync.Mutex
	kev      map[string]KEVEntry
	kevDate  string
	kevAt    time.Time
	epss     map[string]EPSSEntry
	epssDate string
	epssAt   time.Time
}

func (e *Enricher) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}
func (e *Enricher) ttl() time.Duration {
	if e.TTL > 0 {
		return e.TTL
	}
	return 12 * time.Hour
}
func (e *Enricher) client() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	return &http.Client{Timeout: 90 * time.Second}
}

func (e *Enricher) get(ctx context.Context, url string, limit int64) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "orchis-defensebot/1")
	resp, err := e.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return struct {
		io.Reader
		io.Closer
	}{io.LimitReader(resp.Body, limit), resp.Body}, nil
}

// KEV returns the known-exploited catalogue (cve -> entry) and the catalogue release date.
func (e *Enricher) KEV(ctx context.Context) (map[string]KEVEntry, string, error) {
	e.mu.Lock()
	if e.kev != nil && e.now().Sub(e.kevAt) < e.ttl() {
		defer e.mu.Unlock()
		return e.kev, e.kevDate, nil
	}
	e.mu.Unlock()
	url := e.KEVURL
	if url == "" {
		url = DefaultKEVURL
	}
	rc, err := e.get(ctx, url, 16<<20)
	if err != nil {
		return nil, "", err
	}
	defer rc.Close()
	var doc struct {
		DateReleased    string `json:"dateReleased"`
		Vulnerabilities []struct {
			CVE        string `json:"cveID"`
			DateAdded  string `json:"dateAdded"`
			Ransomware string `json:"knownRansomwareCampaignUse"`
		} `json:"vulnerabilities"`
	}
	if err = json.NewDecoder(rc).Decode(&doc); err != nil {
		return nil, "", fmt.Errorf("unreadable KEV catalogue: %w", err)
	}
	m := make(map[string]KEVEntry, len(doc.Vulnerabilities))
	for _, v := range doc.Vulnerabilities {
		m[strings.ToUpper(v.CVE)] = KEVEntry{Added: v.DateAdded, Ransomware: strings.EqualFold(v.Ransomware, "Known")}
	}
	if len(m) == 0 {
		return nil, "", fmt.Errorf("KEV catalogue is empty")
	}
	date := doc.DateReleased
	if len(date) >= 10 {
		date = date[:10]
	}
	e.mu.Lock()
	e.kev, e.kevDate, e.kevAt = m, date, e.now()
	e.mu.Unlock()
	return m, date, nil
}

// EPSS returns the exploit prediction scores (cve -> score) and the score date.
func (e *Enricher) EPSS(ctx context.Context) (map[string]EPSSEntry, string, error) {
	e.mu.Lock()
	if e.epss != nil && e.now().Sub(e.epssAt) < e.ttl() {
		defer e.mu.Unlock()
		return e.epss, e.epssDate, nil
	}
	e.mu.Unlock()
	url := e.EPSSURL
	if url == "" {
		url = DefaultEPSSURL
	}
	rc, err := e.get(ctx, url, 64<<20)
	if err != nil {
		return nil, "", err
	}
	defer rc.Close()
	gz, err := gzip.NewReader(rc)
	if err != nil {
		return nil, "", fmt.Errorf("unreadable EPSS file: %w", err)
	}
	defer gz.Close()
	m := map[string]EPSSEntry{}
	date := ""
	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#"):
			for _, part := range strings.Split(strings.TrimPrefix(line, "#"), ",") {
				if k, v, ok := strings.Cut(part, ":"); ok && strings.TrimSpace(k) == "score_date" && len(v) >= 10 {
					date = strings.TrimSpace(v)[:10]
				}
			}
			continue
		case strings.HasPrefix(line, "cve,"):
			continue
		}
		f := strings.Split(line, ",")
		if len(f) < 3 {
			continue
		}
		score, e1 := strconv.ParseFloat(f[1], 64)
		pct, e2 := strconv.ParseFloat(f[2], 64)
		if e1 != nil || e2 != nil {
			continue
		}
		m[strings.ToUpper(f[0])] = EPSSEntry{Score: score, Percentile: pct}
	}
	if err = sc.Err(); err != nil {
		return nil, "", fmt.Errorf("unreadable EPSS file: %w", err)
	}
	if len(m) == 0 {
		return nil, "", fmt.Errorf("EPSS file is empty")
	}
	e.mu.Lock()
	e.epss, e.epssDate, e.epssAt = m, date, e.now()
	e.mu.Unlock()
	return m, date, nil
}
