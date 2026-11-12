package agentproto

const (
	ControlProtocol            = 3 // maximum supported capability value
	ControlUninstallAndCollect = 2
	ControlCollectOnly         = 3 // Windows does not support the physical-uninstall helper
)

func CanUninstall(v int) bool { return v == 1 || v == ControlUninstallAndCollect }
func CanCollect(v int) bool   { return v == ControlUninstallAndCollect || v == ControlCollectOnly }

// DiagnosticCode is an allowlist, never arbitrary logs, paths or server responses.
func DiagnosticCode(code string) string {
	switch code {
	case "", "update_failed", "release_key_missing", "collection_failed", "uninstall_failed", "uninstall_unsupported":
		return code
	default:
		return ""
	}
}

type ControlCommand struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	AgentID      string   `json:"agent_id"`
	Queries      []string `json:"queries,omitempty"`
	ExecutionIDs []string `json:"execution_ids,omitempty"`
}

type ControlAck struct {
	ID      string `json:"id"`
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`
}
