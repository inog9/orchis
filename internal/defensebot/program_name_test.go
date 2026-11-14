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
