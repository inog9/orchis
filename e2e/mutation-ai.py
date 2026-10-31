#!/usr/bin/env python3
"""Uji penjaga AI pada salinan terisolasi; tidak melakukan panggilan provider nyata."""
from pathlib import Path
import os,shutil,subprocess,tempfile
root=Path(__file__).resolve().parents[1]
sandbox=Path(tempfile.mkdtemp(prefix='ai-mutation-',dir=root/'work'))
for name in ['internal','migrations']:shutil.copytree(root/name,sandbox/name)
for name in ['go.mod','go.sum']:shutil.copy2(root/name,sandbox/name)
cases=[
 ('privacy','internal/ai/contract.go',[('Target{ID: alias,','Target{ID: a.Name,')],'./internal/ai','TestMinimizationAndValidation'),
 ('cross-asset','internal/ai/contract.go',[('if !assets[a.AssetID] ||','if false ||')],'./internal/ai','TestMinimizationAndValidation'),
 ('schema','internal/ai/contract.go',[('d.DisallowUnknownFields()','// mutasi: unknown fields accepted')],'./internal/ai','TestMinimizationAndValidation'),
 ('consent','internal/ai/service.go',[('if !pol.Enabled || (r.Kind','if false || (r.Kind')],'./internal/server','TestAIRevocationCancellationLeaseAndInvalidOutput'),
 ('budget','internal/ai/service.go',[('if calls >= pol.DailyCalls || used+tokens > pol.DailyTokens || spent+cost > pol.DailyCostMicros {','if false {')],'./internal/server','TestAIConsentTenantBudgetAndIdempotency'),
]
def check(name,package,test,success):
 p=subprocess.run(['go','test',package,'-run','^'+test+'$','-count=1'],cwd=sandbox,env={**os.environ,'GOWORK':'off'},stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=180)
 (sandbox/(name+'.log')).write_text(p.stdout)
 if (p.returncode==0)!=success:raise SystemExit('Unexpected result '+name+'; '+str(sandbox))
 if not success and ('--- FAIL:' not in p.stdout or '[build failed]' in p.stdout):raise SystemExit('Mutation failed before assertion '+name)
 print('LULUS: '+name,flush=True)
for name,file,replacements,package,test in cases:
 path=sandbox/file;original=path.read_text();check(name+'-baseline',package,test,True)
 try:
  mutated=original
  for old,new in replacements:
   if old not in mutated:raise SystemExit('Missing anchor '+name)
   mutated=mutated.replace(old,new,1)
  path.write_text(mutated);check(name,package,test,False)
 finally:path.write_text(original)
 check(name+'-restored',package,test,True)
print('AI mutation evidence: '+str(sandbox))
