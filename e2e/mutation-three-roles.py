from pathlib import Path
import hashlib,json,shutil,subprocess,sys,tempfile
root=Path(__file__).resolve().parents[1]
(root/'work/final').mkdir(parents=True,exist_ok=True)
copy=Path(tempfile.mkdtemp(prefix='role-mutants-',dir=root/'work/final'))
for n in ['internal','migrations']:shutil.copytree(root/n,copy/n)
for n in ['go.mod','go.sum']:shutil.copy2(root/n,copy/n)
original={str(p.relative_to(copy)):p.read_bytes() for p in copy.rglob('*') if p.is_file()}
source_hash={n:hashlib.sha256((root/n).read_bytes()).hexdigest() for n in original}
unit='TestThreeRolesHaveProductPermissions';upgrade='TestThreeRolesUpgradeNeverWidensLegacyAccounts';matrix='TestThreeRolePolicyEveryAPIEndpoint'
M=[
 ('bootstrap-admin-status','internal/bootstrap/bootstrap.go'," AND u.status='active'","",'TestBootstrapOnlyRecognizesActiveRealHumanAdministrator','inactive administrator was accepted'),
 ('bootstrap-admin-demo','internal/bootstrap/bootstrap.go'," AND NOT u.is_demo","",'TestBootstrapOnlyRecognizesActiveRealHumanAdministrator','demo administrator prevented fresh human bootstrap'),
 ('bootstrap-admin-human','internal/bootstrap/bootstrap.go'," AND u.kind='human'","",'TestBootstrapOnlyRecognizesActiveRealHumanAdministrator','service administrator prevented fresh human bootstrap'),
 ('viewer-wide','internal/authz/authz.go','Permissions: []string{FindingRead, ReportRead}','Permissions: []string{FindingRead, ReportRead, AssetRead}',unit,'Viewer must read findings and reports only'),
 ('analyst-authorizes','internal/authz/authz.go','var analystExtra = []string{','var analystExtra = []string{AssetAuthorize, ',unit,'Analyst must not grant asset.authorize'),
 ('migration-subset-direction','migrations/00030_three_roles.sql',"old.permissions @> (SELECT permissions FROM canonical_roles WHERE key='admin')","old.permissions <@ (SELECT permissions FROM canonical_roles WHERE key='admin')",upgrade,'mapping'),
 ('legacy-report-effective','migrations/00030_three_roles.sql',"ELSE array_remove(permissions,'report.read')","ELSE permissions",upgrade,'mapping raw-report-not-effective'),
 ('session-revocation','migrations/00030_three_roles.sql',"UPDATE sessions s SET revoked_at=now() FROM users u WHERE s.tenant_id=u.tenant_id AND s.user_id=u.id AND u.role_review_required AND s.revoked_at IS NULL;","",upgrade,'sessionRevoked=false'),
 ('token-revocation','migrations/00030_three_roles.sql',"UPDATE api_tokens s SET revoked_at=now() FROM users u WHERE s.tenant_id=u.tenant_id AND s.user_id=u.id AND u.role_review_required AND s.revoked_at IS NULL;","",upgrade,'tokenRevoked=false'),
 ('viewer-db-cap','migrations/00030_three_roles.sql',"ALTER TABLE roles ADD CONSTRAINT roles_viewer_read_only CHECK(key<>'viewer' OR permissions <@ ARRAY['finding.read','report.read']::text[]);","",upgrade,'unsafe change accepted'),
 ('review-db-status','migrations/00030_three_roles.sql',"ALTER TABLE users ADD CONSTRAINT users_role_review_disabled CHECK(NOT role_review_required OR status='disabled');","",upgrade,'unsafe change accepted'),
 ('three-role-db-keys','migrations/00030_three_roles.sql',"ALTER TABLE roles ADD CONSTRAINT roles_three_keys CHECK(key IN ('admin','analyst','viewer'));","",upgrade,'unsafe change accepted'),
 ('review-explicit-ack','internal/users/users.go','reviewRequired && newStatus == "active" && !in.AcceptRolePermissions','reviewRequired && newStatus == "active" && false','TestMigratedAccountActivationRequiresPermissionReview','unreviewed activation'),
 ('bootstrap-existing-human','internal/bootstrap/bootstrap.go','if existing > 0 {','if existing < 0 {','TestBootstrapDoesNotReplaceExistingUnprivilegedHuman','bootstrap silently created replacement'),
 ('home-read','internal/home/home.go','auth.Require(authz.OrganizationRead,','auth.Require("",',matrix,'viewer forbidden GET /api/v1/home'),
 ('demo-read','internal/demo/http.go','auth.Require(authz.OrganizationRead,','auth.Require("",',matrix,'viewer forbidden GET /api/v1/demo/status'),
]
if len(sys.argv)>1:
 requested=set(sys.argv[1:]);known={m[0] for m in M}
 assert requested<=known, 'unknown mutant name'
 M=[m for m in M if m[0] in requested]
results=[]
def run(args,log):
 p=subprocess.run(args,cwd=copy,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
 (copy/log).write_text(p.stdout);return p
baseline=run(['go','test','-count=1','./internal/authz','./internal/server','-run','TestThreeRole|TestMigratedAccount|TestBootstrap'],'baseline.log')
assert baseline.returncode==0,baseline.stdout
try:
 for name,file,old,new,test,reason in M:
  path=copy/file;s=original[file].decode();assert s.count(old)==1,(name,s.count(old));path.write_text(s.replace(old,new))
  build=run(['go','test','./internal/authz','./internal/server','-run','^$'],name+'-build.log')
  if build.returncode!=0:raise RuntimeError(name+' build failed; NOT a killed mutant')
  package='./internal/authz' if test==unit else './internal/server'
  result=run(['go','test','-count=1',package,'-run','^'+test+'$'],name+'.log')
  killed=result.returncode!=0 and reason in result.stdout and '[build failed]' not in result.stdout
  results.append({'mutant':name,'build_passed':True,'killed':killed,'expected_assertion':reason})
  path.write_bytes(original[file]);print(name,'KILLED' if killed else 'FAILED',flush=True)
  if not killed:raise RuntimeError(name+' survived or failed for unrelated cause')
finally:
 for n,b in original.items():(copy/n).write_bytes(b)
 unchanged=all(hashlib.sha256((root/n).read_bytes()).hexdigest()==h for n,h in source_hash.items())
 (copy/'result.json').write_text(json.dumps({'mutants':results,'source_unchanged':unchanged},indent=2)+'\n')
 assert unchanged,'actual source changed during mutation run'
restored=run(['go','test','-count=1','./internal/authz','./internal/server','-run','TestThreeRole|TestMigratedAccount|TestBootstrap'],'restored.log')
assert restored.returncode==0,restored.stdout
print('Restored copy baseline PASS; actual source unchanged',flush=True)
