package backupseal

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestAuthenticatedStreamingArchive(t *testing.T) {
	key := strings.Repeat("ab", 32)
	plain := bytes.Repeat([]byte("private snapshot fixture"), 10000)
	var sealed bytes.Buffer
	if e := Seal(&sealed, bytes.NewReader(plain), key); e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(sealed.Bytes(), []byte("private snapshot")) {
		t.Fatal("archive exposed plaintext")
	}
	var restored bytes.Buffer
	if e := Open(&restored, bytes.NewReader(sealed.Bytes()), key); e != nil || !bytes.Equal(plain, restored.Bytes()) {
		t.Fatal("roundtrip", e)
	}
	for _, tc := range []struct {
		name string
		b    []byte
		key  string
	}{
		{"wrong key", sealed.Bytes(), strings.Repeat("cd", 32)},
		{"truncated tag", sealed.Bytes()[:sealed.Len()-1], key},
		{"missing final chunk", sealed.Bytes()[:sealed.Len()-65536], key},
		{"trailing data", append(bytes.Clone(sealed.Bytes()), 1, 2, 3), key},
		{"weak key", sealed.Bytes(), "short"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if e := Open(io.Discard, bytes.NewReader(tc.b), tc.key); e == nil {
				t.Fatal("untrusted archive accepted")
			}
		})
	}
	for _, position := range []int{10, sealed.Len() / 2, sealed.Len() - 10} {
		b := bytes.Clone(sealed.Bytes())
		b[position] ^= 1
		if e := Open(io.Discard, bytes.NewReader(b), key); e == nil {
			t.Fatal("modified archive accepted")
		}
	}
}
