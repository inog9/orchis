package execution

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/contract"
)

// AgentTargetConfirmed membedakan identitas yang dilaporkan agent dari sasaran yang ditetapkan admin.
// Aset manual dan range Scan memakai pemeriksaan scope masing-masing; range tidak memanggil fungsi ini.
func AgentTargetConfirmed(ctx context.Context, tx pgx.Tx, tenant, asset uuid.UUID, target string) (bool, error) {
	var source string
	if err := tx.QueryRow(ctx, `SELECT source FROM assets WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, tenant, asset).Scan(&source); err != nil {
		return false, err
	}
	if source != "agent" {
		return true, nil
	}
	rows, err := tx.Query(ctx, `SELECT kind,value FROM asset_addresses WHERE tenant_id=$1 AND asset_id=$2 AND scan_approved`, tenant, asset)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	host := contract.HostOf(target)
	approved := false
	for rows.Next() {
		var kind, value string
		if err = rows.Scan(&kind, &value); err != nil {
			return false, err
		}
		if kind == "ip" || kind == "hostname" || kind == "domain" || kind == "url" {
			if contract.HostOf(value) == host {
				approved = true
			}
		}
	}
	return approved, rows.Err()
}
