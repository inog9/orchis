package reporting

import (
	"strings"
	"testing"
)

func TestRemediationItemsUsesFixedVersionFromSnapshot(t *testing.T) {
	s := Snapshot{Assets: []Asset{{Name: "macbook", Findings: []Finding{{
		ID: "f1", Type: "vulnerability", Severity: "critical", Title: "CVE-2026-59873 in tar 7.5.15",
		Evidence: map[string]any{"method": "osv-exact", "package": "tar", "installed_version": "7.5.15", "fixed_version": "7.5.19", "cve": "CVE-2026-59873", "fix_status": "fix_available"},
	}}}}}
	items := RemediationItems(s)
	if len(items) != 1 || items[0].Fixed != "7.5.19" || items[0].Software != "tar" || items[0].CVEs != "CVE-2026-59873" {
		t.Fatalf("items: %+v", items)
	}
	h := remediationHTML("en", s)
	if !strings.Contains(string(h), "Upgrade tar from 7.5.15 to 7.5.19") {
		t.Fatalf("HTML remediasi tidak memuat upgrade: %s", string(h)[:200])
	}
}
