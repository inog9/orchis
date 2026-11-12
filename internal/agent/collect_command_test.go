package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"orchis-go/internal/agentproto"
	"orchis-go/internal/execution"
	"orchis-go/internal/osquery"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestCollectCommandIsBoundToCatalogAgentAndOS(t *testing.T) {
	id, task := uuid.NewString(), uuid.NewString()
	cases := []struct {
		name string
		c    *agentproto.ControlCommand
		os   string
		want bool
	}{
		{"valid", &agentproto.ControlCommand{ID: task, AgentID: id, Kind: "collect", Queries: []string{"osquery.network_ranges"}}, "windows", true},
		{"valid batch", &agentproto.ControlCommand{ID: task, AgentID: id, Kind: "collect", Queries: []string{"osquery.network_ranges", "osquery.network"}, ExecutionIDs: []string{task, uuid.NewString()}}, "windows", true},
		{"batch missing ids", &agentproto.ControlCommand{ID: task, AgentID: id, Kind: "collect", Queries: []string{"osquery.network_ranges", "osquery.network"}}, "windows", false},
		{"wrong agent", &agentproto.ControlCommand{ID: task, AgentID: uuid.NewString(), Kind: "collect", Queries: []string{"osquery.network_ranges"}}, "windows", false},
		{"empty", &agentproto.ControlCommand{ID: task, AgentID: id, Kind: "collect"}, "linux", false},
		{"sql", &agentproto.ControlCommand{ID: task, AgentID: id, Kind: "collect", Queries: []string{"SELECT * FROM users"}}, "linux", false},
		{"unsupported", &agentproto.ControlCommand{ID: task, AgentID: id, Kind: "collect", Queries: []string{"osquery.packages"}}, "windows", false},
		{"wrong kind", &agentproto.ControlCommand{ID: task, AgentID: id, Kind: "shell", Queries: []string{"osquery.network_ranges"}}, "linux", false},
		{"bad id", &agentproto.ControlCommand{ID: "../bad", AgentID: id, Kind: "collect", Queries: []string{"osquery.network_ranges"}}, "linux", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := validCollectCommand(c.c, id, c.os); got != c.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestCollectCommandClaimsAndReportsUsingExecutionProtocol(t *testing.T) {
	for _, fails := range []bool{false, true} {
		t.Run(fmt.Sprint("failure=", fails), func(t *testing.T) {
			id, task := uuid.NewString(), uuid.New()
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			resultCh := make(chan execution.Result, 1)
			var claims atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/agent/heartbeat":
					json.NewEncoder(w).Encode(map[string]any{"poll_seconds": 2, "command": agentproto.ControlCommand{ID: task.String(), AgentID: id, Kind: "collect", Queries: []string{"osquery.network_ranges"}}})
				case "/api/v1/agent/executions/" + task.String() + "/claim":
					claims.Add(1)
					json.NewEncoder(w).Encode(execution.ClaimSpec{ExecutionID: task, ContractKey: "osquery.network_ranges", ContractVersion: 1, Params: json.RawMessage(`{}`), TimeoutSeconds: 20})
				case "/api/v1/agent/executions/" + task.String() + "/result":
					var out execution.Result
					_ = json.NewDecoder(r.Body).Decode(&out)
					resultCh <- out
					w.WriteHeader(204)
				default:
					w.WriteHeader(204)
				}
			}))
			defer srv.Close()
			a := &Agent{Cfg: &Config{AgentID: id, Secret: "fixture", ServerURL: srv.URL}, Exe: filepath.Join(t.TempDir(), "agent"), Version: "1.0.0", Logf: func(string, ...any) {}}
			a.RunNamedQuery = func(_ context.Context, key string) ([]osquery.Dataset, error) {
				if key != "osquery.network_ranges" {
					t.Errorf("unexpected key %s", key)
				}
				if fails {
					return nil, errors.New("fixture query failed")
				}
				return []osquery.Dataset{{Query: key, Status: osquery.StatusOK}}, nil
			}
			done := make(chan struct{})
			go func() { _ = a.Run(ctx); close(done) }()
			select {
			case got := <-resultCh:
				if claims.Load() != 1 || got.ExitCode == nil || (fails && (*got.ExitCode == 0 || got.Error == "")) || (!fails && (*got.ExitCode != 0 || len(got.Datasets) != 1)) {
					t.Fatalf("bad result: %+v", got)
				}
			case <-ctx.Done():
				t.Fatal("agent failed to report")
			}
			cancel()
			<-done
		})
	}
}

func TestCollectBatchClaimsEachMappedExecution(t *testing.T) {
	id := uuid.NewString()
	tasks := []uuid.UUID{uuid.New(), uuid.New()}
	keys := []string{"osquery.network_ranges", "osquery.network"}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	results := make(chan string, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/agent/heartbeat" {
			json.NewEncoder(w).Encode(map[string]any{"poll_seconds": 2, "command": agentproto.ControlCommand{ID: tasks[0].String(), AgentID: id, Kind: "collect", Queries: keys, ExecutionIDs: []string{tasks[0].String(), tasks[1].String()}}})
			return
		}
		for i, task := range tasks {
			if r.URL.Path == "/api/v1/agent/executions/"+task.String()+"/claim" {
				json.NewEncoder(w).Encode(execution.ClaimSpec{ExecutionID: task, ContractKey: keys[i], ContractVersion: 1, Params: json.RawMessage(`{}`), TimeoutSeconds: 20})
				return
			}
			if r.URL.Path == "/api/v1/agent/executions/"+task.String()+"/result" {
				results <- task.String()
				w.WriteHeader(204)
				return
			}
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	a := &Agent{Cfg: &Config{AgentID: id, Secret: "fixture", ServerURL: srv.URL}, Exe: filepath.Join(t.TempDir(), "agent"), Version: "1.0.0", Logf: func(string, ...any) {}, RunNamedQuery: func(_ context.Context, key string) ([]osquery.Dataset, error) {
		return []osquery.Dataset{{Query: key, Status: osquery.StatusOK}}, nil
	}}
	done := make(chan struct{})
	go func() { _ = a.Run(ctx); close(done) }()
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case v := <-results:
			seen[v] = true
		case <-ctx.Done():
			t.Fatal("batch did not finish")
		}
	}
	cancel()
	<-done
	if !seen[tasks[0].String()] || !seen[tasks[1].String()] {
		t.Fatal("wrong execution mapping")
	}
}
