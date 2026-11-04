package agent

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"path/filepath"
	"testing"

	"orchis-go/internal/agentproto"
	"orchis-go/internal/osquery"
)

// Bentuk tarball macOS resmi: satu berkas "osqueryd" di akar (bukan opt/osquery/bin/osqueryd seperti Linux).
func macRig(t *testing.T, goarch string) *rig {
	t.Helper()
	real, ok := osquery.PinnedArtifact("darwin", goarch)
	if !ok {
		t.Fatalf("darwin/%s tidak dipatok", goarch)
	}
	r := newRig(t, entry{name: real.Binary, body: fakeBinary})
	r.pin.Binary, r.pin.MaxBinaryBytes = real.Binary, real.MaxBinaryBytes
	r.inst.GOOS, r.inst.GOARCH = "darwin", goarch
	r.sigFor = func(sum string) string {
		return base64.StdEncoding.EncodeToString(ed25519.Sign(r.priv, agentproto.SignedToolMessage("osquery", "9.9.9", "darwin", goarch, sum)))
	}
	return r
}

func TestOsqueryInstallWorksForTheMacOSTarballLayout(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		r := macRig(t, arch)
		bin, err := r.inst.Ensure(context.Background())
		if err != nil || !r.installed() {
			t.Fatalf("darwin/%s: %v", arch, err)
		}
		if filepath.Base(bin) != "osqueryd" {
			t.Fatalf("nama biner %s", bin)
		}
		r.noLeftovers()
	}
}

func TestOsqueryInstallRefusesAMacOSSignatureForTheOtherArchitecture(t *testing.T) {
	r := macRig(t, "amd64")
	r.sigFor = func(sum string) string {
		return base64.StdEncoding.EncodeToString(ed25519.Sign(r.priv, agentproto.SignedToolMessage("osquery", "9.9.9", "darwin", "arm64", sum)))
	}
	if _, err := r.inst.Ensure(context.Background()); err == nil || r.installed() {
		t.Fatalf("tanda tangan arm64 diterima untuk amd64: %v", err)
	}
	r.noLeftovers()
}

func TestOsqueryInstallRefusesMacOSTarballWithoutTheBareBinary(t *testing.T) {
	r := macRig(t, "amd64")
	r.data = tarball(t, entry{name: "opt/osquery/bin/osqueryd", body: fakeBinary})
	r.repin()
	r.pin.Binary = "osqueryd"
	if _, err := r.inst.Ensure(context.Background()); err == nil || r.installed() {
		t.Fatalf("tarball tanpa berkas osqueryd diterima: %v", err)
	}
}
