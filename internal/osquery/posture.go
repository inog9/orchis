package osquery

import (
	"fmt"
	"sort"
	"strconv"
	"time"
)

// PortCompare membandingkan port yang terlihat dari LUAR (scan Nmap) dengan port yang didengarkan DI DALAM mesin (osquery).
// Selisih wajar (firewall, NAT, scan terbatas); ini petunjuk untuk diperiksa, bukan bukti kesalahan.
type PortCompare struct {
	Both     []int  `json:"both"`      // terlihat dari luar dan mendengarkan di semua antarmuka
	ScanOnly []int  `json:"scan_only"` // terlihat dari luar, tidak didengarkan di semua antarmuka (mis. proxy/NAT)
	HostOnly []int  `json:"host_only"` // mendengarkan di semua antarmuka, tidak terlihat oleh scan (mis. difilter firewall, di luar profil scan)
	Note     string `json:"note"`
}

type PostureFinding struct {
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Query    string `json:"query"`
}

// Posture adalah ringkasan "postur endpoint" satu aset yang dibekukan ke snapshot laporan.
type Posture struct {
	CollectedAt time.Time        `json:"collected_at"`
	Source      string           `json:"source"`
	Version     string           `json:"osquery_version,omitempty"`
	Attributes  map[string]any   `json:"attributes"`
	Findings    []PostureFinding `json:"findings"`
	Ports       *PortCompare     `json:"ports,omitempty"`
}

// postureAttributes: satu-satunya atribut yang masuk laporan (tanpa nama host, tanpa daftar paket/akun mentah).
var postureAttributes = []string{"os_name", "os_version", "os_arch", "kernel", "cpu_brand", "cpu_logical_cores", "physical_memory", "uptime_seconds",
	"package_count", "local_account_count", "listening_port_count", "iptables_rules"}

var sevRank = map[string]int{"critical": 4, "high": 3, "medium": 2, "low": 1, "info": 0}

func hostPorts(ds *Dataset) []int {
	seen := map[int]bool{}
	for _, r := range ds.Rows {
		if r["protocol"] != "6" || (r["address"] != "0.0.0.0" && r["address"] != "::") {
			continue
		}
		if p, err := strconv.Atoi(r["port"]); err == nil && p > 0 && p < 65536 {
			seen[p] = true
		}
	}
	out := make([]int, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

func diff(a, b map[int]bool) []int {
	out := []int{}
	for p := range a {
		if !b[p] {
			out = append(out, p)
		}
	}
	sort.Ints(out)
	return out
}

// ComparePorts membandingkan dua himpunan port.
func ComparePorts(scan, host []int) *PortCompare {
	s, h := map[int]bool{}, map[int]bool{}
	for _, p := range scan {
		s[p] = true
	}
	for _, p := range host {
		h[p] = true
	}
	both := []int{}
	for p := range s {
		if h[p] {
			both = append(both, p)
		}
	}
	sort.Ints(both)
	return &PortCompare{Both: both, ScanOnly: diff(s, h), HostOnly: diff(h, s),
		Note: "A scan sees the machine from outside; osquery sees it from inside. Differences are expected (firewalls, NAT, proxies, a limited scan profile) and are a prompt to check, not proof of a problem."}
}

// BuildPosture membangun postur dari dataset ok per query dan port hasil scan. Mengembalikan nil bila tidak ada dataset ok.
// Temuan dihitung ulang dari data lewat aturan beku yang sama dengan saat pengumpulan.
func BuildPosture(sets map[string]*Dataset, scanPorts []int) *Posture {
	p := &Posture{Source: "osquery", Attributes: map[string]any{}, Findings: []PostureFinding{}}
	keys := make([]string, 0, len(sets))
	for k := range sets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := map[string]bool{}
	for _, a := range postureAttributes {
		want[a] = true
	}
	any := false
	for _, k := range keys {
		ds := sets[k]
		q := Find(k)
		if q == nil || ds == nil || ds.Status != StatusOK {
			continue
		}
		any = true
		if ds.CollectedAt.After(p.CollectedAt) {
			p.CollectedAt = ds.CollectedAt
		}
		if ds.Version != "" {
			p.Version = ds.Version
		}
		m := q.Map(ds)
		for name, v := range m.Attributes {
			if want[name] {
				p.Attributes[name] = v
			}
		}
		for _, f := range m.Findings {
			p.Findings = append(p.Findings, PostureFinding{Severity: f.Severity, Title: f.Title, Query: k})
		}
		if k == "osquery.network" {
			p.Ports = ComparePorts(scanPorts, hostPorts(ds))
		}
	}
	if !any {
		return nil
	}
	sort.SliceStable(p.Findings, func(i, j int) bool {
		if sevRank[p.Findings[i].Severity] != sevRank[p.Findings[j].Severity] {
			return sevRank[p.Findings[i].Severity] > sevRank[p.Findings[j].Severity]
		}
		return p.Findings[i].Title < p.Findings[j].Title
	})
	if len(p.Findings) > 30 {
		p.Findings = p.Findings[:30]
	}
	return p
}

// Ints memformat daftar port untuk teks laporan.
func Ints(v []int) string {
	if len(v) == 0 {
		return "none"
	}
	s := ""
	for i, n := range v {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprint(n)
	}
	return s
}
