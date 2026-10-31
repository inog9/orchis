package reporting

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

func (s *Service) renderRemote(ctx context.Context, html []byte) ([]byte, error) {
	u, e := url.Parse(s.RendererURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || len(s.RendererToken) < 32 || len(html) > 16<<20 {
		return nil, errors.New("invalid renderer configuration or input")
	}
	run, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(run, "POST", s.RendererURL, bytes.NewReader(html))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+s.RendererToken)
	req.Header.Set("Content-Type", "text/html; charset=utf-8")
	tr := &http.Transport{ResponseHeaderTimeout: 55 * time.Second}
	defer tr.CloseIdleConnections()
	c := &http.Client{Transport: tr, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := c.Do(req)
	if e != nil {
		return nil, errors.New("renderer transport failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("renderer rejected request")
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, (32<<20)+1))
	if e != nil || len(b) > 32<<20 || !bytes.HasPrefix(b, []byte("%PDF-")) {
		return nil, errors.New("renderer returned invalid or oversized PDF")
	}
	return b, nil
}
