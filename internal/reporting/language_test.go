package reporting

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReportLanguageFrozenPresentationAndStableJSON(t *testing.T) {
	s := Snapshot{Language: "id", Organization: `Executive summary <script>alert(1)</script>`, CreatedAt: time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC), Status: "finished", Risk: "Elevated exposure", Profile: "quick", BySeverity: map[string]int{"critical": 12345}, Assets: []Asset{}}
	before, e := json.Marshal(s)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := HTML(s)
	if e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{`lang="id"`, "Ringkasan eksekutif", "Cakupan dan pemeriksaan", "Paparan meningkat", "02 Januari 2026", "12.345", "Executive summary &lt;script&gt;"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("ID report missing %q", want)
		}
	}
	if strings.Contains(string(raw), "<script>") {
		t.Fatal("report language bypassed escaping")
	}
	after, _ := json.Marshal(s)
	if !bytes.Equal(before, after) {
		t.Fatal("presentation mutated frozen snapshot")
	}
	var stable map[string]any
	json.Unmarshal(after, &stable)
	if stable["language"] != "id" || stable["organization"] != s.Organization {
		t.Fatal("JSON keys or data translated")
	}
	for _, legacy := range []string{"", "en", "fr"} {
		s.Language = legacy
		raw, e = HTML(s)
		if e != nil || !bytes.Contains(raw, []byte(`lang="en"`)) || !bytes.Contains(raw, []byte("Scope and coverage")) {
			t.Fatal("historical English fallback", legacy, e)
		}
	}
}
func TestReportLanguagesDoNotLeakBetweenConcurrentRenders(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			language, want := "en", "Scope and coverage"
			if i%2 == 1 {
				language, want = "id", "Cakupan dan pemeriksaan"
			}
			raw, e := HTML(Snapshot{Language: language})
			if e != nil || !bytes.Contains(raw, []byte(want)) {
				t.Error("renderer language leaked", language, e)
			}
		}()
	}
	wg.Wait()
}
func TestReportNumberLocale(t *testing.T) {
	for _, test := range []struct {
		lang string
		n    int
		want string
	}{{"en", 1234567, "1,234,567"}, {"id", 1234567, "1.234.567"}, {"id", -1234, "-1.234"}, {"id", 0, "0"}} {
		if got := reportNumber(test.lang, test.n); got != test.want {
			t.Fatal(got, test.want)
		}
	}
}
