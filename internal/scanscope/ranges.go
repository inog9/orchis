// Package scanscope memeriksa ulang izin jaringan server tanpa mempercayai alamat dari agent.
package scanscope

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/netip"
	"orchis-go/internal/contract"
	"orchis-go/internal/osquery"
	"sort"
	"strings"
)

const NetworkQuery = "osquery.network_ranges"

// ReportedNetworks hanya proposal: default route, metadata, multicast dan link-local diabaikan.
func ReportedNetworks(rows []map[string]string, guard contract.Guard) []netip.Prefix {
	seen := map[netip.Prefix]bool{}
	for _, r := range rows {
		address, mask := r["address"], r["mask"]
		a, err := netip.ParseAddr(address)
		if err != nil || a.IsUnspecified() {
			continue
		}
		var p netip.Prefix
		if strings.Contains(address, "/") {
			continue
		}
		if m, e := netip.ParseAddr(mask); e == nil {
			if m.BitLen() != a.BitLen() {
				continue
			}
			bits := 0
			zero := false
			valid := true
			for _, b := range m.AsSlice() {
				for j := 7; j >= 0; j-- {
					one := b&(1<<j) != 0
					if one && zero {
						valid = false
					}
					if one {
						bits++
					} else {
						zero = true
					}
				}
			}
			if !valid {
				continue
			}
			p = netip.PrefixFrom(a, bits).Masked()
		} else {
			p, err = netip.ParsePrefix(a.String() + "/" + mask)
			if err != nil {
				continue
			}
			p = p.Masked()
		}
		if p.Bits() == 0 || guard.CheckHost(a.String()) != nil || guard.CheckHost(p.Addr().String()) != nil {
			continue
		}
		seen[p] = true
	}
	out := make([]netip.Prefix, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

func ContainsReported(raw string, rows []map[string]string, guard contract.Guard) bool {
	p, err := netip.ParsePrefix(raw)
	if err != nil || p != p.Masked() {
		return false
	}
	for _, source := range ReportedNetworks(rows, guard) {
		if source.Addr().BitLen() == p.Addr().BitLen() && p.Bits() >= source.Bits() && source.Contains(p.Addr()) {
			return true
		}
	}
	return false
}

type Source struct {
	Asset          uuid.UUID
	Revision       int64
	InternetFacing bool
	Rows           []map[string]string
}

// SourceForAgent memakai waktu penerimaan server, consent saat ini dan binding agent/aset/organisasi saat ini.
func SourceForAgent(ctx context.Context, tx pgx.Tx, tenant, org, agent uuid.UUID) (Source, error) {
	return sourceForAgent(ctx, tx, tenant, org, agent, true)
}
func ReadSourceForAgent(ctx context.Context, tx pgx.Tx, tenant, org, agent uuid.UUID) (Source, error) {
	return sourceForAgent(ctx, tx, tenant, org, agent, false)
}
func sourceForAgent(ctx context.Context, tx pgx.Tx, tenant, org, agent uuid.UUID, lock bool) (Source, error) {
	var s Source
	var rows []byte
	c := osquery.Consent()
	locking := ""
	if lock {
		locking = " FOR SHARE OF g,a,o,d,s"
	}
	err := tx.QueryRow(ctx, `SELECT a.id,s.version,a.is_internet_facing,d.rows FROM agents g
 JOIN assets a ON a.tenant_id=g.tenant_id AND a.id=g.asset_id AND a.organization_id=g.organization_id
 JOIN organizations o ON o.tenant_id=a.tenant_id AND o.id=a.organization_id
 JOIN endpoint_datasets d ON d.tenant_id=a.tenant_id AND d.asset_id=a.id AND d.query_key=$4
 JOIN osquery_org_settings s ON s.tenant_id=a.tenant_id AND s.organization_id=a.organization_id AND s.query_key=d.query_key
 WHERE g.tenant_id=$1 AND g.organization_id=$2 AND g.id=$3 AND g.status<>'revoked' AND NOT g.is_demo
 AND a.deleted_at IS NULL AND NOT a.is_demo AND NOT o.is_demo AND a.testing_authorized
 AND s.enabled AND s.consent_version=$5 AND s.consent_sha256=$6 AND s.consented_by IS NOT NULL AND s.consented_at IS NOT NULL
 AND NOT d.is_demo AND d.last_status='ok' AND d.last_attempt_at>now()-interval '24 hours'
 `+locking, tenant, org, agent, NetworkQuery, c.Version, c.SHA256).Scan(&s.Asset, &s.Revision, &s.InternetFacing, &rows)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, fmt.Errorf("agent network data requires current consent, testing authorization and a successful collection within 24 hours")
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(rows, &s.Rows)
	return s, err
}

// CheckJob mengembalikan nil scope untuk eksekusi legacy. Scan memerlukan izin hidup pada Run/Claim/Result.
func CheckJob(ctx context.Context, tx pgx.Tx, tenant, sim uuid.UUID, target string, guard contract.Guard, maxHosts int) (*contract.ScanScope, error) {
	var org, control uuid.UUID
	var rangeID *uuid.UUID
	var revision *int
	var scopeJSON []byte
	err := tx.QueryRow(ctx, `SELECT a.organization_id,j.control_asset_id,j.range_id,j.range_version,j.scope FROM scan_jobs j JOIN assessments a ON a.tenant_id=j.tenant_id AND a.id=j.assessment_id WHERE j.tenant_id=$1 AND j.simulation_id=$2 AND a.is_scan AND a.status='running'`, tenant, sim).Scan(&org, &control, &rangeID, &revision, &scopeJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		var scan bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE tenant_id=$1 AND simulation_id=$2)`, tenant, sim).Scan(&scan); e != nil {
			return nil, e
		}
		if scan {
			return nil, fmt.Errorf("scan is no longer running")
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var locked uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE tenant_id=$1 AND id=$2 FOR SHARE`, tenant, org).Scan(&locked); err != nil {
		return nil, err
	}
	var scope contract.ScanScope
	if err = json.Unmarshal(scopeJSON, &scope); err != nil {
		return nil, err
	}
	var currentOrg *uuid.UUID
	var authorized, public bool
	err = tx.QueryRow(ctx, `SELECT organization_id,testing_authorized,is_internet_facing FROM assets WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL AND NOT is_demo FOR SHARE`, tenant, control).Scan(&currentOrg, &authorized, &public)
	if err != nil || currentOrg == nil || *currentOrg != org || !authorized {
		return nil, fmt.Errorf("asset authorization or organisation changed")
	}
	scope.TestingAuthorized = authorized
	scope.InternetFacing = public
	scope.MaxHosts = maxHosts
	if rangeID != nil {
		var agent uuid.UUID
		var cidr string
		var version int
		var consent int64
		err = tx.QueryRow(ctx, `SELECT agent_id,cidr,version,consent_revision FROM network_range_authorizations WHERE tenant_id=$1 AND id=$2 AND organization_id=$3 AND control_asset_id=$4 AND authorized FOR SHARE`, tenant, *rangeID, org, control).Scan(&agent, &cidr, &version, &consent)
		if err != nil || revision == nil || version != *revision {
			return nil, fmt.Errorf("network authorization changed")
		}
		source, e := SourceForAgent(ctx, tx, tenant, org, agent)
		if e != nil {
			return nil, e
		}
		if source.Asset != control || source.Revision != consent || !ContainsReported(cidr, source.Rows, guard) {
			return nil, fmt.Errorf("approved network is no longer reported under the same consent")
		}
		scope.CIDR = cidr
	}
	if err = scope.Validate(target, guard); err != nil {
		return nil, err
	}
	return &scope, nil
}
