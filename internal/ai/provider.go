package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"
)

func object(props map[string]any) map[string]any {
	required := []string{}
	for k := range props {
		required = append(required, k)
	}
	sort.Strings(required)
	return map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": required}
}
func str() map[string]any { return map[string]any{"type": "string"} }
func OutputSchema(in Input) map[string]any {
	assets, findings := []string{}, []string{}
	for _, a := range in.Assets {
		assets = append(assets, a.ID)
		for _, f := range a.Findings {
			findings = append(findings, f.ID)
		}
	}
	aid, fid := str(), str()
	if len(assets) > 0 {
		aid["enum"] = assets
	}
	if len(findings) > 0 {
		fid["enum"] = findings
	}
	variants := []any{}
	for _, d := range in.Catalog {
		props := map[string]any{}
		for k, p := range d.Schema.Properties {
			v := map[string]any{"type": p.Type}
			if p.ShowWhen != nil {
				v["type"] = []string{p.Type, "null"}
			}
			if len(p.Enum) > 0 {
				v["enum"] = p.Enum
			}
			if p.Type == "array" {
				it := str()
				if len(p.ItemsEnum) > 0 {
					it["enum"] = p.ItemsEnum
				}
				v["items"] = it
			}
			props[k] = v
		}
		variants = append(variants, object(map[string]any{"asset_id": aid, "contract_key": map[string]any{"type": "string", "enum": []string{d.Key}}, "version": map[string]any{"type": "integer", "enum": []int{d.Version}}, "params": object(props), "reason": str()}))
	}
	return object(map[string]any{"summary": str(), "remediations": map[string]any{"type": "array", "maxItems": len(findings), "items": object(map[string]any{"finding_id": fid, "recommendation": str(), "validation": str()})}, "actions": map[string]any{"type": "array", "maxItems": 20, "items": map[string]any{"anyOf": variants}}})
}

type Provider struct {
	Client  *http.Client
	TestURL string
}

// NewProvider menetapkan proxy eksplisit; tidak membaca proxy lingkungan atau mengikuti redirect.
func NewProvider(proxy, testURL string) (*Provider, error) {
	transport := &http.Transport{TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 50 * time.Second}
	if proxy != "" {
		u, e := url.Parse(proxy)
		if e != nil || u.Scheme != "http" || u.Host == "" || u.User != nil {
			return nil, errors.New("invalid AI egress proxy")
		}
		transport.Proxy = http.ProxyURL(u)
	}
	if testURL != "" {
		u, e := url.Parse(testURL)
		if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
			return nil, errors.New("test provider must be an explicit 127.0.0.1 HTTP origin")
		}
	}
	return &Provider{Client: &http.Client{Transport: transport, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, TestURL: testURL}, nil
}
func (p *Provider) Generate(ctx context.Context, c Claim) (json.RawMessage, int, error) {
	if c.Job.Provider != "deepseek" && c.Job.Provider != "openai" {
		return nil, 0, errors.New("unsupported provider")
	}
	if !modelRE.MatchString(c.Job.Model) {
		return nil, 0, errors.New("invalid model")
	}
	endpoint := "https://api.deepseek.com/chat/completions"
	if c.Job.Provider == "openai" {
		endpoint = "https://api.openai.com/v1/responses"
	}
	if p.TestURL != "" {
		if c.Job.Provider == "deepseek" {
			endpoint = p.TestURL + "/chat/completions"
		} else {
			endpoint = p.TestURL + "/v1/responses"
		}
	}
	body, e := ProviderBody(c)
	if e != nil {
		return nil, 0, e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if e != nil {
		return nil, 0, e
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, e := p.Client.Do(req)
	if e != nil {
		return nil, 0, errors.New("AI provider connection failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, 0, errors.New("AI provider rejected request")
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, 128*1024+1))
	if e != nil || len(raw) > 128*1024 {
		return nil, 0, errors.New("AI provider response too large")
	}
	var v struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			Total int `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return nil, 0, errors.New("invalid provider envelope")
	}
	text := ""
	if c.Job.Provider == "deepseek" {
		if len(v.Choices) != 1 || v.Choices[0].Finish != "stop" {
			return nil, 0, errors.New("incomplete provider output")
		}
		text = v.Choices[0].Message.Content
	} else {
		if v.Status != "completed" {
			return nil, 0, errors.New("incomplete provider output")
		}
		for _, out := range v.Output {
			for _, part := range out.Content {
				if part.Type == "refusal" {
					return nil, 0, errors.New("provider refused request")
				}
				if part.Type == "output_text" {
					text += part.Text
				}
			}
		}
	}
	if len(text) == 0 || len(text) > 65536 {
		return nil, 0, errors.New("empty or oversized AI output")
	}
	if !json.Valid([]byte(text)) {
		return nil, 0, errors.New("invalid JSON from AI provider")
	}
	return json.RawMessage(text), v.Usage.Total, nil
}

// ProviderBody juga dipakai saat reservasi anggaran: ukuran wire request dihitung sebelum antre.
func ProviderBody(c Claim) ([]byte, error) {
	input, _ := json.Marshal(c.Job.Input)
	schema := OutputSchema(c.Job.Input)
	if c.Job.Kind == "remediation" {
		schema["properties"].(map[string]any)["actions"].(map[string]any)["maxItems"] = 0
	}
	var payload any
	if c.Job.Provider == "deepseek" {
		schemaJSON, _ := json.Marshal(schema)
		payload = map[string]any{"model": c.Job.Model, "messages": []any{map[string]any{"role": "system", "content": c.Job.Instructions + "\nPurpose: " + c.Job.Kind + ". JSON schema: " + string(schemaJSON)}, map[string]any{"role": "user", "content": string(input)}}, "response_format": map[string]any{"type": "json_object"}, "max_tokens": c.MaxOutputTokens, "stream": false}
	} else {
		payload = map[string]any{"model": c.Job.Model, "store": false, "instructions": c.Job.Instructions + "\nPurpose: " + c.Job.Kind, "input": string(input), "max_output_tokens": c.MaxOutputTokens, "text": map[string]any{"format": map[string]any{"type": "json_schema", "name": "orchis_advice", "strict": true, "schema": schema}}}
	}
	return json.Marshal(payload)
}
