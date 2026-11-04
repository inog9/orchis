//go:build windows

package agent

import "errors"

// Windows remote physical removal remains unavailable until the Windows service
// helper and real device evidence exist. The agent never advertises this feature.
func StartUninstallHelper(string, *Config) error {
	return errors.New("remote uninstall is not supported on Windows yet")
}
func AcquireUninstallLock(string) (func(), error) {
	return nil, errors.New("remote uninstall is not supported on Windows yet")
}
