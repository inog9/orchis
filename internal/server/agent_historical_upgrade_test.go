package server_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orchis-go/internal/agent"
	"orchis-go/internal/agents"
)

// Biner lama dipasok terpisah dari kode saat ini. Rekonstruksi harus memakai
// sumber historis yang dicatat dan diberi label berbeda dari artefak rilis asli.
func TestHistoricalAgentsUpgradeWithExistingReleaseKey(t *testing.T) {
	kind := os.Getenv("ORCHIS_TEST_LEGACY_ARTIFACT_KIND")
	if kind == "" {
		kind = "artefak historis yang dipasok operator"
	}
	candidate := os.Getenv("ORCHIS_TEST_UPGRADE_RELEASE")
	historical := os.Getenv("ORCHIS_TEST_LEGACY_AGENT_DIR")
	if candidate == "" || historical == "" {
		t.Skip("memerlukan kandidat dan artefak historis operator; lihat bagian Agent endpoint di README")
	}
	raw, err := os.ReadFile(filepath.Join(candidate, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version string
		Targets map[string]struct{ File string }
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	target, ok := manifest.Targets["linux/amd64"]
	if !ok || target.File == "" {
		t.Fatal("kandidat Linux amd64 tidak ada")
	}
	for _, version := range []string{"1.1.0", "1.2.0"} {
		t.Run(version, func(t *testing.T) {
			f := newAgentFixture(t)
			enrolled, _ := f.enroll(t, "historical-"+strings.ReplaceAll(version, ".", "-"))
			dir := t.TempDir()
			bin := filepath.Join(dir, "orchis-agent")
			copyBinary := func(from, to string) {
				t.Helper()
				in, err := os.Open(from)
				if err != nil {
					t.Fatal(err)
				}
				defer in.Close()
				out, err := os.OpenFile(to, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
				if err != nil {
					t.Fatal(err)
				}
				_, err = io.Copy(out, in)
				closeErr := out.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("salin biner: %v %v", err, closeErr)
				}
			}
			copyBinary(filepath.Join(historical, "orchis-agent-"+version+"-linux-amd64"), bin)
			out, err := exec.Command(bin, "version").Output()
			if err != nil || !strings.HasPrefix(string(out), version+" ") {
				t.Fatalf("artefak historis salah: %q %v", out, err)
			}
			cfg := agent.Config{ServerURL: f.a.srv.URL, AgentID: enrolled.AgentID, Secret: enrolled.Secret, Mode: "user"}
			if err := cfg.Save(filepath.Join(dir, "agent-config.json")); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bin, "run")
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
			waitVersion := func(want string) {
				t.Helper()
				deadline := time.Now().Add(90 * time.Second)
				for time.Now().Before(deadline) {
					var got string
					err := f.a.env.Owner.QueryRow(context.Background(), `SELECT version FROM agents WHERE id=$1`, enrolled.AgentID).Scan(&got)
					if err != nil {
						t.Fatal(err)
					}
					if got == want {
						return
					}
					time.Sleep(100 * time.Millisecond)
				}
				t.Fatalf("heartbeat tidak mencapai %s", want)
			}
			waitVersion(version)
			for _, entry := range manifest.Targets {
				copyBinary(filepath.Join(candidate, entry.File), filepath.Join(f.release, entry.File))
			}
			temp := filepath.Join(f.release, ".manifest-upgrade")
			if err := os.WriteFile(temp, raw, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(temp, filepath.Join(f.release, "manifest.json")); err != nil {
				t.Fatal(err)
			}
			if _, err := (&agents.Release{Dir: f.release}).Manifest(); err != nil {
				t.Fatalf("fixture manifest tidak lengkap: %v", err)
			}
			waitVersion(manifest.Version)
			out, err = exec.Command(bin, "version").Output()
			if err != nil || !strings.HasPrefix(string(out), manifest.Version+" ") {
				t.Fatalf("biner tidak terganti: %q %v", out, err)
			}
			persisted, err := agent.LoadConfig(filepath.Join(dir, "agent-config.json"))
			if err != nil || persisted.AgentID != enrolled.AgentID || persisted.Secret != enrolled.Secret {
				t.Fatal("identitas agent berubah saat update")
			}
			t.Logf("%s %s memperbarui diri ke %s; heartbeat dan identitas tetap", kind, version, manifest.Version)
		})
	}
}
