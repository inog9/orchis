package server_test

import (
	"context"
	"testing"
)

// Platform dan arsitektur aset agent berasal dari agent: isian yang menyimpang disembuhkan oleh heartbeat dan tidak bisa
// diubah dari konsol (kasus aset MacBook Intel yang tercatat arm64).
func TestAgentAssetPlatformAndArchComeFromTheAgent(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.authorizedAgent(t, "platform-wins")
	ctx := context.Background()
	read := func() (string, string) {
		var p, a string
		f.a.env.Owner.QueryRow(ctx, `SELECT COALESCE(platform,''),COALESCE(arch,'') FROM assets WHERE id=$1`, asset).Scan(&p, &a)
		return p, a
	}
	if _, e := f.a.env.Owner.Exec(ctx, `UPDATE assets SET platform='macos', arch='arm64' WHERE id=$1`, asset); e != nil {
		t.Fatal(e)
	}
	if st := f.agentCall("POST", "heartbeat", map[string]any{"version": "1.3.1", "protocol_version": 1}, secret).Status; st != 200 {
		t.Fatalf("heartbeat: %d", st)
	}
	if p, a := read(); p != "linux" || a != "x86_64" {
		t.Fatalf("agent value did not win: %s/%s", p, a)
	}
	var cur struct{ Version int }
	f.a.do("GET", "/api/v1/assets/"+asset, nil, f.admin).JSON(&cur)
	for name, body := range map[string]map[string]any{
		"platform":       {"platform": "macos"},
		"arch":           {"arch": "arm64"},
		"clear platform": {"clear_platform": true},
		"clear arch":     {"clear_arch": true},
	} {
		body["version"] = cur.Version
		if r := f.a.do("PATCH", "/api/v1/assets/"+asset, body, f.admin); r.Status != 422 {
			t.Fatalf("%s change on an agent asset: %d %s", name, r.Status, r.Body)
		}
	}
	// Mengirim ulang nilai yang sama (seperti form edit) tetap diterima.
	if r := f.a.do("PATCH", "/api/v1/assets/"+asset, map[string]any{"version": cur.Version, "platform": "linux", "arch": "x86_64", "name": "renamed"}, f.admin); r.Status != 200 {
		t.Fatalf("unchanged platform/arch rejected: %d %s", r.Status, r.Body)
	}
}

func TestHeartbeatReportsOSVersionIntoAssetWithoutOverridingManualEntry(t *testing.T) {
	f := newAgentFixture(t)
	secret, asset := f.authorizedAgent(t, "hostinfo")
	ctx := context.Background()
	beat := func(v any) int {
		return f.agentCall("POST", "heartbeat", map[string]any{"version": "1.3.1", "protocol_version": 1, "os_version": v}, secret).Status
	}
	attr := func(key string) string {
		var out *string
		f.a.env.Owner.QueryRow(ctx, `SELECT attributes->>$2 FROM assets WHERE id=$1`, asset, key).Scan(&out)
		if out == nil {
			return ""
		}
		return *out
	}

	if st := beat("macOS 10.15.7"); st != 200 {
		t.Fatalf("heartbeat: %d", st)
	}
	if got := attr("agent_os_version"); got != "macOS 10.15.7" {
		t.Fatalf("agent_os_version = %q", got)
	}

	// Isian manual pengguna tidak pernah ditimpa; agent hanya menulis kunci agent_os_version.
	var cur struct{ Version int }
	f.a.do("GET", "/api/v1/assets/"+asset, nil, f.admin).JSON(&cur)
	r := f.a.do("PATCH", "/api/v1/assets/"+asset, map[string]any{"version": cur.Version, "attributes": map[string]any{"os_version": "manual 1.0", "role": "Laptop", "agent_os_version": "macOS 10.15.7"}}, f.admin)
	if r.Status != 200 {
		t.Fatalf("manual edit: %d %s", r.Status, r.Body)
	}
	if st := beat("macOS 10.15.8"); st != 200 {
		t.Fatalf("heartbeat 2: %d", st)
	}
	if attr("os_version") != "manual 1.0" || attr("role") != "Laptop" {
		t.Fatalf("manual attributes were overwritten: os_version=%q role=%q", attr("os_version"), attr("role"))
	}
	if attr("agent_os_version") != "macOS 10.15.8" {
		t.Fatalf("agent value was not refreshed: %q", attr("agent_os_version"))
	}

	// Data tidak tepercaya: nilai berbentuk perintah/terlalu panjang/kendali diabaikan, heartbeat tetap sukses.
	for _, hostile := range []string{"$(rm -rf /)", "x;y", "line1\nline2", string(make([]byte, 200)), "<script>alert(1)</script>"} {
		if st := beat(hostile); st != 200 {
			t.Fatalf("hostile os_version broke the heartbeat: %d", st)
		}
		if got := attr("agent_os_version"); got != "macOS 10.15.8" {
			t.Fatalf("hostile os_version %q was stored: %q", hostile, got)
		}
	}
}
