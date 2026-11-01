package mitre

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchUsesCommittedPinAndInstallsAtomically(t *testing.T) {
	raw, err := os.ReadFile("testdata/bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	ds, err := ParseVerified(bytes.NewReader(raw), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range []string{"", "hash", "size", "version", "counts", "status"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			requests := 0
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if failure == "status" {
					w.WriteHeader(503)
					return
				}
				w.Write(raw)
			}))
			defer srv.Close()
			m := &Manifest{Matrix: "enterprise", Version: ds.Version, File: "enterprise-attack-" + ds.Version + ".json", SHA256: ds.SHA256, Bytes: int64(len(raw)), SourceURL: srv.URL}
			m.Counts = m.counts(ds)
			switch failure {
			case "hash":
				m.SHA256 = strings.Repeat("0", 64)
			case "size":
				m.Bytes++
			case "version":
				m.Version = "98.0"
				m.File = "enterprise-attack-98.0.json"
			case "counts":
				m.Counts.Techniques++
			}
			manifest, _ := json.Marshal(m)
			if err := os.WriteFile(filepath.Join(dir, ManifestName), manifest, 0o644); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(dir, m.File)
			prior := []byte("prior invalid cache preserved until verified replacement")
			if err := os.WriteFile(dest, prior, 0o644); err != nil {
				t.Fatal(err)
			}
			_, _, err := fetch(context.Background(), dir, srv.Client())
			got, _ := os.ReadFile(dest)
			kept, _ := os.ReadFile(filepath.Join(dir, ManifestName))
			if !bytes.Equal(kept, manifest) {
				t.Fatal("fetch changed the committed manifest")
			}
			if failure != "" {
				if err == nil {
					t.Fatalf("%s pin was ignored", failure)
				}
				if !bytes.Equal(got, prior) {
					t.Fatal("invalid download replaced existing cache")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, raw) {
					t.Fatal("download was not installed")
				}
				info, _ := os.Stat(dest)
				if info.Mode().Perm() != 0o644 {
					t.Fatal("artifact not readable by runtime user")
				}
				if _, _, err := fetch(context.Background(), dir, srv.Client()); err != nil {
					t.Fatal(err)
				}
				if requests != 1 {
					t.Fatal("valid cache triggered another network request")
				}
				changed := *m
				changed.Bytes++
				bad, _ := json.Marshal(changed)
				os.WriteFile(filepath.Join(dir, ManifestName), bad, 0o644)
				if _, _, err := Verify(dir); err == nil {
					t.Fatal("manifest size mismatch was ignored")
				}
			}
			after, _ := os.ReadFile(filepath.Join(dir, ManifestName))
			if failure != "" && !bytes.Equal(after, manifest) {
				t.Fatal("fetch silently changed the pin")
			}
			files, _ := filepath.Glob(filepath.Join(dir, ".fetch-*"))
			if len(files) != 0 {
				t.Fatal("temporary download left behind")
			}
		})
	}
}

func TestManifestRejectsUnsafeMetadataBeforeNetwork(t *testing.T) {
	raw, _ := os.ReadFile("testdata/bundle.json")
	ds, err := ParseVerified(bytes.NewReader(raw), "")
	if err != nil {
		t.Fatal(err)
	}
	base := Manifest{Matrix: "enterprise", Version: ds.Version, File: "enterprise-attack-" + ds.Version + ".json", SHA256: ds.SHA256, Bytes: int64(len(raw)), SourceURL: "https://example.invalid/bundle.json"}
	base.Counts = base.counts(ds)
	for _, change := range []func(*Manifest){
		func(m *Manifest) { m.File = "../outside.json" },
		func(m *Manifest) { m.Version = "../20.0" },
		func(m *Manifest) { m.Matrix = "mobile" },
		func(m *Manifest) { m.SHA256 = "short" },
		func(m *Manifest) { m.Bytes = 0 },
		func(m *Manifest) { m.Bytes = maxDownload + 1 },
		func(m *Manifest) { m.SourceURL = "http://example.invalid/bundle.json" },
		func(m *Manifest) { m.SourceURL = "https://user:pass@example.invalid/bundle.json" },
		func(m *Manifest) { m.Counts.Tactics = 0 },
	} {
		m := base
		change(&m)

		dir := t.TempDir()
		encoded, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(dir, ManifestName), encoded, 0o644); err != nil {
			t.Fatal(err)
		}
		transport := &denyFetchTransport{}
		if _, _, err := fetch(context.Background(), dir, &http.Client{Transport: transport}); err == nil || transport.calls != 0 {
			t.Fatalf("unsafe metadata reached network or was accepted: calls=%d err=%v", transport.calls, err)
		}

	}
}

type denyFetchTransport struct{ calls int }

func (d *denyFetchTransport) RoundTrip(*http.Request) (*http.Response, error) {
	d.calls++
	return nil, errors.New("test refuses all network requests")
}
