#!/usr/bin/env python3
"""Mutasi dokumen persetujuan, penghapusan dataset, dan kredensial produksi pada salinan sumber."""
from pathlib import Path
import json,shutil,subprocess,time
source=Path(__file__).resolve().parents[1]
out=source/'work'/('t5-consent-mutations-'+str(time.time_ns()));repo=out/'src';repo.mkdir(parents=True)
for name in ['internal','migrations','docs']:shutil.copytree(source/name,repo/name)
for name in ['go.mod','go.sum']:shutil.copy2(source/name,repo/name)

def run(label,package,test):
 r=subprocess.run(['go','test',package,'-run',test,'-count=1'],cwd=repo,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=120)
 (out/(label+'.log')).write_text(r.stdout);return r
cases=[
 ('document-version','internal/osquery/consent.go','c.Version == current.Version && c.SHA256 == current.SHA256','c.SHA256 == current.SHA256','./internal/osquery','^TestConsentDocumentMatchesPublishedTemplateAndRef$','referensi dokumen lama'),
 ('document-hash','internal/osquery/consent.go','c.Version == current.Version && c.SHA256 == current.SHA256','c.Version == current.Version','./internal/osquery','^TestConsentDocumentMatchesPublishedTemplateAndRef$','referensi dokumen lama'),
 ('legacy-consent','internal/execution/endpoint.go','($4 OR COALESCE((consent_version=$5 AND consent_sha256=$6 AND consented_by IS NOT NULL AND consented_at IS NOT NULL),false))','($4 OR ($5::text IS NOT NULL AND $6::text IS NOT NULL))','./internal/server','^TestOsqueryOldApprovalNeedsRenewal$','persetujuan lama diklaim'),
 ('delete-version','internal/endpoint/endpoint.go','if current != version {','if false && current != version {','./internal/server','^TestOsqueryDeleteIsVersionedAuditedScopedAndInvalidatesInflightResults$','versi salah'),
 ('delete-inflight','internal/endpoint/endpoint.go','UPDATE assets SET version=version+1,updated_at=now()','UPDATE assets SET version=version,updated_at=now()','./internal/server','^TestOsqueryDeleteIsVersionedAuditedScopedAndInvalidatesInflightResults$','penghapusan tidak membatalkan'),
 ('broker-credential','internal/config/config.go','if c.AMQPURL != "" {','if false && c.AMQPURL != "" {','./internal/config','^TestProductionRejectsDefaultServiceAndInitialAdminCredentials$','unsafe credential accepted'),
 ('store-credential','internal/config/config.go','if c.S3.Endpoint != "" &&','if false && c.S3.Endpoint != "" &&','./internal/config','^TestProductionRejectsDefaultServiceAndInitialAdminCredentials$','unsafe credential accepted'),
 ('renderer-credential','internal/config/config.go','if os.Getenv("ORCHIS_RENDERER_URL") != "" &&','if false && os.Getenv("ORCHIS_RENDERER_URL") != "" &&','./internal/config','^TestProductionRejectsDefaultServiceAndInitialAdminCredentials$','unsafe credential accepted'),
]
results=[]
for label,name,old,new,package,test,expected in cases:
 assert run(label+'-baseline',package,test).returncode==0,'baseline failed: '+label
 path=repo/name;original=path.read_text();count=original.count(old)
 assert count==(2 if label=='legacy-consent' else 1),(label,count)
 try:
  path.write_text(original.replace(old,new));r=run(label,package,test)
  assert r.returncode!=0 and expected in r.stdout,'mutation survived or failed for another reason: '+label
  print(label+': killed',flush=True);results.append({'name':label,'killed':True})
 finally:path.write_text(original)
 assert run(label+'-restored',package,test).returncode==0,'restored source failed: '+label
(out/'results.json').write_text(json.dumps(results,indent=2))
print('All mutants killed and source restored; evidence '+str(out),flush=True)
