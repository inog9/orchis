package agent

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"testing"

	"orchis-go/internal/osquery"
)

// Bukti terhadap artefak resmi: jalankan dengan ORCHIS_OSQUERY_MACOS_TARBALL=<osqueryd-macos-bare-5.24.0.tar.gz>
// (make osquery-fetch menaruhnya di deploy/osquery). Tanpa berkasnya tes ini dilewati. Biner tidak dijalankan:
// hanya bentuk tarball, ukuran, dan header Mach-O universal (x86_64 + arm64) yang diperiksa.
func TestRealMacOSTarballMatchesThePinAndHoldsAUniversalBinary(t *testing.T) {
	path := os.Getenv("ORCHIS_OSQUERY_MACOS_TARBALL")
	if path == "" {
		t.Skip("ORCHIS_OSQUERY_MACOS_TARBALL tidak diset")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		pin, ok := osquery.PinnedArtifact("darwin", arch)
		if !ok {
			t.Fatalf("darwin/%s tidak dipatok", arch)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != pin.SHA256 || int64(len(raw)) != pin.Bytes {
			t.Fatalf("tarball tidak cocok dengan pin darwin/%s", arch)
		}
		tmp, err := extractOne(path, pin, t.TempDir())
		if err != nil {
			t.Fatalf("darwin/%s: %v", arch, err)
		}
		f, err := os.Open(tmp)
		if err != nil {
			t.Fatal(err)
		}
		head := make([]byte, 8+2*20)
		_, err = f.Read(head)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if binary.BigEndian.Uint32(head[0:4]) != 0xcafebabe || binary.BigEndian.Uint32(head[4:8]) != 2 {
			t.Fatalf("bukan biner universal dua arsitektur: % x", head[:8])
		}
		cpus := map[uint32]bool{binary.BigEndian.Uint32(head[8:12]): true, binary.BigEndian.Uint32(head[28:32]): true}
		if !cpus[0x01000007] || !cpus[0x0100000c] {
			t.Fatalf("arsitektur Mach-O tidak mencakup x86_64 dan arm64: %v", cpus)
		}
	}
}
