"""Build-verified control mutations in a private copy; never edit live source."""
from pathlib import Path
import hashlib,json,shutil,subprocess,tempfile,sys
root=Path(__file__).resolve().parents[1]
copy=Path(tempfile.mkdtemp(prefix='notification-mutants-',dir=root/'work/final'))
for n in ['internal','migrations','cmd']:shutil.copytree(root/n,copy/n)
for n in ['go.mod','go.sum']:shutil.copy2(root/n,copy/n)
original={str(p.relative_to(copy)):p.read_bytes() for p in copy.rglob('*') if p.is_file()}
hashes={n:hashlib.sha256((root/n).read_bytes()).hexdigest() for n in original}
a='internal/alerts/alerts.go';p='internal/alerts/policy.go';e='internal/alerts/events.go';g='internal/agents/agents.go';report='internal/reporting/reporting.go'
offline='TestNotificationsOfflinePolicyBoundariesEpisodesAndSafeFailures';org='TestNotificationsOrganizationTenantAndCurrentObjectVisibility';pending='TestNotificationsPendingApprovalsAndPolicyRLS';terminal='TestNotificationsScanTerminalAndReportFailureAreAtomicAndIdempotent';deleted='TestNotificationsDeletedObjectsAndRelatedPermissionsFailClosed'
mutants=[
('critical-finding-audience','internal/execution/findings.go','Audience: authz.FindingRead','Audience: authz.AlertRead','server','TestAlertsForCriticalFindingsFailedScansAndSimulations','Viewer finding notification audience missing'),
('audience',a,'a.audience_permission = ANY($2)','true','server',pending,'pending admin approval leaked to analyst'),
('object-marking',a,'FROM alerts a WHERE ` + objectVisible','FROM alerts a WHERE a.tenant_id=$1 AND a.audience_permission=ANY($2)`','server',org,'mark must enforce object visibility'),
('organization-filter',a,'($4::uuid IS NULL OR a.organization_id=$4)','($4::uuid IS NULL OR true)','server',org,'organization filter mismatch'),
('asset-current-organization',a,'x.id=a.resource_id AND x.deleted_at IS NULL AND x.organization_id IS NOT DISTINCT FROM a.organization_id','x.id=a.resource_id AND x.deleted_at IS NULL','server',org,'stale organization association must fail closed'),
('finding-current-organization',a,'f.id=a.resource_id AND x.organization_id IS NOT DISTINCT FROM a.organization_id','f.id=a.resource_id','server',org,'moved finding organization association leaked'),
('agent-asset-permission',a,"WHEN 'agent' THEN 'agent.read'=ANY($2) AND 'asset.read'=ANY($2)","WHEN 'agent' THEN 'agent.read'=ANY($2)",'server',deleted,'agent alert missing asset read permitted'),
('agent-organization-permission',a,"WHEN 'agent' THEN 'agent.read'=ANY($2) AND 'asset.read'=ANY($2) AND (a.organization_id IS NULL OR 'organization.read'=ANY($2))","WHEN 'agent' THEN 'agent.read'=ANY($2) AND 'asset.read'=ANY($2)",'server',deleted,'assigned agent requires organization permission'),
('report-finding-permission',a,"WHEN 'report' THEN 'report.read'=ANY($2) AND 'finding.read'=ANY($2)","WHEN 'report' THEN 'report.read'=ANY($2)",'server',terminal,'report notification requires finding read'),
('inbox-read-grant',p,'return httpx.Forbidden(authz.AlertRead)','return nil','server',deleted,'inbox without a read grant allowed'),
('policy-version',p,'if old.Version != in.Version {','if false && old.Version != in.Version {','server',offline,'policy CAS bypass'),
('policy-bounds',p,'if in.OfflineMinutes < 2 || in.OfflineMinutes > 10080 || in.Version < 0 {','if false && (in.OfflineMinutes < 2 || in.OfflineMinutes > 10080 || in.Version < 0) {','server',offline,'offline bounds'),
('policy-admin',p,'if !authz.Has(p.Permissions, authz.SettingsManage) {','if false && !authz.Has(p.Permissions, authz.SettingsManage) {','server',pending,'policy read direct permission bypass'),
('policy-rls','migrations/00034_notifications.sql',"SELECT enable_tenant_rls('notification_policies');",'','server',pending,'notification policy tenant RLS missing'),
('offline-threshold',g,"g.last_heartbeat_at<$2::timestamptz-make_interval(mins=>$3)","g.last_heartbeat_at<$2::timestamptz-make_interval(mins=>$3*0+1)",'server',offline,'90-second indicator must not trigger 15-minute notification'),
('offline-end-exclusive',g,'g.last_heartbeat_at<$2::timestamptz-make_interval','g.last_heartbeat_at<=$2::timestamptz-make_interval','server',offline,'threshold is strictly exceeded'),
('offline-episode',g,"offline_episode=offline_episode+CASE WHEN status='offline' THEN 1 ELSE 0 END","offline_episode=offline_episode+CASE WHEN status='offline' THEN 0 ELSE 0 END",'server',offline,'configured threshold and new episode'),
('update-failure-episode',g,"$8 IN('update_failed','release_key_missing') THEN 1 ELSE 0 END","$8 IN('update_failed','release_key_missing') THEN 0 ELSE 0 END",'server',offline,'new failure after recovery must notify'),
('revoke-event',g,'Kind: "agent_revoked"','Kind: "agent_revoked_disabled"','server',offline,'revocation event missing'),
('authorization-admin',e,'Audience: authz.AssetAuthorize','Audience: authz.AlertRead','server',pending,'pending admin approval leaked to analyst'),
('consent-admin',e,'Audience: authz.SettingsManage','Audience: authz.AlertRead','server',pending,'pending admin approval leaked to analyst'),
('retry-generation',e,'strconv.Itoa(generation)','strconv.Itoa(generation*0)','server',terminal,'new PDF retry failure must notify'),
('retry-increments','internal/reporting/http.go','render_generation=render_generation+1','render_generation=render_generation','server',terminal,'new PDF retry failure must notify'),
('report-completion-event',report,'return alerts.ReportEvent(ctx, tx, tenant, id, status, attempt)','return nil','server',terminal,'failed PDF attempt must notify exactly once'),
('report-expired-event',report,'x.id, "failed", x.attempt','x.id, "ready", x.attempt','server',terminal,'expired worker lease failure notification missing'),
('scan-terminal-event','internal/reporting/scan.go','Kind: kind, Severity: severity, Title: title','Kind: kind+"_disabled", Severity: severity, Title: title','server',terminal,'scan terminal event must be exactly once'),
]

