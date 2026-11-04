#!/usr/bin/env python3
"""Uji CAS pilihan koleksi di salinan sumber sekali pakai; gunakan vhost RabbitMQ terisolasi."""
from pathlib import Path
import subprocess,re,json,sys,shutil,time
source=Path(__file__).resolve().parents[1]
evidence=source/'work'/('collection-cas-mutations-'+str(time.time_ns()));root=evidence/'src';root.mkdir(parents=True)
for folder in ['internal','migrations','docs']:shutil.copytree(source/folder,root/folder)
for name in ['go.mod','go.sum']:shutil.copy2(source/name,root/name)
baseline=subprocess.run(['go','test','-count=1','./internal/server','-run','TestOsquery(SettingCAS|PackCAS)'],cwd=root,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
(evidence/'baseline.log').write_text(baseline.stdout)
assert baseline.returncode==0,'baseline failed'
endpoint=root/'internal/endpoint/endpoint.go';guard=root/'internal/endpoint/settings_version.go'
original={p:p.read_text() for p in (endpoint,guard)}
mutants=[
('single-stale-check',endpoint,r'\t\tif err := checkSettingVersions\(ctx, tx, p.TenantID, org, \[\]string\{key\}, map\[string\]int64\{key: \*expected\}\); err != nil \{\n\t\t\treturn err\n\t\t\}', '', 'stale update accepted'),
('pack-stale-check',endpoint,r'\t\tif err := checkSettingVersions\(ctx, tx, p.TenantID, org, keys, expected\); err != nil \{\n\t\t\treturn err\n\t\t\}', '', 'stale pack accepted'),
('missing-single-version',endpoint,r'\tif expected == nil \|\| \*expected < 0 \{\n\t\treturn httpx.Validation\(httpx.FieldError\{Name: "version", Message: "Reload the collection choice before changing it."\}\)\n\t\}', '\tif expected == nil { v := int64(0); expected = &v }', 'missing/negative version accepted'),
('pack-version-count',guard,r'if len\(expected\) != len\(keys\)', 'if len(expected) < 0', 'invalid extra versions accepted'),
('pack-version-keys',guard,r'!known \|\| version < 0','!known || version < -1', 'invalid negative versions accepted')]
results=[]
try:
 for name,path,pattern,replacement,reason in mutants:
  value,n=re.subn(pattern,replacement,original[path]);assert n==1,(name,n)
  path.write_text(value)
  out=subprocess.run(['go','test','-count=1','./internal/server','-run','TestOsquery(SettingCAS|PackCAS)'],cwd=root,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (evidence/('cas-mutant-'+name+'.log')).write_text(out.stdout)
  killed=out.returncode!=0 and reason in out.stdout and '[build failed]' not in out.stdout
  results.append({'mutant':name,'killed':killed,'exit':out.returncode,'assertion':reason})
  path.write_text(original[path])
  print(name, 'KILLED' if killed else 'FAILED',flush=True)
  if not killed:sys.exit(1)
finally:
 for path,value in original.items():path.write_text(value)
 (evidence/'result.json').write_text(json.dumps(results,indent=2)+'\n')

restored=subprocess.run(['go','test','-count=1','./internal/server','-run','TestOsquery(SettingCAS|PackCAS)'],cwd=root,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
(evidence/'restored.log').write_text(restored.stdout)
assert restored.returncode==0,'restored tests failed'
print('Collection CAS mutants killed/restored: '+str(evidence),flush=True)
