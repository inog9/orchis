package injector

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"orchis-go/internal/recommendations"
)

func (w *Worker) RunRecommendations(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		var response struct {
			Claim *recommendations.Claim `json:"claim"`
		}
		if err := w.Client.post(ctx, "recommendations/claim", nil, &response); err != nil {
			if ctx.Err() == nil {
				slog.Warn("recommendation claim unavailable")
			}
		} else if c := response.Claim; c != nil {
			output, err := runOpenCodeReview(ctx, *c, "opencode")
			completion := recommendations.Completion{Token: c.Token, Output: output, Failed: err != nil}
			// Retrying result delivery is safe; never repeat the model call automatically.
			for attempt := 0; attempt < 3; attempt++ {
				postCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
				err = w.Client.post(postCtx, "recommendations/"+c.ID.String()+"/result", completion, nil)
				cancel()
				if err == nil {
					break
				}
				var apiErr *APIError
				if errors.As(err, &apiErr) && apiErr.Status < 500 {
					break
				}
			}
			if err != nil {
				slog.Warn("recommendation result could not be delivered", "job", c.ID)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func runOpenCodeReview(ctx context.Context, c recommendations.Claim, binary string) (json.RawMessage, error) {
	root, err := os.MkdirTemp("", "orchis-review-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(root)
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(home, ".pi/agent/auth.json"))
	if err != nil {
		return nil, errors.New("OpenCode credential unavailable")
	}
	var auth map[string]struct {
		Key string `json:"key"`
	}
	if err = json.Unmarshal(raw, &auth); err != nil || auth["opencode-go"].Key == "" {
		return nil, errors.New("OpenCode credential unavailable")
	}
	rules := []map[string]string{{"action": "*", "resource": "*", "effect": "deny"}}
	config := map[string]any{"default_agent": "recommendations", "permissions": rules, "agents": map[string]any{"recommendations": map[string]any{"description": "Reviews recorded scan findings without using tools", "mode": "primary", "steps": 2, "system": recommendations.Instructions, "permissions": rules}}}
	body, _ := json.Marshal(config)
	if err = os.WriteFile(filepath.Join(root, "opencode.json"), body, 0600); err != nil {
		return nil, err
	}
	input, err := json.Marshal(c.Input)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(runCtx, binary, "run", "--standalone", "--agent", "recommendations", "--model", recommendations.Model, "--format", "json")
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(recommendations.Instructions + "\n\nOBSERVATIONS JSON:\n" + string(input))
	// A private home excludes installed pentest skills, global MCP servers and old conversations.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "XDG_DATA_HOME=" + filepath.Join(root, "data"), "XDG_CACHE_HOME=" + filepath.Join(root, "cache"), "XDG_STATE_HOME=" + filepath.Join(root, "state"), "OPENCODE_API_KEY=" + auth["opencode-go"].Key}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	stdout, stderr := limitedBuffer{max: 2 << 20}, limitedBuffer{max: 8 << 10}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		return nil, errors.New("OpenCode review failed")
	}
	if stdout.truncated {
		return nil, errors.New("OpenCode output exceeded limit")
	}
	output, err := reviewText(stdout.Bytes())
	if err != nil {
		return nil, err
	}
	if _, err = recommendations.Validate(output, c.Input); err != nil {
		return nil, err
	}
	return output, nil
}
func reviewText(stream []byte) (json.RawMessage, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(stream)))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	var text strings.Builder
	for scanner.Scan() {
		var event struct {
			Type string `json:"type"`
			Part struct {
				Text string `json:"text"`
			} `json:"part"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, errors.New("invalid OpenCode event")
		}
		if event.Type == "error" {
			return nil, errors.New("OpenCode returned an error")
		}
		if event.Type == "text" {
			text.WriteString(event.Part.Text)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	value := strings.TrimSpace(text.String())
	if strings.HasPrefix(value, "```json") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "```json"), "```")
	}
	if !json.Valid([]byte(value)) {
		return nil, errors.New("OpenCode did not return recommendation JSON")
	}
	return json.RawMessage(strings.TrimSpace(value)), nil
}
