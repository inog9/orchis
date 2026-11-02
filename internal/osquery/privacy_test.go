package osquery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactCredentialForms(t *testing.T) {
	for _, input := range []string{
		"Authorization: Basic secret-marker", "password=\"secret-marker with spaces", "Authorization: Bearer secret-marker", "bearer secret-marker", "password=\"secret-marker with spaces\"", "token='secret-marker with spaces'",
		"https://user:secret-marker@localhost/path", "-----BEGIN RSA PRIVATE KEY-----\nsecret-marker\n-----END RSA PRIVATE KEY-----",
		"-----BEGIN PRIVATE KEY-----\nsecret-marker", "api-key: secret-marker",
	} {
		got := Redact(input)
		if strings.Contains(got, "secret-marker") || !strings.Contains(got, "[redacted") {
			t.Errorf("redaksi gagal untuk bentuk %q", input)
		}
	}
	if got := Redact("Debian 12 /usr/bin/bash"); got != "Debian 12 /usr/bin/bash" {
		t.Fatalf("data biasa berubah: %q", got)
	}
}

func TestSanitizeMetadataAndJSONBudget(t *testing.T) {
	q := *Find("osquery.users")
	in := &Dataset{Query: q.Key, Status: StatusFailed, Reason: "Authorization: Bearer secret-marker", Version: "token=secret-marker"}
	ds, err := Sanitize(&q, in)
	if err != nil || strings.Contains(ds.Reason, "secret-marker") || ds.Version != "" {
		t.Fatalf("metadata tidak aman: %+v %v", ds, err)
	}
	q.MaxRows = 10000
	in.Reason = strings.Repeat("\x00", 200)
	in.Status = StatusOK
	in.Version = Version
	// Escape JSON membutuhkan lebih banyak byte daripada panjang sel sebelum encoding.
	for i := 0; i < q.MaxRows; i++ {
		in.Rows = append(in.Rows, map[string]string{"username": strings.Repeat("\x00", 512), "shell": strings.Repeat("界", 512)})
	}
	ds, err = Sanitize(&q, in)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(ds)
	if len(b) > MaxDatasetBytes || !ds.Truncated || len(ds.Rows) == 0 || ds.Version != Version {
		t.Fatalf("batas hasil gagal: %d byte, %d baris, terpotong=%v", len(b), len(ds.Rows), ds.Truncated)
	}
	again, _ := Sanitize(&q, in)
	b2, _ := json.Marshal(again)
	// CollectedAt ditentukan pemanggil agar perbandingan deterministik.
	again.CollectedAt = ds.CollectedAt
	b2, _ = json.Marshal(again)
	if string(b) != string(b2) {
		t.Fatal("pemotongan dataset tidak deterministik")
	}
	for _, row := range ds.Rows {
		for _, cell := range row {
			if len([]rune(cell)) > maxCell {
				t.Fatal("sel melebihi batas")
			}
		}
	}
}

func TestRunnerBoundsAndRedactsDiagnostics(t *testing.T) {
	q := Find("osquery.inventory")
	for name, body := range map[string]string{
		"secret": "echo 'Authorization: Bearer secret-marker' >&2; exit 1",
		"large":  "head -c 100000 /dev/zero >&2; exit 1",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "osquery")
			if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			ds := (&Runner{Bin: path, GOOS: "linux"}).Run(context.Background(), q)
			if ds.Status != StatusFailed || strings.Contains(ds.Reason, "secret-marker") || len(ds.Reason) > 200 {
				t.Fatalf("diagnostik: %+v", ds)
			}
			if name == "large" && ds.Reason != "the result was too large" {
				t.Fatalf("batas stderr tidak dikenali: %q", ds.Reason)
			}
		})
	}
}

func TestEveryQueryIsOptInEvenWhenLowSensitivity(t *testing.T) {
	q := Query{Sensitivity: "low"}
	if q.DefaultEnabled() {
		t.Fatal("query rendah juga tidak boleh aktif tanpa persetujuan")
	}
	q.Pack = "future-pack"
	if q.DefaultEnabled() {
		t.Fatal("pack tidak boleh aktif tanpa opt-in")
	}
	q.Pack = ""
	q.Sensitivity = "medium"
	if q.DefaultEnabled() {
		t.Fatal("query sensitif tidak boleh aktif tanpa persetujuan")
	}
}
