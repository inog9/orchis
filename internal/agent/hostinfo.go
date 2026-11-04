package agent

import (
	"bufio"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Versi OS yang dilaporkan agent apa adanya tentang dirinya sendiri (tanpa osquery, tanpa persetujuan koleksi):
// hanya satu string pendek seperti "macOS 10.15.7". Server membersihkannya lagi sebelum menyimpan.
const osVersionMax = 64

var osVersionCache struct {
	sync.Mutex
	at  time.Time
	val string
}

// OSVersion mengembalikan versi OS lokal; hasil disimpan 1 jam supaya heartbeat tidak menjalankan perintah berulang.
func OSVersion() string {
	osVersionCache.Lock()
	defer osVersionCache.Unlock()
	if !osVersionCache.at.IsZero() && time.Since(osVersionCache.at) < time.Hour {
		return osVersionCache.val
	}
	osVersionCache.val, osVersionCache.at = cleanOSVersion(detectOSVersion()), time.Now()
	return osVersionCache.val
}

// cleanOSVersion membuang karakter kendali dan memotong; string yang kosong berarti "tidak diketahui".
func cleanOSVersion(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if len(s) > osVersionMax {
		s = strings.TrimSpace(s[:osVersionMax])
	}
	return s
}

// parseOSRelease membaca isi /etc/os-release dan memilih PRETTY_NAME, atau NAME + VERSION_ID.
func parseOSRelease(content string) string {
	vals := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || strings.HasPrefix(k, "#") {
			continue
		}
		vals[k] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	if v := vals["PRETTY_NAME"]; v != "" {
		return v
	}
	return strings.TrimSpace(vals["NAME"] + " " + vals["VERSION_ID"])
}

// parseSwVers memformat keluaran `sw_vers -productVersion` menjadi "macOS 10.15.7".
func parseSwVers(out string) string {
	v := strings.TrimSpace(out)
	if v == "" {
		return ""
	}
	return "macOS " + v
}

// parseWindowsVer memformat keluaran `cmd /c ver` ("Microsoft Windows [Version 10.0.19045.3693]") menjadi "Windows 10.0.19045".
func parseWindowsVer(out string) string {
	_, rest, ok := strings.Cut(out, "[Version ")
	if !ok {
		return ""
	}
	v, _, _ := strings.Cut(rest, "]")
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return "Windows " + strings.Join(parts, ".")
}
