//go:build windows

package agent

import (
	"context"
	"os/exec"
	"time"
)

// detectOSVersion: Windows menjalankan `cmd /c ver` dengan argumen tetap dan mem-parse versinya.
func detectOSVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "cmd", "/c", "ver").Output()
	if err != nil {
		return ""
	}
	return parseWindowsVer(string(out))
}
