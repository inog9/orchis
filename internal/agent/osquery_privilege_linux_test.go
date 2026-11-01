package agent

import (
	"context"
	"orchis-go/internal/osquery"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOsqueryRootQuerySkipsBeforeInstalling(t *testing.T) {
	if os.Getenv("ORCHIS_TEST_ROOTLESS_OSQUERY") == "1" || os.Geteuid() != 0 {
		var key string
		for _, q := range osquery.Queries {
			if q.NeedsRoot {
				key = q.Key
				break
			}
		}
		if key == "" {
			t.Fatal("katalog tidak punya query administratif")
		}
		ds, err := (&Agent{}).runOsquery(context.Background(), key)
		if err != nil || len(ds) != 1 || ds[0].Status != osquery.StatusSkipped || ds[0].Reason != osquery.ReasonNeedsRoot {
			t.Fatalf("harus dilewati sebelum pemasangan: %+v %v", ds, err)
		}
		return
	}
	// Biner tes disalin ke direktori yang dapat dilintasi pengguna tanpa hak root.
	dir, err := os.MkdirTemp("", "orchis-rootless-query-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err = os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	self, _ := os.Executable()
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "test")
	if err = os.WriteFile(bin, data, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "-test.run=^TestOsqueryRootQuerySkipsBeforeInstalling$")
	cmd.Env = append(os.Environ(), "ORCHIS_TEST_ROOTLESS_OSQUERY=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("uji pengguna biasa: %v %s", err, out)
	}
}
