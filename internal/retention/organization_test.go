package retention

import (
	"context"
	"github.com/google/uuid"
	"orchis-go/internal/auth"
	"orchis-go/internal/authz"
	"orchis-go/internal/testutil"
	"strings"
	"testing"
)

type orgFixture struct {
	org, asset, agent, finding, assessment, report, atomic, execution uuid.UUID
	name                                                              string
}

func fixture(t *testing.T, e *testutil.Env, p *auth.Principal) orgFixture {
	t.Helper()
	ctx := context.Background()
	f := orgFixture{org: uuid.New(), asset: uuid.New(), agent: uuid.New(), finding: uuid.New(), assessment: uuid.New(), report: uuid.New(), atomic: uuid.New(), execution: uuid.New()}
	f.name = "Delete fixture " + f.org.String()[:8]
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := e.Owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO organizations(id,tenant_id,name,slug) VALUES($1,$2,$3,$4)`, f.org, p.TenantID, f.name, "fixture-"+f.org.String())
	must(`INSERT INTO assets(id,tenant_id,organization_id,name,category) VALUES($1,$2,$3,'Fixture asset','host')`, f.asset, p.TenantID, f.org)
	must(`INSERT INTO agents(id,tenant_id,asset_id,organization_id,hostname,os,arch,mode,version,protocol_version,secret_hash) VALUES($1,$2,$3,$4,'fixture','linux','x86_64','user','1.0.0',1,$5)`, f.agent, p.TenantID, f.asset, f.org, []byte("FIXTURE-CREDENTIAL"))
	must(`INSERT INTO agent_enrollment_tokens(id,tenant_id,token_hash,os,arch,mode,created_by,expires_at,used_at,agent_id,organization_id) VALUES($1,$2,$3,'linux','x86_64','user',$4,now()+interval '1 hour',now(),$5,$6)`, uuid.New(), p.TenantID, []byte(f.agent.String()), p.UserID, f.agent, f.org)
	must(`INSERT INTO findings(id,tenant_id,asset_id,type,value,value_hash,title,severity,details) VALUES($1,$2,$3,'vulnerability','fixture exposure',$4,'Fixture exposure','high','{"secret":"DO-NOT-LOG"}')`, f.finding, p.TenantID, f.asset, []byte("fixture-hash"))
	must(`INSERT INTO endpoint_datasets(tenant_id,asset_id,query_key,last_status,collected_at) VALUES($1,$2,'fixture','ok',now())`, p.TenantID, f.asset)
	must(`INSERT INTO assessments(id,tenant_id,organization_id,name,profile,idempotency_key,request_hash,status,created_by,is_scan) VALUES($1,$2,$3,'Fixture scan','quick',$4,'fixture','finished',$5,true)`, f.assessment, p.TenantID, f.org, f.assessment.String(), p.UserID)
	must(`INSERT INTO reports(id,tenant_id,organization_id,assessment_id,snapshot,pdf_key) VALUES($1,$2,$3,$4,'{}',$5)`, f.report, p.TenantID, f.org, f.assessment, p.TenantID.String()+"/reports/"+f.report.String()+".pdf")
	must(`INSERT INTO atomic_tests(id,tenant_id,name,contract_id,contract_version,target_asset_id) SELECT $1,$2,'Fixture atomic',id,version,$3 FROM contracts WHERE tenant_id=$2 AND key='nmap.tcp_connect' ORDER BY version DESC LIMIT 1`, f.atomic, p.TenantID, f.asset)
	must(`INSERT INTO executions(id,tenant_id,atomic_test_id,contract_id,contract_version,target_asset_id,target_address,executor_kind,state,outcome,input_hash,ended_at,raw_output_key) SELECT $1,$2,$3,contract_id,contract_version,$4,'127.0.0.1','injector','ended','success',$5,now(),$6 FROM atomic_tests WHERE tenant_id=$2 AND id=$3`, f.execution, p.TenantID, f.atomic, f.asset, []byte("fixture-input"), "executions/"+p.TenantID.String()+"/"+f.execution.String()+"/output.txt")
	return f
}
func TestOrganizationDeletionScopeConfirmationAndDurableCleanup(t *testing.T) {
	e := testutil.New(t)
	ctx := context.Background()
	p := admin(t, e, e.TenantA)
	pb := admin(t, e, e.TenantB)
	a, b, foreign := fixture(t, e, p), fixture(t, e, p), fixture(t, e, pb)
	s := Service{DB: e.App}
	preview, err := s.Preview(ctx, p, a.org)
	if err != nil || len(preview.Blockers) > 0 || preview.Counts != (Counts{Assets: 1, Agents: 1, Findings: 1, Reports: 1, EndpointDatasets: 1, Scans: 1, Executions: 1}) {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	in := DeleteInput{Name: preview.Name, Version: preview.Version, Counts: preview.Counts}
	for _, role := range authz.SystemRoles[1:] {
		other := *p
		other.Permissions = role.Permissions
		_, err = s.Preview(ctx, &other, a.org)
		problem(t, err, 403)
		_, err = s.DeleteOrganization(ctx, &other, a.org, in, "", "forbidden")
		problem(t, err, 403)
	}
	invalid := in
	invalid.Name = "wrong"
	_, err = s.DeleteOrganization(ctx, p, a.org, invalid, "", "wrong-name")
	problem(t, err, 422)
	invalid = in
	invalid.Version++
	_, err = s.DeleteOrganization(ctx, p, a.org, invalid, "", "stale")
	problem(t, err, 409)
	invalid = in
	invalid.Counts.Assets++
	_, err = s.DeleteOrganization(ctx, p, a.org, invalid, "", "changed-counts")
	problem(t, err, 409)
	deleted, err := s.DeleteOrganization(ctx, p, a.org, in, "", "delete")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Counts != preview.Counts || deleted.Pending != 3 || deleted.Status != "external_cleanup_pending" {
		t.Fatalf("deletion status: %+v", deleted)
	}
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := e.Owner.QueryRow(ctx, q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, table := range []string{"assets", "agents", "findings", "reports", "assessments"} {
		if n := count(`SELECT count(*) FROM `+table+` WHERE tenant_id=$1 AND id=$2`, p.TenantID, map[string]uuid.UUID{"assets": a.asset, "agents": a.agent, "findings": a.finding, "reports": a.report, "assessments": a.assessment}[table]); n != 0 {
			t.Fatal("organisation rows remain:", table)
		}
	}
	for _, f := range []orgFixture{b, foreign} {
		if n := count(`SELECT count(*) FROM organizations WHERE id=$1`, f.org); n != 1 {
			t.Fatal("other organisation removed")
		}
		if n := count(`SELECT count(*) FROM reports WHERE id=$1`, f.report); n != 1 {
			t.Fatal("other organisation report removed")
		}
		if n := count(`SELECT count(*) FROM executions WHERE id=$1`, f.execution); n != 1 {
			t.Fatal("other organisation execution removed")
		}
	}
	var detail string
	if err = e.Owner.QueryRow(ctx, `SELECT detail::text FROM audit_log WHERE tenant_id=$1 AND action='organization.delete' AND resource_id=$2`, p.TenantID, a.org).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"DO-NOT-LOG", "FIXTURE-CREDENTIAL", "fixture exposure"} {
		if strings.Contains(detail, secret) {
			t.Fatal("delete audit contains data contents")
		}
	}
	_, err = s.Deletion(ctx, pb, deleted.ID)
	problem(t, err, 404)
	c := &cleaner{}
	restarted := Service{DB: e.App, Blob: c, Search: c}
	if err = restarted.CleanupOnce(ctx, p.TenantID); err != nil {
		t.Fatal(err)
	}
	done, err := s.Deletion(ctx, p, deleted.ID)
	if err != nil || done.Status != "completed" || done.Pending != 0 || done.CompletedAt == nil {
		t.Fatalf("cleanup completion: %+v %v", done, err)
	}
	if len(c.keys) != 2 || len(c.ids) != 1 || c.ids[0] != a.finding {
		t.Fatalf("wrong external targets: %+v", c)
	}
}
func TestOrganizationDeletionBlocksRendererAndForeignKeysAtomically(t *testing.T) {
	e := testutil.New(t)
	ctx := context.Background()
	p := admin(t, e, e.TenantA)
	a := fixture(t, e, p)
	s := Service{DB: e.App}
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := e.Owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`UPDATE reports SET status='rendering' WHERE id=$1`, a.report)
	preview, err := s.Preview(ctx, p, a.org)
	if err != nil || len(preview.Blockers) == 0 {
		t.Fatal("active renderer was not reported")
	}
	_, err = s.DeleteOrganization(ctx, p, a.org, DeleteInput{Name: preview.Name, Version: preview.Version, Counts: preview.Counts}, "", "render")
	problem(t, err, 409)
	must(`UPDATE reports SET status='queued',pdf_key='foreign/report.pdf' WHERE id=$1`, a.report)
	preview, err = s.Preview(ctx, p, a.org)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DeleteOrganization(ctx, p, a.org, DeleteInput{Name: preview.Name, Version: preview.Version, Counts: preview.Counts}, "", "foreign")
	problem(t, err, 409)
	var tasks, orgs int
	if err = e.Owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM data_cleanup_tasks),(SELECT count(*) FROM organizations WHERE id=$1)`, a.org).Scan(&tasks, &orgs); err != nil || tasks != 0 || orgs != 1 {
		t.Fatalf("failed deletion was not atomic: %d %d %v", tasks, orgs, err)
	}
	var state string
	if err = e.Owner.QueryRow(ctx, `SELECT status FROM agents WHERE id=$1`, a.agent).Scan(&state); err != nil || state != "online" {
		t.Fatal("rollback left agent revoked")
	}
}
