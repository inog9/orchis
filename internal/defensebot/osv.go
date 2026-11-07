package defensebot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// ErrSourceDown means the vulnerability source stopped answering. Callers must report the host as incomplete,
// never as "no vulnerabilities".
var ErrSourceDown = errors.New("vulnerability source is unreachable")

const (
	batchSize       = 400
	maxPages        = 30
	maxRecordBytes  = 16 << 20
	maxBatchBytes   = 32 << 20
	breakerFailures = 6
)

// Pair is the only thing sent to the vulnerability service: an ecosystem, a package name and a version.
type Pair struct {
	Ecosystem string `json:"-"`
	Name      string `json:"-"`
	Version   string `json:"-"`
}

type VulnRef struct {
	ID       string `json:"id"`
	Modified string `json:"modified"`
}

// OSV is a client for the public OSV API with retry, a circuit breaker and caches.
type OSV struct {
	Base        string
	HTTP        *http.Client
	Concurrency int
	TTL         time.Duration
	Now         func() time.Time
	Sleep       func(time.Duration)

	mu       sync.Mutex
	cache    map[string]cachedAdvisory
	failures int
	Requests int // number of HTTP requests actually sent
}

type cachedAdvisory struct {
	a  *Advisory
	at time.Time
}

func (s *OSV) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *OSV) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (s *OSV) base() string {
	if s.Base != "" {
		return s.Base
	}
	return "https://api.osv.dev"
}

func (s *OSV) sleep(d time.Duration) {
	if s.Sleep != nil {
		s.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (s *OSV) tripped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failures >= breakerFailures
}

func (s *OSV) record(ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		s.failures = 0
	} else {
		s.failures++
	}
}

type httpStatusError struct {
	Status int
	Body   string
}

func (e *httpStatusError) Error() string { return fmt.Sprintf("osv http %d", e.Status) }

// do sends one request with bounded retries on network errors, 429 and 5xx.
func (s *OSV) do(ctx context.Context, method, path string, body []byte, limit int64) ([]byte, error) {
	if s.tripped() {
		return nil, ErrSourceDown
	}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			delay += time.Duration(rand.Intn(300)) * time.Millisecond
			s.sleep(delay)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, s.base()+path, rd)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "orchis-defensebot/1")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		s.mu.Lock()
		s.Requests++
		s.mu.Unlock()
		resp, err := s.client().Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, rerr := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		resp.Body.Close()
		if rerr != nil {
			lastErr = rerr
			continue
		}
		if int64(len(data)) > limit {
			s.record(true)
			return nil, errors.New("osv response is too large")
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			s.record(true)
			return data, nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = &httpStatusError{Status: resp.StatusCode}
			if ra, e := strconv.Atoi(resp.Header.Get("Retry-After")); e == nil && ra > 0 && ra <= 30 {
				s.sleep(time.Duration(ra) * time.Second)
			}
			continue
		default:
			s.record(true) // the service answered; the request itself was wrong
			return nil, &httpStatusError{Status: resp.StatusCode, Body: string(data[:min(len(data), 300)])}
		}
	}
	s.record(false)
	if lastErr == nil {
		lastErr = errors.New("unknown error")
	}
	return nil, fmt.Errorf("%w: %v", ErrSourceDown, lastErr)
}

type batchQuery struct {
	Package   batchPackage `json:"package"`
	Version   string       `json:"version"`
	PageToken string       `json:"page_token,omitempty"`
}
type batchPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}
type batchResult struct {
	Vulns         []VulnRef `json:"vulns"`
	NextPageToken string    `json:"next_page_token"`
}

var badIndexRe = regexp.MustCompile(`index (\d+)`)

// QueryBatch asks OSV which advisories apply to each pair. Pairs whose ecosystem OSV does not know are returned in
// unsupported (by ecosystem name) instead of failing the batch.
func (s *OSV) QueryBatch(ctx context.Context, pairs []Pair) (map[Pair][]VulnRef, map[string]bool, error) {
	out := map[Pair][]VulnRef{}
	unsupported := map[string]bool{}
	for start := 0; start < len(pairs); start += batchSize {
		chunk := append([]Pair(nil), pairs[start:min(start+batchSize, len(pairs))]...)
		for len(chunk) > 0 {
			queries := make([]batchQuery, len(chunk))
			for i, p := range chunk {
				queries[i] = batchQuery{Package: batchPackage{Name: p.Name, Ecosystem: p.Ecosystem}, Version: p.Version}
			}
			body, _ := json.Marshal(map[string]any{"queries": queries})
			data, err := s.do(ctx, http.MethodPost, "/v1/querybatch", body, maxBatchBytes)
			var he *httpStatusError
			if errors.As(err, &he) && he.Status == http.StatusBadRequest {
				bad := -1
				if m := badIndexRe.FindStringSubmatch(he.Body); m != nil {
					bad, _ = strconv.Atoi(m[1])
				}
				if bad < 0 || bad >= len(chunk) {
					// Cannot tell which query is wrong: isolate by asking one at a time.
					for _, p := range chunk {
						sub, subUnsupported, subErr := s.queryOne(ctx, p)
						if subErr != nil {
							return out, unsupported, subErr
						}
						for e := range subUnsupported {
							unsupported[e] = true
						}
						if sub != nil {
							out[p] = sub
						}
					}
					chunk = nil
					continue
				}
				eco := chunk[bad].Ecosystem
				unsupported[eco] = true
				kept := chunk[:0]
				for _, p := range chunk {
					if p.Ecosystem != eco {
						kept = append(kept, p)
					}
				}
				chunk = kept
				continue
			}
			if err != nil {
				return out, unsupported, err
			}
			var resp struct {
				Results []batchResult `json:"results"`
			}
			if err = json.Unmarshal(data, &resp); err != nil || len(resp.Results) != len(chunk) {
				return out, unsupported, fmt.Errorf("%w: unreadable batch response", ErrSourceDown)
			}
			for i, r := range resp.Results {
				refs := append([]VulnRef(nil), r.Vulns...)
				token := r.NextPageToken
				for page := 0; token != "" && page < maxPages; page++ {
					more, next, perr := s.page(ctx, chunk[i], token)
					if perr != nil {
						return out, unsupported, perr
					}
					refs = append(refs, more...)
					token = next
				}
				out[chunk[i]] = refs
			}
			chunk = nil
		}
	}
	return out, unsupported, nil
}

