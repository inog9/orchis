package organizations

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/contract"
)

type ReadinessAsset struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Authorized bool      `json:"authorized"`
	Reason     string    `json:"reason"`
}
type Readiness struct {
	Total      int              `json:"total"`
	Authorized int              `json:"authorized"`
	Ready      int              `json:"ready"`
	Items      []ReadinessAsset `json:"items"`
}

// Pratinjau hanya membaca; keputusan otoritatif diulang saat Run, Claim dan pada injector.
func (s *Service) Readiness(ctx context.Context, p *auth.Principal, org uuid.UUID) (*Readiness, error) {
	if err := permitted(p, authz.OrganizationRead, authz.AssetRead); err != nil {
		return nil, err
	}
	if _, err := s.Get(ctx, p, org); err != nil {
		return nil, err
	}
	out := &Readiness{Items: []ReadinessAsset{}}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE testing_authorized) FROM assets WHERE tenant_id=$1 AND organization_id=$2 AND deleted_at IS NULL`, p.TenantID, org).Scan(&out.Total, &out.Authorized); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT a.id,a.name,a.testing_authorized,COALESCE((SELECT jsonb_agg(jsonb_build_object('kind',d.kind,'value',d.value,'is_primary',d.is_primary) ORDER BY d.is_primary DESC,d.created_at) FROM asset_addresses d WHERE d.tenant_id=a.tenant_id AND d.asset_id=a.id),'[]'::jsonb) FROM assets a WHERE a.tenant_id=$1 AND a.organization_id=$2 AND a.deleted_at IS NULL ORDER BY a.name,a.id LIMIT 1000`, p.TenantID, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item ReadinessAsset
			var raw []byte
			var addresses []contract.Address
			if err = rows.Scan(&item.ID, &item.Name, &item.Authorized, &raw); err != nil {
				return err
			}
			if err = json.Unmarshal(raw, &addresses); err != nil {
				return err
			}
			item.Reason = s.targetReason(item.Authorized, addresses)
			if item.Reason == "" {
				out.Ready++
			}
			out.Items = append(out.Items, item)
		}
		return rows.Err()
	})
	return out, err
}
func (s *Service) targetReason(authorized bool, addresses []contract.Address) string {
	if !authorized {
		return "Not authorized for testing"
	}
	for _, key := range []string{contract.KeyNmapTCP, contract.KeyNucleiVS} {
		if _, err := s.Engine.Guard.Target(key, addresses); err != nil {
			return "Target refused: " + err.Error()
		}
	}
	return ""
}
