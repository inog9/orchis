package mitre

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var frozenVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?$`)

// validate menolak manifest rusak sebelum jalurnya dibuka atau sumbernya dihubungi.
func (m *Manifest) validate() error {
	if m.Matrix != "enterprise" || !frozenVersion.MatchString(m.Version) || m.File != "enterprise-attack-"+m.Version+".json" {
		return errors.New("manifest harus menunjuk bundel Enterprise berversi dengan nama berkas baku")
	}
	hash, err := hex.DecodeString(m.SHA256)
	if err != nil || len(hash) != 32 {
		return errors.New("hash manifest harus SHA-256 lengkap")
	}
	if m.Bytes <= 0 || m.Bytes > maxDownload {
		return errors.New("ukuran bundel pada manifest tidak valid")
	}
	u, err := url.Parse(m.SourceURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return errors.New("sumber manifest harus URL HTTPS tanpa kredensial")
	}
	if m.Counts.Tactics <= 0 || m.Counts.Techniques <= 0 || m.Counts.Subtechniques < 0 || m.Counts.Deprecated < 0 || m.Counts.RevokedSkipped < 0 {
		return errors.New("hitungan manifest tidak valid")
	}
	return nil
}

func (m *Manifest) checkDataset(ds *Dataset) error {
	if ds.Version != m.Version {
		return fmt.Errorf("versi bundel %q tidak sama dengan manifest %q", ds.Version, m.Version)
	}
	if got := m.counts(ds); got != m.Counts {
		return fmt.Errorf("hitungan bundel %+v tidak sama dengan manifest %+v", got, m.Counts)
	}
	return nil
}

// Fetch mengambil tepat artefak pada manifest yang sudah ditinjau. Berbeda dari Update,
// perintah ini tidak pernah menerima hash baru atau mengubah manifest.
func Fetch(ctx context.Context, dir string) (*Manifest, *Dataset, error) {
	client := &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || req.URL.User != nil || len(via) >= 5 {
			return errors.New("pengalihan unduhan bundel ditolak")
		}
		return nil
	}}
	return fetch(ctx, dir, client)
}

func fetch(ctx context.Context, dir string, client *http.Client) (*Manifest, *Dataset, error) {
	m, err := LoadManifest(dir)
	if err != nil {
		return nil, nil, err
	}
	if _, ds, err := Verify(dir); err == nil {
		return m, ds, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.SourceURL, nil)
	if err != nil {
		return m, nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return m, nil, fmt.Errorf("unduhan bundel gagal: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return m, nil, fmt.Errorf("sumber bundel menjawab HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp(dir, ".fetch-*")
	if err != nil {
		return m, nil, err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, m.Bytes+1))
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return m, nil, err
	}
	if n != m.Bytes {
		return m, nil, fmt.Errorf("ukuran bundel %d tidak sama dengan manifest %d", n, m.Bytes)
	}
	f, err := os.Open(tmp.Name())
	if err != nil {
		return m, nil, err
	}
	ds, err := ParseVerified(f, m.SHA256)
	f.Close()
	if err != nil {
		return m, nil, err
	}
	if err := m.checkDataset(ds); err != nil {
		return m, nil, err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return m, nil, err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, m.File)); err != nil {
		return m, nil, err
	}
	return m, ds, nil
}
