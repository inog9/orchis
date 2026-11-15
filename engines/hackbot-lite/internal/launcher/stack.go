package launcher

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

func legacyProxy(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "burp") || strings.Contains(name, "caido")
}

// The Docker supervisor owns all browser/proxy children. Native launches leave
// this opt-in unset and retain their existing MCP configuration.
type managedStack struct {
	command *exec.Cmd
	done    chan struct{}
	err     error
	once    sync.Once
}

func configureGori(mcp map[string]any, root string) {
	servers := mcp["mcpServers"].(map[string]any)
	// Docker must never attempt to launch host-only proxy commands. The import
	// compatibility remains intact for native launches.
	for name := range servers {
		if legacyProxy(name) {
			delete(servers, name)
		}
	}
	servers["gori"] = map[string]any{
		"command": "gori", "args": []string{"mcp", "--db", filepath.Join(root, "private", "stack", "gori.db")},
		"env":      map[string]string{"GORI_HOME": filepath.Join(root, "private", "stack", "gori")},
		"exposure": "codemode",
	}
}

func startStack(script, root string, stderr io.Writer, timeout time.Duration) (*managedStack, error) {
	ready := filepath.Join(root, "private", "stack", "ready.json")
	if err := os.Remove(ready); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	command := exec.Command("python3", script, root)
	command.Stdout, command.Stderr = stderr, stderr
	s := &managedStack{command: command, done: make(chan struct{})}
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupts)
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start Docker stack: %w", err)
	}
	go func() { s.err = command.Wait(); close(s.done) }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-interrupts:
			s.stop()
			return nil, fmt.Errorf("Docker stack startup interrupted; session saved")
		case <-s.done:
			return nil, fmt.Errorf("Docker stack exited before readiness (%v); inspect %s", s.err, filepath.Dir(ready))
		case <-timer.C:
			s.stop()
			return nil, fmt.Errorf("Docker stack readiness timed out; inspect %s", filepath.Dir(ready))
		case <-tick.C:
			data, err := os.ReadFile(ready)
			if err != nil {
				continue
			}
			var status struct {
				Ready bool `json:"ready"`
			}
			if json.Unmarshal(data, &status) == nil && status.Ready {
				return s, nil
			}
		}
	}
}

func (s *managedStack) stop() {
	s.once.Do(func() {
		select {
		case <-s.done:
			return
		default:
		}
		_ = s.command.Process.Signal(syscall.SIGTERM)
		select {
		case <-s.done:
		case <-time.After(50 * time.Second):
			_ = s.command.Process.Kill()
			<-s.done
		}
	})
}
