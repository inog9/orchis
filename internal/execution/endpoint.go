package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/osquery"
)

// processEndpointData menangani hasil aksi osquery dari agent. Dataset dari agent TIDAK TEPERCAYA: ia disanitasi terhadap
// skema query bernama (kolom, panjang, redaksi, batas baris), disimpan sebagai hasil terakhir per aset+query, lalu dipetakan
// oleh aturan beku menjadi atribut dan temuan. Temuan hanya ditambahkan bila eksekusi sukses dan datasetnya ok.
func processEndpointData(ctx context.Context, tx pgx.Tx, tenant, execID, asset uuid.UUID, ckey, outcome string, res *Result) error {
	q := osquery.Find(ckey)
	if q == nil {
		return nil
	}
	// Hanya aturan pemetaan server yang boleh menghasilkan temuan osquery.
	res.Findings = nil
	var in *osquery.Dataset
	for i := range res.Datasets {
		if res.Datasets[i].Query == ckey {
			in = &res.Datasets[i]
			break
		}
	}
	if in == nil {
		reason := "the agent returned no data"
		if outcome != "success" && res.Error != "" {
			reason = res.Error
		}
		in = &osquery.Dataset{Query: ckey, Status: osquery.StatusFailed, Reason: reason}
	}
	ds, err := osquery.Sanitize(q, in)
	if err != nil {
		ds = &osquery.Dataset{Query: ckey, Status: osquery.StatusFailed, Reason: "the agent returned invalid data", Rows: []map[string]string{}}
	}
	if outcome != "success" && ds.Status == osquery.StatusOK {
		ds.Status, ds.Reason = osquery.StatusFailed, "the check did not finish"
	}
	mapped := q.Map(ds)
	attrs, err := json.Marshal(mapped.Attributes)
	if err != nil {
		return err
	}
	rows, err := json.Marshal(ds.Rows)
	if err != nil {
		return err
	}
	if ds.Status == osquery.StatusOK {
		_, err = tx.Exec(ctx, `
INSERT INTO endpoint_datasets (tenant_id, asset_id, query_key, rows, attributes, truncated, collected_at, osquery_version, execution_id, last_status, last_reason, last_attempt_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,'ok','',now())
ON CONFLICT (tenant_id, asset_id, query_key) DO UPDATE SET rows=EXCLUDED.rows, attributes=EXCLUDED.attributes, truncated=EXCLUDED.truncated,
  collected_at=EXCLUDED.collected_at, osquery_version=EXCLUDED.osquery_version, execution_id=EXCLUDED.execution_id,
  last_status='ok', last_reason='', last_attempt_at=now(), is_demo=false`,
			tenant, asset, ckey, rows, attrs, ds.Truncated, ds.CollectedAt, ds.Version, execID)
	} else {
		// Percobaan yang dilewati/gagal hanya mencatat status dan alasannya; baris dari pengambilan ok terakhir tetap utuh.
		_, err = tx.Exec(ctx, `
INSERT INTO endpoint_datasets (tenant_id, asset_id, query_key, execution_id, last_status, last_reason, last_attempt_at)
VALUES ($1,$2,$3,$4,$5,$6,now())
ON CONFLICT (tenant_id, asset_id, query_key) DO UPDATE SET execution_id=EXCLUDED.execution_id, last_status=EXCLUDED.last_status,
  last_reason=EXCLUDED.last_reason, last_attempt_at=now()`, tenant, asset, ckey, execID, ds.Status, ds.Reason)
	}
	if err != nil {
		return err
	}
	if outcome != "success" || ds.Status != osquery.StatusOK {
		if ds.Status == osquery.StatusSkipped {
			return logLine(ctx, tx, tenant, execID, "warn", "Skipped: "+ds.Reason)
		}
		return nil
	}
	for _, f := range mapped.Findings {
		details, _ := json.Marshal(map[string]any{"source": "osquery", "query": ckey, "rule": f.Rule, "explain": f.Explain, "osquery_version": ds.Version})
		res.Findings = append(res.Findings, NewFinding{Type: f.Type, Value: f.Value, Title: f.Title, Severity: f.Severity, Details: details})
	}
	return logLine(ctx, tx, tenant, execID, "info", fmt.Sprintf("osquery %s: %d row(s), %d finding(s) mapped.", ckey, len(ds.Rows), len(mapped.Findings)))
}

