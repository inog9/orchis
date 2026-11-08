package reporting

import (
	"strings"
	"testing"
	"time"

	"orchis-go/internal/defensebot"
)

func vuln(pkg, installed, fixed, cve, sev, expo string, kev bool) Finding {
	ev := map[string]any{"cve": cve, "package": pkg, "source_package": pkg, "installed_version": installed, "ecosystem": "Ubuntu:24.04:LTS",
		"method": defensebot.MethodDistro, "fix_status": "fix_available", "exposure": expo, "severity_source": "distribution priority", "summary": "Advisory text <b>for</b> " + cve}
	if fixed != "" {
		ev["fixed_version"] = fixed
	} else {
		ev["fix_status"] = "no_fix_yet"
	}
	if kev {
		ev["kev"] = true
	}
	if expo == "exposed" {
		ev["exposure_ports"] = "443/tcp"
	}
	return Finding{ID: cve + pkg, Type: "vulnerability", Value: cve, Title: cve + " in " + pkg, Severity: sev, Evidence: ev}
}

func defenseAsset(name string, disposition string, findings ...Finding) Asset {
	at := time.Date(2026, 11, 8, 1, 0, 0, 0, time.UTC)
	return Asset{Name: name, Status: "finished", Findings: findings, Defense: &defensebot.Summary{Schema: 1, Disposition: disposition, OSName: "Ubuntu", OSVersion: "24.04.1 LTS", SoftwareChecked: 120,
		Provenance: defensebot.Provenance{Ecosystem: "Ubuntu:24.04:LTS", CollectedAt: &at, QueriedAt: at, KEVDate: "2026-11-05", EPSSDate: "2026-11-07", SeverityPolicy: defensebot.SeverityPolicy},
		Notes:      []defensebot.Note{{Code: "apps_unmapped", Count: 3, Limiting: true}}}}
}

func TestDefenseChapterIsOrderedByRiskAndEscaped(t *testing.T) {
	calm := defenseAsset("calm-host", "complete", vuln("curl", "8.5.0", "8.5.1", "CVE-2024-1", "low", "unknown", false))
	hot := defenseAsset("hot-host", "limited",
		vuln("openssl", "3.0.13", "3.0.14", "CVE-2024-2", "high", "exposed", true),
		vuln("openssl", "3.0.13", "3.0.15", "CVE-2024-3", "medium", "exposed", false),
		vuln("nginx", "1.24", "", "CVE-2024-4", "critical", "listening", false))
	hot.Findings = append(hot.Findings, Finding{ID: "h1", Type: "text", Value: "osquery:aslr-weak", Title: "ASLR is not fully enabled", Severity: "high", Evidence: map[string]any{"method": "osquery-rule", "reason": "ASLR makes exploits harder."}})
	snap := Snapshot{Organization: "Org", Profile: "defense", Language: "en", Status: "incomplete", BySeverity: map[string]int{}, Assets: []Asset{calm, hot}}
	out, err := HTML(snap)
	if err != nil {
		t.Fatal(err)
	}
	page := string(out)
	for _, want := range []string{"Software vulnerabilities (DefenseBot)", "Fleet overview", "Fix first", "Most affected software", "Hardening gaps", "ASLR is not fully enabled", "Incomplete result."} {
		if !strings.Contains(page, want) {
			t.Errorf("report lacks %q", want)
		}
	}
	// Hosts are ordered by risk, not by name: hot-host precedes calm-host in the fleet table.
	fleet := page[strings.Index(page, "Fleet overview"):]
	if strings.Index(fleet, "hot-host") > strings.Index(fleet, "calm-host") {
		t.Error("fleet overview must list the riskier host first")
	}
	// The known-exploited, exposed openssl is the first thing to fix.
	fix := page[strings.Index(page, "Fix first"):]
	if !strings.HasPrefix(fix[strings.Index(fix, "<li>"):], "<li><strong>openssl") {
		t.Errorf("fix-first order wrong: %.200s", fix[strings.Index(fix, "<li>"):])
	}
	// Per-software upgrade target is the highest fixed version among its advisories.
	if !strings.Contains(page, "Upgrade to 3.0.15 or later") || !strings.Contains(page, "No fix available yet") {
		t.Error("upgrade targets are wrong")
	}
	if strings.Contains(page, "<b>for</b>") || !strings.Contains(page, "&lt;b&gt;for&lt;/b&gt;") {
		t.Error("advisory text must be escaped")
	}
	// DefenseBot hosts are not repeated in the generic findings list.
	generic := page[strings.Index(page, `id="findings-target"`):]
	if strings.Contains(generic[:strings.Index(generic, `id="remediation"`)], "CVE-2024-2 in openssl") {
		t.Error("DefenseBot findings must not be duplicated in the generic list")
	}
	again, _ := HTML(snap)
	if string(again) != page {
		t.Error("rendering must be deterministic")
	}
}

func TestDefenseChapterIndonesianKeepsAdvisoryTextInEnglish(t *testing.T) {
	hot := defenseAsset("hot-host", "complete", vuln("openssl", "3.0.13", "3.0.14", "CVE-2024-2", "high", "exposed", true))
	out, err := HTML(Snapshot{Organization: "Org", Profile: "defense", Language: "id", Status: "finished", BySeverity: map[string]int{}, Assets: []Asset{hot}})
	if err != nil {
		t.Fatal(err)
	}
	page := string(out)
	for _, want := range []string{"Kerentanan perangkat lunak (DefenseBot)", "Perbaiki lebih dulu", "Perbarui ke 3.0.14 atau lebih baru", "Terlihat oleh scanner", "Sudah dieksploitasi", "Advisori (Inggris)", "Advisory text &lt;b&gt;for&lt;/b&gt; CVE-2024-2"} {
		if !strings.Contains(page, want) {
			t.Errorf("Indonesian report lacks %q", want)
		}
	}
	if strings.Contains(page, "Fix first") {
		t.Error("English heading leaked into the Indonesian report")
	}
}

func TestReportWithoutDefenseBotIsUnchanged(t *testing.T) {
	out, err := HTML(Snapshot{Organization: "Org", Profile: "quick", Language: "en", BySeverity: map[string]int{}, Assets: []Asset{{Name: "web", Status: "finished"}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "DefenseBot") {
		t.Fatal("a report without DefenseBot data must not mention it")
	}
}
