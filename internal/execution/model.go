// Package execution: atomic test, eksekusi aksi oleh injector, dan finding yang dihasilkannya.
package execution

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"orchis-go/internal/contract"
)

// ObjectStore menyimpan keluaran mentah (MinIO). Boleh nil: keluaran mentah tidak disimpan.
type ObjectStore interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
}

// Searcher mencari finding di Elasticsearch. Boleh nil: pencarian jatuh ke PostgreSQL.
type Searcher interface {
	SearchFindings(ctx context.Context, tenant uuid.UUID, q string, limit int) ([]uuid.UUID, error)
}

type ContractView struct {
	Version           int             `json:"version"`
	Category          string          `json:"category"`
	Schema            contract.Schema `json:"input_schema"`
	Key               string          `json:"key"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	Kind              string          `json:"kind"`
	OutputTypes       []string        `json:"output_types"`
	TimeoutSeconds    int             `json:"timeout_seconds"`
	Integration       string          `json:"integration"`
	IntegrationStatus string          `json:"integration_status"` // online | offline | disabled
	NeedsAgent        bool            `json:"needs_agent"`
	LastHeartbeatAt   *time.Time      `json:"last_heartbeat_at"`
}

type ExecutionSummary struct {
	IsDemo       bool       `json:"is_demo"`
	ID           uuid.UUID  `json:"id"`
	State        string     `json:"state"`
	Outcome      *string    `json:"outcome"`
	Error        *string    `json:"error"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at"`
	FindingCount int        `json:"finding_count"`
}

type AtomicTest struct {
	ID              uuid.UUID          `json:"id"`
	Name            string             `json:"name"`
	ContractKey     string             `json:"contract_key"`
	ContractName    string             `json:"contract_name"`
	Params          json.RawMessage    `json:"params"`
	TargetAssetID   *uuid.UUID         `json:"target_asset_id"`
	TargetAssetName *string            `json:"target_asset_name"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
	Version         int                `json:"version"`
	LastExecution   *ExecutionSummary  `json:"last_execution"`
	Executions      []ExecutionSummary `json:"executions,omitempty"` // hanya di detail
}

type LogLine struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

type ExecutionDetail struct {
	ExecutionSummary
	AtomicTestID  *uuid.UUID      `json:"atomic_test_id"`
	ContractKey   string          `json:"contract_key"`
	TargetAssetID uuid.UUID       `json:"target_asset_id"`
	TargetAddress string          `json:"target_address"`
	Params        json.RawMessage `json:"params"`
	ExitCode      *int            `json:"exit_code"`
	RawOutputKey  *string         `json:"raw_output_key"`
	Logs          []LogLine       `json:"logs"`
	Findings      []Finding       `json:"findings"`
}

type Finding struct {
	ID              uuid.UUID       `json:"id"`
	AssetID         *uuid.UUID      `json:"asset_id"`
	AssetName       *string         `json:"asset_name"`
	Type            string          `json:"type"`
	Value           string          `json:"value"`
	Title           string          `json:"title"`
	Severity        string          `json:"severity"`
	Details         json.RawMessage `json:"details"`
	FirstSeenAt     time.Time       `json:"first_seen_at"`
	LastSeenAt      time.Time       `json:"last_seen_at"`
	OccurrenceCount int             `json:"occurrence_count"`
}

type FindingPage struct {
	Items      []Finding `json:"items"`
	NextCursor *string   `json:"next_cursor"`
	Search     string    `json:"search"` // elasticsearch | database | none: bagian mana yang menjawab pencarian teks
}
