"""Build-verified isolated retention mutations. No live files are changed."""
from pathlib import Path
import hashlib,json,shutil,subprocess,tempfile,sys
root=Path(__file__).resolve().parents[1];(root/'work/final').mkdir(parents=True,exist_ok=True)
copy=Path(tempfile.mkdtemp(prefix='retention-mutants-',dir=root/'work/final'))
for n in ['internal','migrations']:shutil.copytree(root/n,copy/n)
for n in ['go.mod','go.sum']:shutil.copy2(root/n,copy/n)
original={str(p.relative_to(copy)):p.read_bytes() for p in copy.rglob('*') if p.is_file()};hashes={n:hashlib.sha256((root/n).read_bytes()).hexdigest() for n in original}
f='internal/retention/';policy='TestRetentionPolicyCASMinimumAndTenantIsolation';delete='TestOrganizationDeletionScopeConfirmationAndDurableCleanup';scope='TestOrganizationDeletionPreservesEveryOtherOrganisationRecord';expiry='TestSweepIndependentLifetimesClockIsolationAndRestart';deps='TestOrganizationDeletionProtectsForeignHistoryAndDemo'
# name, file, old, new, package, focused test, required failure assertion
mutants=[
 ('admin',f+'policy.go','if !authz.Has(p.Permissions, perm) {','if false && !authz.Has(p.Permissions, perm) {','retention','TestDeletionRequiresEveryRelatedPermission/settings.manage','expected HTTP 403'),
 ('policy-cas',f+'policy.go','if current.Version != in.Version {','if false && current.Version != in.Version {','retention',policy,'expected HTTP 409'),
 ('audit-api-min',f+'policy.go','{"audit_days", p.AuditDays, MinimumAuditDays}','{"audit_days", p.AuditDays, 1}','retention',policy,'expected HTTP 422'),
 ('audit-db-min','migrations/00032_retention.sql','audit_days BETWEEN 90 AND 3650','audit_days BETWEEN 1 AND 3650','retention',policy,'database allowed audit retention'),
 ('exact-name',f+'organization.go','if current.Name != in.Name {','if false && current.Name != in.Name {','retention',delete,'expected HTTP 422'),
 ('org-version',f+'organization.go','current.Version != in.Version || !reflect.DeepEqual','false && current.Version != in.Version || !reflect.DeepEqual','retention',delete,'expected HTTP 409'),
 ('org-count-cas',f+'organization.go','|| !reflect.DeepEqual(current.Counts, in.Counts)','|| false && !reflect.DeepEqual(current.Counts, in.Counts)','retention',delete,'expected HTTP 409'),
 ('foreign-history',f+'organization.go','if protected {','if false && protected {','retention',deps,'missing deletion blocker'),
 ('foreign-dependency',f+'organization.go','if crossReference {','if false && crossReference {','retention',deps,'missing deletion blocker'),
 ('demo-delete',f+'organization.go','if demo {','if false && demo {','retention',deps,'missing deletion blocker'),
 ('report-rendering',f+'remove.go','if v.state == "rendering" {','if false && v.state == "rendering" {','retention','TestArtifactQueuesRejectActiveRendererAndForeignRawKey','expected HTTP 409'),
 ('pdf-key',f+'remove.go','if v.key != "" && v.key != expected {','if false && v.key != "" && v.key != expected {','retention','TestOrganizationDeletionBlocksRendererAndForeignKeysAtomically','expected HTTP 409',1),
 ('raw-key',f+'remove.go','if v.key != "" && v.key != expected {','if false && v.key != "" && v.key != expected {','retention','TestArtifactQueuesRejectActiveRendererAndForeignRawKey','expected HTTP 409',2),
 ('finding-scope',f+'remove.go','SELECT id FROM findings WHERE tenant_id=$1 AND (organization_id=$2 OR asset_id=ANY($3::uuid[]))','SELECT id FROM findings WHERE tenant_id=$1 AND (organization_id=$2 OR asset_id=ANY($3::uuid[]) OR true)','retention',scope,'other organisation record changed: findings'),
 ('report-scope',f+'remove.go','SELECT id FROM reports WHERE tenant_id=$1 AND organization_id=$2','SELECT id FROM reports WHERE tenant_id=$1 AND (organization_id=$2 OR true)','retention',scope,'other organisation record changed: reports'),
 ('execution-scope',f+'remove.go','OR simulation_id=ANY($4::uuid[]))`, tenant, org, assets, ownedSims)','OR simulation_id=ANY($4::uuid[]) OR true)`, tenant, org, assets, ownedSims)','retention',scope,'other organisation record changed: executions'),
 ('agent-revoke-scope',f+'organization.go',"WHERE tenant_id=$1 AND (organization_id=$2 OR asset_id IN(SELECT id FROM assets WHERE tenant_id=$1 AND organization_id=$2))`, p.TenantID, org, p.UserID)","WHERE tenant_id=$1 AND (organization_id=$2 OR asset_id IN(SELECT id FROM assets WHERE tenant_id=$1 AND organization_id=$2) OR true)`, p.TenantID, org, p.UserID)",'retention',scope,'other organisation agent revoked'),
 ('scan-expiry-active',f+'sweep.go',"AND a.status<>'running' AND a.results_expired_at","AND a.results_expired_at",'retention','TestSweepPinsRenderingActiveRunsAndRecentEndpoint','running scan expired'),
 ('endpoint-age',f+'sweep.go','COALESCE(x.collected_at,x.last_attempt_at)<$2','COALESCE(x.last_attempt_at,x.collected_at)<$2','retention',expiry,'failed attempt extended old successful dataset'),
 ('finding-epoch',f+'sweep.go','findings_epoch=findings_epoch+1','findings_epoch=findings_epoch+0','retention',expiry,'finding expiry did not reset comparison identity'),
 ('cleanup-blob',f+'cleanup.go','if err = s.Blob.Remove(run, key); err != nil {','_ = key; if err = nil; err != nil {','retention','TestRetentionCleanupExactKeysRestartAndTenantScope',''),
 ('cleanup-search',f+'cleanup.go','if err = s.Search.RemoveFindings(run, tenant, ids); err != nil {','if err = nil; err != nil {','retention','TestOrganizationDeletionRemovesRealOwnedMinIOAndESArtifacts','wrong scoped search cleanup'),
 ('snapshot-epoch','internal/reporting/comparison_history.go','SELECT findings_epoch FROM organizations','SELECT 0 FROM organizations','reporting','TestComparisonHistoryIsolationLatestFailureAndFreeze','retention identity reset lost at freeze'),
 ('comparison-epoch','internal/reporting/comparison.go','epoch: current.FindingsEpoch','epoch: 0','reporting','TestComparisonRetentionEpochPreventsFalseResolution',''),
 ('enrollment-lock','internal/agents/agents.go','err = s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {\n\t\tif err := scancontrol.Lock(ctx, tx, s.Tenant); err != nil {\n\t\t\treturn err\n\t\t}\n\t\tvar tokID','err = s.DB.InTenant(ctx, s.Tenant, func(tx pgx.Tx) error {\n\t\tvar tokID','server','TestEnrollmentWaitsForDeletionLockBeforeTokenRow','enrollment bypassed deletion serialization'),
]
mutants.extend([
 ('alert-resource-scope',f+'metadata.go','AND resource_id=ANY($3::uuid[])','AND (resource_id=ANY($3::uuid[]) OR true)','retention','TestOrganizationDeletionRemovesRelatedAlertAndOutboxContentsOnly','wrong scoped related metadata cleanup alerts'),
 ('outbox-resource-scope',f+'metadata.go','AND aggregate_id=ANY($3::uuid[])','AND (aggregate_id=ANY($3::uuid[]) OR true)','retention','TestOrganizationDeletionRemovesRelatedAlertAndOutboxContentsOnly','wrong scoped related metadata cleanup outbox'),
])
mutants.append(('foreign-frozen-snapshot',f+'organization.go',"AND r.snapshot @> jsonb_build_object('assets'","AND false AND r.snapshot @> jsonb_build_object('assets'",'retention',deps,'missing deletion blocker'))
for package,file in [('scan','internal/scans/worker.go'),('legacy','internal/reporting/reporting.go')]:
 for field in ['results_expired_at','report_expired_at']:
  mutants.append((package+'-no-resurrection-'+field,file,'AND a.'+field+' IS NULL ','','server','TestExpiredResultsAndReportsAreNeverFrozenAgain','expired snapshot recreated'))
