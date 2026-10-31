//go:build windows

package agent

import "os"

// Reexec di Windows: keluar dengan kode 3; Service Control Manager menghidupkan ulang (kebijakan pemulihan service).
func Reexec(exe string) { os.Exit(3) }
