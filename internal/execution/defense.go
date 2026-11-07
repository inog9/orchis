package execution

import (
	"encoding/json"
	"time"

	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/contract"
	"orchis-go/internal/defensebot"
)

// scannerFreshness is how long a finished Nmap scan of the same asset still counts for the "seen by scanner" exposure tag.
const scannerFreshness = 7 * 24 * time.Hour

// defenseInput builds the input of a DefenseBot job from data the server already holds for the asset: the latest stored
// osquery datasets, and the ports a recent Nmap scan of the same asset found open. The worker never contacts the agent.
func (r *Runtime) defenseInput(ctx context.Context, tx pgx.Tx, asset uuid.UUID, params json.RawMessage) (*defensebot.Input, error) {
	in := &defensebot.Input{Datasets: map[string]defensebot.DatasetInput{}}
	var p struct {
		Language string `json:"language"`
	}
	_ = json.Unmarshal(params, &p)
	in.Language = p.Language
	rows, err := tx.Query(ctx, `SELECT query_key, rows, truncated, collected_at, last_status, last_reason FROM endpoint_datasets
 WHERE tenant_id=$1 AND asset_id=$2 AND query_key=ANY($3)`, r.Tenant, asset, defensebot.Queries)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key, status, reason string
		var raw []byte
		var truncated bool
		var collected *time.Time
		if err = rows.Scan(&key, &raw, &truncated, &collected, &status, &reason); err != nil {
			rows.Close()
			return nil, err
		}
		d := defensebot.DatasetInput{Status: status, Reason: reason, Truncated: truncated}
		if collected != nil {
			// rows always belong to the last successful collection; later failed attempts keep them.
			d.Status, d.CollectedAt = "ok", collected
			if err = json.Unmarshal(raw, &d.Rows); err != nil {
				rows.Close()
				return nil, err
			}
		}
		in.Datasets[key] = d
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for _, key := range defensebot.Queries {
		if _, ok := in.Datasets[key]; !ok {
			in.Datasets[key] = defensebot.DatasetInput{Status: "never"}
		}
	}
	var started *time.Time
	if err = tx.QueryRow(ctx, `SELECT max(e.started_at) FROM executions e JOIN contracts c ON c.tenant_id=e.tenant_id AND c.id=e.contract_id
 WHERE e.tenant_id=$1 AND e.target_asset_id=$2 AND c.key=$3 AND e.state='ended' AND e.outcome='success' AND e.ended_at > now() - make_interval(secs => $4)`,
		r.Tenant, asset, contract.KeyNmapTCP, scannerFreshness.Seconds()).Scan(&started); err != nil {
		return nil, err
	}
	if started == nil {
		return in, nil
	}
	in.ScannerSeenAt = started
	prows, err := tx.Query(ctx, `SELECT details, last_seen_at FROM findings WHERE tenant_id=$1 AND asset_id=$2 AND type='open_port' AND last_seen_at >= $3`, r.Tenant, asset, *started)
	if err != nil {
		return nil, err
	}
	defer prows.Close()
	for prows.Next() {
		var raw []byte
		var seen time.Time
		if err = prows.Scan(&raw, &seen); err != nil {
			return nil, err
		}
		var d struct {
			Port     int    `json:"port"`
			Protocol string `json:"protocol"`
		}
		if json.Unmarshal(raw, &d) == nil && d.Port > 0 && d.Port < 65536 {
			in.ExternalPorts = append(in.ExternalPorts, defensebot.ExternalPort{Port: d.Port, Protocol: d.Protocol, SeenAt: seen})
		}
	}
	return in, prows.Err()
}
