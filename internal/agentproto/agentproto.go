// Package agentproto: hal yang HARUS sama persis di server, agent, dan alat rilis: versi protokol, nama OS/arsitektur,
// perbandingan versi, dan pesan yang ditandatangani untuk rilis. Sengaja kecil dan tanpa dependensi.
package agentproto

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const ProtocolVersion = 1

var versionRe = regexp.MustCompile(`^[0-9]{1,4}\.[0-9]{1,4}\.[0-9]{1,4}$`)

// ValidVersion: hanya MAJOR.MINOR.PATCH (angka). Versi lain ditolak agar perbandingan tidak ambigu.
func ValidVersion(v string) bool { return versionRe.MatchString(v) }

// Newer melaporkan apakah a lebih baru dari b. Versi yang tidak valid tidak pernah dianggap lebih baru.
func Newer(a, b string) bool {
	if !ValidVersion(a) || !ValidVersion(b) {
		return false
	}
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x > y
		}
	}
	return false
}

// SignedMessage mengikat versi, platform, dan hash berkas: biner sah untuk platform/versi lain tidak bisa dipakai ulang.
func SignedMessage(version, goos, goarch, sha256hex string) []byte {
	return []byte(fmt.Sprintf("orchis-agent-release\n%s\n%s/%s\n%s", version, goos, goarch, sha256hex))
}

// SignedToolMessage mengikat nama alat, versi, platform, dan hash artefak (mis. osquery). Domain "orchis-agent-tool" berbeda dari
// rilis agent, sehingga tanda tangan satu jenis tidak bisa dipakai sebagai tanda tangan jenis lain.
func SignedToolMessage(name, version, goos, goarch, sha256hex string) []byte {
	return []byte(fmt.Sprintf("orchis-agent-tool\n%s\n%s\n%s/%s\n%s", name, version, goos, goarch, sha256hex))
}

// PlatformOS memetakan runtime.GOOS ke nama yang dipakai di database/API.
func PlatformOS(goos string) (string, bool) {
	switch goos {
	case "linux", "windows":
		return goos, true
	case "darwin":
		return "macos", true
	}
	return "", false
}

// PlatformArch memetakan runtime.GOARCH ke nama yang dipakai di database/API.
func PlatformArch(goarch string) (string, bool) {
	switch goarch {
	case "amd64":
		return "x86_64", true
	case "arm64":
		return "arm64", true
	}
	return "", false
}

// GoNames kebalikan dari PlatformOS/PlatformArch (untuk mencari berkas rilis).
func GoNames(os, arch string) (goos, goarch string, ok bool) {
	switch os {
	case "linux", "windows":
		goos = os
	case "macos":
		goos = "darwin"
	default:
		return "", "", false
	}
	switch arch {
	case "x86_64":
		goarch = "amd64"
	case "arm64":
		goarch = "arm64"
	default:
		return "", "", false
	}
	return goos, goarch, true
}
