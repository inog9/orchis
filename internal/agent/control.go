package agent

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"net/http"
	"orchis-go/internal/agentproto"
	"os"
	"path/filepath"
	"regexp"
)

// Stored privately before starting the fixed helper. No credential or arbitrary
// command/path is passed through process arguments.
type UninstallReceipt struct {
	ID      string `json:"id"`
	Managed bool   `json:"managed"`
}

func (a *Agent) beginUninstall(ctx context.Context, c *agentproto.ControlCommand) error {
	if c.Kind != "uninstall" || c.AgentID != a.Cfg.AgentID {
		return errors.New("invalid control command binding")
	}
	if _, e := uuid.Parse(c.ID); e != nil {
		return errors.New("invalid control command ID")
	}
	if a.StartUninstall == nil {
		_, _ = a.do(ctx, http.MethodPost, "control/ack", agentproto.ControlAck{ID: c.ID, Code: "uninstall_unsupported"}, nil)
		return errors.New("remote uninstall is unsupported by this binary")
	}
	a.Cfg.Uninstall = &UninstallReceipt{ID: c.ID, Managed: a.ServiceManaged}
	if e := a.Cfg.Save(a.ConfigPath); e != nil {
		return e
	}
	if e := a.StartUninstall(); e != nil {
		a.Cfg.Uninstall = nil
		_ = a.Cfg.Save(a.ConfigPath)
		_, _ = a.do(ctx, http.MethodPost, "control/ack", agentproto.ControlAck{ID: c.ID, Code: "uninstall_failed"}, nil)
		return e
	}
	return nil
}

func (a *Agent) setDiagnostic(code string) {
	a.mu.Lock()
	a.diagnosticCode = agentproto.DiagnosticCode(code)
	a.mu.Unlock()
}

var instanceNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

func ValidInstanceName(name string) bool { return name == "" || instanceNameRE.MatchString(name) }

// RemoveOwnedFiles removes exact owned files only, never the installation parent,
// OS osquery packages or a sibling instance. Refuse symlinked cache directories.
// cfg is removed last so an earlier failure leaves a recoverable private receipt.
func (a *Agent) RemoveOwnedFiles() error {
	dir := filepath.Dir(a.Exe)
	if !filepath.IsAbs(a.Exe) || filepath.Clean(a.ConfigPath) != filepath.Join(dir, "agent-config.json") {
		return errors.New("invalid instance paths")
	}
	inst := a.osqueryInstaller()
	bin := inst.BinPath()
	cache := filepath.Join(dir, "osquery")
	versions := []string{}
	if st, e := os.Lstat(cache); e == nil {
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe osquery cache directory")
		}
		entries, e := os.ReadDir(cache)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if !agentproto.ValidVersion(entry.Name()) {
				continue
			}
			versionDir := filepath.Join(cache, entry.Name())
			st, e := os.Lstat(versionDir)
			if e != nil {
				return e
			}
			if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
				return errors.New("unsafe osquery version directory")
			}
			versions = append(versions, versionDir)
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	files := []string{}
	for _, versionDir := range versions {
		files = append(files, filepath.Join(versionDir, filepath.Base(bin)))
	}
	files = append(files, a.Exe+".old", a.Exe, a.ConfigPath+".tmp", a.ConfigPath)
	for _, f := range files {
		if e := os.Remove(f); e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	// Empty directories only; preserve unrelated contents and system packages.
	for _, versionDir := range versions {
		_ = os.Remove(versionDir)
	}
	_ = os.Remove(cache)
	return nil
}

// UninstallAck is deliberately called after physical removal. Retries use only
// in-memory credentials; a lost ACK remains pending on the server.
func (a *Agent) UninstallAck(ctx context.Context, receipt UninstallReceipt, success bool) error {
	code := ""
	if !success {
		code = "uninstall_failed"
	}
	_, e := a.do(ctx, http.MethodPost, "control/ack", agentproto.ControlAck{ID: receipt.ID, Success: success, Code: code}, nil)
	return e
}
