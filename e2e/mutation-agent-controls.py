"""Build-verified control mutations in a private copy; never edit live source."""
from pathlib import Path
import hashlib,json,shutil,subprocess,tempfile,sys
root=Path(__file__).resolve().parents[1]
copy=Path(tempfile.mkdtemp(prefix='agent-control-mutants-',dir=root/'work/final'))
for n in ['internal','migrations','cmd']:shutil.copytree(root/n,copy/n)
for n in ['go.mod','go.sum']:shutil.copy2(root/n,copy/n)
original={str(p.relative_to(copy)):p.read_bytes() for p in copy.rglob('*') if p.is_file()}
hashes={n:hashlib.sha256((root/n).read_bytes()).hexdigest() for n in original}
a='internal/agents/control.go';c='internal/agent/control.go';api='TestAgentControlsManualRolloutCASAndAuthenticatedUninstall';pause='TestAgentPauseBlocksQueuedClaimNewRunAndEveryCollectionPath';tenant='TestAgentControlTenantIsolationAndRelatedPermissions';bind='TestUninstallCommandBindingAndPrivateReceipt';remove='TestRemoveOwnedFilesRefusesSymlinkCacheAndForeignConfig'
# name, file, old, new, package, test, required assertion
mutants=[
('related-permissions',a,'if !authz.Has(p.Permissions, permission) {','if false && !authz.Has(p.Permissions, permission) {','server',tenant,'missing related permission allowed'),
('control-version',a,'if current != in.Version {','if false && current != in.Version {','server',api,'stale control accepted'),
('rollout-version',a,'if current.Version != in.Version {','if false && current.Version != in.Version {','server',api,'stale rollout accepted'),
('manual-rollout','internal/agents/agents.go','policy.Mode == "automatic" || requested == m.Version','true || policy.Mode == "automatic" || requested == m.Version','server',api,'manual rollout leaked release'),
('per-agent-update',a,'SET requested_update_version=$3,control_version=control_version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2','SET requested_update_version=$3,control_version=control_version+1,updated_at=now() WHERE tenant_id=$1 AND (id=$2 OR true)','server',api,'other agent received manual release'),
('anti-downgrade',a,'if !agentproto.Newer(m.Version, version) {','if false && !agentproto.Newer(m.Version, version) {','server',api,'equal release update allowed'),
('uninstall-confirmation',a,'if in.Confirmation != id.String() {','if false && in.Confirmation != id.String() {','server',api,'uninstall name guard missing'),
('uninstall-capability',a,'if capability != agentproto.ControlProtocol {','if false && capability != agentproto.ControlProtocol {','server',api,'old agent must not pretend uninstall support'),
('pending-command',a,'if uninstall == "queued" || uninstall == "dispatched" {','if false && (uninstall == "queued" || uninstall == "dispatched") {','server',api,'pending uninstall resumed'),
('ack-binding',a,'|| *current != command ||','|| false && *current != command ||','server',api,'wrong ACK accepted'),
('ack-revokes',a,'if in.Success {\n\t\t\tif e := s.revokeTx','if false && in.Success {\n\t\t\tif e := s.revokeTx','server',api,'uninstalled credential still valid'),
('paused-claim','internal/execution/runtime.go','if blocked {','if false && blocked {','server',pause,'paused agent claimed queued work'),
('paused-queue','internal/execution/service.go','if paused {','if false && paused {','server',pause,'paused new run allowed'),
('paused-periodic','internal/endpoint/collection.go','AND NOT g.paused AND','AND (NOT g.paused OR true) AND','server',pause,'paused periodic collection must skip cleanly'),
('paused-collect','internal/endpoint/collect_now.go','if status == "revoked" || !live || paused {','if status == "revoked" || !live || false && paused {','server',pause,'paused manual collect allowed'),
('collection-authorization','internal/endpoint/collect_now.go','if !authorized {','if false && !authorized {','server',pause,'collection after authorization revoked allowed'),
('rollout-rls','migrations/00033_agent_controls.sql',"SELECT enable_tenant_rls('agent_rollout_policies');",'','server',tenant,'rollout tenant RLS missing'),
('command-kind-binding',c,'if c.Kind != "uninstall" || c.AgentID != a.Cfg.AgentID {','if false && (c.Kind != "uninstall" || c.AgentID != a.Cfg.AgentID) {','agent',bind,'unsafe command accepted'),
('command-id',c,'if _, e := uuid.Parse(c.ID); e != nil {','if _, e := uuid.Parse(c.ID); false && e != nil {','agent',bind,'unsafe command accepted'),
('receipt-before-helper',c,'if e := a.Cfg.Save(a.ConfigPath); e != nil {','if e := error(nil); e != nil {','agent',bind,'helper started without durable receipt'),
('owned-config',c,'if !filepath.IsAbs(a.Exe) || filepath.Clean(a.ConfigPath) != filepath.Join(dir, "agent-config.json") {','if false && (!filepath.IsAbs(a.Exe) || filepath.Clean(a.ConfigPath) != filepath.Join(dir, "agent-config.json")) {','agent',remove,'unsafe removal accepted'),
('cache-symlink',c,'if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {','if false && (!st.IsDir() || st.Mode()&os.ModeSymlink != 0) {','agent',remove,'unsafe removal accepted'),
('diagnostic-allowlist','internal/agentproto/control.go','return ""','return code','agentproto','TestDiagnosticCodesNeverContainRawLogData','raw diagnostic accepted'),
]
# This condition appears twice: mutate only the cache parent check, preserving the version-directory guard.
if len(sys.argv)>1:mutants=[m for m in mutants if m[0] in sys.argv[1:]]
def run(args,name):
 p=subprocess.run(args,cwd=copy,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True);(copy/name).write_text(p.stdout);return p
pattern='^('+api+'|'+pause+'|'+tenant+'|'+bind+'|'+remove+'|TestDiagnosticCodesNeverContainRawLogData)$'
def baseline(name):
 p=run(['go','test','-count=1','-run',pattern,'./internal/server','./internal/agent','./internal/agentproto'],name);assert p.returncode==0,p.stdout
baseline('baseline.log');results=[]
try:
 for name,file,old,new,package,test,reason in mutants:
  path=copy/file;s=original[file].decode();expected=2 if name=='cache-symlink' else 1
  assert s.count(old)==expected,(name,s.count(old))
  path.write_text(s.replace(old,new,1))
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