mutants.extend([
 ('policy-lower-bound',f+'policy.go','if v.days < v.min || v.days > 3650 {','if false && v.days < v.min || v.days > 3650 {','retention','TestPolicyBoundsForEveryDataKind','expected HTTP 422'),
 ('policy-upper-bound',f+'policy.go','if v.days < v.min || v.days > 3650 {','if v.days < v.min || false && v.days > 3650 {','retention','TestPolicyBoundsForEveryDataKind','expected HTTP 422'),
 ('policy-version-bound',f+'policy.go','if p.Version < 0 {','if false && p.Version < 0 {','retention','TestPolicyBoundsForEveryDataKind','expected HTTP 422'),
])
for perm in ['OrganizationWrite' ,'AssetDelete','AgentManage','FindingRead','ReportRead']:
 mutants.append(('permission-'+perm,f+'organization.go','authz.'+perm+',','', 'retention','TestDeletionRequiresEveryRelatedPermission','expected HTTP 403') if perm!='ReportRead' else ('permission-'+perm,f+'organization.go',', authz.ReportRead)',')','retention','TestDeletionRequiresEveryRelatedPermission','expected HTTP 403'))
for table in ['retention_policies','data_deletions','data_cleanup_tasks']:
 mutants.append(('rls-'+table,'migrations/00032_retention.sql',"SELECT enable_tenant_rls('"+table+"');",'','retention','TestRetentionTablesForceTenantIsolation/'+table,'tenant RLS'))
