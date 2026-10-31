//go:build !linux

package telemetry

func (m *Registry) Disk(path string) { m.Set("orchis_disk_sample_up", 0) }
