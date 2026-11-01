#!/usr/bin/env python3
"""Mutasi batas data osquery di salinan sumber; tidak mengubah checkout yang dipakai runtime."""
from pathlib import Path
import json
import shutil
import subprocess
import time

source=Path(__file__).resolve().parents[1]
out=source/'work'/('osquery-privacy-mutations-'+str(time.time_ns()))
repo=out/'src'
repo.mkdir(parents=True)
for name in ['internal','migrations']:
    shutil.copytree(source/name,repo/name)
for name in ['go.mod','go.sum']:
    shutil.copy2(source/name,repo/name)

def run(label,package,test):
    result=subprocess.run(['go','test',package,'-run',test,'-count=1'],cwd=repo,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=120)
    (out/(label+'.log')).write_text(result.stdout)
    return result

cases=[
 ('redaction','internal/osquery/data.go','s = authorizationRe.ReplaceAllString(s, "Authorization=[redacted]")','s = s // mutasi','./internal/osquery','^TestRedactCredentialForms$','redaksi gagal'),
 ('json-budget','internal/osquery/data.go','remaining := MaxDatasetBytes - len(metadata)','remaining := maxResult - len(metadata)','./internal/osquery','^TestSanitizeMetadataAndJSONBudget$','batas hasil gagal'),
 ('stderr-limit','internal/osquery/run.go','stderr := &capWriter{max: 64 << 10}','stderr := &capWriter{max: maxResult}','./internal/osquery','^TestRunnerBoundsAndRedactsDiagnostics$','batas stderr tidak dikenali'),
 ('pack-opt-in','internal/osquery/osquery.go','q.Pack == "" && q.Sensitivity == "low"','q.Sensitivity == "low"','./internal/osquery','^TestEveryPackIsOptInEvenWhenLowSensitivity$','pack tidak boleh aktif'),
 ('mapped-only','internal/execution/endpoint.go','res.Findings = nil','// mutasi: percaya temuan kiriman agent','./internal/server','^TestOsqueryResultFromAgentIsSanitizedStoredAndMapped$','temuan harus berasal'),
 ('early-root-skip','internal/agent/osquery_install.go','if q.NeedsRoot && os.Geteuid() != 0 {','if false {','./internal/agent','^TestOsqueryRootQuerySkipsBeforeInstalling$','TestOsqueryRootQuerySkipsBeforeInstalling'),
]
# Kanal findings dibuang di dua batas; mutasi keduanya menguji pertahanan secara bersamaan.
runtime=repo/'internal/execution/runtime.go'
runtime_original=runtime.read_text()
results=[]
for label,name,old,new,package,test,expected in cases:
    assert run(label+'-baseline',package,test).returncode==0,'baseline failed: '+label
    path=repo/name
    original=path.read_text()
    assert original.count(old)==1,(label,original.count(old))
    try:
        path.write_text(original.replace(old,new))
        if label=='mapped-only':
            runtime.write_text(runtime_original.replace('res.Output, res.Findings, res.Logs = "", nil, nil','res.Output, res.Logs = "", nil'))
        result=run(label,package,test)
        assert result.returncode!=0 and expected in result.stdout,'mutation survived or failed for another reason: '+label
        print(label+': killed',flush=True)
        results.append({'name':label,'killed':True})
    finally:
        path.write_text(original)
        runtime.write_text(runtime_original)
    assert run(label+'-restored',package,test).returncode==0,'restored source failed: '+label
(out/'results.json').write_text(json.dumps(results,indent=2))
print('All mutants killed and source restored; evidence '+str(out),flush=True)
