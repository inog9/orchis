package agents

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadInstallTrustAcceptsOnlyWellFormedValues(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	pin := "sha256//" + base64.StdEncoding.EncodeToString(make([]byte, 32))
	fp := strings.Repeat("ab", 32)
	t.Setenv("ORCHIS_AGENT_RELEASE_PUBKEY", key)
	t.Setenv("ORCHIS_AGENT_CA_FINGERPRINT", "SHA256:"+strings.ToUpper(fp[:2])+":"+fp[2:])
	t.Setenv("ORCHIS_AGENT_TLS_PIN", pin)
	got := LoadInstallTrust("")
	if got.ReleaseKey != key || got.CAFingerprint != fp || got.TLSPin != "" {
		t.Fatalf("CA fingerprint must win over the pin: %+v", got)
	}
	t.Setenv("ORCHIS_AGENT_CA_FINGERPRINT", "")
	if got = LoadInstallTrust(""); got.TLSPin != pin || got.CAFingerprint != "" {
		t.Fatalf("pin expected: %+v", got)
	}
	for _, bad := range []string{"short", base64.StdEncoding.EncodeToString(make([]byte, 31)), "not base64!!"} {
		t.Setenv("ORCHIS_AGENT_RELEASE_PUBKEY", bad)
		t.Setenv("ORCHIS_AGENT_CA_FINGERPRINT", "zz")
		t.Setenv("ORCHIS_AGENT_TLS_PIN", "sha256//short")
		if got = LoadInstallTrust(""); got != (InstallTrust{}) {
			t.Fatalf("malformed %q must be ignored: %+v", bad, got)
		}
	}
}

func TestLoadInstallTrustReadsReleasePubFromReleaseDir(t *testing.T) {
	dir := t.TempDir()
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if err := os.WriteFile(filepath.Join(dir, "release.pub"), []byte(key+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORCHIS_AGENT_RELEASE_PUBKEY", "")
	if got := LoadInstallTrust(dir); got.ReleaseKey != key {
		t.Fatalf("%+v", got)
	}
}
