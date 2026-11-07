// Package osquery: katalog query osquery BERNAMA yang dibekukan, pemeriksaan statis tabel/kolom terlarang,
// sanitasi hasil yang tidak tepercaya, pemetaan deklaratif hasil -> atribut aset/temuan, dan runner sekali jalan.
//
// Aturan keamanan: teks SQL hanya ada di katalog ini (di biner agent dan server). Server TIDAK PERNAH mengirim SQL;
// ia hanya meminta kunci query bernama. Setiap query lulus pemeriksaan statis: hanya tabel dalam daftar izin, tidak ada
// tabel/kolom sensitif (riwayat browser, kunci SSH, environment proses, isi file, hash kata sandi, dst.).
package osquery

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"orchis-go/internal/agentproto"
)

//go:embed catalog.json
var catalogJSON []byte

type Match struct {
	Col string `json:"col"`
	Op  string `json:"op"` // eq ne gt lt ge in has_any re
	Val any    `json:"val"`
}

type FindingTpl struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Value    string `json:"value"`
}

type Rule struct {
	ID         string  `json:"id"`
	Scope      string  `json:"scope"` // row | agg
	Match      []Match `json:"match"`
	CountWhere []Match `json:"count_where"`
	CountGT    int     `json:"count_gt"`
	// CountLT dan Require: temuan bila jumlah baris CountWhere kurang dari CountLT, asalkan ada baris yang cocok dengan Require
	// (mencegah temuan palsu saat tidak ada data sama sekali).
	CountLT int        `json:"count_lt"`
	Require []Match    `json:"require"`
	Finding FindingTpl `json:"finding"`
	Explain string     `json:"explain"`
}

type Aggregate struct {
	Key   string   `json:"key"`
	Op    string   `json:"op"` // count | list
	Cols  []string `json:"cols"`
	Limit int      `json:"limit"`
	Where []Match  `json:"where"`
}

type Attributes struct {
	FirstRow   map[string]string `json:"first_row"`
	Aggregates []Aggregate       `json:"aggregates"`
}

