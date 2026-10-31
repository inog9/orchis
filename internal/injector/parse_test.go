package injector

import (
	"os"
	"strings"
	"testing"
)

func TestParseNmapKeepsOnlyOpenPortsAndDetectsServices(t *testing.T) {
	raw, _ := os.ReadFile("testdata/nmap.xml")
	fs, err := ParseNmap(raw)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range fs {
		got[f.Type+"|"+f.Value] = f.Title
	}
	want := map[string]string{
		"open_port|tcp/22":           "Open port 22/tcp (ssh)",
		"service|tcp/22 openssh 9.6": "",
		"open_port|tcp/80":           "Open port 80/tcp (http)",
	}
	for k := range want {
		if _, ok := got[strings.ToLower(k)]; !ok {
			// nilai dinormalisasi di server; parser menjaga huruf aslinya
			found := false
			for _, f := range fs {
				if strings.EqualFold(f.Type+"|"+f.Value, k) {
					found = true
				}
			}
			if !found {
				t.Errorf("finding %q tidak ada: %+v", k, fs)
			}
		}
	}
	if len(fs) != 3 {
		t.Fatalf("closed dan open|filtered tidak boleh masuk; dapat %d finding: %+v", len(fs), fs)
	}
}

func TestParseNmapRejectsGarbage(t *testing.T) {
	if _, err := ParseNmap([]byte("this is not xml")); err == nil {
		t.Fatal("keluaran rusak harus menjadi error, bukan 'tidak ada temuan'")
	}
}

func TestParseNucleiDropsRawRequestAndResponse(t *testing.T) {
	raw, _ := os.ReadFile("testdata/nuclei.jsonl")
	fs, err := ParseNuclei(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 2 {
		t.Fatalf("baris rusak dan tanpa template-id harus dilewati; dapat %d: %+v", len(fs), fs)
	}
	if fs[0].Severity != "high" || fs[0].Value != "exposed-env@http://127.0.0.1:8081/.env" || fs[0].Title != "Exposed .env file" {
		t.Errorf("finding pertama salah: %+v", fs[0])
	}
	if fs[1].Severity != "info" {
		t.Errorf("severity unknown harus menjadi info, dapat %q", fs[1].Severity)
	}
	for _, f := range fs {
		d := string(f.Details)
		if strings.Contains(d, "hunter2") || strings.Contains(d, "SECRET") {
			t.Errorf("request/response mentah bocor ke details: %s", d)
		}
	}
}

// XML berasal dari bentuk keluaran nmap -sT -Pn tanpa --open; port tertutup tetap membuktikan respons.
func TestNmapUnavailableIsNotClean(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		fail       bool
	}{
		{"not-nmap", `<other/>`, true},
		{"missing-host", `<nmaprun/>`, true},
		{"down", `<nmaprun><host><status state="down"/></host></nmaprun>`, true},
		{"timeout", `<nmaprun><host timedout="true"><status state="up"/><ports><port><state state="open"/></port></ports></host></nmaprun>`, true},
		{"no-response", `<nmaprun><host><status state="up"/><ports><extraports state="filtered" count="100"/></ports></host></nmaprun>`, true},
		{"closed-port", `<nmaprun><host><status state="up"/><ports><port><state state="closed"/></port></ports></host></nmaprun>`, false},
		{"all-closed", `<nmaprun><host><status state="up"/><ports><extraports state="closed" count="100"/></ports></host></nmaprun>`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := ParseNmap([]byte(tc.body))
			if (err != nil) != tc.fail {
				t.Fatalf("unexpected conclusion: findings=%v err=%v", fs, err)
			}
			if len(fs) != 0 {
				t.Fatal("closed/unresponsive ports must not become findings")
			}
		})
	}
}