// OsqueryEnabled menentukan apakah query disetujui untuk organisasi: baris pengaturan admin bila ada, selain itu bawaan katalog
// Semua query bernama memerlukan dokumen consent saat ini, termasuk query sensitivitas rendah.
func OsqueryEnabled(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, org *uuid.UUID, q *osquery.Query) (enabled, explicit bool, err error) {
	def := false
	if org == nil {
		return def, false, nil
	}
	var e bool
	err = tx.QueryRow(ctx, `SELECT enabled AND ($4 OR COALESCE((consent_version=$5 AND consent_sha256=$6 AND consented_by IS NOT NULL AND consented_at IS NOT NULL),false)) FROM osquery_org_settings WHERE tenant_id=$1 AND organization_id=$2 AND query_key=$3`, tenant, *org, q.Key, false, osquery.Consent().Version, osquery.Consent().SHA256).Scan(&e)
	if errors.Is(err, pgx.ErrNoRows) {
		return def, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return e, true, nil
}

// skipUnapproved menutup eksekusi osquery yang belum disetujui organisasinya sebagai berhasil-dilewati (bukan galat), mencatat
// alasannya pada dataset dan log. Mengembalikan true bila eksekusi sudah ditutup.
func skipUnapproved(ctx context.Context, tx pgx.Tx, tenant, execID, asset uuid.UUID, ckey string) (bool, error) {
	q := osquery.Find(ckey)
	if q == nil {
		return false, nil
	}
	policy, ok, err := currentEndpointPolicy(ctx, tx, tenant, asset, q)
	if err != nil {
		return false, err
	}
	reason := osquery.ReasonDisabled
	if ok {
		var platform, arch string
		e := tx.QueryRow(ctx, `SELECT g.os,g.arch FROM executions e JOIN agents g ON g.tenant_id=e.tenant_id AND g.id=e.agent_id WHERE e.tenant_id=$1 AND e.id=$2`, tenant, execID).Scan(&platform, &arch)
		if e != nil {
			return false, e
		}
		if platform == "macos" {
			platform = "darwin"
		}
		if arch == "x86_64" {
			arch = "amd64"
		}
		if _, supported := osquery.PinnedArtifact(platform, arch); !supported {
			ok = false
			reason = osquery.ReasonUnsupported
		}
	}
	if ok {
		frozen, err := json.Marshal(policy)
		if err != nil {
			return false, err
		}
		_, err = tx.Exec(ctx, `UPDATE executions SET endpoint_policy=$3 WHERE tenant_id=$1 AND id=$2`, tenant, execID, frozen)
		return false, err
	}
	zero := 0
	if _, err := tx.Exec(ctx, `UPDATE executions SET state='ended', outcome='success', exit_code=$3, ended_at=now(), version=version+1, updated_at=now() WHERE tenant_id=$1 AND id=$2`, tenant, execID, zero); err != nil {
		return false, err
	}
	res := &Result{Datasets: []osquery.Dataset{{Query: ckey, Status: osquery.StatusSkipped, Reason: reason, Rows: []map[string]string{}}}}
	return true, processEndpointData(ctx, tx, tenant, execID, asset, ckey, "success", res)
}

// endpointPolicy dibuat server saat Claim; tidak berasal dari klaim agent.
type endpointPolicy struct {
	Organization *uuid.UUID `json:"organization_id"`
	Revision     int64      `json:"revision"`
	AssetVersion int64      `json:"asset_version"`
}

// currentEndpointPolicy mengunci aset dan organisasi agar perubahan izin/setting
// terserialisasi dengan Claim dan penerimaan Result, termasuk pengaturan yang belum ada.
func currentEndpointPolicy(ctx context.Context, tx pgx.Tx, tenant, asset uuid.UUID, q *osquery.Query) (*endpointPolicy, bool, error) {
	p := &endpointPolicy{}
	var authorized bool
	err := tx.QueryRow(ctx, `SELECT organization_id,testing_authorized,version FROM assets WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL FOR SHARE`, tenant, asset).Scan(&p.Organization, &authorized, &p.AssetVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	enabled := false
	if p.Organization != nil {
		var org uuid.UUID
		if err = tx.QueryRow(ctx, `SELECT id FROM organizations WHERE tenant_id=$1 AND id=$2 FOR SHARE`, tenant, *p.Organization).Scan(&org); err != nil {
			return nil, false, err
		}
		err = tx.QueryRow(ctx, `SELECT enabled AND ($4 OR COALESCE((consent_version=$5 AND consent_sha256=$6 AND consented_by IS NOT NULL AND consented_at IS NOT NULL),false)),version FROM osquery_org_settings WHERE tenant_id=$1 AND organization_id=$2 AND query_key=$3`, tenant, *p.Organization, q.Key, false, osquery.Consent().Version, osquery.Consent().SHA256).Scan(&enabled, &p.Revision)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, false, err
		}
	}
	return p, authorized && enabled, nil
}

func acceptEndpointResult(ctx context.Context, tx pgx.Tx, tenant, asset uuid.UUID, q *osquery.Query, policyJSON []byte, agent *uuid.UUID) (string, error) {
	var claimed endpointPolicy
	if len(policyJSON) == 0 || json.Unmarshal(policyJSON, &claimed) != nil {
		return "Endpoint collection has no valid approval snapshot; run it again.", nil
	}
	current, enabled, err := currentEndpointPolicy(ctx, tx, tenant, asset, q)
	if err != nil {
		return "", err
	}
	sameOrg := (claimed.Organization == nil && current.Organization == nil) || (claimed.Organization != nil && current.Organization != nil && *claimed.Organization == *current.Organization)
	if !enabled || !sameOrg || claimed.Revision != current.Revision || claimed.AssetVersion != current.AssetVersion {
		return "Endpoint collection approval or asset authorization changed; the returned data was discarded.", nil
	}
	var active bool
	if agent != nil {
		err = tx.QueryRow(ctx, `SELECT status<>'revoked' AND NOT is_demo AND asset_id=$3 FROM agents WHERE tenant_id=$1 AND id=$2 FOR SHARE`, tenant, *agent, asset).Scan(&active)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}
	if !active {
		return "The endpoint agent is no longer authorized; the returned data was discarded.", nil
	}
	return "", nil
}