import re
source=(copy/a).read_text()
for kind in ['agent','asset','finding','report','scan','simulation','execution','user']:
 line=next(x for x in source.splitlines() if x.startswith(" WHEN '"+kind+"' THEN") and " AND EXISTS(" in x)
 at=line.index(' AND EXISTS(') if ' AND EXISTS(' in line else -1
 # Agent has an earlier EXISTS only for its object; all cases use one correlated final EXISTS.
 assert at>=0,(kind,line)
 old=line[at+5:];new='true'
 mutants.append(('deleted-'+kind,a,old,new,'server',deleted,'deleted object notification exposed'))
if len(sys.argv)>1:mutants=[m for m in mutants if m[0] in sys.argv[1:]]
def run(args,name):
 p=subprocess.run(args,cwd=copy,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True);(copy/name).write_text(p.stdout);return p
for name,file,old,new,package,test,reason in mutants:
 expected=2 if name in ['organization-filter','policy-admin'] else 1
 assert original[file].decode().count(old)==expected,(name,original[file].decode().count(old))
pattern='^(TestNotifications|TestAlertsFor)'
def baseline(name):
 p=run(['go','test','-count=1','-run',pattern,'./internal/server'],name);assert p.returncode==0,p.stdout
baseline('baseline.log');results=[]
try:
 for name,file,old,new,package,test,reason in mutants:
  path=copy/file;s=original[file].decode();expected=2 if name in ['organization-filter','policy-admin'] else 1
  assert s.count(old)==expected,(name,s.count(old))
  path.write_text(s.replace(old,new))
  build=run(['go','test','-c','-o',str(copy/'mutant.test'),'./internal/'+package],name+'-build.log')
  if build.returncode:raise RuntimeError(name+' failed to build; not a killed mutation')
  result=run([str(copy/'mutant.test'),'-test.count=1','-test.run=^'+test+'$'],name+'.log')
  killed=result.returncode!=0 and ('--- FAIL: '+test) in result.stdout and reason in result.stdout
  results.append({'mutant':name,'build_passed':True,'killed':killed,'assertion':reason})
  path.write_bytes(original[file]);print(name,'KILLED' if killed else 'NOT KILLED',flush=True)
  if not killed:raise RuntimeError(name+' survived or failed for an unrelated cause; '+str(copy/(name+'.log')))
finally:
 for n,b in original.items():(copy/n).write_bytes(b)
 unchanged=all(hashlib.sha256((root/n).read_bytes()).hexdigest()==h for n,h in hashes.items())
 (copy/'result.json').write_text(json.dumps({'mutants':results,'source_unchanged':unchanged},indent=2)+'\n');assert unchanged
baseline('restored.log');print('Restored baseline PASS; live source unchanged; '+str(copy),flush=True)
