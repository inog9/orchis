package agent

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"orchis-go/internal/agentproto"
	"orchis-go/internal/osquery"
)

// OsqueryInstaller memasang osquery di DIREKTORI AGENT SENDIRI (mode pengguna; tidak mengubah paket sistem). Artefak dipatok
// dua kali: hash SHA-256 yang tertanam di kode (katalog beku) dan tanda tangan Ed25519 dengan kunci rilis agent. Hanya satu berkas
// diekstrak dari tarball (nama pasti, bukan symlink, batas ukuran), lalu lulus uji asap versi sebelum dipasang secara atomik.
type OsqueryInstaller struct {
	Dir          string // direktori agent
	PubKey       ed25519.PublicKey
	GOOS, GOARCH string
	Pin          func(goos, goarch string) (osquery.Artifact, bool)
	Version      string
	// Fetch mengunduh artefak tertanda tangan: badan, tanda tangan (base64), dan versi yang diklaim server.
	Fetch func(ctx context.Context, version string) (io.ReadCloser, string, string, error)
	Smoke func(ctx context.Context, bin string) (string, error)

	mu sync.Mutex
}

func (i *OsqueryInstaller) goos() string {
	if i.GOOS != "" {
		return i.GOOS
	}
	return runtime.GOOS
}
func (i *OsqueryInstaller) goarch() string {
	if i.GOARCH != "" {
		return i.GOARCH
	}
	return runtime.GOARCH
}
func (i *OsqueryInstaller) pin() (osquery.Artifact, bool) {
	if i.Pin != nil {
		return i.Pin(i.goos(), i.goarch())
	}
	return osquery.PinnedArtifact(i.goos(), i.goarch())
}
func (i *OsqueryInstaller) version() string {
	if i.Version != "" {
		return i.Version
	}
	return osquery.Version
}

// BinPath adalah lokasi biner terpasang untuk versi yang dipatok.
func (i *OsqueryInstaller) BinPath() string {
	name := "osqueryd"
	if i.goos() == "windows" {
		name += ".exe"
	}
	return filepath.Join(i.Dir, "osquery", i.version(), name)
}

func (i *OsqueryInstaller) smoke(ctx context.Context, bin string) (string, error) {
	if i.Smoke != nil {
		return i.Smoke(ctx, bin)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	return strings.TrimSpace(string(out)), err
}

func (i *OsqueryInstaller) installedOK(ctx context.Context) bool {
	st, err := os.Lstat(i.BinPath())
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	out, err := i.smoke(ctx, i.BinPath())
	return err == nil && strings.Contains(out, i.version())
}

// Ensure memastikan biner osquery yang dipatok terpasang dan sehat; memasangnya bila belum. Mengembalikan jalur biner.
func (i *OsqueryInstaller) Ensure(ctx context.Context) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	pin, ok := i.pin()
	if !ok {
		return "", fmt.Errorf("osquery is not available for %s/%s yet", i.goos(), i.goarch())
	}
	if i.installedOK(ctx) {
		return i.BinPath(), nil
	}
	if i.Fetch == nil {
		return "", errors.New("no osquery source configured")
	}
	verDir := filepath.Dir(i.BinPath())
	if err := os.MkdirAll(verDir, 0o700); err != nil {
		return "", err
	}
	body, sigB64, claimed, err := i.Fetch(ctx, i.version())
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer body.Close()
	if claimed != i.version() {
		return "", fmt.Errorf("server offered osquery %q, expected %q: refusing", claimed, i.version())
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return "", errors.New("osquery signature is not valid base64")
	}
	stage, err := os.CreateTemp(verDir, ".stage-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(stage.Name())
	h := sha256.New()
	// Batas keras = ukuran yang dipatok + 1: artefak lebih besar dari pin tidak pernah diterima utuh.
	n, err := io.Copy(io.MultiWriter(stage, h), io.LimitReader(body, pin.Bytes+1))
	if cerr := stage.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if n != pin.Bytes || sum != pin.SHA256 {
		return "", fmt.Errorf("osquery artifact does not match the pinned hash/size (got %s, %d bytes): refusing", sum, n)
	}
	if !ed25519.Verify(i.PubKey, agentproto.SignedToolMessage("osquery", i.version(), i.goos(), i.goarch(), sum), sig) {
		return "", errors.New("osquery signature does not match the trusted release key: refusing")
	}
	tmpBin, err := extractOne(stage.Name(), pin, verDir)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpBin) // tidak ada bila sudah di-rename
	out, err := i.smoke(ctx, tmpBin)
	if err != nil || !strings.Contains(out, i.version()) {
		return "", fmt.Errorf("installed osquery failed its smoke test (err=%v, output=%q): refusing", err, out)
	}
	if err := os.Rename(tmpBin, i.BinPath()); err != nil {
		return "", err
	}
	return i.BinPath(), nil
}

