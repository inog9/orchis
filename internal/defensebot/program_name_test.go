package defensebot

import "testing"

func TestNormalizeProgramName(t *testing.T) {
	cases := []struct{ in, clean, ver string }{
		{"7-Zip 26.00 (x64)", "7-zip", "26.00"},
		{"Audacity 3.7.8", "audacity", "3.7.8"},
		{"Burp Suite 2026.4.3", "burp suite", "2026.4.3"},
		{"Apache NetBeans IDE 19", "apache netbeans ide", ""},
		{"Git", "git", ""},
		{"Google Chrome", "google chrome", ""},
		{"Python 3.12.1 (64-bit)", "python", "3.12.1"},
		{"Cursor (User)", "cursor", ""},
		{"Microsoft Edge", "microsoft edge", ""},
	}
	for _, c := range cases {
		clean, ver := normalizeProgramName(c.in)
		if clean != c.clean || ver != c.ver {
			t.Errorf("%q => (%q,%q), want (%q,%q)", c.in, clean, ver, c.clean, c.ver)
		}
	}
}

func TestWindowsProgramsMapThroughNormalizedNames(t *testing.T) {
	for _, name := range []string{"7-Zip 26.00 (x64)", "Git", "Cursor (User)", "Google Chrome", "Docker Desktop", "Mozilla Firefox 130.0 (x64 en-US)"} {
		clean, _ := normalizeProgramName(name)
		if _, _, ok := cpeFor(name, ""); !ok {
			if _, _, ok = cpeFor(clean, ""); !ok {
				t.Errorf("%q (clean %q) not mapped", name, clean)
			}
		}
	}
}

func TestNoiseProgramsAreSeparatedFromRealUnmappedApps(t *testing.T) {
	noise := []string{"", "  ", "E2A4F912-2574-4A75-9BB0-0D023378592B", "26720RandomSaladGamesLLC.SimpleSolitaire", "Microsoft.AAD.BrokerPlugin",
		"AppUp.IntelGraphicsExperience", "Windows Driver Package - Google, Inc. (WinUSB) AndroidUsbDeviceClass (08/27/2012 7.0.0000.00004)",
		"Security Update for Microsoft Office 2016 (KB1234567)", "Microsoft Visual C++ 2013 x64 Minimum Runtime - 12.0.40664", "Microsoft Visual C++ 2010 x64 Redistributable - 10.0.40219",
		"Python 3.10.6 Core Interpreter (64-bit)", "NVIDIA Graphics Driver 610.91", "vs_minshellmsi", "HP Support Assistant"}
	for _, n := range noise {
		if !isNoiseProgram(n) {
			t.Errorf("%q should be treated as noise", n)
		}
	}
	real := []string{"Audacity 3.7.8", "Microsoft .NET Runtime - 8.0.1", "ExpressVPN",
		"Microsoft Office Home and Student 2021 - en-us", "FortiClient VPN", "Dota 2"}
	for _, n := range real {
		if isNoiseProgram(n) {
			t.Errorf("%q is a real application and must stay in the unmapped count", n)
		}
	}
}

func TestUnmappedCountExcludesNoiseAndDuplicates(t *testing.T) {
	in := Input{Datasets: map[string]DatasetInput{QueryPrograms: {Status: "ok", Rows: []map[string]string{
		{"name": "Obscure Tool", "version": "1.0"},
		{"name": "Obscure Tool", "version": "1.0"}, // duplikat
		{"name": "Microsoft.AAD.BrokerPlugin", "version": "1"},
		{"name": "", "version": ""},
	}}}}
	p := buildPlan(in, nil)
	if p.Unmapped != 1 || p.Ignored != 3 {
		t.Fatalf("unmapped=%d ignored=%d, want 1 and 3", p.Unmapped, p.Ignored)
	}
}

func TestArchTokensWithoutParenthesesAreStripped(t *testing.T) {
	cases := map[string]string{"Wireshark 4.6.8 x64": "wireshark", "Postman x64 11.92.0": "postman", "OpenVPN 2.7.0-I017 amd64": "openvpn"}
	for in, want := range cases {
		if got, _ := normalizeProgramName(in); got != want {
			t.Errorf("%q => %q, want %q", in, got, want)
		}
		if clean, _ := normalizeProgramName(in); true {
			if _, _, ok := cpeFor(clean, ""); !ok {
				t.Errorf("%q (clean %q) is not mapped", in, clean)
			}
		}
	}
}

