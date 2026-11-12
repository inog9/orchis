// Package agent: logika agent endpoint. Hanya melakukan koneksi KELUAR ke server, dan hanya menjalankan aksi baku
// yang dikenalnya (tanpa perintah dari server, tanpa shell, tanpa parameter).
package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"orchis-go/internal/contract"
	"orchis-go/internal/execution"
)

// Task menjalankan satu aksi baku dan mengembalikan temuan. Kunci tak dikenal = galat (bukan perintah acak).
func Task(ctx context.Context, key string) ([]execution.NewFinding, error) {
	switch key {
	case contract.KeyAgentPorts:
		return ListeningPorts(ctx)
	case contract.KeyAgentInfo:
		return SystemInfo(ctx)
	case contract.KeyBlueteamConnections:
		return Connections(ctx)
	}
	return nil, fmt.Errorf("aksi %q tidak dikenal agent ini", key)
}

type bind struct {
	port int
	addr string
}

func scope(addr string) string {
	ip := net.ParseIP(addr)
	switch {
	case addr == "*" || (ip != nil && ip.IsUnspecified()):
		return "all interfaces"
	case ip != nil && ip.IsLoopback():
		return "loopback only"
	}
	return "specific address"
}

func portFindings(binds []bind) []execution.NewFinding {
	type agg struct {
		addrs []string
		wide  bool // terbuka ke luar loopback
	}
	byPort := map[int]*agg{}
	for _, b := range binds {
		a := byPort[b.port]
		if a == nil {
			a = &agg{}
			byPort[b.port] = a
		}
		a.addrs = append(a.addrs, b.addr)
		if scope(b.addr) != "loopback only" {
			a.wide = true
		}
	}
	ports := make([]int, 0, len(byPort))
	for p := range byPort {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	out := make([]execution.NewFinding, 0, len(ports))
	for _, p := range ports {
		a := byPort[p]
		sort.Strings(a.addrs)
		sc := "loopback only"
		if a.wide {
			sc = "reachable from the network"
		}
		d, _ := json.Marshal(map[string]any{"port": p, "protocol": "tcp", "bind_addresses": dedupe(a.addrs), "scope": sc})
		out = append(out, execution.NewFinding{Type: "open_port", Value: fmt.Sprintf("tcp/%d", p), Title: fmt.Sprintf("Listening port %d/tcp (%s)", p, sc), Severity: "info", Details: d})
	}
	return out
}

func dedupe(s []string) []string {
	out := s[:0:0]
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

// ParseProcNet membaca /proc/net/tcp atau tcp6 dan mengembalikan soket yang LISTEN (status 0A).
func ParseProcNet(r io.Reader, v6 bool) []bind {
	var out []bind
	sc := bufio.NewScanner(r)
	sc.Scan() // judul kolom
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 4 || f[3] != "0A" {
			continue
		}
		local := strings.Split(f[1], ":")
		if len(local) != 2 {
			continue
		}
		port, err := strconv.ParseUint(local[1], 16, 16)
		if err != nil {
			continue
		}
		addr := procAddr(local[0], v6)
		if addr == "?" {
			continue // alamat tak terbaca: baris rusak, bukan soket yang boleh dilaporkan
		}
		out = append(out, bind{port: int(port), addr: addr})
	}
	return out
}

// procAddr mengubah alamat heks little-endian per kata 32-bit di /proc/net/tcp* menjadi teks.
func procAddr(h string, v6 bool) string {
	raw := make([]byte, len(h)/2)
	for i := range raw {
		b, err := strconv.ParseUint(h[2*i:2*i+2], 16, 8)
		if err != nil {
			return "?"
		}
		raw[i] = byte(b)
	}
	if !v6 && len(raw) == 4 {
		return net.IPv4(raw[3], raw[2], raw[1], raw[0]).String()
	}
	if v6 && len(raw) == 16 {
		for w := 0; w < 16; w += 4 { // setiap kata 32-bit disimpan terbalik
			raw[w], raw[w+1], raw[w+2], raw[w+3] = raw[w+3], raw[w+2], raw[w+1], raw[w]
		}
		return net.IP(raw).String()
	}
	return "?"
}

// ParseNetstat membaca keluaran `netstat -an` (macOS dan Windows) dan mengembalikan soket TCP yang LISTEN.
func ParseNetstat(out string) []bind {
	var res []bind
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || !strings.HasPrefix(strings.ToLower(f[0]), "tcp") || !strings.Contains(strings.ToUpper(line), "LISTEN") {
			continue
		}
		// Windows: TCP 0.0.0.0:135 0.0.0.0:0 LISTENING pid. macOS: tcp4 0 0 *.22 *.* LISTEN
		local := f[1]
		if len(f) >= 4 && isNumber(f[1]) && isNumber(f[2]) { // macOS: kolom 2 dan 3 adalah antrean
			local = f[3]
		}
		sep := strings.LastIndexAny(local, ":.")
		if sep < 0 {
			continue
		}
		port, err := strconv.Atoi(local[sep+1:])
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		addr := strings.Trim(local[:sep], "[]")
		if addr == "" {
			addr = "*"
		}
		res = append(res, bind{port: port, addr: addr})
	}
	return res
}