func (s *OSV) page(ctx context.Context, p Pair, token string) ([]VulnRef, string, error) {
	q := batchQuery{Package: batchPackage{Name: p.Name, Ecosystem: p.Ecosystem}, Version: p.Version, PageToken: token}
	body, _ := json.Marshal(map[string]any{"queries": []batchQuery{q}})
	data, err := s.do(ctx, http.MethodPost, "/v1/querybatch", body, maxBatchBytes)
	if err != nil {
		return nil, "", err
	}
	var resp struct {
		Results []batchResult `json:"results"`
	}
	if err = json.Unmarshal(data, &resp); err != nil || len(resp.Results) != 1 {
		return nil, "", fmt.Errorf("%w: unreadable page response", ErrSourceDown)
	}
	return resp.Results[0].Vulns, resp.Results[0].NextPageToken, nil
}

func (s *OSV) queryOne(ctx context.Context, p Pair) ([]VulnRef, map[string]bool, error) {
	q := batchQuery{Package: batchPackage{Name: p.Name, Ecosystem: p.Ecosystem}, Version: p.Version}
	body, _ := json.Marshal(map[string]any{"queries": []batchQuery{q}})
	data, err := s.do(ctx, http.MethodPost, "/v1/querybatch", body, maxBatchBytes)
	var he *httpStatusError
	if errors.As(err, &he) && he.Status == http.StatusBadRequest {
		return nil, map[string]bool{p.Ecosystem: true}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var resp struct {
		Results []batchResult `json:"results"`
	}
	if err = json.Unmarshal(data, &resp); err != nil || len(resp.Results) != 1 {
		return nil, nil, fmt.Errorf("%w: unreadable response", ErrSourceDown)
	}
	refs := resp.Results[0].Vulns
	token := resp.Results[0].NextPageToken
	for page := 0; token != "" && page < maxPages; page++ {
		more, next, perr := s.page(ctx, p, token)
		if perr != nil {
			return nil, nil, perr
		}
		refs = append(refs, more...)
		token = next
	}
	return refs, nil, nil
}

// Advisory returns one parsed advisory, using the cache while it is fresher than TTL and unchanged (same modified time).
func (s *OSV) Advisory(ctx context.Context, ref VulnRef) (*Advisory, error) {
	ttl := s.TTL
	if ttl == 0 {
		ttl = 6 * time.Hour
	}
	s.mu.Lock()
	if s.cache == nil {
		s.cache = map[string]cachedAdvisory{}
	}
	if c, ok := s.cache[ref.ID]; ok && s.now().Sub(c.at) < ttl && (ref.Modified == "" || c.a.Modified == ref.Modified) {
		s.mu.Unlock()
		return c.a, nil
	}
	s.mu.Unlock()
	data, err := s.do(ctx, http.MethodGet, "/v1/vulns/"+url.PathEscape(ref.ID), nil, maxRecordBytes)
	if err != nil {
		return nil, err
	}
	a, err := ParseAdvisory(data)
	if err != nil {
		return nil, fmt.Errorf("unreadable advisory %s: %w", ref.ID, err)
	}
	s.mu.Lock()
	s.cache[ref.ID] = cachedAdvisory{a: a, at: s.now()}
	s.mu.Unlock()
	return a, nil
}

// Advisories fetches many advisories concurrently. Advisories that cannot be read are returned in failed; a tripped breaker
// returns ErrSourceDown together with whatever was fetched.
func (s *OSV) Advisories(ctx context.Context, refs []VulnRef) (map[string]*Advisory, []string, error) {
	n := s.Concurrency
	if n <= 0 {
		n = 12
	}
	out := make(map[string]*Advisory, len(refs))
	var failed []string
	var mu sync.Mutex
	var down error
	sem := make(chan struct{}, n)
	var wg sync.WaitGroup
	for _, ref := range refs {
		if ctx.Err() != nil {
			break
		}
		mu.Lock()
		stop := down != nil
		mu.Unlock()
		if stop {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(ref VulnRef) {
			defer wg.Done()
			defer func() { <-sem }()
			a, err := s.Advisory(ctx, ref)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				out[ref.ID] = a
			case errors.Is(err, ErrSourceDown):
				down = err
			default:
				failed = append(failed, ref.ID)
			}
		}(ref)
	}
	wg.Wait()
	if down == nil && ctx.Err() != nil {
		down = ctx.Err()
	}
	return out, failed, down
}
