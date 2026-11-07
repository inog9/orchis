package hackbot

import "encoding/json"

// ScannerInput is an immutable preliminary observation snapshot, not a Hackbot
// validation verdict. Execution and asset ownership are selected by the server.
type ScannerInput struct {
	Tool        string          `json:"tool"`
	Status      string          `json:"status"`
	Target      string          `json:"target"`
	Error       string          `json:"error,omitempty"`
	ExecutionID string          `json:"execution_id,omitempty"`
	Params      json.RawMessage `json:"params"`
	Findings    json.RawMessage `json:"findings"`
}
