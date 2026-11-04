"""Security mutations in an isolated module; compile failures never count as kills."""
from pathlib import Path
import hashlib,json,shutil,subprocess,sys,tempfile
root=Path(__file__).resolve().parents[1]
(root/'work/final').mkdir(parents=True,exist_ok=True)
copy=Path(tempfile.mkdtemp(prefix='scan-control-mutants-',dir=root/'work/final'))
for n in ['internal','migrations']:shutil.copytree(root/n,copy/n)
for n in ['go.mod','go.sum']:shutil.copy2(root/n,copy/n)
original={str(p.relative_to(copy)):p.read_bytes() for p in copy.rglob('*') if p.is_file()}
source_hash={n:hashlib.sha256((root/n).read_bytes()).hexdigest() for n in original}
creation='TestScanWindowCreationCASAndExpiry';runtime='TestScanWindowHeartbeatAndLateResultAreCanceled';stop='TestEmergencyStopIsolationDurabilityCASAndAudit';moved='TestScanControlFrozenAndCurrentOrganisationCannotBeBypassed'
M=[
 ('end-exclusive','internal/scancontrol/window.go','','m < start || m >= end','m < start || m > end','TestWindowBoundariesAndTimezones','end'),
 ('local-zone','internal/scancontrol/window.go','',' || w.Timezone == "Local"','','TestInvalidWindowFailsClosed','Local'),
 ('window-check','internal/scancontrol/policy.go','func Check(','if !allowed {','if false && !allowed {',creation,'creation outside window'),
 ('global-hold','internal/scancontrol/policy.go','func Check(','state.GlobalPaused || state.Paused','state.Paused',stop,'not a documented stop'),
 ('organisation-hold','internal/scancontrol/policy.go','func Check(','state.GlobalPaused || state.Paused','state.GlobalPaused',stop,'not a documented stop'),
 ('frozen-organisation','internal/scancontrol/policy.go','func CheckAsset(','tenant, frozen, now','tenant, nil, now',moved,'not a documented stop'),
 ('current-organisation','internal/scancontrol/policy.go','func CheckAsset(','if current != nil &&','if false && current != nil &&',moved,'not a documented stop'),
 ('atomic-create','internal/execution/service.go','func (s *Service) QueueTest(','if err := scancontrol.CheckAsset(ctx, tx, p.TenantID, *target, nil, scancontrol.Now(s.Now)); err != nil {','if err := scancontrol.CheckAsset(ctx, tx, p.TenantID, *target, nil, scancontrol.Now(s.Now)); false && err != nil {',creation,'atomic outside window'),
 ('scan-create','internal/scans/scans.go','func (s *Service) Launch(','if err := scancontrol.Check(ctx, tx, p.TenantID, &in.OrganizationID, scancontrol.Now(s.Now)); err != nil {','if err := scancontrol.Check(ctx, tx, p.TenantID, &in.OrganizationID, scancontrol.Now(s.Now)); false && err != nil {',creation,'creation outside window'),
 ('legacy-create','internal/organizations/assessments.go','func (s *Service) Launch(','if err := scancontrol.Check(ctx, tx, p.TenantID, &org, scancontrol.Now(s.Now)); err != nil {','if err := scancontrol.Check(ctx, tx, p.TenantID, &org, scancontrol.Now(s.Now)); false && err != nil {',creation,'legacy outside window'),
 ('chain-create','internal/simulation/service.go','func (s *Service) Launch(','if err := scancontrol.CheckAsset(ctx, tx, p.TenantID, t.ID, nil, scancontrol.Now(s.Engine.Now)); err != nil {','if err := scancontrol.CheckAsset(ctx, tx, p.TenantID, t.ID, nil, scancontrol.Now(s.Engine.Now)); false && err != nil {','TestScanControlChainCreationAndQueuedStepAreGuarded','chain launch escaped hold'),
 ('claim-window','internal/execution/runtime.go','func (r *Runtime) Claim(','if e := scancontrol.CheckAsset(ctx, tx, r.Tenant, asset, frozen, scancontrol.Now(r.Now)); e != nil {','if e := scancontrol.CheckAsset(ctx, tx, r.Tenant, asset, frozen, scancontrol.Now(r.Now)); false && e != nil {',creation,'claim crossed closed window'),
 ('heartbeat-window','internal/execution/runtime.go','func (r *Runtime) Heartbeat(','if err := scancontrol.Enforce(ctx, tx, r.Tenant, scancontrol.Now(r.Now), r.AfterEnd); err != nil {','if err := scancontrol.Lock(ctx, tx, r.Tenant); err != nil {',runtime+'/heartbeat','heartbeat did not stop'),
 ('result-window','internal/execution/runtime.go','func (r *Runtime) Complete(','if err := scancontrol.Enforce(ctx, tx, r.Tenant, scancontrol.Now(r.Now), r.AfterEnd); err != nil {','if err := scancontrol.Lock(ctx, tx, r.Tenant); err != nil {',runtime+'/complete','late successful result accepted'),
 ('restart-window','internal/execution/runtime.go','func (r *Runtime) Sweep(','if err := scancontrol.Enforce(ctx, tx, r.Tenant, scancontrol.Now(r.Now), r.AfterEnd); err != nil {','if err := scancontrol.Lock(ctx, tx, r.Tenant); err != nil {',runtime+'/restart','not a documented stop'),
 ('queued-stop','internal/scancontrol/enforce.go','','err = Check(ctx, tx, tenant, &a.org, now)','err = Check(ctx, tx, tenant, nil, now)','TestEmergencyStopSerializesConcurrentLaunch','job escaped committed stop'),
 ('control-cas','internal/scancontrol/service.go','','action != "stop" && current.Version != in.Version','false && action != "stop" && current.Version != in.Version',creation,'stale window'),
 ('transaction-lock','internal/scancontrol/policy.go','func Lock(','"scan-control:"+tenant.String()','uuid.NewString()','TestScanControlLockIsTransactionalAndTenantScoped','scan mutation escaped tenant transaction lock'),
 ('admin-service','internal/scancontrol/service.go','func permission(','if write || org == nil {','if org == nil {','TestScanControlServiceRequiresAdministrator','service allowed nonadministrator scan control'),
 ('engine-queue-policy','internal/simulation/engine.go','func (e *Engine) launchStep(','if err := scancontrol.CheckAsset(ctx, tx, tenant, t.ID, t.OrganizationID, scancontrol.Now(e.Now)); err != nil {','if err := scancontrol.CheckAsset(ctx, tx, tenant, t.ID, t.OrganizationID, scancontrol.Now(e.Now)); false && err != nil {','TestScanControlEngineStopsUnstartedChild','engine queued a child despite hold'),
 ('policy-cancel-reason','internal/scancontrol/enforce.go','func Code(','return b.Code','return ""','TestScanControlEngineStopsUnstartedChild','policy stopped simulation not marked canceled'),
 ('scan-control-rls','migrations/00031_scan_controls.sql','',"SELECT enable_tenant_rls('scan_controls');",'','TestScanControlRLSAndCompositeOrganisationForeignKey','scan controls crossed tenant RLS'),
 ('atomic-stop-finalization','internal/scans/worker.go','',"stopped.status='canceled'","stopped.status='failed'",'TestScanControlStoppedScanFinalizesWithAtomicSnapshot','policy scan did not finalize canceled'),
]
if len(sys.argv)>1:
 requested=set(sys.argv[1:]);assert requested<={m[0] for m in M},'unknown mutant';M=[m for m in M if m[0] in requested]
