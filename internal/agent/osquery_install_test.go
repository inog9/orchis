package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"orchis-go/internal/agentproto"
	"orchis-go/internal/osquery"
)

type entry struct {
	name, body, link string
	typ              byte
}

func tarball(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o755, Typeflag: e.typ, Size: int64(len(e.body)), Linkname: e.link}
		if e.typ == 0 {
			hdr.Typeflag = tar.TypeReg
		}
		if hdr.Typeflag != tar.TypeReg {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			tw.Write([]byte(e.body))
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

const fakeBinary = "#!/bin/sh\necho 'osqueryd version 9.9.9'\n"

type rig struct {
	t       *testing.T
	inst    *OsqueryInstaller
	pub     ed25519.PublicKey
	priv    ed25519.PrivateKey
	data    []byte
	pin     osquery.Artifact
	fetches int
	sigFor  func(sum string) string
	version string
}

func newRig(t *testing.T, entries ...entry) *rig {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	if len(entries) == 0 {
		entries = []entry{{name: "opt/osquery/bin/osqueryd", body: fakeBinary}}
	}
	r := &rig{t: t, pub: pub, priv: priv, data: tarball(t, entries...), version: "9.9.9"}
	r.repin()
	r.inst = &OsqueryInstaller{Dir: t.TempDir(), PubKey: pub, GOOS: "linux", GOARCH: "amd64", Version: "9.9.9",
		Pin: func(string, string) (osquery.Artifact, bool) { return r.pin, true },
		Fetch: func(ctx context.Context, version string) (io.ReadCloser, string, string, error) {
			r.fetches++
			sum := sha256.Sum256(r.data)
			return io.NopCloser(bytes.NewReader(r.data)), r.sigFor(hex.EncodeToString(sum[:])), r.version, nil
		}}
	r.sigFor = func(sum string) string {
		return base64.StdEncoding.EncodeToString(ed25519.Sign(r.priv, agentproto.SignedToolMessage("osquery", "9.9.9", "linux", "amd64", sum)))
	}
	return r
}

func (r *rig) repin() {
	sum := sha256.Sum256(r.data)
	r.pin = osquery.Artifact{File: "x.tgz", SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(r.data)), Binary: "opt/osquery/bin/osqueryd", MaxBinaryBytes: 1 << 20}
}

func (r *rig) installed() bool {
	_, err := os.Stat(r.inst.BinPath())
	return err == nil
}

func (r *rig) noLeftovers() {
	r.t.Helper()
	entries, _ := os.ReadDir(filepath.Dir(r.inst.BinPath()))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			r.t.Fatalf("sisa berkas sementara: %s", e.Name())
		}
	}
}

func TestOsqueryInstallVerifiesAndIsIdempotent(t *testing.T) {
	r := newRig(t)
	bin, err := r.inst.Ensure(context.Background())
	if err != nil || !r.installed() || bin != r.inst.BinPath() {
		t.Fatalf("pasang: %v %s", err, bin)
	}
	st, _ := os.Stat(bin)
	if st.Mode().Perm() != 0o755 {
		t.Fatalf("mode biner %v", st.Mode())
	}
	r.noLeftovers()
	if out, err := exec.Command(bin).Output(); err != nil || !strings.Contains(string(out), "9.9.9") {
		t.Fatalf("biner terpasang tidak berjalan: %v %s", err, out)
	}
	// Panggilan kedua tidak mengunduh lagi.
	if _, err := r.inst.Ensure(context.Background()); err != nil || r.fetches != 1 {
		t.Fatalf("idempoten: %v fetches=%d", err, r.fetches)
	}
	// Biner rusak (gagal uji asap) dipasang ulang.
	os.WriteFile(bin, []byte("#!/bin/sh\necho corrupted\n"), 0o755)
	if _, err := r.inst.Ensure(context.Background()); err != nil || r.fetches != 2 {
		t.Fatalf("pasang ulang biner rusak: %v fetches=%d", err, r.fetches)
	}
	if out, _ := exec.Command(bin).Output(); !strings.Contains(string(out), "9.9.9") {
		t.Fatal("biner rusak tidak diganti")
	}
}

