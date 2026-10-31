package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseKeyCannotBeReplacedByInit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "release.key")
	if _, e := loadOrCreateKey(path, false); e == nil {
		t.Fatal("missing key accepted")
	}
	first, e := loadOrCreateKey(path, true)
	if e != nil {
		t.Fatal(e)
	}
	again, e := loadOrCreateKey(path, true)
	if e != nil || !bytes.Equal(first, again) {
		t.Fatal("init replaced existing trust root")
	}
	if e = os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = loadOrCreateKey(path, true); e == nil {
		t.Fatal("public key file permissions accepted")
	}
	if e = os.Chmod(path, 0600); e != nil {
		t.Fatal(e)
	}
	link := path + ".link"
	if e = os.Symlink(path, link); e != nil {
		t.Fatal(e)
	}
	if _, e = loadOrCreateKey(link, true); e == nil {
		t.Fatal("symlink key accepted")
	}
	if e = os.WriteFile(path, []byte("invalid"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = loadOrCreateKey(path, true); e == nil {
		t.Fatal("damaged key replaced")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "invalid" {
		t.Fatal("damaged trust root overwritten")
	}
}
