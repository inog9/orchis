package defensebot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// DefaultNVDCacheTTL: hasil NVD per produk dan versi dianggap segar selama ini. Pendek supaya CVE baru tidak terlewat lama.
const DefaultNVDCacheTTL = 24 * time.Hour

// maxNVDCacheFile membatasi satu berkas cache; yang lebih besar diabaikan (bukan dipercaya).
const maxNVDCacheFile = 4 << 20

// nvdCacheVersion: naikkan bila cara membentuk/menafsirkan hasil berubah; entri versi lain diabaikan.
// v2: entri v1 memuat negatif palsu akibat parameter isVulnerable yang membuat NVD membalas 404.
// v3: batas tanggal diabaikan dan CPE Docker diganti ke desktop; hasil lama berisi positif palsu.
const nvdCacheVersion = 3

type nvdDiskEntry struct {
	V         int       `json:"v"`
	CPE       string    `json:"cpe"`
	FetchedAt time.Time `json:"fetched_at"`
	Vulns     []NVDVuln `json:"vulns"`
}

func (n *NVD) cacheTTL() time.Duration {
	if n.CacheTTL > 0 {
		return n.CacheTTL
	}
	return DefaultNVDCacheTTL
}

func (n *NVD) now() time.Time {
	if n.Now != nil {
		return n.Now()
	}
	return time.Now()
}

func (n *NVD) cachePath(cpe string) string {
	sum := sha256.Sum256([]byte(cpe))
	return filepath.Join(n.CacheDir, hex.EncodeToString(sum[:16])+".json")
}

// diskGet membaca hasil yang masih segar. Berkas rusak, kedaluwarsa, atau tidak cocok CPE-nya dianggap tidak ada.
func (n *NVD) diskGet(cpe string) ([]NVDVuln, bool) {
	if n.CacheDir == "" {
		return nil, false
	}
	f, err := os.Open(n.cachePath(cpe))
	if err != nil {
		return nil, false
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || st.Size() > maxNVDCacheFile {
		return nil, false
	}
	var e nvdDiskEntry
	if err := json.NewDecoder(f).Decode(&e); err != nil || e.V != nvdCacheVersion || e.CPE != cpe {
		return nil, false
	}
	if age := n.now().Sub(e.FetchedAt); age < 0 || age > n.cacheTTL() {
		return nil, false
	}
	return e.Vulns, true
}

// diskPut menyimpan hasil sukses (termasuk "tidak ada CVE"). Kegagalan tidak pernah disimpan. Galat tulis diabaikan:
// cache hanya optimasi, bukan sumber kebenaran.
func (n *NVD) diskPut(cpe string, vulns []NVDVuln) {
	if n.CacheDir == "" {
		return
	}
	if err := os.MkdirAll(n.CacheDir, 0o700); err != nil {
		return
	}
	raw, err := json.Marshal(nvdDiskEntry{V: nvdCacheVersion, CPE: cpe, FetchedAt: n.now().UTC(), Vulns: vulns})
	if err != nil || len(raw) > maxNVDCacheFile {
		return
	}
	tmp, err := os.CreateTemp(n.CacheDir, ".nvd-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(raw)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), n.cachePath(cpe)) != nil {
		_ = os.Remove(tmp.Name())
	}
}
