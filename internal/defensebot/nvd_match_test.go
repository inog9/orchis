package defensebot

import "testing"

func nvdDoc(cve string, matches ...cpeMatch) nvdResponse {
	var d nvdResponse
	d.Vulnerabilities = append(d.Vulnerabilities, struct {
		CVE struct {
			ID           string `json:"id"`
			Descriptions []struct {
				Lang  string `json:"lang"`
				Value string `json:"value"`
			} `json:"descriptions"`
			Metrics struct {
				V31 []struct {
					Data struct {
						BaseScore    float64 `json:"baseScore"`
						BaseSeverity string  `json:"baseSeverity"`
					} `json:"cvssData"`
				} `json:"cvssMetricV31"`
				V30 []struct {
					Data struct {
						BaseScore    float64 `json:"baseScore"`
						BaseSeverity string  `json:"baseSeverity"`
					} `json:"cvssData"`
				} `json:"cvssMetricV30"`
			} `json:"metrics"`
			References []struct {
				URL string `json:"url"`
			} `json:"references"`
			Configurations []struct {
				Nodes []struct {
					CPEMatch []cpeMatch `json:"cpeMatch"`
				} `json:"nodes"`
			} `json:"configurations"`
		} `json:"cve"`
	}{})
	v := &d.Vulnerabilities[0].CVE
	v.ID = cve
	v.Configurations = append(v.Configurations, struct {
		Nodes []struct {
			CPEMatch []cpeMatch `json:"cpeMatch"`
		} `json:"nodes"`
	}{Nodes: []struct {
		CPEMatch []cpeMatch `json:"cpeMatch"`
	}{{CPEMatch: matches}}})
	return d
}

// Entri versi tunggal hanya cocok untuk versi itu; entri wildcard mengikuti rentangnya.
func TestParseNVDHonoursTheVersionInsideTheCriteria(t *testing.T) {
	const pre = "cpe:2.3:a:oracle:vm_virtualbox:"
	const tail = ":*:*:*:*:*:*:*"
	single := nvdDoc("CVE-2010-0001", cpeMatch{Vulnerable: true, Criteria: pre + "3.2.8" + tail})
	if got := parseNVD(single, "oracle", "vm_virtualbox", "7.0.8"); len(got) != 0 {
		t.Fatalf("a CVE for 3.2.8 must not match 7.0.8: %+v", got)
	}
	if got := parseNVD(single, "oracle", "vm_virtualbox", "3.2.8"); len(got) != 1 {
		t.Fatal("a CVE for 3.2.8 must match 3.2.8")
	}
	ranged := nvdDoc("CVE-2023-0002", cpeMatch{Vulnerable: true, Criteria: pre + "*" + tail, EndExcl: "7.0.10"})
	if got := parseNVD(ranged, "oracle", "vm_virtualbox", "7.0.8"); len(got) != 1 || got[0].Fixed != "7.0.10" {
		t.Fatalf("7.0.8 is inside <7.0.10 and the fix is 7.0.10: %+v", got)
	}
	if got := parseNVD(ranged, "oracle", "vm_virtualbox", "7.0.12"); len(got) != 0 {
		t.Fatalf("7.0.12 is outside the range: %+v", got)
	}
	na := nvdDoc("CVE-2020-0003", cpeMatch{Vulnerable: true, Criteria: pre + "-" + tail})
	if got := parseNVD(na, "oracle", "vm_virtualbox", "7.0.8"); len(got) != 0 {
		t.Fatalf("an unversioned ('-') entry cannot be compared and must not match: %+v", got)
	}
}

func TestDateBoundedRangesDoNotMatchBuildNumbers(t *testing.T) {
	const crit = "cpe:2.3:a:valvesoftware:steam_client:*:*:*:*:*:*:*:*"
	d := nvdDoc("CVE-2021-30481", cpeMatch{Vulnerable: true, Criteria: crit, EndExcl: "2021-04-17"})
	if got := parseNVD(d, "valvesoftware", "steam_client", "2.10.91.91"); len(got) != 0 {
		t.Fatalf("a date bound cannot be compared with a build number: %+v", got)
	}
}

func TestPythonVersionFromMSIAndName(t *testing.T) {
	if got := pythonCPEVersion("3.10.6150.0"); got != "3.10.6" {
		t.Fatalf("MSI version: %s", got)
	}
	if got := pythonCPEVersion("3.14.2150.0"); got != "3.14.2" {
		t.Fatalf("MSI version: %s", got)
	}
	if got := pythonCPEVersion("3.12.1"); got != "3.12.1" {
		t.Fatalf("plain version: %s", got)
	}
	if v, p, ok := cpeFor("docker desktop", ""); !ok || v != "docker" || p != "desktop" {
		t.Fatalf("Docker Desktop must map to docker:desktop, got %s:%s", v, p)
	}
}

// CVE-2020-1907: entri iphone_os/android/portal tidak berlaku untuk WhatsApp Windows.
func TestMobileAndPortalTargetsDoNotMatchDesktopApps(t *testing.T) {
	d := nvdDoc("CVE-2020-1907",
		cpeMatch{Vulnerable: true, Criteria: "cpe:2.3:a:whatsapp:whatsapp:*:*:*:*:*:portal:*:*", EndExcl: "173.0.0.29.505"},
		cpeMatch{Vulnerable: true, Criteria: "cpe:2.3:a:whatsapp:whatsapp:*:*:*:*:*:android:*:*", EndExcl: "2.20.196.16"},
		cpeMatch{Vulnerable: true, Criteria: "cpe:2.3:a:whatsapp:whatsapp:*:*:*:*:*:iphone_os:*:*", EndExcl: "2.20.90"})
	if got := parseNVD(d, "whatsapp", "whatsapp", "2.2644.101.0"); len(got) != 0 {
		t.Fatalf("mobile/portal entries must not match a Windows desktop app: %+v", got)
	}
	win := nvdDoc("CVE-2026-23863", cpeMatch{Vulnerable: true, Criteria: "cpe:2.3:a:whatsapp:whatsapp:*:*:*:*:*:windows:*:*", EndExcl: "2.3000.1032164386.258709"})
	if got := parseNVD(win, "whatsapp", "whatsapp", "2.2644.101.0"); len(got) != 1 {
		t.Fatal("a Windows-targeted entry must still match")
	}
}
