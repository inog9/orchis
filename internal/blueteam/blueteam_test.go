package blueteam

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func baseInput() Input {
	return Input{
		Tenant: uuid.New(), Asset: uuid.New(), AssetName: "macbook", Now: time.Now(), OfflineMinutes: 15,
		Datasets:  map[string][]map[string]string{},
		Baselines: map[string][]map[string]string{},
	}
}

func TestTamperRevokedIsCritical(t *testing.T) {
	in := baseInput()
	rev := time.Now().Add(-time.Hour)
	in.Agents = []AgentRow{{ID: uuid.New(), Hostname: "h", Status: "online", UninstallStatus: "none", RevokedAt: &rev}}
	fs := Run(in)
	if len(fs) != 1 || fs[0].Type != "agent_tamper" || fs[0].Severity != "critical" {
		t.Fatalf("got %+v", fs)
	}
}

func TestNewSoftwareAndPortAndAccountDiffs(t *testing.T) {
	in := baseInput()
	in.Baselines["osquery.packages"] = []map[string]string{{"name": "tar", "version": "7.5.15"}}
	in.Datasets["osquery.packages"] = []map[string]string{{"name": "tar", "version": "7.5.15"}, {"name": "evil", "version": "1.0"}}
	in.Baselines["osquery.network"] = []map[string]string{}
	in.Datasets["osquery.network"] = []map[string]string{{"address": "0.0.0.0", "port": "4444"}}
	in.Baselines["osquery.users"] = []map[string]string{}
	in.Datasets["osquery.users"] = []map[string]string{{"uid": "0", "username": "toor"}}
	fs := Run(in)
	types := map[string]bool{}
	for _, f := range fs {
		types[f.Type] = true
	}
	if !types["new_software_installed"] || !types["new_listening_port"] || !types["new_local_account"] {
		t.Fatalf("missing types: %+v", fs)
	}
	for _, f := range fs {
		if f.Type == "new_local_account" && f.Severity != "critical" {
			t.Fatalf("uid0 account must be critical: %+v", f)
		}
	}
}

func TestC2NonStandardPort(t *testing.T) {
	in := baseInput()
	in.Connections = []Connection{{Process: "x", DstIP: "203.0.113.9", DstPort: 4444, Count: 3, State: "ESTABLISHED"}}
	fs := Run(in)
	found := false
	for _, f := range fs {
		if f.Type == "c2_suspect_connection" && f.Severity == "high" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no c2 finding: %+v", fs)
	}
}

func TestStandardPortNotFlagged(t *testing.T) {
	in := baseInput()
	in.Connections = []Connection{{Process: "browser", DstIP: "93.184.216.34", DstPort: 443, Count: 2}}
	fs := Run(in)
	for _, f := range fs {
		if f.Type == "c2_suspect_connection" {
			t.Fatalf("443 should not be flagged: %+v", f)
		}
	}
}

// Putaran pertama (belum ada baseline) hanya mencatat baseline: tidak boleh membanjiri temuan "baru".
func TestFirstPassWithoutBaselineCreatesNoFindings(t *testing.T) {
	in := baseInput()
	in.Datasets["osquery.packages"] = []map[string]string{{"name": "tar", "version": "7.5.15"}, {"name": "curl", "version": "8.0"}}
	in.Datasets["osquery.programs"] = []map[string]string{{"name": "Autopsy", "version": "4.23.1"}}
	in.Datasets["osquery.network"] = []map[string]string{{"address": "0.0.0.0", "port": "22"}}
	in.Datasets["osquery.users"] = []map[string]string{{"uid": "0", "username": "root"}, {"uid": "1000", "username": "ada"}}
	in.Datasets["osquery.user_groups"] = []map[string]string{{"user_id": "1000", "group_id": "0"}}
	if fs := Run(in); len(fs) != 0 {
		t.Fatalf("first pass must only record a baseline, got %d findings: %+v", len(fs), fs[:1])
	}
	// Setelah baseline ada, perubahan nyata tetap terdeteksi.
	in.Baselines = map[string][]map[string]string{"osquery.network": in.Datasets["osquery.network"]}
	in.Datasets["osquery.network"] = append(in.Datasets["osquery.network"], map[string]string{"address": "0.0.0.0", "port": "9999"})
	fs := Run(in)
	if len(fs) != 1 || fs[0].Type != "new_listening_port" {
		t.Fatalf("a port opened after the baseline must be detected: %+v", fs)
	}
}

func TestEphemeralListeningPortsAreNotReported(t *testing.T) {
	in := Input{AssetName: "h",
		Baselines: map[string][]map[string]string{"osquery.network": {{"port": "445", "address": "0.0.0.0", "protocol": "6", "process": "System"}}},
		Datasets: map[string][]map[string]string{"osquery.network": {
			{"port": "445", "address": "0.0.0.0", "protocol": "6", "process": "System"},
			{"port": "9999", "address": "0.0.0.0", "protocol": "6", "process": "python.exe"},
			{"port": "52377", "address": "0.0.0.0", "protocol": "6", "process": "svchost.exe"},
			{"port": "49152", "address": "::", "protocol": "6", "process": "x"},
			{"port": "40000", "address": "0.0.0.0", "protocol": "6", "process": "linux-ephemeral"}}}}
	var ports []string
	for _, f := range Run(in) {
		if f.Type == "new_listening_port" {
			ports = append(ports, f.Value)
			if f.Severity != "critical" { // 9999 dibuka python.exe pada 0.0.0.0: server ad-hoc terpapar
				t.Fatalf("an exposed ad-hoc server must be critical, got %s", f.Severity)
			}
		}
	}
	if len(ports) != 1 || ports[0] != "0.0.0.0:9999" {
		t.Fatalf("expected only port 9999, got %v", ports)
	}
}

func TestListeningSeverityUsesExposureAndProcess(t *testing.T) {
	cases := []struct{ addr, port, proc, want string }{
		{"0.0.0.0", "9999", "python.exe", "critical"},
		{"0.0.0.0", "3389", "svchost.exe", "critical"},
		{"::", "445", "System", "critical"},
		{"0.0.0.0", "3000", "node.exe", "critical"},
		{"0.0.0.0", "8080", "nginx", "high"},
		{"192.168.1.23", "8443", "myapp", "high"},
		{"127.0.0.1", "9999", "python3", "low"},
		{"::1", "3389", "x", "low"},
		{"fe80::2db7:7cdf:acb1:492b", "546", "svchost.exe", "low"},
		{"169.254.6.3", "8000", "x", "low"},
	}
	for _, c := range cases {
		if got := listeningSeverity(c.addr, c.port, c.proc); got != c.want {
			t.Errorf("%s:%s (%s) = %s, want %s", c.addr, c.port, c.proc, got, c.want)
		}
	}
}