func isNumber(s string) bool { _, err := strconv.Atoi(s); return err == nil }

// ListeningPorts: soket TCP yang mendengarkan di endpoint ini. Hanya-baca; tidak ada paket yang dikirim.
func ListeningPorts(ctx context.Context) ([]execution.NewFinding, error) {
	var binds []bind
	if runtime.GOOS == "linux" {
		for _, f := range []struct {
			path string
			v6   bool
		}{{"/proc/net/tcp", false}, {"/proc/net/tcp6", true}} {
			fh, err := os.Open(f.path)
			if err != nil {
				if f.v6 {
					continue // IPv6 mungkin dinonaktifkan
				}
				return nil, err
			}
			binds = append(binds, ParseProcNet(fh, f.v6)...)
			fh.Close()
		}
	} else {
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		out, err := exec.CommandContext(cctx, "netstat", "-an").Output()
		if err != nil {
			return nil, fmt.Errorf("netstat gagal: %w", err)
		}
		binds = ParseNetstat(string(out))
	}
	return portFindings(binds), nil
}

func readFirstLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
}

func osRelease() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

func memTotalMB() int {
	fh, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "MemTotal:"); ok {
			f := strings.Fields(v)
			if len(f) > 0 {
				kb, _ := strconv.Atoi(f[0])
				return kb / 1024
			}
		}
	}
	return 0
}

func run(ctx context.Context, name string, args ...string) string {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// SystemInfo: fakta dasar tentang endpoint (OS, kernel, arsitektur, nama host). Hanya-baca; perintah dan argumennya tetap.
func SystemInfo(ctx context.Context) ([]execution.NewFinding, error) {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	facts := map[string]any{"hostname": host, "os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": runtime.NumCPU()}
	pretty := runtime.GOOS
	switch runtime.GOOS {
	case "linux":
		if v := osRelease(); v != "" {
			pretty = v
		}
		facts["kernel"] = readFirstLine("/proc/sys/kernel/osrelease")
		facts["memory_mb"] = memTotalMB()
	case "darwin":
		v := run(ctx, "sw_vers", "-productVersion")
		pretty = strings.TrimSpace("macOS " + v)
		facts["kernel"] = run(ctx, "uname", "-r")
	case "windows":
		pretty = "Windows"
		facts["version"] = run(ctx, "cmd", "/c", "ver")
	}
	facts["os_name"] = pretty
	d, _ := json.Marshal(facts)
	return []execution.NewFinding{{Type: "text", Value: "system:" + strings.ToLower(host), Title: fmt.Sprintf("System information: %s (%s)", pretty, runtime.GOARCH), Severity: "info", Details: d}}, nil
}
