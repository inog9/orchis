// Package domain berisi nilai tetap yang dipakai bersama (satu sumber, agar validasi dan aturan grup tidak menyimpang).
package domain

var (
	Categories = []string{"host", "web_application", "cloud_resource", "container", "network_device",
		"mobile_device", "iot_ot_device", "identity", "saas_application", "ai_target", "generic"}
	Platforms     = []string{"windows", "linux", "macos", "ios", "android", "other"}
	Arches        = []string{"x86_64", "arm64", "other"}
	Criticalities = []string{"low", "medium", "high", "critical"}
)

func OneOf(v string, set []string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}
