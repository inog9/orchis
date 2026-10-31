// Package audit mencatat aksi penting ke audit_log (hanya-tambah) dalam transaksi yang sama dengan perubahannya.
package audit

import (
	"context"
	"encoding/json"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/ids"
)

type Entry struct {
	Tenant       uuid.UUID
	ActorUser    *uuid.UUID
	Action       string // contoh: asset.create
	ResourceType string
	ResourceID   *uuid.UUID
	IP           string
	RequestID    string
	Detail       map[string]any
}

func Write(ctx context.Context, tx pgx.Tx, e Entry) error {
	detail, err := json.Marshal(e.Detail)
	if err != nil {
		return err
	}
	if e.Detail == nil {
		detail = []byte("{}")
	}
	var ip *netip.Addr
	if a, err := netip.ParseAddr(e.IP); err == nil {
		ip = &a
	}
	_, err = tx.Exec(ctx, `
INSERT INTO audit_log (id, tenant_id, actor_user_id, action, resource_type, resource_id, ip, request_id, detail)
VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,NULLIF($8,''),$9)`,
		ids.New(), e.Tenant, e.ActorUser, e.Action, e.ResourceType, e.ResourceID, ip, e.RequestID, detail)
	return err
}
