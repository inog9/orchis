package injector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"orchis-go/internal/defensebot"
)

// appCPEMapper asks an AI endpoint to propose CPE vendor:product pairs for installed apps.
// The defensebot run validates every proposal against NVD before using it, so a wrong guess
// can never become a finding. Disabled unless ORCHIS_DEFENSEBOT_AI_URL is set.
type appCPEMapper struct {
	url   string
	token string
	hc    *http.Client
}

func newAppCPEMapper() defensebot.AppMapper {
	u := strings.TrimSpace(os.Getenv("ORCHIS_DEFENSEBOT_AI_URL"))
	if u == "" {
		return nil
	}
	return &appCPEMapper{url: u, token: os.Getenv("ORCHIS_DEFENSEBOT_AI_TOKEN"), hc: &http.Client{Timeout: 60 * time.Second}}
}

// Expected request:  {"apps":[{"name","bundle_identifier","version"}]}
// Expected response: {"mappings":{"<lower-case name>":"vendor:product"}}
func (m *appCPEMapper) ProposeCPE(ctx context.Context, apps []defensebot.AppRef) (map[string]string, error) {
	body, err := json.Marshal(map[string]any{"apps": apps})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.token != "" {
		req.Header.Set("Authorization", "Bearer "+m.token)
	}
	resp, err := m.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ai mapper: %s", resp.Status)
	}
	var out struct {
		Mappings map[string]string `json:"mappings"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, err
	}
	return out.Mappings, nil
}