func TestCommonProgramsAreCountedSeparatelyFromTheRest(t *testing.T) {
	in := Input{Datasets: map[string]DatasetInput{QueryPrograms: {Status: "ok", Rows: []map[string]string{
		{"name": "Warp", "version": "7.0"}, {"name": "Total War: SHOGUN 2", "version": "1"}, {"name": "Obscure Tool", "version": "1"}}}}}
	p := buildPlan(in, nil)
	if p.Unmapped != 3 || p.UnmappedCommon != 1 || len(p.UnmappedCommonNames) != 1 || p.UnmappedCommonNames[0] != "Warp" {
		t.Fatalf("unmapped=%d common=%d names=%v", p.Unmapped, p.UnmappedCommon, p.UnmappedCommonNames)
	}
}

func TestPopularWindowsAppsMapToVerifiedCPEs(t *testing.T) {
	cases := map[string][2]string{
		"R for Windows 4.3.3": {"r-project", "r"}, "Bonjour": {"apple", "bonjour"}, "osquery": {"linuxfoundation", "osquery"},
		"Autopsy": {"sleuthkit", "autopsy"}, "Graphviz": {"graphviz", "graphviz"}, "ExpressVPN": {"expressvpn", "expressvpn"},
		"Internet Download Manager": {"tonec", "internet_download_manager"}, "OpenSSL 3.6.1 Light (64-bit)": {"openssl", "openssl"},
		"FortiClient VPN": {"fortinet", "forticlient"}, "Wazuh Agent": {"wazuh", "wazuh"}, "Adobe Flash Player 11 Plugin": {"adobe", "flash_player"},
	}
	for name, want := range cases {
		clean, _ := normalizeProgramName(name)
		v, p, ok := cpeFor(name, "")
		if !ok {
			v, p, ok = cpeFor(clean, "")
		}
		if !ok || v != want[0] || p != want[1] {
			t.Errorf("%q => %s:%s ok=%v, want %s:%s", name, v, p, ok, want[0], want[1])
		}
	}
}

func TestMacAppNoiseIsIgnoredAndHelpersAreDeduplicated(t *testing.T) {
	rows := []map[string]string{
		{"name": "Safari.app", "bundle_identifier": "com.apple.Safari", "version": "17"},
		{"name": "Brave Browser.app", "bundle_identifier": "com.brave.Browser", "version": "1.70"},
		{"name": "Brave Browser Helper (GPU).app", "bundle_identifier": "com.brave.Browser.helper", "version": "1.70"},
		{"name": "Claude Helper.app", "bundle_identifier": "com.anthropic.claudefordesktop.helper", "version": "1"},
		{"name": "Uninstall OpenVPN Connect.app", "bundle_identifier": "net.openvpn.uninstaller", "version": "1"},
		{"name": "IDLE 3.app", "bundle_identifier": "org.python.IDLE", "version": "3.9"},
		{"name": "Burp Suite.app", "bundle_identifier": "com.install4j.6592-1155-2163-3973.70", "version": "2026.4"},
		{"name": "Python.app", "bundle_identifier": "org.python.python", "version": "3.9.6"},
		{"name": "osquery.app", "bundle_identifier": "io.osquery.agent", "version": "5.23.1"},
		{"name": "Warp.app", "bundle_identifier": "dev.warp.Warp-Stable", "version": "0.1"},
		{"name": "Obscure.app", "bundle_identifier": "com.example.obscure", "version": "1"},
	}
	p := buildPlan(Input{Datasets: map[string]DatasetInput{QueryApps: {Status: "ok", Rows: rows}}}, nil)
	want := map[string]bool{"brave:browser": true, "portswigger:burp_suite": true, "python:python": true, "linuxfoundation:osquery": true}
	if len(p.CPE) != len(want) {
		t.Fatalf("mapped products = %d (%+v), want %d", len(p.CPE), p.CPE, len(want))
	}
	for _, c := range p.CPE {
		if !want[c.Vendor+":"+c.Product] {
			t.Errorf("unexpected mapping %s:%s", c.Vendor, c.Product)
		}
	}
	// Tidak terpetakan relevan: Warp (umum) dan Obscure. Diabaikan: Safari, helper Brave, Claude Helper, Uninstall, IDLE.
	if p.Unmapped != 2 || p.UnmappedCommon != 1 || p.Ignored != 5 {
		t.Fatalf("unmapped=%d common=%d ignored=%d, want 2/1/5", p.Unmapped, p.UnmappedCommon, p.Ignored)
	}
}
