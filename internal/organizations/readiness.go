package organizations

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/contract"
	"orchis-go/internal/defensebot"
	"orchis-go/internal/osquery"
	"orchis-go/internal/scancontrol"
)

type ReadinessAsset struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	Authorized     bool       `json:"authorized"`
	Reason         string     `json:"reason"`
	WebReason      string     `json:"web_reason"`
	DefenseReason  string     `json:"defense_reason"`
	OS             string     `json:"os,omitempty"`
	Arch           string     `json:"arch,omitempty"`
	AgentStatus    string     `json:"agent_status"`            // none | online | offline
	URL            string     `json:"url,omitempty"`           // URL web disetujui utama, untuk tautan perbaikan
	LastScanAt     *time.Time `json:"last_scan_at,omitempty"`  // selesai scan terakhir
	LastScanStatus string     `json:"last_scan_status,omitempty"`
}
type Readiness struct {
	Total          int              `json:"total"`
	Authorized     int              `json:"authorized"`
	Ready          int              `json:"ready"`
	ScanWindowOpen bool             `json:"scan_window_open"`
	EmergencyStop  bool             `json:"emergency_stop"`
	ControlReason  string           `json:"control_reason,omitempty"`
	Items          []ReadinessAsset `json:"items"`
}

// Pratinjau hanya membaca; keputusan otoritatif diulang saat Run, Claim dan pada injector.
// Satu sumber untuk wizard scan baru, checklist kesiapan aset, dan dashboard cakupan organisasi.
func (s *Service) Readiness(ctx context.Context, p *auth.Principal, org uuid.UUID) (*Readiness, error) {
	if err := permitted(p, authz.OrganizationRead, authz.AssetRead); err != nil {
		return nil, err
	}
	if _, err := s.Get(ctx, p, org); err != nil {
		return nil, err
	}
	out := &Readiness{Items: []ReadinessAsset{}}
	err := s.DB.InTenantRead(ctx, p.TenantID, func(tx pgx.Tx) error {
		state, err := scancontrol.Read(ctx, tx, p.TenantID, &org)
		if err != nil {
			return err
		}
		out.EmergencyStop = state.Paused || state.GlobalPaused
		if err := scancontrol.Check(ctx, tx, p.TenantID, &org, time.Now()); err != nil {
			var blocked *scancontrol.Blocked
			if !errors.As(err, &blocked) {
				return err
			}
			out.ControlReason = blocked.Reason
		} else {
			out.ScanWindowOpen = true
		}
		if err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE testing_authorized) FROM assets WHERE tenant_id=$1 AND organization_id=$2 AND deleted_at IS NULL`, p.TenantID, org).Scan(&out.Total, &out.Authorized); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT a.id,a.name,a.testing_authorized,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('kind',d.kind,'value',d.value,'is_primary',d.is_primary) ORDER BY d.is_primary DESC,d.created_at) FROM asset_addresses d WHERE d.tenant_id=a.tenant_id AND d.asset_id=a.id AND d.scan_approved),'[]'::jsonb),
 COALESCE((SELECT jsonb_build_object('status',g.status,'os',g.os,'arch',g.arch) FROM agents g WHERE g.tenant_id=a.tenant_id AND g.asset_id=a.id AND NOT g.is_demo AND g.status<>'revoked' ORDER BY g.last_heartbeat_at DESC NULLS LAST LIMIT 1),'{}'::jsonb),
 (SELECT jsonb_build_object('status',j.status,'at',COALESCE(j.ended_at,j.created_at)) FROM scan_jobs j WHERE j.tenant_id=a.tenant_id AND j.asset_id=a.id ORDER BY j.id DESC LIMIT 1)
 FROM assets a WHERE a.tenant_id=$1 AND a.organization_id=$2 AND a.deleted_at IS NULL ORDER BY a.name,a.id LIMIT 1000`, p.TenantID, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		type agentInfo struct {
			Status string `json:"status"`
			OS     string `json:"os"`
			Arch   string `json:"arch"`
		}
		type jobInfo struct {
			Status string     `json:"status"`
			At     *time.Time `json:"at"`
		}
		consent := osquery.Consent()
		for rows.Next() {
			var item ReadinessAsset
			var raw, agentRaw, jobRaw []byte
			var addresses []contract.Address
			if err = rows.Scan(&item.ID, &item.Name, &item.Authorized, &raw, &agentRaw, &jobRaw); err != nil {
				return err
			}
			if err = json.Unmarshal(raw, &addresses); err != nil {
				return err
			}
			var agent agentInfo
			var job jobInfo
			if len(agentRaw) > 0 {
				if err = json.Unmarshal(agentRaw, &agent); err != nil {
					return err
				}
			}
			if len(jobRaw) > 0 {
				if err = json.Unmarshal(jobRaw, &job); err != nil {
					return err
				}
			}
			item.WebReason = s.targetReason(item.Authorized, addresses)
			item.Reason = item.WebReason
			for _, d := range addresses {
				if strings.HasPrefix(d.Value, "http://") || strings.HasPrefix(d.Value, "https://") {
					item.URL = d.Value
					break
				}
			}
			switch {
			case agent.Status == "":
				item.AgentStatus = "none"
				item.DefenseReason = "No agent is installed on this asset."
			case agent.Status == "offline":
				item.AgentStatus = "offline"
				item.DefenseReason = "The agent is offline; bring it back online to collect software data."
			default:
				item.AgentStatus = "online"
				item.OS, item.Arch = agent.OS, agent.Arch
				if !osquery.CollectionSupported(agent.OS, agent.Arch) {
					item.DefenseReason = "DefenseBot supports Linux and macOS agents only."
				} else if s.defenseConsentCount(ctx, tx, p.TenantID, org, agent.OS, consent.Version, consent.SHA256) == 0 {
					item.DefenseReason = "Collection is not approved for the current consent document."
				}
			}
			if job.Status != "" {
				item.LastScanStatus = job.Status
				item.LastScanAt = job.At
			}
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
func (s *Service) defenseConsentCount(ctx context.Context, tx pgx.Tx, tenant, org uuid.UUID, osName, version, sha string) int {
	var n int
	tx.QueryRow(ctx, `SELECT count(*) FROM osquery_org_settings WHERE tenant_id=$1 AND organization_id=$2 AND enabled AND consent_version=$3 AND consent_sha256=$4 AND consented_by IS NOT NULL AND consented_at IS NOT NULL AND query_key=ANY($5)`, tenant, org, version, sha, defensebot.Queries).Scan(&n)
	return n
}
