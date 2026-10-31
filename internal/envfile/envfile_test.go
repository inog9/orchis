package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		mode       os.FileMode
		valid      bool
	}{
		{"private", `{"ORCHIS_FILE_TEST":"private-value"}`, 0600, true},
		{"public", `{"ORCHIS_FILE_TEST":"private-value"}`, 0644, false},
		{"foreign", `{"PATH":"/evil"}`, 0600, false},
		{"recursive", `{"ORCHIS_ENV_FILE":"other"}`, 0600, false},
		{"trailing", `{} {}`, 0600, false},
		{"null", `null`, 0600, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "secret.json")
			if e := os.WriteFile(p, []byte(tc.body), tc.mode); e != nil {
				t.Fatal(e)
			}
			t.Setenv("ORCHIS_ENV_FILE", p)
			t.Setenv("ORCHIS_FILE_TEST", "")
			e := Load()
			if (e == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, e)
			}
			if tc.valid && os.Getenv("ORCHIS_FILE_TEST") != "private-value" {
				t.Fatal("secret was not loaded")
			}
		})
	}
}
