package agents

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// InstallTrust adalah bahan kepercayaan yang diberikan konsol kepada install.sh lewat sesi admin (jalur terpisah dari
// unduhan biner): kunci publik rilis, serta, bila server memakai CA internal atau sertifikat self-signed, sidik jari CA atau pin SPKI.
// Nilai yang tidak valid diabaikan; tidak pernah ada nilai bawaan yang melemahkan verifikasi.
type InstallTrust struct {
	ReleaseKey    string `json:"release_key,omitempty"`
	CAFingerprint string `json:"ca_fingerprint,omitempty"`
	TLSPin        string `json:"tls_pin,omitempty"`
}

var (
	hexSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)
	spkiPin   = regexp.MustCompile(`^sha256//[A-Za-z0-9+/]{43}=$`)
)

// LoadInstallTrust membaca ORCHIS_AGENT_RELEASE_PUBKEY (atau berkas release.pub di direktori rilis),
// ORCHIS_AGENT_CA_FINGERPRINT dan ORCHIS_AGENT_TLS_PIN.
func LoadInstallTrust(releaseDir string) InstallTrust {
	var t InstallTrust
	key := strings.TrimSpace(os.Getenv("ORCHIS_AGENT_RELEASE_PUBKEY"))
	if key == "" && releaseDir != "" {
		if b, err := os.ReadFile(filepath.Join(releaseDir, "release.pub")); err == nil && len(b) < 256 {
			key = strings.TrimSpace(string(b))
		}
	}
	if raw, err := base64.StdEncoding.DecodeString(key); err == nil && len(raw) == 32 {
		t.ReleaseKey = key
	}
	fp := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(os.Getenv("ORCHIS_AGENT_CA_FINGERPRINT"))), "sha256:")
	fp = strings.NewReplacer(":", "", " ", "").Replace(fp)
	pin := strings.TrimSpace(os.Getenv("ORCHIS_AGENT_TLS_PIN"))
	// CA internal dan pin saling meniadakan (install.sh menolak keduanya sekaligus); pin hanya dipakai bila tidak ada CA.
	switch {
	case hexSHA256.MatchString(fp):
		t.CAFingerprint = fp
	case spkiPin.MatchString(pin):
		t.TLSPin = pin
	}
	return t
}