func TestOsqueryInstallRefusesEverythingUnpinnedOrUnsigned(t *testing.T) {
	cases := map[string]func(r *rig){
		"isi tarball diubah (hash tidak cocok pin)": func(r *rig) {
			r.data = tarball(r.t, entry{name: "opt/osquery/bin/osqueryd", body: "#!/bin/sh\necho 'osqueryd version 9.9.9'\n# evil\n"})
		},
		"ukuran lebih besar dari pin": func(r *rig) { r.data = append(r.data, make([]byte, 100)...) },
		"tanda tangan kunci lain": func(r *rig) {
			_, other, _ := ed25519.GenerateKey(rand.Reader)
			r.sigFor = func(sum string) string {
				return base64.StdEncoding.EncodeToString(ed25519.Sign(other, agentproto.SignedToolMessage("osquery", "9.9.9", "linux", "amd64", sum)))
			}
		},
		"tanda tangan untuk platform lain": func(r *rig) {
			r.sigFor = func(sum string) string {
				return base64.StdEncoding.EncodeToString(ed25519.Sign(r.priv, agentproto.SignedToolMessage("osquery", "9.9.9", "darwin", "arm64", sum)))
			}
		},
		"tanda tangan rilis agent dipakai ulang sebagai alat": func(r *rig) {
			r.sigFor = func(sum string) string {
				return base64.StdEncoding.EncodeToString(ed25519.Sign(r.priv, agentproto.SignedMessage("9.9.9", "linux", "amd64", sum)))
			}
		},
		"tanda tangan untuk versi lain": func(r *rig) {
			r.sigFor = func(sum string) string {
				return base64.StdEncoding.EncodeToString(ed25519.Sign(r.priv, agentproto.SignedToolMessage("osquery", "1.0.0", "linux", "amd64", sum)))
			}
		},
		"tanda tangan bukan base64":    func(r *rig) { r.sigFor = func(string) string { return "!!!" } },
		"server menawarkan versi lain": func(r *rig) { r.version = "9.9.8" },
	}
	for name, mutate := range cases {
		r := newRig(t)
		mutate(r)
		if name != "isi tarball diubah (hash tidak cocok pin)" && name != "ukuran lebih besar dari pin" {
			r.repin()
		} // untuk dua kasus pertama pin TETAP milik tarball asli
		if name == "isi tarball diubah (hash tidak cocok pin)" || name == "ukuran lebih besar dari pin" {
			good := tarball(t, entry{name: "opt/osquery/bin/osqueryd", body: fakeBinary})
			sum := sha256.Sum256(good)
			r.pin = osquery.Artifact{File: "x.tgz", SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(good)), Binary: "opt/osquery/bin/osqueryd", MaxBinaryBytes: 1 << 20}
		}
		if _, err := r.inst.Ensure(context.Background()); err == nil {
			t.Errorf("%s: harus ditolak", name)
		}
		if r.installed() {
			t.Errorf("%s: tidak boleh terpasang", name)
		}
		r.noLeftovers()
	}
}

func TestOsqueryInstallExtractsOnlyTheNamedRegularFile(t *testing.T) {
	cases := map[string][]entry{
		"symlink bukan berkas biasa":         {{name: "opt/osquery/bin/osqueryd", typ: tar.TypeSymlink, link: "/etc/passwd"}},
		"berkas yang dicari tidak ada":       {{name: "usr/bin/other", body: "x"}},
		"traversal nama diabaikan":           {{name: "../../opt/osquery/bin/osqueryd", body: fakeBinary}},
		"nama mirip tidak cocok":             {{name: "opt/osquery/bin/osqueryd2", body: fakeBinary}},
		"biner kosong":                       {{name: "opt/osquery/bin/osqueryd", body: ""}},
		"biner melebihi batas ukuran":        {{name: "opt/osquery/bin/osqueryd", body: fakeBinary + "# " + strings.Repeat("A", 2<<20) + "\n"}},
		"biner gagal uji asap (versi salah)": {{name: "opt/osquery/bin/osqueryd", body: "#!/bin/sh\necho 'osqueryd version 1.2.3'\n"}},
		"biner tidak bisa dijalankan":        {{name: "opt/osquery/bin/osqueryd", body: "not a program"}},
	}
	for name, entries := range cases {
		r := newRig(t, entries...)
		if _, err := r.inst.Ensure(context.Background()); err == nil {
			t.Errorf("%s: harus ditolak", name)
		}
		if r.installed() {
			t.Errorf("%s: tidak boleh terpasang", name)
		}
		r.noLeftovers()
		// Tidak ada berkas lain yang tertulis di luar direktori versi (mis. dari traversal).
		_ = filepath.Walk(r.inst.Dir, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && !strings.HasPrefix(p, filepath.Dir(r.inst.BinPath())) {
				t.Errorf("%s: berkas tak terduga %s", name, p)
			}
			return nil
		})
	}
	// Entri berbahaya LAIN di tarball tidak mengganggu pemasangan yang sah.
	r := newRig(t,
		entry{name: "../../evil", body: "x"}, entry{name: "usr/bin/osqueryi", typ: tar.TypeSymlink, link: "/opt/osquery/bin/osqueryd"},
		entry{name: "opt/osquery/bin/osqueryd", body: fakeBinary})
	if _, err := r.inst.Ensure(context.Background()); err != nil || !r.installed() {
		t.Fatalf("entri lain harus diabaikan: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.inst.Dir, "..", "evil")); err == nil {
		t.Fatal("entri traversal tertulis")
	}
}

