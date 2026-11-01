package mitre

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type coverageRecord struct {
	Mapped            bool
	Runs              int
	LastRunAt         *time.Time
	SimulatedRuns     int
	SimulatedFindings int
}

// loadCoverage memisahkan bukti nyata dan sintetis. ID eksekusi/temuan dihitung
// unik per teknik dan induknya agar satu aksi dengan dua pemetaan sub-teknik
// tidak menggandakan jumlah run pada induk.
func loadCoverage(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (map[string]*coverageRecord, error) {
	stats := map[string]*coverageRecord{}
	get := func(id string) *coverageRecord {
		if stats[id] == nil {
			stats[id] = &coverageRecord{}
		}
		return stats[id]
	}
	mr, err := tx.Query(ctx, `SELECT ct.technique_id,t.parent_id FROM contract_techniques ct
JOIN mitre_techniques t ON t.id=ct.technique_id
WHERE ct.tenant_id=$1 AND NOT t.is_deprecated`, tenant)
	if err != nil {
		return nil, err
	}
	for mr.Next() {
		var id string
		var parent *string
		if err := mr.Scan(&id, &parent); err != nil {
			mr.Close()
			return nil, err
		}
		get(id).Mapped = true
		if parent != nil {
			get(*parent).Mapped = true
		}
	}
	mr.Close()
	if err := mr.Err(); err != nil {
		return nil, err
	}
	rr, err := tx.Query(ctx, `
SELECT refs.technique_id,count(DISTINCT e.id),max(e.ended_at) FROM executions e
JOIN contract_techniques ct ON ct.tenant_id=e.tenant_id AND ct.contract_id=e.contract_id
JOIN mitre_techniques t ON t.id=ct.technique_id AND NOT t.is_deprecated
CROSS JOIN LATERAL (SELECT ct.technique_id UNION SELECT t.parent_id WHERE t.parent_id IS NOT NULL) refs
WHERE e.tenant_id=$1 AND NOT e.is_demo AND e.state='ended' AND e.outcome='success' GROUP BY 1`, tenant)
	if err != nil {
		return nil, err
	}
	for rr.Next() {
		var id string
		var n int
		var last *time.Time
		if err := rr.Scan(&id, &n, &last); err != nil {
			rr.Close()
			return nil, err
		}
		st := get(id)
		st.Runs = n
		st.LastRunAt = last
	}
	rr.Close()
	if err := rr.Err(); err != nil {
		return nil, err
	}
	sr, err := tx.Query(ctx, `
WITH source_refs AS (
 SELECT ct.technique_id,t.parent_id,e.id AS execution_id,NULL::uuid AS finding_id
 FROM executions e
 JOIN contract_techniques ct ON ct.tenant_id=e.tenant_id AND ct.contract_id=e.contract_id
 JOIN mitre_techniques t ON t.id=ct.technique_id AND NOT t.is_deprecated
 WHERE e.tenant_id=$1 AND e.is_demo AND e.state='ended' AND e.outcome='success'
 UNION ALL
 SELECT t.id,t.parent_id,NULL::uuid,f.id FROM findings f
 JOIN mitre_techniques t ON t.id=f.details->>'mitre_technique' AND NOT t.is_deprecated
 WHERE f.tenant_id=$1 AND f.is_demo
), expanded AS (
 SELECT technique_id,execution_id,finding_id FROM source_refs
 UNION ALL SELECT parent_id,execution_id,finding_id FROM source_refs WHERE parent_id IS NOT NULL
)
SELECT technique_id,count(DISTINCT execution_id),count(DISTINCT finding_id) FROM expanded GROUP BY 1`, tenant)
	if err != nil {
		return nil, err
	}
	for sr.Next() {
		var id string
		var runs, findings int
		if err := sr.Scan(&id, &runs, &findings); err != nil {
			sr.Close()
			return nil, err
		}
		st := get(id)
		st.SimulatedRuns = runs
		st.SimulatedFindings = findings
	}
	sr.Close()
	if err := sr.Err(); err != nil {
		return nil, err
	}
	return stats, nil
}

func applyCoverage(c *Cell, st *coverageRecord) {
	if st == nil {
		return
	}
	c.Mapped = st.Mapped
	c.Runs = st.Runs
	c.LastRunAt = st.LastRunAt
	c.Exercised = st.Runs > 0
	c.SimulatedRuns = st.SimulatedRuns
	c.SimulatedFindings = st.SimulatedFindings
	c.Simulated = st.SimulatedRuns > 0 || st.SimulatedFindings > 0
}
