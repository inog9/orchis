#!/usr/bin/env python3
"""Menguji consent, CAS, eligibility, due time dan pin platform pada modul terisolasi."""
import pathlib,shutil,subprocess,time,json
root=pathlib.Path(__file__).resolve().parents[1];evidence=root/'work'/('endpoint-collection-mutations-'+str(time.time_ns()));module=evidence/'src';module.mkdir(parents=True)
for name in ['internal','migrations','docs']:shutil.copytree(root/name,module/name)
for name in ['go.mod','go.sum']:shutil.copy2(root/name,module/name)
cases=[
 ('consent-document','internal/endpoint/collection.go','!in.Consent.Valid()','false','TestEndpointCollectionRequires','old automatic consent accepted'),
 ('concurrent-choice','internal/endpoint/collection.go','current != expected','current < 0 || expected < 0','TestEndpointCollectionRequires','stale collection CAS accepted'),
 ('scheduler-consent','internal/endpoint/collection.go','s.consent_version=$2 AND s.consent_sha256=$3','($2::text IS NOT NULL AND $3::text IS NOT NULL)','TestEndpointCollectionScheduler','unsafe automatic agent accepted (consent)'),
 ('scheduler-revoked','internal/endpoint/collection.go',"g.status='online'",'true','TestEndpointCollectionScheduler','eligible-only collection failed (revoked)'),
 ('scheduler-demo','internal/endpoint/collection.go','NOT g.is_demo','true','TestEndpointCollectionScheduler','eligible-only collection failed (demo)'),
 ('scheduler-platform','internal/endpoint/collection.go',"g.os='linux'",'true','TestEndpointCollectionScheduler','unsafe automatic agent accepted (platform)'),
 ('scheduler-architecture','internal/endpoint/collection.go',"g.arch='x86_64'",'true','TestEndpointCollectionScheduler','unsafe automatic agent accepted (architecture)'),
 ('scheduler-due','internal/endpoint/collection.go',"(d.last_enqueued_at IS NULL OR d.last_enqueued_at<=now()-make_interval(secs=>$4))",'($4::double precision>0)','TestEndpointCollectionRequires','collection ran before due time'),
 ('scheduler-busy','internal/endpoint/collection.go',"e.state IN ('ready','running')",'false','TestEndpointCollectionRequires','already queued or running'),
 ('claim-platform-pin','internal/execution/endpoint.go','!supported {','!supported && platform == "linux" {','TestOsqueryUnpinned','unpinned platform received osquery'),
]
results=[]
def run(test,path):
 p=subprocess.run(['go','test','./internal/server','-run',test,'-count=1'],cwd=module,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=180);path.write_text(p.stdout);return p
for name,file,old,new,test,marker in cases:
 target=module/file;original=target.read_text()
 if original.count(old)!=1:raise SystemExit(f'{name}: mutation sites {original.count(old)}')
 if run(test,evidence/(name+'-baseline.log')).returncode:raise SystemExit(name+': baseline failed')
 try:target.write_text(original.replace(old,new,1));mutant=run(test,evidence/(name+'-mutant.log'))
 finally:target.write_text(original)
 if not mutant.returncode or marker not in mutant.stdout:raise SystemExit(name+': survived or unrelated failure')
 if run(test,evidence/(name+'-restored.log')).returncode:raise SystemExit(name+': restored failed')
 results.append({'name':name,'killed':True,'restored':True});print(name+': killed/restored',flush=True)
(evidence/'result.json').write_text(json.dumps(results,indent=2)+'\n');print('All endpoint collection mutants killed and restored: '+str(evidence),flush=True)
