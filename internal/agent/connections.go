package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"orchis-go/internal/execution"
)

// conn adalah satu koneksi keluar.
type conn struct {
	proto   string
	src     string
	dst     string
	dstPort int
	state   string
	inode   string
}

// Connections mengumpulkan koneksi jaringan KELUAR (outbound) aktif dari endpoint,
// di-agregasi per (proses, tujuan) menjadi finding bertipe net_connection.
// Linux memakai /proc (cepat, tanpa exec); OS lain fallback ke netstat.
func Connections(ctx context.Context) ([]execution.NewFinding, error) {
	var rows []conn
	if runtime.GOOS == "linux" {
		rows = procNetConnections()
	} else {
		out := run(ctx, "netstat", netstatArgs()...)
		rows = parseNetstatConns(out)
	}
	// mapping inode -> pid untuk linux; untuk netstat, pid sudah ada di output
	pidByInode := map[string]string{}
	if runtime.GOOS == "linux" {
		pidByInode = procSocketOwners()
	}
	procName := map[string][2]string{} // pid -> {name, path}
	if runtime.GOOS == "linux" {
		procName = procNames()
	}
	type agg struct {
		c    conn
		pid  string
		count int
	}
	groups := map[string]*agg{}
	for _, c := range rows {
		if c.state != "" && c.state != "ESTABLISHED" && c.state != "UNCONN" {
			// tetap catat, tapi tandai state
		}
		pid := pidByInode[c.inode]
		key := fmt.Sprintf("%s|%s|%d|%s", pid, c.dst, c.dstPort, c.proto)
		g := groups[key]
		if g == nil {
			g = &agg{c: c, pid: pid}
			groups[key] = g
		}
		g.count++
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 200 {
		keys = keys[:200]
	}
	out := make([]execution.NewFinding, 0, len(keys))
	for _, k := range keys {
		g := groups[k]
		name, path := "", ""
		if n, ok := procName[g.pid]; ok {
			name, path = n[0], n[1]
		}
		det, _ := json.Marshal(map[string]any{
			"process": name, "path": path, "pid": g.pid, "proto": g.c.proto,
			"src": g.c.src, "dst_port": g.c.dstPort, "state": g.c.state,
		})
		out = append(out, execution.NewFinding{
			Type: "net_connection", Value: fmt.Sprintf("%s:%d", g.c.dst, g.c.dstPort),
			Title: fmt.Sprintf("%s -> %s:%d (%s, x%d)", orUnknown(name), g.c.dst, g.c.dstPort, g.c.proto, g.count),
			Severity: "info", Details: det,
		})
	}
	return out, nil
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func netstatArgs() []string {
	if runtime.GOOS == "windows" {
		return []string{"-ano"}
	}
	return []string{"-anp", "tcp"}
}

// parseNetstatConns mem-parse output netstat untuk koneksi outbound (ESTABLISHED / UNCONN UDP).
func parseNetstatConns(out string) []conn {
	var rows []conn
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		proto := strings.ToLower(f[0])
		if !strings.HasPrefix(proto, "tcp") && !strings.HasPrefix(proto, "udp") {
			continue
		}
		local, remote := f[1], f[2]
		state := ""
		if strings.HasPrefix(proto, "tcp") {
			if len(f) < 6 {
				continue
			}
			state = f[5]
			if state != "ESTABLISHED" && state != "SYN_SENT" {
				continue
			}
		} else {
			state = "UNCONN"
			if len(f) < 5 {
				continue
			}
		}
		// lewati yang masih listening/local
		if strings.Contains(local, "127.0.0.1") || strings.HasPrefix(local, "[::1]") || local == "0.0.0.0:0" {
			// tetap bisa outbound dari localhost; jangan skip
		}
		dst, port := splitHostPort(remote)
		if port == 0 || isLocalIP(dst) && state == "" {
			continue
		}
		rows = append(rows, conn{proto: proto, src: local, dst: dst, dstPort: port, state: state})
	}
	return rows
}

func splitHostPort(s string) (string, int) {
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return s, 0
	}
	p, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return s, 0
	}
	return strings.Trim(s[:i], "[]"), p
}

func isLocalIP(ip string) bool {
	return ip == "127.0.0.1" || ip == "::1" || ip == "0.0.0.0" || strings.HasPrefix(ip, "169.254.") || strings.HasPrefix(ip, "fe80:")
}

// ---- Linux /proc ----

func procNetConnections() []conn {
	var rows []conn
	for _, f := range []struct {
		path  string
		proto string
		v6    bool
	}{
		{"/proc/net/tcp", "tcp", false},
		{"/proc/net/tcp6", "tcp", true},
		{"/proc/net/udp", "udp", false},
		{"/proc/net/udp6", "udp", true},
	} {
		data, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}
		rows = append(rows, parseProcNetAll(string(data), f.proto, f.v6)...)
	}
	return rows
}

// parseProcNetAll mem-parse /proc/net/{tcp,udp} untuk SEMUA koneksi (bukan hanya listening).
func parseProcNetAll(data, proto string, v6 bool) []conn {
	var rows []conn
	for i, line := range strings.Split(data, "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 10 {
			continue
		}
		local, remote := f[1], f[2]
		state := f[3]
		inode := f[9]
		if proto == "tcp" && state != "01" && state != "02" { // ESTABLISHED, SYN_SENT
			continue
		}
		if proto == "udp" && state != "07" { // UNCONN
			continue
		}
		dst, port := procSplit(remote, v6)
		if port == 0 {
			continue
		}
		src, _ := procSplit(local, v6)
		rows = append(rows, conn{proto: proto, src: src, dst: dst, dstPort: port, state: state, inode: inode})
	}
	return rows
}

func procSplit(s string, v6 bool) (string, int) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return s, 0
	}
	p, err := strconv.ParseInt(parts[1], 16, 32)
	if err != nil {
		return s, 0
	}
	return procAddr(parts[0], v6), int(p)
}

// procSocketOwners memetakan inode socket -> pid dengan membaca /proc/<pid>/fd (butuh root untuk proses lain).
func procSocketOwners() map[string]string {
	out := map[string]string{}
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		pid := e.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}
		fds, err := os.ReadDir("/proc/" + pid + "/fd")
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink("/proc/" + pid + "/fd/" + fd.Name())
			if err != nil {
				continue
			}
			if strings.HasPrefix(link, "socket:[") {
				inode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
				out[inode] = pid
			}
		}
	}
	return out
}

// procNames mengembalikan pid -> {nama, path}.
func procNames() map[string][2]string {
	out := map[string][2]string{}
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		pid := e.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}
		name := readFirstLine("/proc/" + pid + "/comm")
		exe, _ := os.Readlink("/proc/" + pid + "/exe")
		out[pid] = [2]string{name, exe}
	}
	return out
}

var _ = exec.Command // netstat fallback tetap memakai exec via run()
