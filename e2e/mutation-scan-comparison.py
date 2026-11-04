"""Isolated, build-verified comparison guard mutations; live sources stay unchanged."""
from pathlib import Path
import hashlib,json,shutil,subprocess,tempfile
root=Path(__file__).resolve().parents[1];(root/'work/final').mkdir(parents=True,exist_ok=True)
copy=Path(tempfile.mkdtemp(prefix='comparison-mutants-',dir=root/'work/final'))
for n in ['internal','migrations']:shutil.copytree(root/n,copy/n)
for n in ['go.mod','go.sum']:shutil.copy2(root/n,copy/n)
original={str(p.relative_to(copy)):p.read_bytes() for p in copy.rglob('*') if p.is_file()}
hashes={n:hashlib.sha256((root/n).read_bytes()).hexdigest() for n in original}
f='internal/reporting/comparison.go';guard='TestComparisonResolutionGuards';history='TestComparisonLatestMatchingAttemptAndPerToolHistory';identity='TestComparisonCoverageIdentityIncludesVersionAndTool'
mutants=[
 ('current-completed',f,'if !canUse(a.asset, m) || !canUse(base.asset, base.state) {','_ = m; if !canUse(base.asset, base.state) {',guard+'/current_failed','unsafe resolved'),
 ('previous-completed',f,'if !canUse(a.asset, m) || !canUse(base.asset, base.state) {','_ = base; if !canUse(a.asset, m) {',guard+'/previous_failed','unsafe resolved'),
 ('skip-reason',f,'a.SkipReason != "" || ','',guard+'/skip_reason','unsafe resolved'),
 ('stopped-state',f,'if a.Status != "incomplete" {','if a.Status != "incomplete" && a.Status != "canceled" {',guard+'/stopped_stale_success','unsafe resolved'),
 ('duplicate-coverage',f,'if !m.valid || a.SkipReason','if a.SkipReason',guard+'/duplicate_methods','unsafe resolved'),
 ('profile',f,'if previous.Profile != b.profile {','if false && previous.Profile != b.profile {',guard+'/profile','unsafe resolved'),
 ('target',f,'[]any{m.Action, m.Version, m.Target, json.RawMessage(canonical)}','[]any{m.Action, m.Version, json.RawMessage(canonical)}',guard+'/target','unsafe resolved'),
 ('parameters',f,'data, _ := json.Marshal([]any{m.Action, m.Version, m.Target, json.RawMessage(canonical)})','_ = canonical; data, _ := json.Marshal([]any{m.Action, m.Version, m.Target})',guard+'/parameters','unsafe resolved'),
 ('version',f,'[]any{m.Action, m.Version, m.Target, json.RawMessage(canonical)}','[]any{m.Action, m.Target, json.RawMessage(canonical)}',identity,'definition version omitted'),
 ('tool',f,'[]any{m.Action, m.Version, m.Target, json.RawMessage(canonical)}','[]any{m.Version, m.Target, json.RawMessage(canonical)}',identity,'tool omitted'),
 ('latest-attempt',f,'if _, found := current.previous[key]; found {','if _, found := current.previous[key]; false && found {',history,'latest failed must shadow success'),
 ('all-producers',f,'for _, k := range f.Coverage {\n\t\t\t\tm, hasCurrent','for _, k := range f.Coverage[:1] {\n\t\t\t\tm, hasCurrent','TestComparisonRequiresEveryProducingCheck','resolution ignored one'),
 ('producer-type',f,'if !allowed {','if false && !allowed {',guard+'/incompatible_producer','unsafe resolved'),
 ('asset-boundary',f,'current := b.assets[id]','var current *comparisonAsset; for _,candidate:=range b.assets{current=candidate;break}',guard+'/asset','unsafe resolved'),
 ('organisation-history' ,'internal/reporting/comparison_history.go','AND r.organization_id=$2','AND ($2::uuid IS NOT NULL)', 'TestComparisonHistoryIsolationLatestFailureAndFreeze','wrong organisation/profile baseline'),
]
def run(args,name):
 p=subprocess.run(args,cwd=copy,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True);(copy/name).write_text(p.stdout);return p
baseline=run(['go','test','-count=1','./internal/reporting','-run','TestComparison'],'baseline.log');assert baseline.returncode==0,baseline.stdout
results=[]
try:
 for name,file,old,new,test,reason in mutants:
  path=copy/file;s=original[file].decode();assert s.count(old)==1,(name,s.count(old));path.write_text(s.replace(old,new))
  build=run(['go','test','./internal/reporting','-run','^$'],name+'-build.log')
  if build.returncode:raise RuntimeError(name+' build failure does not count as killed')
  result=run(['go','test','-count=1','./internal/reporting','-run','^'+test+'$'],name+'.log');killed=result.returncode!=0 and reason in result.stdout and '[build failed]' not in result.stdout
  results.append({'mutant':name,'build_passed':True,'killed':killed,'assertion':reason});path.write_bytes(original[file]);print(name,'KILLED' if killed else 'NOT KILLED',flush=True)
  if not killed:raise RuntimeError(name+' survived or failed for an unrelated cause')
finally:
 for n,b in original.items():(copy/n).write_bytes(b)
 unchanged=all(hashlib.sha256((root/n).read_bytes()).hexdigest()==h for n,h in hashes.items());(copy/'result.json').write_text(json.dumps({'mutants':results,'source_unchanged':unchanged},indent=2)+'\n');assert unchanged
restored=run(['go','test','-count=1','./internal/reporting','-run','TestComparison'],'restored.log');assert restored.returncode==0,restored.stdout
print('Restored baseline PASS; live source unchanged',flush=True)
