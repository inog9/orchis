#!/usr/bin/env python3
# Runs only against an owned disposable source copy; no live API/provider calls.
from pathlib import Path
import subprocess,time,json,shutil
source=Path(__file__).resolve().parents[1]
out=source/'work'/('related-mutations-'+str(time.time_ns()));out.mkdir(parents=True)
r=out/'src';r.mkdir()
for name in ['internal','migrations']:shutil.copytree(source/name,r/name)
for name in ['go.mod','go.sum']:shutil.copy2(source/name,r/name)
cases=[
 ('demo-login','internal/auth/service.go',"AND u.kind='human' AND NOT u.is_demo`","AND u.kind='human'`",'TestDemoIdentityAuthenticationRejectedEvenAfterCredentialChanges','demo login accepted'),
 ('demo-api-token','internal/auth/service.go','AND NOT t.is_demo AND t.revoked_at','AND t.revoked_at','TestDemoIdentityAuthenticationRejectedEvenAfterCredentialChanges','demo-marked API token accepted'),
 ('demo-agent','internal/agents/agents.go',"AND g.status<>'revoked' AND NOT g.is_demo`","AND g.status<>'revoked'`",'TestDemoIdentityAuthenticationRejectedEvenAfterCredentialChanges','demo agent accepted'),
 ('demo-injector','internal/execution/runtime.go',"AND kind='injector' AND NOT is_demo AND enabled", "AND kind='injector' AND enabled",'TestSyntheticIntegrationStatusIsInertAndStable','synthetic integration authenticated'),
]
baseline=subprocess.run(['go','test','./internal/demo','-run','^Test(DemoIdentityAuthenticationRejectedEvenAfterCredentialChanges|SyntheticIntegrationStatusIsInertAndStable)$','-count=1'],cwd=r,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=90)
(out/'baseline.log').write_text(baseline.stdout)
if baseline.returncode:raise RuntimeError('Baseline verification failed')
results=[]
for name,file,old,new,test,expected in cases:
 p=r/file;original=p.read_text();assert original.count(old)==1,(name,original.count(old))
 try:
  p.write_text(original.replace(old,new,1))
  run=subprocess.run(['go','test','./internal/demo','-run','^'+test+'$','-count=1'],cwd=r,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=90)
  (out/(name+'.log')).write_text(run.stdout)
  if run.returncode==0 or expected not in run.stdout:raise RuntimeError('Mutation not killed for intended reason: '+name)
  results.append({'name':name,'killed':True});print(name+': killed',flush=True)
 finally:p.write_text(original)
run=subprocess.run(['go','test','./internal/demo','-run','^Test(DemoIdentityAuthenticationRejectedEvenAfterCredentialChanges|SyntheticIntegrationStatusIsInertAndStable)$','-count=1'],cwd=r,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=90)
(out/'restored.log').write_text(run.stdout)
if run.returncode:raise RuntimeError('Restored verification failed')
(out/'results.json').write_text(json.dumps(results,indent=2));print('Restored source passed; evidence '+str(out),flush=True)
