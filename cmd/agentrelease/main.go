// agentrelease membangun biner agent untuk beberapa platform, menandatanganinya (Ed25519), dan menulis manifest.json
// yang disajikan server. Kunci publik ditanam ke biner saat build, sehingga agent hanya mau memasang rilis yang ditandatangani
// oleh pemegang kunci privat ini.
//
//	agentrelease -version 1.0.0 -key release.key -out dist/agent [-targets linux/amd64,windows/amd64] [-init]
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"orchis-go/internal/agentproto"
)

var defaultTargets = []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64", "windows/arm64"}

type target struct {
	File      string `json:"file"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
}

func loadOrCreateKey(path string, create bool) (ed25519.PrivateKey, error) {
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 128 {
			return nil, fmt.Errorf("release key must be a private regular file (0600)")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("cannot read release key")
		}
		seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if err != nil || len(seed) != ed25519.SeedSize {
			return nil, fmt.Errorf("invalid release key")
		}
		return ed25519.NewKeyFromSeed(seed), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("cannot inspect release key")
	}
	if !create {
		return nil, fmt.Errorf("release key missing; use -init once")
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("cannot create key directory")
	}
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("cannot create release key exclusively")
	}
	_, err = out.WriteString(base64.StdEncoding.EncodeToString(priv.Seed()) + "\n")
	if err == nil {
		err = out.Sync()
	}
	closed := out.Close()
	if err == nil {
		err = closed
	}
	if err != nil {
		return nil, fmt.Errorf("cannot persist release key")
	}
	fmt.Println("kunci rilis baru dibuat di", path, "(simpan dengan aman; siapa pun yang memegangnya bisa merilis agent)")
	return priv, nil
}

func main() {
	version := flag.String("version", "", "versi rilis MAJOR.MINOR.PATCH")
	keyPath := flag.String("key", "release.key", "berkas kunci privat (seed Ed25519, base64)")
	out := flag.String("out", "dist/agent", "direktori keluaran")
	targets := flag.String("targets", strings.Join(defaultTargets, ","), "daftar platform GOOS/GOARCH dipisah koma")
	initKey := flag.Bool("init", false, "buat kunci bila belum ada")
	flag.Parse()
	if !agentproto.ValidVersion(*version) {
		log.Fatal("-version harus berbentuk MAJOR.MINOR.PATCH, mis. 1.0.0")
	}
	priv, err := loadOrCreateKey(*keyPath, *initKey)
	if err != nil {
		log.Fatal(err)
	}
	pub := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	// Kunci publik (bukan rahasia) disajikan ke konsol supaya perintah pemasangan memuat --release-key.
	if err := os.WriteFile(filepath.Join(*out, "release.pub"), []byte(pub+"\n"), 0o644); err != nil {
		log.Fatal(err)
	}
	m := struct {
		Version string            `json:"version"`
		Targets map[string]target `json:"targets"`
	}{Version: *version, Targets: map[string]target{}}
	for _, t := range strings.Split(*targets, ",") {
		goos, goarch, ok := strings.Cut(strings.TrimSpace(t), "/")
		if !ok {
			log.Fatalf("platform %q tidak valid", t)
		}
		name := fmt.Sprintf("orchis-agent-%s-%s-%s", *version, goos, goarch)
		if goos == "windows" {
			name += ".exe"
		}
		file := filepath.Join(*out, name)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", fmt.Sprintf("-s -w -X main.version=%s -X main.pubKeyB64=%s", *version, pub), "-o", file, "./cmd/agent")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatalf("build %s gagal: %v", t, err)
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			log.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		h := hex.EncodeToString(sum[:])
		sig := ed25519.Sign(priv, agentproto.SignedMessage(*version, goos, goarch, h))
		m.Targets[goos+"/"+goarch] = target{File: name, SHA256: h, Signature: base64.StdEncoding.EncodeToString(sig)}
		fmt.Printf("%-16s %s  %s\n", t, h[:16], name)
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	tmp := filepath.Join(*out, ".manifest.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		log.Fatal(err)
	}
	// manifest ditulis TERAKHIR dan secara atomik: server tidak pernah melihat manifest yang menunjuk ke berkas yang belum ada.
	if err := os.Rename(tmp, filepath.Join(*out, "manifest.json")); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("rilis %s siap di %s. Kunci publik tertanam di biner: %s\n", *version, *out, pub)
}
