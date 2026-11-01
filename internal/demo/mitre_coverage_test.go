package demo_test

import (
	"context"
	"crypto/sha256"
	"testing"

	"orchis-go/internal/auth"
	"orchis-go/internal/demo"
	"orchis-go/internal/ids"
	"orchis-go/internal/mitre"
	"orchis-go/internal/testutil"
)

// Dua sub-teknik dari satu eksekusi tetap satu run pada induk. Label demo tidak
// hilang ketika teknik yang sama juga dipetakan atau mempunyai kontrol run nyata.
func TestMITRECoverageKeepsOverlappingEvidenceSeparateAndDeduplicated(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	addAdmin(t, env, env.TenantA)
	if err := (&demo.Service{DB: env.App}).Seed(ctx, env.TenantA, 7, nil, "", "coverage"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `
INSERT INTO mitre_tactics(id,name,shortname) VALUES('TA0007','Discovery','discovery'),('TA0043','Reconnaissance','reconnaissance');
INSERT INTO mitre_techniques(id,name,parent_id,tactic_ids,platforms) VALUES
('T1046','Network Service Discovery',NULL,ARRAY['TA0007'],ARRAY['Linux']),
('T1595','Active Scanning',NULL,ARRAY['TA0043'],ARRAY['PRE']),
('T1595.001','Scanning IP Blocks','T1595',ARRAY['TA0043'],ARRAY['PRE']),
('T1595.002','Vulnerability Scanning','T1595',ARRAY['TA0043'],ARRAY['PRE']);`); err != nil {
		t.Fatal(err)
	}
	// Pemetaan ganda ini hanya fixture untuk membuktikan deduplikasi hierarki.
	if _, err := env.Owner.Exec(ctx, `INSERT INTO contract_techniques(tenant_id,contract_id,technique_id)
SELECT c.tenant_id,c.id,t.id FROM contracts c CROSS JOIN mitre_techniques t
WHERE c.tenant_id=$1 AND c.key='nmap.tcp_connect' AND t.id IN ('T1046','T1595.001','T1595.002')`, env.TenantA); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Owner.Exec(ctx, `UPDATE findings SET details=jsonb_set(details,'{mitre_technique}','"T1595.002"'::jsonb)
WHERE id=(SELECT id FROM findings WHERE tenant_id=$1 AND is_demo ORDER BY id LIMIT 1)`, env.TenantA); err != nil {
		t.Fatal(err)
	}
	wantRuns := count(t, env, `SELECT count(*) FROM executions e JOIN contracts c ON c.tenant_id=e.tenant_id AND c.id=e.contract_id WHERE e.tenant_id=$1 AND e.is_demo AND e.outcome='success' AND c.key='nmap.tcp_connect'`, env.TenantA)
	if wantRuns == 0 || count(t, env, `SELECT count(*) FROM executions e JOIN contracts c ON c.tenant_id=e.tenant_id AND c.id=e.contract_id WHERE e.tenant_id=$1 AND e.is_demo AND e.outcome<>'success' AND c.key='nmap.tcp_connect'`, env.TenantA) == 0 {
		t.Fatal("fixture needs both successful and failed synthetic runs")
	}
	svc := &mitre.Service{DB: env.App}
	p := &auth.Principal{TenantID: env.TenantA}
	check := func(real bool) {
		t.Helper()
		matrix, err := svc.Matrix(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		if matrix.Total != 2 || matrix.Mapped != 2 || matrix.Simulated != 2 {
			t.Fatalf("mapped and simulated must overlap: %+v", matrix)
		}
		if matrix.Exercised != map[bool]int{false: 0, true: 2}[real] {
			t.Fatalf("incorrect real training count: %d", matrix.Exercised)
		}
		for _, col := range matrix.Tactics {
			for _, cell := range col.Techniques {
				if !cell.Mapped || !cell.Simulated || cell.SimulatedRuns != wantRuns || cell.Runs != map[bool]int{false: 0, true: 1}[real] || cell.Exercised != real {
					t.Fatalf("evidence conflated or duplicated: %+v", cell)
				}
				if !real && cell.LastRunAt != nil {
					t.Fatal("synthetic timestamp exposed as last real run")
				}
				if cell.ID == "T1595" && cell.SimulatedFindings != 1 {
					t.Fatalf("parent lost child demo finding: %+v", cell)
				}
			}
		}
		detail, err := svc.Technique(ctx, p, "T1595")
		if err != nil {
			t.Fatal(err)
		}
		if !detail.Simulated || detail.SimulatedRuns != wantRuns || detail.SimulatedFindings != 1 || detail.Runs != map[bool]int{false: 0, true: 1}[real] {
			t.Fatalf("detail and matrix disagree: %+v", detail)
		}
		for _, sub := range detail.Subs {
			if !sub.Mapped || !sub.Simulated || sub.Runs != detail.Runs || sub.SimulatedRuns != wantRuns {
				t.Fatalf("sub-technique evidence lost: %+v", sub)
			}
		}
	}
	check(false)
	// Kontrol penyimpanan non-demo untuk predikat agregasi; ini tidak menjalankan scanner.

	atomic := ids.New()
	if _, err := env.Owner.Exec(ctx, `INSERT INTO atomic_tests(id,tenant_id,name,contract_id,contract_version,params)
 SELECT $2,tenant_id,'Coverage storage control',id,version,'{}'::jsonb FROM contracts WHERE tenant_id=$1 AND key='nmap.tcp_connect' ORDER BY version DESC LIMIT 1`, env.TenantA, atomic); err != nil {
		t.Fatal(err)
	}
	control := ids.New()
	if _, err := env.Owner.Exec(ctx, `INSERT INTO executions(id,tenant_id,atomic_test_id,contract_id,contract_version,target_asset_id,target_address,executor_kind,integration_id,params,input_hash,state,outcome,started_at,ended_at,is_demo)
SELECT $2,e.tenant_id,$3,e.contract_id,e.contract_version,e.target_asset_id,e.target_address,e.executor_kind,e.integration_id,e.params,e.input_hash,'ended','success',now(),now(),false
FROM executions e JOIN contracts c ON c.tenant_id=e.tenant_id AND c.id=e.contract_id
WHERE e.tenant_id=$1 AND e.is_demo AND e.outcome='success' AND c.key='nmap.tcp_connect' ORDER BY e.id LIMIT 1`, env.TenantA, control, atomic); err != nil {
		t.Fatal(err)
	}

	controlValue := "coverage-non-demo-control"
	controlHash := sha256.Sum256([]byte(controlValue))
	if _, err := env.Owner.Exec(ctx, `INSERT INTO findings(id,tenant_id,asset_id,type,value,value_hash,title,severity,details,is_demo)
 SELECT $2,tenant_id,asset_id,type,$3,$4,title,severity,details,false FROM findings
 WHERE tenant_id=$1 AND is_demo AND details->>'mitre_technique'='T1595.002' ORDER BY id LIMIT 1`, env.TenantA, ids.New(), controlValue, controlHash[:]); err != nil {
		t.Fatal(err)
	}
	check(true)
	other, err := svc.Matrix(ctx, &auth.Principal{TenantID: env.TenantB})
	if err != nil {
		t.Fatal(err)
	}
	if other.Mapped != 0 || other.Exercised != 0 || other.Simulated != 0 {
		t.Fatalf("coverage leaked to tenant B: %+v", other)
	}
}
