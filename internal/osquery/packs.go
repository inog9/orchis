package osquery

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Status query pack. Hanya "runnable" yang menjadi query katalog dan dapat dijalankan; sisanya ditampilkan apa adanya
// beserta alasannya supaya admin tahu persis apa yang TIDAK dibaca.
const (
	PackRunnable   = "runnable"     // lolos tinjauan, dicoba pada osquery asli, menjadi query katalog
	PackSensitive  = "sensitive"    // menyentuh tabel/kolom terlarang: dilewati, data sensitif
	PackNotReview  = "not_reviewed" // tabel belum ada di daftar izin
	PackOtherOS    = "other_os"     // khusus macOS/Windows: belum terbukti
	PackUnverified = "unverified"   // tidak menghasilkan sampel nyata saat pembekuan, kolomnya tidak dapat dipastikan
)

type PackQuery struct {
	Name        string   `json:"name"`
	Platform    string   `json:"platform"`
	Key         string   `json:"key,omitempty"` // hanya untuk status runnable
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Reason      string   `json:"reason"`
	Tables      []string `json:"tables"`
}

type Pack struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	File        string      `json:"file"`
	SHA256      string      `json:"sha256"` // hash berkas .conf di dalam tarball osquery yang dipatok
	Platform    string      `json:"platform"`
	Queries     []PackQuery `json:"queries"`
}

type packsFile struct {
	OsqueryVersion string  `json:"osquery_version"`
	ArtifactSHA256 string  `json:"artifact_sha256"`
	Packs          []Pack  `json:"packs"`
	Queries        []Query `json:"queries"`
}

//go:embed packs.json
var packsJSON []byte

// Packs adalah pack resmi yang dibekukan (versi + hash). Tidak diunduh saat runtime.
var Packs []Pack

func loadPacks() {
	packs, qs, err := parsePacks(packsJSON, Version, Artifacts, Queries)
	if err != nil {
		panic("osquery packs: " + err.Error())
	}
	Packs = packs
	Queries = append(Queries, qs...)
}

// parsePacks memeriksa berkas pack beku terhadap versi dan artefak yang dipatok, lalu memvalidasi setiap query. Tidak mengubah
// keadaan global supaya dapat diuji dengan berkas yang dirusak.
func parsePacks(raw []byte, version string, artifacts map[string]Artifact, existing []Query) ([]Pack, []Query, error) {
	var f packsFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, nil, err
	}
	if f.OsqueryVersion != version {
		return nil, nil, fmt.Errorf("dibekukan untuk versi %s, katalog memakai %s", f.OsqueryVersion, version)
	}
	if pin, ok := artifacts["linux/amd64"]; !ok || pin.SHA256 != f.ArtifactSHA256 {
		return nil, nil, fmt.Errorf("hash artefak tidak sama dengan yang dipatok")
	}
	have := map[string]bool{}
	for i := range existing {
		have[existing[i].Key] = true
	}
	for i := range f.Queries {
		q := f.Queries[i]
		if q.Pack == "" || !strings.HasPrefix(q.Key, "osquery.pack.") || have[q.Key] {
			return nil, nil, fmt.Errorf("query tidak valid atau kunci ganda: %s", q.Key)
		}
		if err := q.Validate(); err != nil {
			return nil, nil, fmt.Errorf("%s: %v", q.Key, err)
		}
		have[q.Key] = true
	}
	return f.Packs, f.Queries, nil
}

// FindPack mengembalikan pack beku menurut nama, atau nil.
func FindPack(name string) *Pack {
	for i := range Packs {
		if Packs[i].Name == name {
			return &Packs[i]
		}
	}
	return nil
}

// RunnableKeys: kunci query katalog milik pack (yang benar-benar dapat berjalan).
func (p *Pack) RunnableKeys() []string {
	out := []string{}
	for _, q := range p.Queries {
		if q.Status == PackRunnable {
			out = append(out, q.Key)
		}
	}
	return out
}

// PackQueryKeys: kunci semua query milik pack resmi yang dapat dijalankan (urutan katalog).
func PackQueryKeys() []string {
	out := []string{}
	for i := range Queries {
		if Queries[i].Pack != "" {
			out = append(out, Queries[i].Key)
		}
	}
	return out
}
