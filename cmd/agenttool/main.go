// agenttool menandatangani artefak alat pendamping agent (osquery) dengan kunci rilis agent dan menaruhnya di <rilis>/tools.
// Hanya artefak yang hash-nya dipatok di katalog (internal/osquery) yang mau ditandatangani.
//
//	agenttool -name osquery -file osquery-5.24.0_1.linux_x86_64.tar.gz -key release.key -out dist/agent [-os linux -arch amd64]
//	agenttool -fetch [-dir deploy/osquery]     unduh artefak yang dipatok dan verifikasi SHA-256/ukuran
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"orchis-go/internal/agentproto"
	"orchis-go/internal/osquery"
)

type target struct {
	File      string `json:"file"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
}
type entry struct {
	Version string            `json:"version"`
	Targets map[string]target `json:"targets"`
}
type manifest struct {
	Tools map[string]entry `json:"tools"`
}

func loadKey(path string) (ed25519.PrivateKey, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("kunci rilis tidak ditemukan: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 128 {
		return nil, fmt.Errorf("kunci rilis harus berkas pribadi biasa (0600)")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	seed, err := base64.StdEncoding.DecodeString(string(trim(b)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("kunci rilis tidak valid")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func trim(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}

func main() {
	name := flag.String("name", "osquery", "nama alat")
	file := flag.String("file", "", "berkas artefak (tarball resmi)")
	keyPath := flag.String("key", "release.key", "kunci privat rilis agent")
	out := flag.String("out", "dist/agent", "direktori rilis agent")
	goos := flag.String("os", "linux", "GOOS artefak")
	goarch := flag.String("arch", "amd64", "GOARCH artefak")
	fetch := flag.Bool("fetch", false, "unduh artefak yang dipatok ke -dir lalu verifikasi")
	cacheDir := flag.String("dir", "deploy/osquery", "direktori cache artefak untuk -fetch")
	flag.Parse()
	if *fetch {
		path, err := fetchPinned(*goos, *goarch, *cacheDir)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("artefak terverifikasi:", path)
		return
	}
	if *name != "osquery" || *file == "" {
		log.Fatal("pemakaian: agenttool -name osquery -file <tarball> -key <kunci> -out <rilis>")
	}
	pin, ok := osquery.PinnedArtifact(*goos, *goarch)
	if !ok {
		log.Fatalf("tidak ada artefak yang dipatok untuk %s/%s", *goos, *goarch)
	}
	f, err := os.Open(*file)
	if err != nil {
		log.Fatal(err)
	}
	h := sha256.New()
	n, err := io.Copy(h, f)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if sum != pin.SHA256 || n != pin.Bytes {
		log.Fatalf("menolak menandatangani: hash/ukuran tidak cocok dengan artefak yang dipatok (dapat %s %d byte, diharapkan %s %d byte)", sum, n, pin.SHA256, pin.Bytes)
	}
	priv, err := loadKey(*keyPath)
	if err != nil {
		log.Fatal(err)
	}
	dir := filepath.Join(*out, "tools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	base := filepath.Base(pin.File)
	if err := copyFile(*file, filepath.Join(dir, base)); err != nil {
		log.Fatal(err)
	}
	sig := ed25519.Sign(priv, agentproto.SignedToolMessage(*name, osquery.Version, *goos, *goarch, sum))
	m := manifest{Tools: map[string]entry{}}
	if b, err := os.ReadFile(filepath.Join(dir, "manifest.json")); err == nil {
		_ = json.Unmarshal(b, &m)
		if m.Tools == nil {
			m.Tools = map[string]entry{}
		}
	}
	e := m.Tools[*name]
	if e.Version != osquery.Version || e.Targets == nil {
		e = entry{Version: osquery.Version, Targets: map[string]target{}}
	}
	e.Targets[*goos+"/"+*goarch] = target{File: base, SHA256: sum, Signature: base64.StdEncoding.EncodeToString(sig)}
	m.Tools[*name] = e
	raw, _ := json.MarshalIndent(m, "", "  ")
	tmp := filepath.Join(dir, ".manifest.tmp")
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		log.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, "manifest.json")); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s %s (%s/%s) ditandatangani: sha256=%s\n", *name, osquery.Version, *goos, *goarch, sum)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// fetchPinned mengunduh artefak yang dipatok dan memverifikasinya terhadap hash dan ukuran yang dipatok sebelum menyimpannya.
func fetchPinned(goos, goarch, dir string) (string, error) {
	pin, ok := osquery.PinnedArtifact(goos, goarch)
	if !ok {
		return "", fmt.Errorf("tidak ada artefak yang dipatok untuk %s/%s", goos, goarch)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, filepath.Base(pin.File))
	if ok, _ := verifyFile(dst, pin); ok {
		return dst, nil
	}
	resp, err := (&http.Client{Timeout: 15 * time.Minute}).Get(pin.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("sumber menjawab %d", resp.StatusCode)
	}
	tmp := dst + ".part"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(out, io.LimitReader(resp.Body, pin.Bytes+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	if ok, why := verifyFile(tmp, pin); !ok {
		os.Remove(tmp)
		return "", fmt.Errorf("artefak unduhan ditolak: %s", why)
	}
	return dst, os.Rename(tmp, dst)
}

func verifyFile(path string, pin osquery.Artifact) (bool, string) {
	f, err := os.Open(path)
	if err != nil {
		return false, err.Error()
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return false, err.Error()
	}
	if sum := hex.EncodeToString(h.Sum(nil)); n != pin.Bytes || sum != pin.SHA256 {
		return false, fmt.Sprintf("hash/ukuran tidak cocok (dapat %s, %d byte)", sum, n)
	}
	return true, ""
}