type MitreRef struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Query adalah satu query bernama beku. Version naik bila SQL/kolom/aturan berubah.
type Query struct {
	Key      string `json:"key"`
	Platform string `json:"platform"`
	// Pack: nama pack resmi asal query ("" untuk query bawaan katalog). Diisi hanya oleh packs.json yang dibekukan.
	Pack           string     `json:"pack,omitempty"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Category       string     `json:"category"`
	SQL            string     `json:"sql"`
	Tables         []string   `json:"tables"`
	Columns        []string   `json:"columns"`
	OS             []string   `json:"os"`
	Sensitivity    string     `json:"sensitivity"`
	Load           string     `json:"load"`
	NeedsRoot      bool       `json:"needs_root"`
	MaxRows        int        `json:"max_rows"`
	TimeoutSeconds int        `json:"timeout_seconds"`
	Mitre          []MitreRef `json:"mitre"`
	Attributes     Attributes `json:"attributes"`
	Rules          []Rule     `json:"rules"`
	// Example: baris nyata dari server uji Linux (nama host disamarkan), dipakai UI untuk menunjukkan data -> dipakai di mana.
	Example []map[string]string `json:"example"`
}

// Artifact adalah artefak osquery resmi yang DIPATOK (versi + SHA-256). Agent menolak artefak dengan hash lain, apa pun
// yang dikatakan server; alat rilis hanya menandatangani artefak ini.
type Artifact struct {
	File           string `json:"file"`
	URL            string `json:"url"`
	SHA256         string `json:"sha256"`
	Bytes          int64  `json:"bytes"`
	Binary         string `json:"binary"` // satu-satunya berkas yang diekstrak dari tarball
	MaxBinaryBytes int64  `json:"max_binary_bytes"`
}

type catalogFile struct {
	Version   string              `json:"osquery_version"`
	License   string              `json:"license"`
	Artifacts map[string]Artifact `json:"artifacts"`
	Queries   []Query             `json:"queries"`
}

var (
	// Version osquery yang dipatok (lihat install.go untuk hash artefak).
	Version   string
	License   string
	Artifacts map[string]Artifact
	Queries   []Query
)

func init() {
	var c catalogFile
	if err := json.Unmarshal(catalogJSON, &c); err != nil {
		panic(err)
	}
	Version, License, Artifacts, Queries = c.Version, c.License, c.Artifacts, c.Queries
	loadPacks()
	for i := range Queries {
		if err := Queries[i].Validate(); err != nil {
			panic(fmt.Sprintf("osquery catalog: %s: %v", Queries[i].Key, err))
		}
	}
}

// PlatformName mengikuti nama platform pack resmi; protokol agent lama memakai macos.
func (q *Query) PlatformName() string {
	if q.Platform != "" {
		return q.Platform
	}
	if len(q.OS) != 1 {
		return "all"
	}
	if q.OS[0] == "macos" {
		return "darwin"
	}
	return q.OS[0]
}

// PinnedArtifact mengembalikan artefak yang dipatok untuk platform Go (mis. linux, amd64).
func PinnedArtifact(goos, goarch string) (Artifact, bool) {
	a, ok := Artifacts[goos+"/"+goarch]
	return a, ok && a.SHA256 != "" && a.Binary != ""
}

// CollectionSupported: ada artefak osquery yang dipatok untuk pasangan OS/arsitektur bernama database
// (linux|macos|windows, x86_64|arm64). Satu-satunya sumber kebenaran untuk "agent ini bisa mengumpulkan data endpoint".
func CollectionSupported(osName, arch string) bool {
	goos, goarch, ok := agentproto.GoNames(osName, arch)
	if !ok {
		return false
	}
	_, ok = PinnedArtifact(goos, goarch)
	return ok
}

// SupportedPlatforms mengembalikan pasangan "os/arch" bernama database yang punya artefak dipatok, terurut.
func SupportedPlatforms() []string {
	out := []string{}
	for _, osName := range []string{"linux", "macos", "windows"} {
		for _, arch := range []string{"x86_64", "arm64"} {
			if CollectionSupported(osName, arch) {
				out = append(out, osName+"/"+arch)
			}
		}
	}
	return out
}

// PlatformHasArtifact: ada artefak dipatok untuk OS ini pada arsitektur mana pun (linux|macos|darwin).
func PlatformHasArtifact(osName string) bool {
	if osName == "darwin" {
		osName = "macos"
	}
	return CollectionSupported(osName, "x86_64") || CollectionSupported(osName, "arm64")
}

// Find mengembalikan query bernama, atau nil. Hanya kunci katalog yang dikenal; tidak ada jalur SQL bebas.
func Find(key string) *Query {
	for i := range Queries {
		if Queries[i].Key == key {
			return &Queries[i]
		}
	}
	return nil
}

// IsOsquery menyatakan apakah kunci kontrak adalah aksi osquery.
func IsOsquery(key string) bool { return strings.HasPrefix(key, "osquery.") }

// AllowedTables: satu-satunya tabel yang boleh dibaca query katalog. Menambah tabel = keputusan sadar lewat tinjauan kode.
var AllowedTables = map[string]bool{
	"system_info": true, "os_version": true, "kernel_info": true, "uptime": true, "deb_packages": true, "rpm_packages": true,
	"users": true, "groups": true, "user_groups": true, "listening_ports": true, "processes": true, "system_controls": true,
	"disk_encryption": true, "iptables": true, "interface_addresses": true, "routes": true,
	// Ditinjau saat membekukan pack resmi (metadata sistem; tanpa isi berkas, perintah, atau kredensial).
	"kernel_modules": true, "mounts": true, "etc_hosts": true, "time": true, "pci_devices": true, "usb_devices": true,
	// DefenseBot: metadata perangkat lunak dan layanan (nama, versi, pemilik paket); tanpa isi berkas, perintah, atau kredensial.
	"deb_package_files": true, "rpm_package_files": true, "npm_packages": true, "python_packages": true, "apps": true,
	"homebrew_packages": true, "systemd_units": true,
}

// DeniedTables: tabel yang TIDAK PERNAH dibaca, apa pun pack/query-nya (data sensitif pengguna).
var DeniedTables = map[string]bool{
	"user_ssh_keys": true, "authorized_keys": true, "known_hosts": true, "ssh_configs": true, "process_envs": true,
	"process_open_files": true, "process_memory_map": true, "shell_history": true,
	"bash_history": true, "curl": true, "curl_certificate": true, "carves": true, "file": true, "hash": true,
	"file_events": true, "yara": true, "yara_events": true, "shadow": true, "keychain_items": true, "keychain_acls": true,
	"mdfind": true, "wifi_networks": true, "wifi_status": true, "wifi_survey": true, "augeas": true, "magic": true,
	"user_events": true, "socket_events": true, "process_events": true, "chrome_extensions": true, "chrome_extension_content_scripts": true,
	"firefox_addons": true, "safari_extensions": true, "ie_extensions": true, "browser_plugins": true, "opera_extensions": true,
	"edge_extensions": true, "docker_container_envs": true, "ec2_instance_metadata": true,
	"ec2_instance_tags": true,
}

var deniedPrefixes = []string{"chrome_", "firefox_", "safari_", "edge_", "opera_", "brave_", "keychain_", "wifi_", "docker_container_env"}

// DeniedColumns: kolom yang tidak boleh dipilih di query mana pun.
var DeniedColumns = map[string]bool{
	"cmdline": true, "environ": true, "environment": true, "env": true, "password": true, "passwd": true, "secret": true,
	"token": true, "private_key": true, "content": true, "contents": true, "cwd": true, "root": true,
}

// IsDeniedTable melaporkan apakah tabel masuk daftar terlarang (nama persis atau awalan peramban/kredensial).
func IsDeniedTable(t string) bool {
	t = strings.ToLower(t)
	if DeniedTables[t] {
		return true
	}
	for _, p := range deniedPrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

var (
	wordRe   = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*|[(),]`)
	fromStop = map[string]bool{"where": true, "group": true, "order": true, "limit": true, "union": true, "on": true, "using": true, "having": true}
)

