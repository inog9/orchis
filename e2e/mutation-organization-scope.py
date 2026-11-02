#!/usr/bin/env python3
"""Mutasi scope organisasi dan izin penambahan aset pada salinan sumber."""
from pathlib import Path
import json,shutil,subprocess,time
source=Path(__file__).resolve().parents[1];out=source/'work'/('org-scope-mutations-'+str(time.time_ns()));repo=out/'src';repo.mkdir(parents=True)
for name in ['internal','migrations','docs']:shutil.copytree(source/name,repo/name)
for name in ['go.mod','go.sum']:shutil.copy2(source/name,repo/name)
def run(label,pkg,test):
 r=subprocess.run(['go','test',pkg,'-run',test,'-count=1'],cwd=repo,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=120);(out/(label+'.log')).write_text(r.stdout);return r
cases=[
 ('finding-org','internal/execution/findings.go','if f.OrganizationID != nil {','if false && f.OrganizationID != nil {','./internal/server','^TestOrganizationFiltersScopePaginationSearchAndUntrustedIndexResults$','finding escaped organisation'),
 ('alias-conflict','internal/httpx/organization.go','value != legacy {','false && value != legacy {','./internal/httpx','^TestOrganizationParamRejectsConflictingScopeAndInvalidIdentifiers$','scope'),
 ('identifier','internal/httpx/organization.go','err != nil {','false && err != nil {','./internal/httpx','^TestOrganizationParamRejectsConflictingScopeAndInvalidIdentifiers$','scope'),
 ('assignment-permission','internal/assets/service.go','in.OrganizationID != nil && !authz.Has(p.Permissions, authz.OrganizationWrite)','false && in.OrganizationID != nil && !authz.Has(p.Permissions, authz.OrganizationWrite)','./internal/server','^TestAssetCreateOrganisationRequiresWriteAndSameTenantAndRejectsDemo$','assignment without organisation.write'),
 ('demo-assignment','internal/assets/service.go','if demo {','if false && demo {','./internal/server','^TestAssetCreateOrganisationRequiresWriteAndSameTenantAndRejectsDemo$','demo org assignment'),
]
results=[]
for label,name,old,new,pkg,test,expected in cases:
 assert run(label+'-baseline',pkg,test).returncode==0,'baseline failed: '+label
 path=repo/name;original=path.read_text();assert original.count(old)==1,(label,original.count(old))
 try:
  path.write_text(original.replace(old,new));r=run(label,pkg,test)
  assert r.returncode!=0 and expected in r.stdout,'mutant survived or failed for another reason: '+label
  print(label+': killed',flush=True);results.append({'name':label,'killed':True})
 finally:path.write_text(original)
 assert run(label+'-restored',pkg,test).returncode==0,'restored source failed: '+label
(out/'results.json').write_text(json.dumps(results,indent=2));print('All mutants killed and source restored; evidence '+str(out),flush=True)
