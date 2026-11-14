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
		"Security Update for Microsoft Office 2016 (KB1234567)", "Microsoft Visual C++ 2013 x64 Minimum Runtime - 12.0.40664"}
	for _, n := range noise {
		if !isNoiseProgram(n) {
			t.Errorf("%q should be treated as noise", n)
		}
	}
	real := []string{"Audacity 3.7.8", "Microsoft Visual C++ 2010 x64 Redistributable - 10.0.40219", "Microsoft .NET Runtime - 8.0.1", "ExpressVPN",
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
