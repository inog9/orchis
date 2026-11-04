//go:build !windows

package agent

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

func StartUninstallHelper(exe string, cfg *Config) error {
	if cfg == nil || cfg.Uninstall == nil {
		return errors.New("missing uninstall receipt")
	}
	id, e := uuid.Parse(cfg.Uninstall.ID)
	if e != nil {
		return errors.New("invalid uninstall receipt")
	}
	if runtime.GOOS == "linux" && cfg.Uninstall.Managed {
		// Setsid remains in the original systemd cgroup. The transient service
		// survives stopping that unit. No credentials are put in process arguments.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		unit := "orchis-uninstall-" + id.String()
		args := []string{"--quiet", "--collect", "--property=Type=exec", "--unit=" + unit}
		if cfg.Mode == "user" {
			args = append(args, "--user")
		}
		args = append(args, exe, "remote-uninstall")
		cmd := exec.CommandContext(ctx, "systemd-run", args...)
		if e := cmd.Run(); e != nil {
			checkArgs := []string{"is-active", "--quiet", unit}
			if cfg.Mode == "user" {
				checkArgs = append(checkArgs, "--user")
			}
			check := exec.CommandContext(ctx, "systemctl", checkArgs...)
			if check.Run() == nil {
				return nil
			}
			return errors.New("cannot start independent uninstall service")
		}
		return nil
	}
	cmd := exec.Command(exe, "remote-uninstall")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e := cmd.Start(); e != nil {
		return e
	}
	return cmd.Process.Release()
}

// AcquireUninstallLock is process-scoped and automatically released after crash.
func AcquireUninstallLock(path string) (func(), error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
