#!/usr/bin/env python3
# Runs only against an owned disposable source copy; no live API/provider calls.
from pathlib import Path
import subprocess,time,json,shutil
source=Path(__file__).resolve().parents[1]
out=source/'work'/('demo-ai-mutations-'+str(time.time_ns()));out.mkdir(parents=True)
r=out/'src';r.mkdir()
for name in ['internal','migrations']:shutil.copytree(source/name,r/name)
for name in ['go.mod','go.sum']:shutil.copy2(source/name,r/name)
cases=[
 ('fake-origin','internal/demo/fake_ai.go','r.URL.Scheme != "http" || r.URL.Host != o.origin','r.URL.Scheme != "http"','TestLocalFakeAIRejectsOtherOriginsBeforeDial','unowned demo provider origin accepted','./internal/demo'),
 ('demo-ai-approval','internal/ai/service.go','if j.IsDemo {','if false {','TestAIAndAtomicFixturesAreMarkedValidatedInertAndPurgeable','synthetic AI plan lacked explicit refusal','./internal/demo'),
 ('demo-ai-queue','migrations/00024_demo_ai.sql',"CHECK(NOT is_demo OR status IN('review','completed','rejected','canceled'))",'CHECK(true)','TestAIAndAtomicFixturesAreMarkedValidatedInertAndPurgeable','synthetic AI became queueable','./internal/demo'),
 ('advice-renderer','internal/server/server.go','Svc: AIService(o), Reports: ReportService(o)','Svc: AIService(o)','TestAIAdvisoryPDFUsesConfiguredRemoteRenderer','remote AI renderer not used','./internal/server'),
]

baseline=subprocess.run(['go','test','./internal/demo','./internal/server','-run','^Test(LocalFakeAIRejectsOtherOriginsBeforeDial|AIAndAtomicFixturesAreMarkedValidatedInertAndPurgeable|AIAdvisoryPDFUsesConfiguredRemoteRenderer)$','-count=1'],cwd=r,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=90)
(out/'baseline.log').write_text(baseline.stdout)
if baseline.returncode:raise RuntimeError('Baseline verification failed')
results=[]
for name,file,old,new,test,expected,package in cases:
 p=r/file;original=p.read_text();assert original.count(old)==1,(name,original.count(old))
 try:
  p.write_text(original.replace(old,new,1))
  run=subprocess.run(['go','test',package,'-run','^'+test+'$','-count=1'],cwd=r,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=90)
  (out/(name+'.log')).write_text(run.stdout)
  if run.returncode==0 or expected not in run.stdout:raise RuntimeError('Mutation not killed for intended reason: '+name)
  results.append({'name':name,'killed':True});print(name+': killed',flush=True)
 finally:p.write_text(original)
run=subprocess.run(['go','test','./internal/demo','./internal/server','-run','^Test(LocalFakeAIRejectsOtherOriginsBeforeDial|AIAndAtomicFixturesAreMarkedValidatedInertAndPurgeable|AIAdvisoryPDFUsesConfiguredRemoteRenderer)$','-count=1'],cwd=r,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=90)
(out/'restored.log').write_text(run.stdout)
if run.returncode:raise RuntimeError('Restored verification failed')
(out/'results.json').write_text(json.dumps(results,indent=2));print('Restored source passed; evidence '+str(out),flush=True)
