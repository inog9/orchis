package scans

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"orchis-go/internal/audit"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/contract"
	"orchis-go/internal/httpx"
	"orchis-go/internal/ids"
	"orchis-go/internal/scanscope"
	"time"
)

type Range struct {
	ID           uuid.UUID `json:"id"`
	CIDR         string    `json:"cidr"`
	Authorized   bool      `json:"authorized"`
	Version      int       `json:"version"`
	AuthorizedBy uuid.UUID `json:"authorized_by"`
	AuthorizedAt time.Time `json:"authorized_at"`
}
type Networks struct {
	Reported    []string `json:"reported"`
	Approvals   []Range  `json:"approvals"`
	MaxHosts    int      `json:"max_hosts"`
	Runner      string   `json:"runner"`
	SourceError string   `json:"source_error,omitempty"`
}
type ApproveInput struct {
	CIDR       string `json:"cidr"`
	Authorized bool   `json:"authorized"`
	Version    int    `json:"version"`
}

func (s *Service) Networks(ctx context.Context, p *auth.Principal, org, agent uuid.UUID) (*Networks, error) {
	if err := require(p, authz.OrganizationRead, authz.AssetRead, authz.AgentRead); err != nil {
		return nil, err
	}
	out := &Networks{Reported: []string{}, Approvals: []Range{}, MaxHosts: s.hostLimit(), Runner: "injector"}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		// READ ONLY transaksi tidak mengambil kunci; binding dan consent dibaca ulang pada approval/launch/claim.
		var exists bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE tenant_id=$1 AND organization_id=$2 AND id=$3)`, p.TenantID, org, agent).Scan(&exists); e != nil {
			return e
		}
		if !exists {
			return httpx.NotFound("Agent")
		}
		source, e := scanscope.ReadSourceForAgent(ctx, tx, p.TenantID, org, agent)
		if e != nil {
			out.SourceError = e.Error()
		}
		for _, prefix := range scanscope.ReportedNetworks(source.Rows, s.Guard) {
			out.Reported = append(out.Reported, prefix.String())
		}
		rows, e := tx.Query(ctx, `SELECT id,cidr,authorized,version,authorized_by,authorized_at FROM network_range_authorizations WHERE tenant_id=$1 AND organization_id=$2 AND agent_id=$3 ORDER BY cidr`, p.TenantID, org, agent)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var r Range
			if e = rows.Scan(&r.ID, &r.CIDR, &r.Authorized, &r.Version, &r.AuthorizedBy, &r.AuthorizedAt); e != nil {
				return e
			}
			out.Approvals = append(out.Approvals, r)
		}
		return rows.Err()
	})
	return out, err
}
func (s *Service) AuthorizeRange(ctx context.Context, p *auth.Principal, org, agent uuid.UUID, in ApproveInput, ip, req string) (*Range, error) {
	if err := require(p, authz.OrganizationWrite, authz.AssetAuthorize, authz.AssetRead, authz.AgentRead, authz.SettingsManage); err != nil {
		return nil, err
	}
	prefix, err := contract.BoundedNetwork(in.CIDR, s.hostLimit(), s.Guard)
	if err != nil {
		return nil, invalid("cidr", err.Error())
	}
	in.CIDR = prefix.String()
	out := &Range{}
	err = s.DB.InTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var locked uuid.UUID
		if e := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE tenant_id=$1 AND id=$2 AND NOT is_demo FOR UPDATE`, p.TenantID, org).Scan(&locked); errors.Is(e, pgx.ErrNoRows) {
			return httpx.NotFound("Organisation")
		} else if e != nil {
			return e
		}
		source, e := scanscope.SourceForAgent(ctx, tx, p.TenantID, org, agent)
		// Pencabutan harus tetap mungkin ketika data endpoint sudah hilang atau consent dicabut.
		if e != nil && in.Authorized {
			return invalid("cidr", e.Error())
		}
		if in.Authorized {
			if !scanscope.ContainsReported(in.CIDR, source.Rows, s.Guard) {
				return invalid("cidr", "Choose a range reported by this agent, or a smaller subnet within it.")
			}
			scope := contract.ScanScope{Target: prefix.Addr().String(), CIDR: in.CIDR, TestingAuthorized: true, InternetFacing: source.InternetFacing, MaxHosts: s.hostLimit()}
			if e = scope.Validate(scope.Target, s.Guard); e != nil {
				return invalid("cidr", e.Error())
			}
		}
		var id uuid.UUID
		var version int
		e = tx.QueryRow(ctx, `SELECT id,version FROM network_range_authorizations WHERE tenant_id=$1 AND organization_id=$2 AND agent_id=$3 AND cidr=$4 FOR UPDATE`, p.TenantID, org, agent, in.CIDR).Scan(&id, &version)
		if errors.Is(e, pgx.ErrNoRows) {
			if in.Version != 0 || !in.Authorized {
				return httpx.Conflict("Range approval changed; reload and retry.")
			}
			id = ids.New()
			_, e = tx.Exec(ctx, `INSERT INTO network_range_authorizations(id,tenant_id,organization_id,agent_id,control_asset_id,cidr,authorized,consent_revision,authorized_by) VALUES($1,$2,$3,$4,$5,$6,true,$7,$8)`, id, p.TenantID, org, agent, source.Asset, in.CIDR, source.Revision, p.UserID)
		} else if e == nil {
			if version != in.Version {
				return httpx.Conflict("Range approval changed; reload and retry.")
			}
			if in.Authorized {
				_, e = tx.Exec(ctx, `UPDATE network_range_authorizations SET authorized=$3,version=version+1,consent_revision=$4,authorized_by=$5,authorized_at=now(),control_asset_id=$6 WHERE tenant_id=$1 AND id=$2`, p.TenantID, id, in.Authorized, source.Revision, p.UserID, source.Asset)
			} else {
				_, e = tx.Exec(ctx, `UPDATE network_range_authorizations SET authorized=false,version=version+1,authorized_by=$3,authorized_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id, p.UserID)
			}
		}
		if e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, `SELECT id,cidr,authorized,version,authorized_by,authorized_at FROM network_range_authorizations WHERE tenant_id=$1 AND id=$2`, p.TenantID, id).Scan(&out.ID, &out.CIDR, &out.Authorized, &out.Version, &out.AuthorizedBy, &out.AuthorizedAt); e != nil {
			return e
		}
		return audit.Write(ctx, tx, audit.Entry{Tenant: p.TenantID, ActorUser: &p.UserID, Action: "scan.range.authorize", ResourceType: "network_range", ResourceID: &id, IP: ip, RequestID: req, Detail: map[string]any{"organization_id": org, "agent_id": agent, "cidr": in.CIDR, "authorized": in.Authorized, "version": out.Version}})
	})
	return out, err
}
