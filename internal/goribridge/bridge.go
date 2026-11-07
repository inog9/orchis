package goribridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DB, Events, State, ScopeFile, IngestURL, Gori string
	Poll                                          time.Duration
	HistoryLimit                                  int
	Once                                          bool
	Logger                                        *log.Logger
}

type checkpoint struct {
	Schema  int               `json:"schema"`
	Offset  int64             `json:"eventOffset"`
	Scope   string            `json:"scopeFingerprint"`
	Flows   map[string]string `json:"flows"`
	Digests map[string]bool   `json:"digests"`
}

type Bridge struct {
	config     Config
	checkpoint checkpoint
	client     *http.Client
	// run is injectable for tests; it only executes local read-only Gori CLI calls.
	run   func(context.Context, ...string) ([]byte, error)
	scope *Scope
}

func New(config Config) (*Bridge, error) {
	if config.DB == "" || config.Events == "" || config.State == "" || config.ScopeFile == "" {
		return nil, errors.New("db, events, state and scope-file are required")
	}
	if config.Gori == "" {
		config.Gori = "gori"
	}
	if config.IngestURL == "" {
		config.IngestURL = "http://127.0.0.1:3333/caido-ingest"
	}
	if config.Poll <= 0 {
		config.Poll = time.Second
	}
	if config.HistoryLimit <= 0 {
		config.HistoryLimit = 100000
	}
	if config.Logger == nil {
		config.Logger = log.New(os.Stderr, "gori-jxscout: ", log.LstdFlags)
	}
	// No ProxyFromEnvironment: this private ingestion channel must not loop back
	// through Gori, even if the caller has inherited HTTP_PROXY.
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	b := &Bridge{config: config, client: client, checkpoint: checkpoint{Schema: 1, Flows: map[string]string{}, Digests: map[string]bool{}}}
	if data, err := os.ReadFile(config.State); err == nil {
		if err = json.Unmarshal(data, &b.checkpoint); err != nil {
			return nil, fmt.Errorf("invalid checkpoint: %w", err)
		}
		if b.checkpoint.Schema != 1 || b.checkpoint.Flows == nil || b.checkpoint.Digests == nil {
			return nil, errors.New("invalid checkpoint schema")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	b.run = func(ctx context.Context, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, config.Gori, args...)
		var output limitedBuffer
		cmd.Stdout = &output
		// Do not surface captured credential/error text in diagnostics.
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("Gori CLI failed: %w", err)
		}
		return output.Bytes(), nil
	}
	return b, nil
}

// Large bundles are bounded consistently with Gori's 32 MiB decoded-body cap.
type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 96*1024*1024 {
		return 0, errors.New("Gori export exceeds 96 MiB")
	}
	return b.Buffer.Write(p)
}

