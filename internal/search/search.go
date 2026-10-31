// Package search: klien Elasticsearch tipis (HTTP/JSON). PostgreSQL tetap sumber kebenaran; indeks ini dapat dibangun ulang.
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const FindingsIndex = "orchis-findings"

type Client struct {
	URL   string
	HTTP  *http.Client
	Index string // nama indeks; tes memakai indeks sendiri agar tidak bentrok dengan data pengembangan
}

func New(url string) *Client {
	return &Client{URL: strings.TrimRight(url, "/"), HTTP: &http.Client{Timeout: 10 * time.Second}, Index: FindingsIndex}
}

func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, rd)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return out, resp.StatusCode, err
}

// EnsureIndex membuat indeks finding dengan pemetaan eksplisit bila belum ada (tenant_id wajib ada di setiap kueri).
func (c *Client) EnsureIndex(ctx context.Context) error {
	_, code, err := c.do(ctx, http.MethodHead, "/"+c.Index, nil)
	if err != nil {
		return err
	}
	if code == 200 {
		return nil
	}
	mapping := map[string]any{"mappings": map[string]any{"dynamic": false, "properties": map[string]any{
		"tenant_id":    map[string]string{"type": "keyword"},
		"asset_id":     map[string]string{"type": "keyword"},
		"asset_name":   map[string]string{"type": "text"},
		"type":         map[string]string{"type": "keyword"},
		"severity":     map[string]string{"type": "keyword"},
		"title":        map[string]string{"type": "text"},
		"value":        map[string]any{"type": "text", "fields": map[string]any{"raw": map[string]string{"type": "keyword"}}},
		"last_seen_at": map[string]string{"type": "date"},
	}}}
	body, code, err := c.do(ctx, http.MethodPut, "/"+c.Index, mapping)
	if err != nil {
		return err
	}
	if code >= 300 && !strings.Contains(string(body), "resource_already_exists_exception") {
		return fmt.Errorf("elasticsearch %d: %s", code, body)
	}
	return nil
}

type FindingDoc struct {
	TenantID   uuid.UUID  `json:"tenant_id"`
	AssetID    *uuid.UUID `json:"asset_id,omitempty"`
	AssetName  string     `json:"asset_name,omitempty"`
	Type       string     `json:"type"`
	Severity   string     `json:"severity"`
	Title      string     `json:"title"`
	Value      string     `json:"value"`
	LastSeenAt time.Time  `json:"last_seen_at"`
}

// IndexFinding mengindeks (atau menimpa) satu dokumen. ID dokumen = ID finding, sehingga pengulangan aman.
func (c *Client) IndexFinding(ctx context.Context, id uuid.UUID, d FindingDoc) error {
	body, code, err := c.do(ctx, http.MethodPut, "/"+c.Index+"/_doc/"+id.String(), d)
	if err != nil {
		return err
	}
	if code >= 300 {
		return fmt.Errorf("elasticsearch %d: %s", code, body)
	}
	return nil
}

// Refresh membuat dokumen terbaru langsung dapat dicari (dipakai tes).
func (c *Client) Refresh(ctx context.Context) error {
	_, _, err := c.do(ctx, http.MethodPost, "/"+c.Index+"/_refresh", nil)
	return err
}

// SearchFindings mengembalikan ID finding yang cocok untuk tenant tersebut, paling relevan lebih dulu.
func (c *Client) SearchFindings(ctx context.Context, tenant uuid.UUID, q string, limit int) ([]uuid.UUID, error) {
	req := map[string]any{
		"size": limit, "_source": false,
		"query": map[string]any{"bool": map[string]any{
			"filter": []any{map[string]any{"term": map[string]string{"tenant_id": tenant.String()}}},
			"must": []any{map[string]any{"simple_query_string": map[string]any{
				"query": q, "fields": []string{"title^2", "value", "asset_name", "type", "severity"}, "default_operator": "and",
				"flags": "AND|OR|PHRASE|PREFIX|WHITESPACE", // tanpa FUZZY/NEAR/ESCAPE agar kueri pengguna tetap murah
			}}},
		}},
	}
	body, code, err := c.do(ctx, http.MethodPost, "/"+c.Index+"/_search", req)
	if err != nil {
		return nil, err
	}
	if code >= 300 {
		return nil, fmt.Errorf("elasticsearch %d: %s", code, body)
	}
	var out struct {
		Hits struct {
			Hits []struct {
				ID string `json:"_id"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(out.Hits.Hits))
	for _, h := range out.Hits.Hits {
		if id, err := uuid.Parse(h.ID); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// DeleteIndex menghapus indeks (dipakai tes).
func (c *Client) DeleteIndex(ctx context.Context) error {
	_, _, err := c.do(ctx, http.MethodDelete, "/"+c.Index, nil)
	return err
}

// Ping memeriksa bahwa klaster Elasticsearch menjawab.
func (c *Client) Ping(ctx context.Context) error {
	_, code, err := c.do(ctx, http.MethodGet, "/_cluster/health", nil)
	if err != nil {
		return err
	}
	if code >= 300 {
		return fmt.Errorf("elasticsearch %d", code)
	}
	return nil
}
