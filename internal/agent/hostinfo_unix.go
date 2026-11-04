//go:build !windows

package agent

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// detectOSVersion: Linux membaca /etc/os-release; macOS menjalankan /usr/bin/sw_vers dengan argumen tetap.
func detectOSVersion() string {
	switch runtime.GOOS {
	case "linux":
		for _, p := range []string{"/etc/os-release", "/usr/lib/os-release"} {
			if b, err := os.ReadFile(p); err == nil {
				return parseOSRelease(string(b))
			}
		}
	case "darwin":
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "/usr/bin/sw_vers", "-productVersion").Output(); err == nil {
			return parseSwVers(string(out))
		}
	}
	return ""
}
