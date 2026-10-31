package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"orchis-go/internal/agentproto"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuiltReleaseArtifacts(t *testing.T) {
	dir := os.Getenv("ORCHIS_TEST_RELEASE_DIR")
	key := os.Getenv("ORCHIS_TEST_RELEASE_KEY")
	if dir == "" || key == "" {
		t.Skip("Opt-in built release proof")
	}
	priv, err := loadOrCreateKey(key, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Version string            `json:"version"`
		Targets map[string]target `json:"targets"`
	}
	if err = json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if !agentproto.ValidVersion(m.Version) || len(m.Targets) != 6 {
		t.Fatal("Incomplete platform manifest")
	}
	for platform, item := range m.Targets {
		osName, arch, _ := strings.Cut(platform, "/")
		if filepath.Base(item.File) != item.File {
			t.Fatal("Unsafe artifact path")
		}
		b, err := os.ReadFile(filepath.Join(dir, item.File))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != item.SHA256 {
			t.Fatal("Artifact digest mismatch", platform)
		}
		sig, err := base64.StdEncoding.DecodeString(item.Signature)
		if err != nil || !ed25519.Verify(priv.Public().(ed25519.PublicKey), agentproto.SignedMessage(m.Version, osName, arch, item.SHA256), sig) {
			t.Fatal("Artifact signature mismatch", platform)
		}
		sum[0] ^= 1
		if ed25519.Verify(priv.Public().(ed25519.PublicKey), agentproto.SignedMessage(m.Version, osName, arch, hex.EncodeToString(sum[:])), sig) {
			t.Fatal("Altered artifact accepted", platform)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, filepath.Join(dir, m.Targets["linux/amd64"].File), "version").Output()
	if err != nil || strings.TrimSpace(string(out)) != m.Version+" linux/amd64" {
		t.Fatal("Linux version runtime proof failed")
	}
	t.Log("Six platform hashes/signatures verified; altered digests rejected; Linux amd64 version command executed. Other platforms build-only.")
}