// extractOne mengekstrak SATU berkas bernama pasti dari tarball ke berkas sementara di dir. Entri lain (termasuk symlink,
// direktori, nama berbahaya) diabaikan; berkas yang dicari harus berkas biasa dan tidak melebihi batas ukuran.
func extractOne(tarball string, pin osquery.Artifact, dir string) (string, error) {
	if strings.HasSuffix(strings.ToLower(pin.File), ".zip") {
		return extractOneZip(tarball, pin, dir)
	}
	f, err := os.Open(tarball)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("artifact is not a gzip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	limit := pin.MaxBinaryBytes
	if limit <= 0 {
		limit = 256 << 20
	}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("artifact does not contain %s", pin.Binary)
		}
		if err != nil {
			return "", fmt.Errorf("artifact is corrupt: %w", err)
		}
		if strings.TrimPrefix(hdr.Name, "./") != pin.Binary {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			return "", fmt.Errorf("%s is not a regular file in the artifact: refusing", pin.Binary)
		}
		if hdr.Size <= 0 || hdr.Size > limit {
			return "", fmt.Errorf("%s has an unreasonable size (%d bytes): refusing", pin.Binary, hdr.Size)
		}
		out, err := os.CreateTemp(dir, ".bin-*")
		if err != nil {
			return "", err
		}
		n, err := io.Copy(out, io.LimitReader(tr, hdr.Size))
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err == nil && n != hdr.Size {
			err = errors.New("artifact is truncated")
		}
		if err == nil {
			err = os.Chmod(out.Name(), 0o755)
		}
		if err != nil {
			os.Remove(out.Name())
			return "", err
		}
		return out.Name(), nil
	}
}

// ---------- integrasi dengan agent ----------

func (a *Agent) osqueryInstaller() *OsqueryInstaller {
	return &OsqueryInstaller{
		Dir: filepath.Dir(a.Exe), PubKey: a.PubKey,
		Fetch: func(ctx context.Context, version string) (io.ReadCloser, string, string, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(a.Cfg.ServerURL, "/")+"/api/v1/agent/tool?name=osquery&version="+version, nil)
			if err != nil {
				return nil, "", "", err
			}
			req.Header.Set("Authorization", "Bearer "+a.Cfg.Secret)
			resp, err := clientFor(a.caFile(), 10*time.Minute).Do(req)
			if err != nil {
				return nil, "", "", err
			}
			if resp.StatusCode != 200 {
				resp.Body.Close()
				return nil, "", "", fmt.Errorf("HTTP %d (the server may not offer osquery)", resp.StatusCode)
			}
			return resp.Body, resp.Header.Get("X-Signature-Ed25519"), resp.Header.Get("X-Tool-Version"), nil
		},
	}
}

// runOsquery menjalankan SATU query bernama dari katalog beku. SQL tidak pernah datang dari server: server hanya mengirim
// kunci kontrak, dan agent mencarinya di katalognya sendiri.
func (a *Agent) runOsquery(ctx context.Context, key string) ([]osquery.Dataset, error) {
	q := osquery.Find(key)
	if q == nil {
		return nil, fmt.Errorf("aksi %q tidak dikenal agent ini", key)
	}
	if !q.SupportsOS(runtime.GOOS) {
		return []osquery.Dataset{{Query: key, Status: osquery.StatusSkipped, Reason: osquery.ReasonUnsupported, Rows: []map[string]string{}, CollectedAt: time.Now().UTC()}}, nil
	}
	if q.NeedsRoot && os.Geteuid() != 0 {
		return []osquery.Dataset{{Query: key, Status: osquery.StatusSkipped, Reason: osquery.ReasonNeedsRoot, Rows: []map[string]string{}, CollectedAt: time.Now().UTC()}}, nil
	}
	bin, err := a.osqueryInstaller().Ensure(ctx)
	if err != nil {
		a.setDiagnostic("collection_failed")
		reason := "osquery could not be installed: " + err.Error()
		return []osquery.Dataset{{Query: key, Status: osquery.StatusFailed, Reason: reason, Rows: []map[string]string{}, CollectedAt: time.Now().UTC()}}, errors.New(reason)
	}
	a.mu.Lock()
	a.osqueryVersion = osquery.Version
	a.mu.Unlock()
	ds := (&osquery.Runner{Bin: bin}).Run(ctx, q)
	if ds.Status == osquery.StatusFailed {
		a.setDiagnostic("collection_failed")
		return []osquery.Dataset{*ds}, errors.New(ds.Reason)
	}
	return []osquery.Dataset{*ds}, nil
}

// extractOneZip mengekstrak SATU berkas bernama pasti dari arsip zip (artefak Windows) ke
// berkas sementara di dir. Aturan sama dengan extractOne: hanya berkas biasa dengan nama
// persis dan ukuran wajar yang diterima.
func extractOneZip(zipPath string, pin osquery.Artifact, dir string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("artifact is not a zip archive: %w", err)
	}
	defer zr.Close()
	limit := pin.MaxBinaryBytes
	if limit <= 0 {
		limit = 256 << 20
	}
	for _, e := range zr.File {
		if strings.TrimPrefix(e.Name, "./") != pin.Binary {
			continue
		}
		if e.FileInfo().IsDir() {
			return "", fmt.Errorf("%s is a directory in the artifact: refusing", pin.Binary)
		}
		if e.UncompressedSize64 == 0 || e.UncompressedSize64 > uint64(limit) {
			return "", fmt.Errorf("%s has an unreasonable size (%d bytes): refusing", pin.Binary, e.UncompressedSize64)
		}
		rc, err := e.Open()
		if err != nil {
			return "", err
		}
		out, err := os.CreateTemp(dir, ".bin-*")
		if err != nil {
			rc.Close()
			return "", err
		}
		n, err := io.Copy(out, io.LimitReader(rc, int64(e.UncompressedSize64)))
		rc.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err == nil && uint64(n) != e.UncompressedSize64 {
			err = errors.New("artifact is truncated")
		}
		if err == nil {
			err = os.Chmod(out.Name(), 0o755)
		}
		if err != nil {
			os.Remove(out.Name())
			return "", err
		}
		return out.Name(), nil
	}
	return "", fmt.Errorf("artifact does not contain %s", pin.Binary)
}