if len(sys.argv)>1:mutants=[m for m in mutants if m[0] in sys.argv[1:]]
def run(args,name):
 p=subprocess.run(args,cwd=copy,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True);(copy/name).write_text(p.stdout);return p
baseline=run(['go','test','-count=1','./internal/retention','./internal/reporting'],'baseline.log');assert baseline.returncode==0,baseline.stdout
results=[]
try:
 for m in mutants:
  name,file,old,new,package,test,reason=m[:7];path=copy/file;s=original[file].decode()
  if len(m)==8:
   assert s.count(old)==2,(name,s.count(old));parts=s.split(old);idx=m[7];s=old.join(parts[:idx])+new+old.join(parts[idx:])
  else:assert s.count(old)==1,(name,s.count(old));s=s.replace(old,new)
  path.write_text(s)
  if name=='scan-expiry-active':
   migration=copy/'migrations/00032_retention.sql';before=original['migrations/00032_retention.sql'].decode();guard="ADD CHECK(results_expired_at IS NULL OR status<>'running');";assert before.count(guard)==1;migration.write_text(before.replace(guard,'ADD CHECK(true);'))
  build=run(['go','test','-c','-o',str(copy/'mutant.test'),'./internal/'+package],name+'-build.log')
  if build.returncode:raise RuntimeError(name+' build failure does not count as killed')
  result=run([str(copy/'mutant.test'),'-test.count=1','-test.run=^'+test+'$'],name+'.log')
  killed=result.returncode!=0 and ('--- FAIL: '+test.split('/')[0]) in result.stdout and reason in result.stdout
  results.append({'mutant':name,'build_passed':True,'killed':killed,'assertion':reason});path.write_bytes(original[file]);(copy/'migrations/00032_retention.sql').write_bytes(original['migrations/00032_retention.sql']);print(name,'KILLED' if killed else 'NOT KILLED',flush=True)
  if not killed:raise RuntimeError(name+' survived or failed for unrelated cause; see '+str(copy/(name+'.log')))
finally:
 for n,b in original.items():(copy/n).write_bytes(b)
 unchanged=all(hashlib.sha256((root/n).read_bytes()).hexdigest()==h for n,h in hashes.items());(copy/'result.json').write_text(json.dumps({'mutants':results,'source_unchanged':unchanged},indent=2)+'\n');assert unchanged
restored=run(['go','test','-count=1','./internal/retention','./internal/reporting'],'restored.log');assert restored.returncode==0,restored.stdout
print('Restored baseline PASS; live source unchanged; '+str(copy),flush=True)
