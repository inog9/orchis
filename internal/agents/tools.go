package agents

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"orchis-go/internal/agentproto"
	"orchis-go/internal/httpx"
)

// Tool = artefak pendamping yang dipasang agent di direktorinya sendiri (mis. osquery). Disajikan dari <rilis>/tools/ dan
// ditandatangani dengan kunci rilis yang sama dengan biner agent (domain pesan berbeda, lihat agentproto.SignedToolMessage).
// Agent memverifikasi tanda tangan DAN hash yang dipatok di kodenya; server hanya menyajikan.
type ToolManifest struct {
	Tools map[string]ToolEntry `json:"tools"`
}

type ToolEntry struct {
	Version string            `json:"version"`
	Targets map[string]Target `json:"targets"` // kunci: goos/goarch
}

var allowedTools = map[string]bool{"osquery": true}

const toolsDir = "tools"

func (r *Release) toolManifest() (*ToolManifest, error) {
	if r == nil || r.Dir == "" {
		return nil, errNoRelease
	}
	b, err := os.ReadFile(filepath.Join(r.Dir, toolsDir, "manifest.json"))
	if err != nil {
		return nil, errNoRelease
	}
	var m ToolManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("manifest alat rusak: %w", err)
	}
	for name, e := range m.Tools {
		if !allowedTools[name] || e.Version == "" {
			return nil, fmt.Errorf("alat %q tidak diizinkan", name)
		}
		for k, t := range e.Targets {
			if t.File == "" || t.File != filepath.Base(t.File) || t.File == "." || t.File == ".." {
				return nil, fmt.Errorf("berkas alat %q tidak valid untuk %s/%s", t.File, name, k)
			}
			if raw, err := hex.DecodeString(t.SHA256); err != nil || len(raw) != 32 || t.Signature == "" {
				return nil, fmt.Errorf("hash/tanda tangan alat tidak valid untuk %s/%s", name, k)
			}
		}
	}
	return &m, nil
}

// Tool mencari artefak alat untuk platform database dan mengembalikan entri, target, dan jalur berkasnya.
func (r *Release) Tool(name, osName, arch string) (ToolEntry, Target, string, error) {
	m, err := r.toolManifest()
	if err != nil {
		return ToolEntry{}, Target{}, "", err
	}
	e, ok := m.Tools[name]
	if !ok {
		return ToolEntry{}, Target{}, "", errors.New("tool not offered")
	}
	goos, goarch, ok := agentproto.GoNames(osName, arch)
	if !ok {
		return ToolEntry{}, Target{}, "", errors.New("unknown platform")
	}
	t, ok := e.Targets[goos+"/"+goarch]
	if !ok {
		return ToolEntry{}, Target{}, "", errors.New("tool not offered for this platform")
	}
	path := filepath.Join(r.Dir, toolsDir, t.File)
	if _, err := os.Stat(path); err != nil {
		return ToolEntry{}, Target{}, "", errors.New("tool file missing")
	}
	return e, t, path, nil
}

// tool menyajikan artefak alat ke agent terautentikasi (oag_) untuk platform milik kredensialnya.
func (h *Handlers) tool(w http.ResponseWriter, r *http.Request, p *Principal) {
	name := r.URL.Query().Get("name")
	if !allowedTools[name] {
		httpx.Write(w, r, httpx.NotFound("Tool"))
		return
	}
	e, t, path, err := h.Svc.Release.Tool(name, p.OS, p.Arch)
	if err != nil {
		httpx.Write(w, r, httpx.NotFound("Tool for this platform"))
		return
	}
	if want := r.URL.Query().Get("version"); want != "" && want != e.Version {
		httpx.Write(w, r, httpx.Conflict("The server now offers "+name+" "+e.Version+", not "+want+"."))
		return
	}
	f, err := os.Open(path)
	if err != nil {
		httpx.Write(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Tool-Name", name)
	w.Header().Set("X-Tool-Version", e.Version)
	w.Header().Set("X-Sha256", t.SHA256)
	w.Header().Set("X-Signature-Ed25519", t.Signature)
	w.Header().Set("Content-Disposition", `attachment; filename="`+t.File+`"`)
	http.ServeContent(w, r, t.File, time.Time{}, f)
}