func (b *Bridge) save() error {
	if err := os.MkdirAll(filepath.Dir(b.config.State), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(b.checkpoint)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(b.config.State), ".bridge-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), b.config.State)
}

func (b *Bridge) loadScope() (bool, error) {
	data, err := os.ReadFile(b.config.ScopeFile)
	if errors.Is(err, os.ErrNotExist) {
		b.scope = nil
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var state struct {
		Scope *Scope `json:"scope"`
		Mode  string `json:"mode"`
	}
	if err = json.Unmarshal(data, &state); err != nil {
		return false, err
	}
	b.scope = state.Scope
	if state.Mode != "running" {
		b.scope = nil
	}
	if !b.scope.Active(time.Now()) {
		return false, nil
	}
	fingerprint := b.scope.fingerprint()
	if fingerprint == b.checkpoint.Scope {
		return false, nil
	}
	b.checkpoint.Scope = fingerprint
	for id, reason := range b.checkpoint.Flows {
		if reason == "out_of_scope" {
			delete(b.checkpoint.Flows, id)
		}
	}
	return true, b.save()
}

func (b *Bridge) finish(id, verdict string) error {
	b.checkpoint.Flows[id] = verdict
	b.config.Logger.Printf("flow=%s verdict=%s", id, verdict)
	return b.save()
}

func (b *Bridge) process(ctx context.Context, row Row) error {
	id := strconv.FormatInt(row.ID, 10)
	if row.ID <= 0 || b.checkpoint.Flows[id] != "" {
		return nil
	}
	if !b.scope.Allows(row) {
		return b.finish(id, "out_of_scope")
	}
	if row.Status == 0 {
		return nil
	} // in-flight at recovery; completion event retries it
	if !Eligible(row) {
		return b.finish(id, "not_html_or_javascript")
	}
	// This private ingestion channel needs captured source and actor headers, even
	// when the operator has enabled default redaction for shareable exports.
	data, err := b.run(ctx, "run", "show", id, "--db", b.config.DB, "--format", "json", "--no-redact")
	if err != nil {
		return err
	}
	var flow Flow
	if err = json.Unmarshal(data, &flow); err != nil {
		return fmt.Errorf("invalid Gori show export for flow %s: %w", id, err)
	}
	if flow.Flow.ID != row.ID || flow.Flow.URL != row.URL {
		return errors.New("Gori flow identity changed")
	}
	payload, digest, err := MapFlow(flow)
	if err != nil {
		return b.finish(id, "skipped_"+err.Error())
	}
	if b.checkpoint.Digests[digest] {
		return b.finish(id, "duplicate_asset")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// Re-read authorization immediately before delivery; scope changes can stop a
	// slow export from triggering automatic discovery under stale authorization.
	if _, err = b.loadScope(); err != nil {
		return err
	}
	if !b.scope.Allows(row) {
		return errors.New("authorization changed before ingestion; delivery deferred")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.config.IngestURL, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("jxscout ingestion unavailable: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("jxscout ingestion returned HTTP %d", resp.StatusCode)
	}
	b.checkpoint.Digests[digest] = true
	return b.finish(id, "delivered")
}

// Recover includes flows committed just before capture stdout was flushed. Gori
// v0.9.1 has no history cursor: refuse explicitly rather than silently truncate
// a project larger than the configurable metadata recovery bound.
func (b *Bridge) recover(ctx context.Context) error {
	data, err := b.run(ctx, "run", "history", "--db", b.config.DB, "--count", "--format", "json")
	if err != nil {
		return err
	}
	var count struct {
		Count int `json:"count"`
	}
	if err = json.Unmarshal(data, &count); err != nil {
		return err
	}
	if count.Count > b.config.HistoryLimit {
		return fmt.Errorf("history has %d flows; increase --history-limit above %d to recover without truncation", count.Count, b.config.HistoryLimit)
	}
	if count.Count == 0 {
		return nil
	}
	data, err = b.run(ctx, "run", "history", "--db", b.config.DB, "--limit", strconv.Itoa(b.config.HistoryLimit), "--format", "jsonl")
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var row Row
		if err = decoder.Decode(&row); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
		if err = b.process(ctx, row); err != nil {
			return err
		}
	}
}

func (b *Bridge) events(ctx context.Context) error {
	f, err := os.Open(b.config.Events)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() < b.checkpoint.Offset {
		b.checkpoint.Offset = 0
	}
	if _, err = f.Seek(b.checkpoint.Offset, io.SeekStart); err != nil {
		return err
	}
	r := bufio.NewReader(f)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line, err := r.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return nil
		} // retain partial line for the next tick
		if err != nil {
			return err
		}
		var row Row
		if err = json.Unmarshal(line, &row); err != nil {
			return fmt.Errorf("invalid capture event at byte %d", b.checkpoint.Offset)
		}
		if err = b.process(ctx, row); err != nil {
			return err
		}
		b.checkpoint.Offset += int64(len(line))
		if err = b.save(); err != nil {
			return err
		}
	}
}

func (b *Bridge) Run(ctx context.Context) error {
	recovered := false
	delay := b.config.Poll
	for {
		changed, err := b.loadScope()
		if changed {
			recovered = false
		}
		if err == nil && b.scope.Active(time.Now()) {
			if !recovered {
				err = b.recover(ctx)
				if err == nil {
					recovered = true
				}
			}
			if err == nil {
				err = b.events(ctx)
			}
		}
		if b.config.Once {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			b.config.Logger.Print(err)
			delay *= 2
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
		} else {
			delay = b.config.Poll
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// Keep diagnostics safe to parse as a single line when paths are operator supplied.
func CleanError(err error) string {
	return strings.ReplaceAll(strings.ReplaceAll(err.Error(), "\n", " "), "\r", " ")
}
