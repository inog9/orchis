// ai-worker memanggil provider dari container; tidak mempunyai executor shell atau akses target.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"orchis-go/internal/ai"
	"orchis-go/internal/envfile"
	"orchis-go/internal/telemetry"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if e := run(); e != nil {
		slog.Error("AI worker stopped: configuration or transport failure")
		os.Exit(1)
	}
}
func run() error {
	if err := envfile.Load(); err != nil {
		return err
	}
	base := strings.TrimRight(os.Getenv("ORCHIS_SERVER_URL"), "/")
	u, e := url.Parse(base)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return errors.New("invalid server")
	}
	token := os.Getenv("ORCHIS_AI_WORKER_TOKEN")
	if len(token) < 32 {
		return errors.New("worker token is required")
	}
	provider, e := ai.NewProvider(os.Getenv("ORCHIS_AI_PROXY"), os.Getenv("ORCHIS_AI_TEST_URL"))
	if e != nil {
		return e
	}
	if provider.TestURL == "" && os.Getenv("ORCHIS_AI_PROXY") == "" {
		return errors.New("production requires an egress proxy")
	}
	client := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 8 * time.Second}, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	call := func(ctx context.Context, path string, in, out any) (int, error) {
		b, _ := json.Marshal(in)
		req, e := http.NewRequestWithContext(ctx, "POST", base+path, bytes.NewReader(b))
		if e != nil {
			return 0, e
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "orchis-web")
		res, e := client.Do(req)
		if e != nil {
			return 0, e
		}
		defer res.Body.Close()
		if res.StatusCode == 200 && out != nil {
			e = json.NewDecoder(io.LimitReader(res.Body, 256*1024)).Decode(out)
		}
		return res.StatusCode, e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	metrics, e := telemetry.Start(ctx)
	if e != nil {
		return e
	}
	for ctx.Err() == nil {
		var claim ai.Claim
		status, e := call(ctx, "/api/v1/ai-worker/claim", nil, &claim)
		if e == nil && (status == 200 || status == 204) {
			metrics.Heartbeat()
		}
		if e != nil || status != 200 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
				continue
			}
		}
		jobctx, stop := context.WithTimeout(ctx, 60*time.Second)
		done := make(chan struct{})
		go func() {
			defer close(done)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-jobctx.Done():
					return
				case <-ticker.C:
					var check struct {
						Active bool `json:"active"`
					}
					status, e := call(jobctx, "/api/v1/ai-worker/jobs/"+claim.Job.ID.String()+"/check", map[string]string{"token": claim.Token}, &check)
					if e != nil || status != 200 || !check.Active {
						stop()
						return
					}
				}
			}
		}()
		var check struct {
			Active bool `json:"active"`
		}
		status, e = call(jobctx, "/api/v1/ai-worker/jobs/"+claim.Job.ID.String()+"/check", map[string]string{"token": claim.Token}, &check)
		result := ai.Completion{Token: claim.Token}
		if e == nil && status == 200 && check.Active {
			result.Output, result.UsageTokens, e = provider.Generate(jobctx, claim)
		} else {
			e = errors.New("consent revoked")
		}
		stop()
		<-done
		metrics.Add("orchis_ai_completed_total", 1)
		if e != nil {
			metrics.Add("orchis_ai_failures_total", 1)
			result.Output = nil
			result.Failure = "provider_failed"
		}
		// Satu completion saja; koneksi terputus tidak menyebabkan panggilan provider diulang.
		finish, release := context.WithTimeout(ctx, 10*time.Second)
		_, _ = call(finish, "/api/v1/ai-worker/jobs/"+claim.Job.ID.String()+"/complete", result, nil)
		release()
		claim.APIKey = ""
	}
	return nil
}
