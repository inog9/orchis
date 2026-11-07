// Package injector: pekerja yang menjalankan nmap/nuclei atas tugas dari RabbitMQ. Ia tidak pernah menyentuh database;
// semua lewat API server dengan token bearer, dan server yang memutuskan sasaran serta menyimpan hasil.
package injector

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

	"orchis-go/internal/execution"
)

type Client struct {
	Base  string
	Token string
	HTTP  *http.Client
}

func NewClient(base, token string) *Client {
	return &Client{Base: strings.TrimRight(base, "/"), Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// APIError membawa status HTTP agar pemanggil bisa membedakan "tidak boleh dijalankan" dari "server sedang bermasalah".
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string { return fmt.Sprintf("server menjawab %d: %s", e.Status, e.Body) }

func (c *Client) post(ctx context.Context, path string, in, out any) error {
	var rd io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	} else {
		rd = strings.NewReader("{}")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/api/v1/injector/"+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	limit := int64(1 << 20)
	if strings.HasSuffix(path, "/claim") {
		limit = 64 << 20
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if readErr != nil {
		return readErr
	}
	if int64(len(body)) > limit {
		return fmt.Errorf("server response exceeds bounded claim size")
	}

	if resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	if out != nil && len(body) > 0 {
		return json.Unmarshal(body, out)
	}
	return nil
}

func (c *Client) Heartbeat(ctx context.Context, version string) error {
	return c.post(ctx, "heartbeat", map[string]string{"version": version}, nil)
}

func (c *Client) Claim(ctx context.Context, id uuid.UUID) (*execution.ClaimSpec, error) {
	var spec execution.ClaimSpec
	if err := c.post(ctx, "executions/"+id.String()+"/claim", nil, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

func (c *Client) ExecHeartbeat(ctx context.Context, id uuid.UUID) (cancel bool, err error) {
	var out struct {
		Cancel bool `json:"cancel"`
	}
	err = c.post(ctx, "executions/"+id.String()+"/heartbeat", nil, &out)
	return out.Cancel, err
}

func (c *Client) Result(ctx context.Context, id uuid.UUID, res execution.Result) error {
	return c.post(ctx, "executions/"+id.String()+"/result", res, nil)
}
