package blueteam

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// staleAfterMisses: berapa pengambilan data BERURUTAN (yang baru, dari agent yang hidup) tanpa melihat port itu lagi
// sebelum temuannya ditandai stale.
const staleAfterMisses = 2

// c2StaleAfter: temuan koneksi C2 yang tidak terlihat lagi selama ini (sementara agent hidup) ditandai stale.
const c2StaleAfter = 30 * time.Minute

// MarkStale menandai temuan jaringan yang tidak teramati lagi sebagai 'stale'. Temuan TIDAK dihapus dan tidak dicampur
// dengan triase manual: hanya status 'open' yang berubah. Agent offline tidak pernah menyebabkan stale, karena pengambilan
// data baru tidak datang (collectedAt tidak maju) dan heartbeat tidak segar.
func MarkStale(ctx context.Context, tx pgx.Tx, in Input, networkCollectedAt *time.Time) error {
	if rows, ok := in.Datasets["osquery.network"]; ok && rows != nil && networkCollectedAt != nil {
		present := map[string]bool{}
		for _, r := range rows {
			present[r["port"]+"|"+r["address"]] = true
		}
		fr, err := tx.Query(ctx, `SELECT id, details->'row'->>'port', details->'row'->>'address', absent_count, absent_collected_at, triage_status
FROM findings WHERE tenant_id=$1 AND asset_id=$2 AND type='new_listening_port' AND triage_status IN ('open','stale')`, in.Tenant, in.Asset)
		if err != nil {
			return err
		}
		type row struct {
			id         uuid.UUID
			port, addr string
			count      int16
			lastAbsent *time.Time
			status     string
		}
		var all []row
		for fr.Next() {
			var r row
			var port, addr *string
			if err := fr.Scan(&r.id, &port, &addr, &r.count, &r.lastAbsent, &r.status); err != nil {
				fr.Close()
				return err
			}
			if port != nil {
				r.port = *port
			}
			if addr != nil {
				r.addr = *addr
			}
			all = append(all, r)
		}
		fr.Close()
		if err := fr.Err(); err != nil {
			return err
		}
		for _, r := range all {
			if present[r.port+"|"+r.addr] {
				if r.count > 0 || r.status == "stale" {
					if _, err := tx.Exec(ctx, `UPDATE findings SET absent_count=0, absent_collected_at=NULL, triage_status='open', triage_reason='', triage_at=NULL WHERE tenant_id=$1 AND id=$2`, in.Tenant, r.id); err != nil {
						return err
					}
				}
				continue
			}
			if r.status == "stale" {
				continue
			}
			if r.lastAbsent != nil && !networkCollectedAt.Truncate(time.Microsecond).After(*r.lastAbsent) {
				continue // pengambilan data yang sama sudah dihitung
			}
			n := r.count + 1
			if n >= staleAfterMisses {
				if _, err := tx.Exec(ctx, `UPDATE findings SET absent_count=$3, absent_collected_at=$4, triage_status='stale', triage_reason='No longer observed in consecutive collections', triage_at=now() WHERE tenant_id=$1 AND id=$2`, in.Tenant, r.id, n, *networkCollectedAt); err != nil {
					return err
				}
			} else if _, err := tx.Exec(ctx, `UPDATE findings SET absent_count=$3, absent_collected_at=$4 WHERE tenant_id=$1 AND id=$2`, in.Tenant, r.id, n, *networkCollectedAt); err != nil {
				return err
			}
		}
	}
	// Koneksi C2: aturannya dievaluasi ulang tiap putaran dan menyegarkan last_seen_at; bila lama tidak segar saat agent hidup,
	// koneksinya sudah berhenti.
	agentFresh := false
	for _, a := range in.Agents {
		if a.RevokedAt == nil && a.LastHeartbeat != nil && in.Now.Sub(*a.LastHeartbeat) < 5*time.Minute {
			agentFresh = true
		}
	}
	if agentFresh {
		if _, err := tx.Exec(ctx, `UPDATE findings SET triage_status='stale', triage_reason='No longer observed', triage_at=now()
WHERE tenant_id=$1 AND asset_id=$2 AND type='c2_suspect_connection' AND triage_status='open' AND last_seen_at < $3`, in.Tenant, in.Asset, in.Now.Add(-c2StaleAfter)); err != nil {
			return err
		}
	}
	return nil
}
