// Package authz mendefinisikan permission dan peran bawaan. Kode HANYA memeriksa permission, tidak pernah nama peran.
package authz

const (
	AIRead            = "ai.read"
	AIGenerate        = "ai.generate"
	AIApprove         = "ai.approve"
	AIManage          = "ai.manage"
	OrganizationRead  = "organization.read"
	OrganizationWrite = "organization.write"
	AssessmentRead    = "assessment.read"
	AssessmentRun     = "assessment.run"
	ReportRead        = "report.read"
	ReportGenerate    = "report.generate"
	AssetRead         = "asset.read"
	AssetWrite        = "asset.write"
	AssetDelete       = "asset.delete"
	AssetAuthorize    = "asset.authorize" // menandai aset boleh dijadikan sasaran pengujian
	ScenarioRead      = "scenario.read"
	ScenarioWrite     = "scenario.write"
	ScenarioDelete    = "scenario.delete"
	SimulationRead    = "simulation.read"
	SimulationRun     = "simulation.run"
	FindingRead       = "finding.read"
	FindingWrite      = "finding.write" // menetapkan status triage per finding
	AtomicRead        = "atomic.read"
	AtomicWrite       = "atomic.write" // membuat dan mengubah atomic test
	AtomicRun         = "atomic.run"   // menjalankan dan menghentikan eksekusi
	AtomicDelete      = "atomic.delete"
	PeopleRead        = "people.read"
	PeopleWrite       = "people.write"
	AgentRead         = "agent.read"
	AgentManage       = "agent.manage"
	IntegrationRead   = "integration.read"
	IntegrationManage = "integration.manage"
	UserManage        = "user.manage"
	SettingsManage    = "settings.manage"
	AuditRead         = "audit.read"
	AlertRead         = "alert.read"
)

var readPerms = []string{AIRead, OrganizationRead, AssessmentRead, ReportRead, AtomicRead, AssetRead, ScenarioRead, SimulationRead, FindingRead, PeopleRead, AgentRead, IntegrationRead, AlertRead}

var analystExtra = []string{AIGenerate, AIApprove, OrganizationWrite, AssessmentRun, ReportGenerate, AtomicWrite, AtomicRun, AssetWrite, ScenarioWrite, SimulationRun, PeopleWrite, FindingWrite}

var adminExtra = []string{AIManage, AtomicDelete, AssetDelete, AssetAuthorize, ScenarioDelete, AgentManage, IntegrationManage, UserManage, SettingsManage, AuditRead}

type Role struct {
	Key         string
	Name        string
	Description string
	Permissions []string
}

func concat(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// SystemRoles = peran bawaan; disinkronkan dari kode setiap kali server naik.
var SystemRoles = []Role{
	{Key: "admin", Name: "Admin", Description: "Full access, including users, integrations, agents and settings.", Permissions: concat(readPerms, analystExtra, adminExtra)},
	{Key: "analyst", Name: "Analyst", Description: "Manage assets, run authorized scans and generate reports; cannot authorize targets or consent.", Permissions: concat(readPerms, analystExtra)},
	{Key: "viewer", Name: "Viewer", Description: "Read findings and frozen reports only.", Permissions: []string{FindingRead, ReportRead}},
}

func Has(perms []string, want string) bool {
	for _, p := range perms {
		if p == want {
			return true
		}
	}
	return false
}
