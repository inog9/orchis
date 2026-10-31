package agentproto

import "testing"

func TestNewerAndValidVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.0.1", "1.0.0", true}, {"1.0.0", "1.0.0", false}, {"1.0.0", "1.0.1", false}, {"2.0.0", "1.99.99", true},
		{"1.10.0", "1.9.0", true}, {"1.9.0", "1.10.0", false},
		{"dev", "1.0.0", false}, {"1.0.0", "dev", false}, {"1.0", "1.0.0", false}, {"1.0.0-rc1", "1.0.0", false}, {"", "1.0.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q,%q) = %v, harapan %v", c.a, c.b, got, c.want)
		}
	}
}

func TestSignedMessageBindsEveryField(t *testing.T) {
	base := string(SignedMessage("1.0.0", "linux", "amd64", "aa"))
	for _, other := range [][4]string{{"1.0.1", "linux", "amd64", "aa"}, {"1.0.0", "darwin", "amd64", "aa"}, {"1.0.0", "linux", "arm64", "aa"}, {"1.0.0", "linux", "amd64", "bb"}} {
		if string(SignedMessage(other[0], other[1], other[2], other[3])) == base {
			t.Errorf("pesan harus berbeda untuk %v", other)
		}
	}
}

func TestPlatformNamesRoundTrip(t *testing.T) {
	for _, p := range [][2]string{{"linux", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}} {
		os, ok1 := PlatformOS(p[0])
		arch, ok2 := PlatformArch(p[1])
		if !ok1 || !ok2 {
			t.Fatalf("tidak dikenal: %v", p)
		}
		g, a, ok := GoNames(os, arch)
		if !ok || g != p[0] || a != p[1] {
			t.Errorf("bolak-balik %v -> %s/%s -> %s/%s", p, os, arch, g, a)
		}
	}
	if _, ok := PlatformOS("plan9"); ok {
		t.Error("OS asing harus ditolak")
	}
	if _, _, ok := GoNames("macos", "mips"); ok {
		t.Error("arsitektur asing harus ditolak")
	}
}
