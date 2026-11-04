package agentproto

const ControlProtocol = 1

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
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	AgentID string `json:"agent_id"`
}

type ControlAck struct {
	ID      string `json:"id"`
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`
}
