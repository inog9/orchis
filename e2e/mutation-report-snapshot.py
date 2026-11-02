#!/usr/bin/env python3
"""Buktikan snapshot beku dan penolakan bukti response mentah pada salinan modul."""
from pathlib import Path
import shutil,subprocess,time,json
root=Path(__file__).resolve().parents[1];evidence=root/'work'/('report-snapshot-mutations-'+str(time.time_ns()));module=evidence/'src';module.mkdir(parents=True)
for folder in ['internal','migrations','docs']:shutil.copytree(root/folder,module/folder)
for name in ['go.mod','go.sum']:shutil.copy2(root/name,module/name)
cases=[('immutable-snapshot','internal/reporting/scan.go','ON CONFLICT(tenant_id,assessment_id) DO NOTHING','ON CONFLICT(tenant_id,assessment_id) DO UPDATE SET snapshot=EXCLUDED.snapshot','TestFreezeScanSnapshotCannotOverwrite','existing scan snapshot was overwritten'),('evidence-whitelist','internal/reporting/export.go','[]string{"template_id",','[]string{"response", "template_id",','TestReportRedactionAndPrintEscaping','unexpected evidence')]
results=[]
def run(test,path):
 p=subprocess.run(['go','test','./internal/reporting','-run',test,'-count=1'],cwd=module,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=180);path.write_text(p.stdout);return p
for name,file,old,new,test,marker in cases:
 target=module/file;original=target.read_text();assert original.count(old)==1,(name,original.count(old))
 assert run(test,evidence/(name+'-baseline.log')).returncode==0,name+': baseline failed'
 try:target.write_text(original.replace(old,new,1));mutant=run(test,evidence/(name+'-mutant.log'))
 finally:target.write_text(original)
 assert mutant.returncode and marker in mutant.stdout,name+': survived or unrelated failure'
 assert run(test,evidence/(name+'-restored.log')).returncode==0,name+': restore failed'
 results.append({'name':name,'killed':True,'restored':True});print(name+': killed/restored',flush=True)
(evidence/'result.json').write_text(json.dumps(results,indent=2)+'\n');print('Report mutants killed/restored: '+str(evidence),flush=True)
