package mitre

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manifest mematok bundel ATT&CK yang dibekukan: versi, hash isi, dan hitungan hasil parse. Manifest ikut repo;
// bundelnya (puluhan MB) tidak, tetapi hash di sini menjamin bundel yang dipakai persis yang dibekukan.
type Manifest struct {
	Matrix    string        `json:"matrix"`
	Version   string        `json:"version"`
	File      string        `json:"file"`
	SHA256    string        `json:"sha256"`
	Bytes     int64         `json:"bytes"`
	SourceURL string        `json:"source_url"`
	Counts    ManifestCount `json:"counts"`
}

type ManifestCount struct {
	Tactics        int `json:"tactics"`
	Techniques     int `json:"techniques"`
	Subtechniques  int `json:"subtechniques"`
	Deprecated     int `json:"deprecated"`
	RevokedSkipped int `json:"revoked_skipped"`
}

const (
	ManifestName = "manifest.json"
	// SourceTemplate: bundel Enterprise per versi di repo resmi MITRE.
	SourceTemplate = "https://raw.githubusercontent.com/mitre-attack/attack-stix-data/master/enterprise-attack/enterprise-attack-%s.json"
)

func LoadManifest(dir string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("manifest rusak: %w", err)
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) counts(ds *Dataset) ManifestCount {
	c := ds.Count()
	return ManifestCount{Tactics: c.Tactics, Techniques: c.Parents, Subtechniques: c.Subs, Deprecated: c.Deprecated, RevokedSkipped: ds.Revoked}
}

// Verify memastikan bundel di dir cocok dengan manifest (hash, hitungan, versi) dan mengembalikan datasetnya.
func Verify(dir string) (*Manifest, *Dataset, error) {
	m, err := LoadManifest(dir)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(filepath.Join(dir, m.File))
	if err != nil {
		return m, nil, fmt.Errorf("bundel beku belum ada (%w); jalankan: make mitre-update VERSION=%s", err, m.Version)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return m, nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != m.Bytes {
		return m, nil, fmt.Errorf("ukuran bundel tidak sama dengan manifest: %d", m.Bytes)
	}
	ds, err := ParseVerified(f, m.SHA256)
	if err != nil {
		return m, nil, err
	}
	if err := m.checkDataset(ds); err != nil {
		return m, ds, err
	}
	return m, ds, nil
}

// Diff membandingkan dua dataset per ID teknik.
type Diff struct {
	Added, Removed, Changed, NewlyDeprecated []string
}

func (d Diff) Empty() bool {
	return len(d.Added)+len(d.Removed)+len(d.Changed)+len(d.NewlyDeprecated) == 0
}

func techSig(t Technique) string {
	b, _ := json.Marshal([]any{t.Name, t.Description, t.ParentID, t.TacticIDs, t.Platforms, t.Mitigations})
	return string(b)
}

func Compare(old, cur *Dataset) Diff {
	prev := map[string]Technique{}
	for _, t := range old.Techniques {
		prev[t.ID] = t
	}
	var d Diff
	seen := map[string]bool{}
	for _, t := range cur.Techniques {
		seen[t.ID] = true
		p, ok := prev[t.ID]
		switch {
		case !ok:
			d.Added = append(d.Added, t.ID)
		case t.Deprecated && !p.Deprecated:
			d.NewlyDeprecated = append(d.NewlyDeprecated, t.ID)
		case techSig(t) != techSig(p):
			d.Changed = append(d.Changed, t.ID)
		}
	}
	for id := range prev {
		if !seen[id] {
			d.Removed = append(d.Removed, id)
		}
	}
	for _, s := range [][]string{d.Added, d.Removed, d.Changed, d.NewlyDeprecated} {
		sort.Strings(s)
	}
	return d
}

// Update membekukan versi baru ke dir: unduh (atau salin dari berkas lokal), validasi, tampilkan diff terhadap bundel beku
// sebelumnya, lalu tulis bundel + manifest. Tidak menyentuh database, jadi laporan lama (snapshot beku) tidak terpengaruh;
// teknik yang hilang dari sumber akan ditandai deprecated oleh importer, bukan dihapus.
func Update(ctx context.Context, dir, version, from string, w io.Writer) (*Manifest, Diff, error) {
	if !frozenVersion.MatchString(version) {
		return nil, Diff{}, errors.New("VERSION wajib diisi (mis. 20.0)")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, Diff{}, err
	}
	url := fmt.Sprintf(SourceTemplate, version)
	tmp, err := os.CreateTemp(dir, ".incoming-*")
	if err != nil {
		return nil, Diff{}, err
	}
	defer os.Remove(tmp.Name())
	var src io.ReadCloser
	if from != "" {
		if src, err = os.Open(from); err != nil {
			return nil, Diff{}, err
		}
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, Diff{}, err
		}
		resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
		if err != nil {
			return nil, Diff{}, fmt.Errorf("unduhan gagal: %w", err)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, Diff{}, fmt.Errorf("sumber menjawab %d untuk versi %s", resp.StatusCode, version)
		}
		src = resp.Body
	}
	n, err := io.Copy(tmp, io.LimitReader(src, maxDownload))
	src.Close()
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, Diff{}, err
	}
	f, err := os.Open(tmp.Name())
	if err != nil {
		return nil, Diff{}, err
	}
	ds, err := ParseVerified(f, "")
	f.Close()
	if err != nil {
		return nil, Diff{}, fmt.Errorf("bundel tidak valid: %w", err)
	}
	if ds.Version != version {
		return nil, Diff{}, fmt.Errorf("bundel berversi %q, diminta %q", ds.Version, version)
	}
	m := &Manifest{Matrix: "enterprise", Version: version, File: "enterprise-attack-" + version + ".json", SHA256: ds.SHA256, Bytes: n, SourceURL: url}
	m.Counts = m.counts(ds)
	if err := m.validate(); err != nil {
		return nil, Diff{}, err
	}

	var diff Diff
	prev, perr := LoadManifest(dir)
	if perr == nil {
		if f, err := os.Open(filepath.Join(dir, prev.File)); err == nil {
			old, err := ParseVerified(f, prev.SHA256)
			f.Close()
			if err == nil {
				diff = Compare(old, ds)
				fmt.Fprintf(w, "Diff %s -> %s: %d baru, %d berubah, %d jadi deprecated, %d hilang dari sumber\n", prev.Version, version, len(diff.Added), len(diff.Changed), len(diff.NewlyDeprecated), len(diff.Removed))
				for label, ids := range map[string][]string{"baru": diff.Added, "berubah": diff.Changed, "deprecated": diff.NewlyDeprecated, "hilang": diff.Removed} {
					if len(ids) > 0 {
						fmt.Fprintf(w, "  %s: %s\n", label, strings.Join(ids, " "))
					}
				}
			}
		}
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return nil, Diff{}, err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, m.File)); err != nil {
		return nil, Diff{}, err
	}
	raw, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, ManifestName), append(raw, '\n'), 0o644); err != nil {
		return nil, Diff{}, err
	}
	if perr == nil && prev.File != m.File {
		_ = os.Remove(filepath.Join(dir, prev.File))
	}
	return m, diff, nil
}
