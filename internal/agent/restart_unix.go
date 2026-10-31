//go:build !windows

package agent

import (
	"os"
	"syscall"
)

// Reexec menggantikan proses ini dengan biner yang baru dipasang (mode non-service). Bila gagal, keluar dengan kode 3
// supaya pengawas proses (systemd/launchd) menghidupkannya ulang.
func Reexec(exe string) {
	_ = syscall.Exec(exe, os.Args, os.Environ())
	os.Exit(3)
}
