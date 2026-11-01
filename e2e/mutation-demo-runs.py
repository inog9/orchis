#!/usr/bin/env python3
# Runs only against an owned disposable source copy; no live API/provider calls.
from pathlib import Path
import subprocess,time,json,shutil
source=Path(__file__).resolve().parents[1]
out=source/'work'/('synthetic-mutations-'+str(time.time_ns()));out.mkdir(parents=True)
r=out/'src';r.mkdir()
for name in ['internal','migrations']:shutil.copytree(source/name,r/name)
for name in ['go.mod','go.sum']:shutil.copy2(source/name,r/name)
cases=[
 ('synthetic-dispatch','internal/simulation/engine.go','if synthetic || status != "running" {','if status != "running" {','TestSyntheticHistoryCannotDispatchAndIsNotMITREExecutionEvidence','synthetic dispatch queued'),
 ('synthetic-trained','internal/mitre/coverage.go',"WHERE e.tenant_id=$1 AND NOT e.is_demo AND e.state='ended'", "WHERE e.tenant_id=$1 AND e.state='ended'",'TestSyntheticHistoryCannotDispatchAndIsNotMITREExecutionEvidence','synthetic success counted as trained evidence'),
 ('snapshot-counts','internal/demo/snapshots.go','if previous != nil {','snap.BySeverity["critical"]++\n if previous != nil {','TestSyntheticSnapshotTotalsCoverageAndComparisonsAgree','totals differ'),
 ('synthetic-queue-constraint','migrations/00023_synthetic_runs.sql',"CHECK(NOT is_demo OR (state='ended' AND raw_output_key IS NULL))",'CHECK(true)','TestSyntheticHistoryCannotDispatchAndIsNotMITREExecutionEvidence','demo execution became dispatchable'),
]

baseline=subprocess.run(['go','test','./internal/demo','-run','^Test(SyntheticSnapshotTotalsCoverageAndComparisonsAgree|SyntheticHistoryCannotDispatchAndIsNotMITREExecutionEvidence)$','-count=1'],cwd=r,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=90)
(out/'baseline.log').write_text(baseline.stdout)
if baseline.returncode:raise RuntimeError('Baseline verification failed')
results=[]
for name,file,old,new,test,expected in cases:
 p=r/file;original=p.read_text();assert original.count(old)==1,(name,original.count(old))
 try:
  p.write_text(original.replace(old,new))
  run=subprocess.run(['go','test','./internal/demo','-run','^'+test+'$','-count=1'],cwd=r,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=90)
  (out/(name+'.log')).write_text(run.stdout)
  if run.returncode==0 or expected not in run.stdout:raise RuntimeError('Mutation not killed for intended reason: '+name)
  results.append({'name':name,'killed':True});print(name+': killed',flush=True)
 finally:p.write_text(original)
run=subprocess.run(['go','test','./internal/demo','-run','^Test(SyntheticSnapshotTotalsCoverageAndComparisonsAgree|SyntheticHistoryCannotDispatchAndIsNotMITREExecutionEvidence)$','-count=1'],cwd=r,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=90)
(out/'restored.log').write_text(run.stdout)
if run.returncode:raise RuntimeError('Restored verification failed')
(out/'results.json').write_text(json.dumps(results,indent=2));print('Restored source passed; evidence '+str(out),flush=True)