// TablesIn mengurai nama tabel yang dibaca oleh SQL (setelah FROM/JOIN, termasuk daftar bergaya koma dan subquery).
// Sengaja konservatif: kata apa pun yang menyerupai tabel di posisi tabel dihitung.
func TablesIn(sql string) []string {
	toks := wordRe.FindAllString(sql, -1)
	seen := map[string]bool{}
	for i := 0; i < len(toks); i++ {
		w := strings.ToLower(toks[i])
		if w != "from" && w != "join" {
			continue
		}
		for j := i + 1; j < len(toks); {
			t := strings.ToLower(toks[j])
			if t == "(" || t == ")" || fromStop[t] || t == "select" {
				break
			}
			seen[t] = true
			j++
			if j < len(toks) && strings.EqualFold(toks[j], "as") {
				j += 2
			} else if j < len(toks) && !fromStop[strings.ToLower(toks[j])] && toks[j] != "," && !isKeyword(toks[j]) {
				j++ // alias
			}
			if j < len(toks) && toks[j] == "," {
				j++
				continue
			}
			break
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func isKeyword(s string) bool {
	switch strings.ToLower(s) {
	case "join", "left", "inner", "cross", "outer", "natural", "from", "select":
		return true
	}
	return false
}

// Validate memastikan query aman: tabel ada di daftar izin dan sama dengan yang dideklarasikan, tidak ada tabel/kolom
// terlarang, kolom keluaran dideklarasikan, dan batas-batas wajar terisi.
func (q *Query) Validate() error {
	if !IsOsquery(q.Key) || q.SQL == "" || len(q.Columns) == 0 {
		return fmt.Errorf("definisi tidak lengkap")
	}
	low := strings.ToLower(q.SQL)
	if strings.Contains(low, ";") || strings.Contains(low, "--") || strings.Contains(low, "/*") || strings.Contains(low, "pragma") || strings.Contains(low, "attach") {
		return fmt.Errorf("SQL memuat konstruksi terlarang")
	}
	if !strings.HasPrefix(strings.TrimSpace(low), "select") {
		return fmt.Errorf("hanya SELECT yang diizinkan")
	}
	parsed := TablesIn(q.SQL)
	declared := append([]string(nil), q.Tables...)
	sort.Strings(declared)
	if strings.Join(parsed, ",") != strings.Join(declared, ",") {
		return fmt.Errorf("tabel yang dibaca %v tidak sama dengan yang dideklarasikan %v", parsed, declared)
	}
	for _, t := range parsed {
		if IsDeniedTable(t) {
			return fmt.Errorf("tabel terlarang: %s", t)
		}
		if !AllowedTables[t] {
			return fmt.Errorf("tabel %s tidak ada di daftar izin", t)
		}
	}
	for _, w := range regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`).FindAllString(low, -1) {
		if DeniedColumns[w] {
			return fmt.Errorf("kolom terlarang: %s", w)
		}
	}
	if q.MaxRows <= 0 || q.MaxRows > 10000 || q.TimeoutSeconds <= 0 || q.TimeoutSeconds > 300 {
		return fmt.Errorf("batas baris/waktu tidak masuk akal")
	}
	if len(q.OS) == 0 {
		return fmt.Errorf("OS yang didukung wajib diisi")
	}
	if len(q.Example) == 0 {
		return fmt.Errorf("contoh data wajib ada")
	}
	if ds, err := Sanitize(q, &Dataset{Query: q.Key, Status: StatusOK, Rows: q.Example}); err != nil || len(ds.Rows) != len(q.Example) {
		return fmt.Errorf("contoh data tidak lolos sanitasi")
	} else {
		for i, row := range q.Example {
			if len(ds.Rows[i]) != len(row) || fmt.Sprint(ds.Rows[i]) != fmt.Sprint(row) {
				return fmt.Errorf("contoh data memuat kolom di luar skema atau nilai yang diredaksi")
			}
		}
	}
	for _, r := range q.Rules {
		switch r.Finding.Type {
		case "open_port", "service", "text", "vulnerability", "url", "port_scan":
		default:
			return fmt.Errorf("aturan %s: tipe temuan %q tidak dikenal", r.ID, r.Finding.Type)
		}
		switch r.Finding.Severity {
		case "info", "low", "medium", "high", "critical":
		default:
			return fmt.Errorf("aturan %s: severity %q tidak dikenal", r.ID, r.Finding.Severity)
		}
	}
	return nil
}

// TablesRead mengembalikan tabel yang dibaca query (untuk ditampilkan sebelum admin menyetujui).
// DefaultEnabled tidak memberi izin koleksi tanpa consent; pilihan awal UI terpisah dari izin efektif.
func (q *Query) DefaultEnabled() bool { return false }

func (q *Query) TablesRead() []string { return append([]string(nil), q.Tables...) }

// SupportsOS: Linux terbukti; OS lain hanya struktur (ditandai belum terbukti di UI).
func (q *Query) SupportsOS(os string) bool {
	if os == "darwin" {
		os = "macos"
	}
	for _, o := range q.OS {
		if o == os {
			return true
		}
	}
	return false
}
