package reporting

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"orchis-go/internal/execution"
	"orchis-go/internal/osquery"
)

// endpointPosture membekukan postur endpoint satu aset: hanya data hasil terakhir dari query yang SAAT INI disetujui organisasinya
// (mencabut persetujuan menghapus data itu dari laporan baru), dengan port scan sebagai pembanding. Nil bila tidak ada data.
func endpointPosture(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, a Asset) (*osquery.Posture, error) {
	var org *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT organization_id FROM assets WHERE tenant_id=$1 AND id=$2`, tenant, a.ID).Scan(&org); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT query_key, rows, collected_at, COALESCE(osquery_version,'') FROM endpoint_datasets WHERE tenant_id=$1 AND asset_id=$2 AND collected_at IS NOT NULL`, tenant, a.ID)
	if err != nil {
		return nil, err
	}
	var list []osquery.Dataset
	for rows.Next() {
		var key, ver string
		var raw []byte
		var at *time.Time
		if err := rows.Scan(&key, &raw, &at, &ver); err != nil {
			rows.Close()
			return nil, err
		}
		ds := osquery.Dataset{Query: key, Status: osquery.StatusOK, Version: ver, Rows: []map[string]string{}}
		if at != nil {
			ds.CollectedAt = *at
		}
		_ = json.Unmarshal(raw, &ds.Rows)
		list = append(list, ds)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sets := map[string]*osquery.Dataset{}
	for i := range list {
		q := osquery.Find(list[i].Query)
		if q == nil {
			continue
		}
		ok, _, err := execution.OsqueryEnabled(ctx, tx, tenant, org, q)
		if err != nil {
			return nil, err
		}
		if ok {
			sets[list[i].Query] = &list[i]
		}
	}
	var scan []int
	for _, f := range a.Findings {
		if f.Type == "open_port" && strings.HasPrefix(f.Value, "tcp/") {
			if p, err := strconv.Atoi(strings.TrimPrefix(f.Value, "tcp/")); err == nil {
				scan = append(scan, p)
			}
		}
	}
	return osquery.BuildPosture(sets, scan), nil
}

func summarizeEndpoint(assets []Asset) *EndpointSummary {
	s := &EndpointSummary{AssetsTotal: len(assets), BySeverity: map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}}
	for _, a := range assets {
		if a.Endpoint == nil {
			continue
		}
		s.AssetsWithData++
		for _, f := range a.Endpoint.Findings {
			s.BySeverity[f.Severity]++
		}
	}
	if s.AssetsWithData == 0 {
		return nil
	}
	return s
}
