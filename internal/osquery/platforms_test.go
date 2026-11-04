package osquery

import (
	"sort"
	"strings"
	"testing"
)

// Hash dan ukuran macOS dibekukan di sini juga: mengubah pin di catalog.json tanpa sengaja membuat tes ini gagal.
// Nilai berasal dari rilis resmi osquery 5.24.0 (osqueryd-macos-bare-5.24.0.tar.gz, biner universal x86_64+arm64).
const (
	macSHA256 = "b2b8fe5428885755e8243f209dc051dc328a1c622c68c777c8662f575f63c74b"
	macBytes  = 16657746
)

func TestMacOSArtifactIsPinnedForBothArchitectures(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		a, ok := PinnedArtifact("darwin", arch)
		if !ok {
			t.Fatalf("darwin/%s tidak dipatok", arch)
		}
		if a.SHA256 != macSHA256 || a.Bytes != macBytes || a.Binary != "osqueryd" {
			t.Fatalf("darwin/%s: pin berubah: %+v", arch, a)
		}
		if !strings.HasPrefix(a.URL, "https://github.com/osquery/osquery/releases/download/"+Version+"/") || !strings.HasSuffix(a.URL, "/"+a.File) {
			t.Fatalf("darwin/%s: URL bukan rilis resmi versi %s: %s", arch, Version, a.URL)
		}
		if a.MaxBinaryBytes <= 0 {
			t.Fatalf("darwin/%s: batas ukuran biner kosong", arch)
		}
	}
}

func TestCollectionSupportedFollowsPinnedArtifacts(t *testing.T) {
	cases := []struct {
		os, arch string
		want     bool
	}{
		{"linux", "x86_64", true}, {"macos", "x86_64", true}, {"macos", "arm64", true},
		{"linux", "arm64", false}, {"windows", "x86_64", false}, {"windows", "arm64", false},
		{"ios", "arm64", false}, {"macos", "ppc", false}, {"", "", false},
	}
	for _, c := range cases {
		if got := CollectionSupported(c.os, c.arch); got != c.want {
			t.Errorf("CollectionSupported(%q,%q)=%v, want %v", c.os, c.arch, got, c.want)
		}
	}
	if got := strings.Join(SupportedPlatforms(), ","); got != "linux/x86_64,macos/x86_64,macos/arm64" {
		t.Fatalf("SupportedPlatforms=%s", got)
	}
	if !PlatformHasArtifact("darwin") || !PlatformHasArtifact("macos") || !PlatformHasArtifact("linux") || PlatformHasArtifact("windows") {
		t.Fatal("PlatformHasArtifact tidak konsisten")
	}
}

// Query yang dibuka untuk macOS hanya memakai tabel yang sama dengan Linux (sudah ditinjau) dan tetap valid.
func TestMacOSQueriesAreTheReviewedCrossPlatformSet(t *testing.T) {
	var got []string
	for i := range Queries {
		q := &Queries[i]
		if q.Pack == "" && q.SupportsOS("macos") {
			got = append(got, q.Key)
			if err := q.Validate(); err != nil {
				t.Errorf("%s: %v", q.Key, err)
			}
		}
	}
	sort.Strings(got)
	want := "osquery.hardening,osquery.inventory,osquery.network,osquery.network_ranges,osquery.users"
	if strings.Join(got, ",") != want {
		t.Fatalf("query macOS = %v", got)
	}
	// Query yang bergantung pada tabel khusus Linux tidak boleh dijanjikan untuk macOS.
	for _, key := range []string{"osquery.packages", "osquery.firewall"} {
		if Find(key).SupportsOS("macos") {
			t.Errorf("%s memakai tabel khusus Linux", key)
		}
	}
}
