package agents

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"orchis-go/internal/agentproto"
)

// Target = satu biner rilis untuk satu platform (nama Go: linux/amd64, darwin/arm64, ...).
type Target struct {
	File      string `json:"file"`      // nama berkas di direktori rilis (tanpa path)
	SHA256    string `json:"sha256"`    // heks
	Signature string `json:"signature"` // Ed25519 base64 atas agentproto.SignedMessage
}

type Manifest struct {
	Version string            `json:"version"`
	Targets map[string]Target `json:"targets"`
}

// Release memuat manifest rilis agent dari sebuah direktori dan memuat ulang bila berkasnya berubah (tanpa restart server).
// Server hanya MENYAJIKAN rilis; agent yang memverifikasi tanda tangannya terhadap kunci publik yang tertanam di biner.
type Release struct {
	Dir string

	mu    sync.Mutex
	mtime time.Time
	m     *Manifest
	err   error
}

var errNoRelease = errors.New("no agent release is available")

func (r *Release) Manifest() (*Manifest, error) {
	if r == nil || r.Dir == "" {
		return nil, errNoRelease
	}
	path := filepath.Join(r.Dir, "manifest.json")
	st, err := os.Stat(path)
	if err != nil {
		return nil, errNoRelease
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m != nil && st.ModTime().Equal(r.mtime) {
		return r.m, nil
	}
	if r.err != nil && st.ModTime().Equal(r.mtime) {
		return nil, r.err
	}
	r.mtime = st.ModTime()
	m, err := r.load(path)
	r.m, r.err = m, err
	return m, err
}

func (r *Release) load(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("manifest rilis rusak: %w", err)
	}
	if !agentproto.ValidVersion(m.Version) {
		return nil, fmt.Errorf("versi rilis %q tidak valid", m.Version)
	}
	for k, t := range m.Targets {
		if t.File == "" || t.File != filepath.Base(t.File) || t.File == "." || t.File == ".." {
			return nil, fmt.Errorf("berkas rilis %q tidak valid untuk %s", t.File, k)
		}
		if raw, err := hex.DecodeString(t.SHA256); err != nil || len(raw) != 32 {
			return nil, fmt.Errorf("sha256 tidak valid untuk %s", k)
		}
		if t.Signature == "" {
			return nil, fmt.Errorf("rilis %s tidak bertanda tangan", k)
		}
		if _, err := os.Stat(filepath.Join(r.Dir, t.File)); err != nil {
			return nil, fmt.Errorf("berkas rilis %s tidak ada", t.File)
		}
	}
	return &m, nil
}

// Find mengembalikan target untuk platform database (linux|windows|macos, x86_64|arm64).
func (m *Manifest) Find(os, arch string) (Target, bool) {
	goos, goarch, ok := agentproto.GoNames(os, arch)
	if !ok {
		return Target{}, false
	}
	t, ok := m.Targets[goos+"/"+goarch]
	return t, ok
}

func (r *Release) Path(t Target) string { return filepath.Join(r.Dir, t.File) }
