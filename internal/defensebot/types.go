// Package defensebot maps the software installed on an enrolled endpoint to known vulnerabilities and a small set of
// hardening gaps. The data comes from fixed read-only osquery checks run by the agent; matching uses public advisory data
// (OSV, CISA KEV, FIRST EPSS). Matching is deterministic for a given advisory snapshot, and no model decides that
// something is vulnerable.
package defensebot

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

const (
	// EngineVersion is recorded in the provenance of every run.
	EngineVersion = "defensebot/1"

	QueryPackages  = "osquery.packages"
	QueryOSRelease = "osquery.os_release"
	QueryKernel    = "osquery.kernel"
	QueryNPM       = "osquery.npm_global"
	QueryPip       = "osquery.pip_packages"
	QueryApps      = "osquery.macos_apps"
	QueryPrograms  = "osquery.programs"
	QueryBrew      = "osquery.homebrew"
	QueryExposure  = "osquery.exposure"
	QueryHardening = "osquery.hardening"
	QueryFirewall  = "osquery.firewall"

	MethodDistro = "osv-distro"
	MethodExact  = "osv-exact"
	MethodCPE    = "nvd-cpe"
)

// Queries lists every osquery check a orchis-defense-agent run reads, in a stable order.
var Queries = []string{QueryOSRelease, QueryPackages, QueryNPM, QueryApps, QueryPrograms, QueryBrew, QueryExposure, QueryHardening, QueryFirewall}

// requiredGroups mengembalikan data yang wajib ada agar satu run berguna, per platform.
// Setiap grup adalah alternatif yang saling menggantikan: cukup satu anggota terkumpul.
// macOS tidak pernah punya paket deb/rpm, jadi daftar software-nya adalah aplikasi atau Homebrew.
func requiredGroups(platform string) [][]string {
	switch platform {
	case "darwin":
		return [][]string{{QueryOSRelease}, {QueryApps, QueryBrew}}
	case "windows":
		return [][]string{{QueryOSRelease}, {QueryPrograms}}
	}
	return [][]string{{QueryOSRelease}, {QueryPackages}}
}

// DatasetInput is the latest stored result of one check for the asset, as prepared by the server.
type DatasetInput struct {
	Status      string              `json:"status"` // ok | skipped | failed | never
	Reason      string              `json:"reason,omitempty"`
	CollectedAt *time.Time          `json:"collected_at,omitempty"`
	Truncated   bool                `json:"truncated,omitempty"`
	Rows        []map[string]string `json:"rows,omitempty"`
}

// ExternalPort is a port that a recent Nmap scan of the same asset reported as open.
type ExternalPort struct {
	Port     int       `json:"port"`
	Protocol string    `json:"protocol"`
	SeenAt   time.Time `json:"seen_at"`
}

// Input is everything the worker needs. It is built by the server from stored data; the worker never contacts the agent.
type Input struct {
	Datasets      map[string]DatasetInput `json:"datasets"`
	ExternalPorts []ExternalPort          `json:"external_ports,omitempty"`
	ScannerSeenAt *time.Time              `json:"scanner_seen_at,omitempty"`
	Language      string                  `json:"language,omitempty"`
}

// Note is a coverage statement. Codes are translated when a report is rendered.
type Note struct {
	Code     string `json:"code"`
	Count    int    `json:"count,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Limiting bool   `json:"limiting,omitempty"`
}

// Provenance records exactly which data a run used so a frozen report can be audited later.
type Provenance struct {
	Engine         string     `json:"engine"`
	Ecosystem      string     `json:"ecosystem,omitempty"`
	CollectedAt    *time.Time `json:"collected_at,omitempty"`
	QueriedAt      time.Time  `json:"queried_at"`
	OSVPairs       int        `json:"osv_pairs"`
	OSVAdvisories  int        `json:"osv_advisories"`
	KEVDate        string     `json:"kev_date,omitempty"`
	EPSSDate       string     `json:"epss_date,omitempty"`
	SeverityPolicy string     `json:"severity_policy"`
	Sources        []string   `json:"sources"`
}

// Summary is stored with the execution and read by scans and reports.
type Summary struct {
	Schema           int            `json:"schema"`
	Disposition      string         `json:"disposition"` // complete | limited
	Platform         string         `json:"platform,omitempty"`
	OSName           string         `json:"os_name,omitempty"`
	OSVersion        string         `json:"os_version,omitempty"`
	SoftwareChecked  int            `json:"software_checked"`
	SoftwareUnmapped int            `json:"software_unmapped"`
	Findings         int            `json:"findings"`
	Omitted          int            `json:"omitted"`
	BySeverity       map[string]int `json:"by_severity"`
	KnownExploited   int            `json:"known_exploited"`
	Exposed          int            `json:"exposed"`
	RebootRequired   int            `json:"reboot_required"`
	Hardening        int            `json:"hardening"`
	Notes            []Note         `json:"notes"`
	Provenance       Provenance     `json:"provenance"`
}

// Finding mirrors the finding shape accepted by the execution service.
type Finding struct {
	Type     string          `json:"type"`
	Value    string          `json:"value"`
	Title    string          `json:"title"`
	Severity string          `json:"severity"`
	Details  json.RawMessage `json:"details"`
}

type Output struct {
	Findings []Finding
	Summary  Summary
	Logs     []string
}

// AppRef is one installed application as collected by the agent.
type AppRef struct {
	Name    string `json:"name"`
	Bundle  string `json:"bundle_identifier,omitempty"`
	Version string `json:"version"`
}

// AppKey is the stable lookup key for one application row.
func AppKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// AppMapper proposes a CPE vendor:product for installed applications that the reviewed
// dictionary does not know. Proposals are only used after NVD confirms the CPE exists,
// so a wrong guess can never create a finding.
type AppMapper interface {
	ProposeCPE(ctx context.Context, apps []AppRef) (map[string]string, error) // AppKey(name) -> "vendor:product"
}