def run(args,log):
 p=subprocess.run(args,cwd=copy,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
 (copy/log).write_text(p.stdout);return p
selection='TestWindow|TestInvalidWindow|TestWeeklyWindow|TestScanWindow|TestEmergencyStop|TestScanControl'
baseline=run(['go','test','-count=1','./internal/scancontrol','./internal/server','-run',selection],'baseline.log');assert baseline.returncode==0,baseline.stdout
results=[]
try:
 for name,file,marker,old,new,test,reason in M:
  path=copy/file;s=original[file].decode();a=s.index(marker) if marker else 0;b=s.find('\nfunc ',a+len(marker)) if marker else -1
  if b<0:b=len(s)
  segment=s[a:b];assert segment.count(old)==1,(name,segment.count(old));path.write_text(s[:a]+segment.replace(old,new)+s[b:])
  build=run(['go','test','./internal/scancontrol','./internal/server','-run','^$'],name+'-build.log')
  if build.returncode:raise RuntimeError(name+' build failed; NOT a killed mutant')
  package='./internal/scancontrol' if test.startswith(('TestWindow','TestInvalid','TestWeekly')) else './internal/server'
  result=run(['go','test','-count=1',package,'-run','^'+test+'$'],name+'.log')
  killed=result.returncode!=0 and reason in result.stdout and '[build failed]' not in result.stdout
  results.append({'mutant':name,'build_passed':True,'killed':killed,'expected_assertion':reason})
  path.write_bytes(original[file]);print(name,'KILLED' if killed else 'FAILED',flush=True)
  if not killed:raise RuntimeError(name+' survived or failed for unrelated cause')
finally:
 for n,b in original.items():(copy/n).write_bytes(b)
 unchanged=all(hashlib.sha256((root/n).read_bytes()).hexdigest()==h for n,h in source_hash.items())
 (copy/'result.json').write_text(json.dumps({'mutants':results,'source_unchanged':unchanged},indent=2)+'\n');assert unchanged,'actual source changed'
restored=run(['go','test','-count=1','./internal/scancontrol','./internal/server','-run',selection],'restored.log');assert restored.returncode==0,restored.stdout
print('Restored baseline PASS; actual source unchanged',flush=True)
