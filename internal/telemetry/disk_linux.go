//go:build linux

package telemetry

import "syscall"

func (m *Registry) Disk(path string) {
	var stat syscall.Statfs_t
	if syscall.Statfs(path, &stat) != nil {
		m.Set("orchis_disk_sample_up", 0)
		return
	}
	m.Set("orchis_disk_sample_up", 1)
	m.Set("orchis_backup_disk_free_bytes", float64(stat.Bavail)*float64(stat.Bsize))
	m.Set("orchis_backup_disk_total_bytes", float64(stat.Blocks)*float64(stat.Bsize))
}