func TestOsqueryInstallPlatformWithoutPinAndFetchErrors(t *testing.T) {
	r := newRig(t)
	r.inst.Pin = func(string, string) (osquery.Artifact, bool) { return osquery.Artifact{}, false }
	if _, err := r.inst.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("platform tanpa artefak: %v", err)
	}
	r2 := newRig(t)
	r2.inst.Fetch = func(context.Context, string) (io.ReadCloser, string, string, error) {
		return nil, "", "", errors.New("HTTP 404")
	}
	if _, err := r2.inst.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("unduhan gagal: %v", err)
	}
}

// Artefak resmi yang sebenarnya dipatok harus konsisten dan terformat benar.
func TestPinnedOsqueryArtifactIsWellFormed(t *testing.T) {
	a, ok := osquery.PinnedArtifact("linux", "amd64")
	if !ok || len(a.SHA256) != 64 || a.Bytes < 1<<20 || a.Binary == "" || !strings.HasPrefix(a.URL, "https://github.com/osquery/osquery/releases/download/"+osquery.Version+"/") {
		t.Fatalf("pin linux/amd64 tidak sah: %+v", a)
	}
	if _, ok := osquery.PinnedArtifact("windows", "amd64"); ok {
		t.Fatal("Windows belum terbukti: tidak boleh ada pin")
	}
}

// Setiap pemeriksaan artefak diisolasi: yang lain sengaja dibuat lolos supaya kegagalannya hanya bisa karena pemeriksaan itu.
func TestOsqueryInstallEachArtifactCheckIsIndependent(t *testing.T) {
	// Hash salah, ukuran pas, tanda tangan sah atas isi sebenarnya: hanya cek hash yang bisa menolak.
	r := newRig(t)
	r.pin.SHA256 = strings.Repeat("ab", 32)
	if _, err := r.inst.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "pinned hash") || r.installed() {
		t.Fatalf("hash saja: %v", err)
	}
	// Hash pas, ukuran pin salah (lebih kecil dari isi): hanya cek ukuran/batas yang bisa menolak.
	r = newRig(t)
	r.pin.Bytes = int64(len(r.data)) - 1
	if _, err := r.inst.Ensure(context.Background()); err == nil || r.installed() {
		t.Fatalf("ukuran saja: %v", err)
	}
	// Sumber sangat besar: pembacaan dibatasi dekat ukuran pin (tidak menyedot seluruh aliran ke disk).
	r = newRig(t)
	var read int64
	r.inst.Fetch = func(context.Context, string) (io.ReadCloser, string, string, error) {
		huge := io.LimitReader(zeroReader{}, 64<<20)
		return io.NopCloser(io.TeeReader(huge, countWriter{&read})), "AAAA", "9.9.9", nil
	}
	if _, err := r.inst.Ensure(context.Background()); err == nil {
		t.Fatal("aliran besar harus ditolak")
	}
	if read > r.pin.Bytes+(256<<10) {
		t.Fatalf("pembacaan tidak dibatasi: %d byte dibaca untuk pin %d byte", read, r.pin.Bytes)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { return len(p), nil }

type countWriter struct{ n *int64 }

func (c countWriter) Write(p []byte) (int, error) { *c.n += int64(len(p)); return len(p), nil }

// Biner osquery lama milik administrator tidak boleh digunakan atau diganti oleh agent.
func TestOsqueryInstallLeavesOlderAndSystemBinariesUntouched(t *testing.T) {
	r := newRig(t)
	old := filepath.Join(r.inst.Dir, "osquery", "1.2.3", "osqueryd")
	systemDir := t.TempDir()
	system := filepath.Join(systemDir, "osqueryd")
	oldBody := []byte("#!/bin/sh\necho 'osqueryd version 1.2.3'\n")
	for _, path := range []string{old, system} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, oldBody, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", systemDir)
	bin, err := r.inst.Ensure(context.Background())
	if err != nil || bin != r.inst.BinPath() || r.fetches != 1 {
		t.Fatalf("pinned install: %s %v fetch=%d", bin, err, r.fetches)
	}
	for _, path := range []string{old, system} {
		body, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(body, oldBody) {
			t.Fatalf("older binary changed: %s %v", path, err)
		}
	}
	if out, err := exec.Command(bin, "--version").Output(); err != nil || !strings.Contains(string(out), "9.9.9") {
		t.Fatalf("selected older binary: %s %v", out, err)
	}
}
