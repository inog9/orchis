#!/usr/bin/env python3
"""Mutasi guard pada salinan modul, bukan checkout aktif atau layanan review."""
import pathlib, shutil, subprocess, time, json
root=pathlib.Path(__file__).resolve().parents[1]
evidence=root/'work'/('scan-scope-mutations-'+str(time.time_ns()))
module=evidence/'src';module.mkdir(parents=True)
for name in ['internal','migrations','docs']:
 shutil.copytree(root/name,module/name)
for name in ['go.mod','go.sum']:
 shutil.copy2(root/name,module/name)
cases=[
 ('target-substitution','internal/contract/scan_scope.go','target != s.Target','false','./internal/contract','TestScanScope','unsafe scan scope accepted'),
 ('testing-permission','internal/contract/scan_scope.go','!s.TestingAuthorized','false','./internal/contract','TestScanScope','unsafe scan scope accepted'),
 ('public-target','internal/contract/scan_scope.go','!a.IsPrivate() && !a.IsLoopback() && !s.InternetFacing','false','./internal/contract','TestScanScope','unsafe scan scope accepted'),
 ('prefix-size','internal/contract/scan_scope.go','bits > 12 || (1<<bits) > maxHosts','bits > 12','./internal/contract','TestScanScope','unsafe scan scope accepted'),
 ('network-membership','internal/contract/scan_scope.go','!a.IsValid() || !p.Contains(a)','!a.IsValid() || !p.IsValid()','./internal/contract','TestScanScope','unsafe scan scope accepted'),
 ('reported-network','internal/scanscope/ranges.go','source.Contains(p.Addr())','true','./internal/scanscope','TestReportedRanges','unreported network accepted'),
 ('source-consent','internal/scanscope/ranges.go','s.consent_version=$5 AND s.consent_sha256=$6','($5::text IS NOT NULL AND $6::text IS NOT NULL)','./internal/server','TestNetworkSource','unsafe network source accepted (consent)'),
 ('source-freshness','internal/scanscope/ranges.go',"d.last_attempt_at>now()-interval '24 hours'",'true','./internal/server','TestNetworkSource','unsafe network source accepted (freshness)'),
 ('source-testing','internal/scanscope/ranges.go','AND a.testing_authorized','AND true','./internal/server','TestNetworkSource','unsafe network source accepted (testing permission)'),
 ('source-revoked','internal/scanscope/ranges.go',"g.status<>'revoked'",'true','./internal/server','TestNetworkSource','unsafe network source accepted (revoked agent)'),
 ('admin-range-approval','internal/scans/ranges.go','authz.SettingsManage','authz.OrganizationRead','./internal/server','TestNetworkApprovalNeeds','range approval without settings.manage'),
 ('range-asset-approval','internal/scans/ranges.go','authz.AssetAuthorize','authz.OrganizationRead','./internal/server','TestNetworkApprovalNeeds','range approval without asset.authorize'),
 ('range-version','internal/scans/ranges.go','version != in.Version','false','./internal/server','TestNetworkRangeApproval','stale approval'),
 ('claim-live-scope','internal/execution/runtime.go','why == "" && sim != nil','false && sim != nil','./internal/server','TestNetworkRangeApproval','revoked range reached injector'),
 ('injector-scope','internal/injector/worker.go','spec.ScanScope != nil','false','./internal/injector','TestInjectorScanScope','injector accepted scope substitution'),
 ('result-live-scope','internal/execution/runtime.go','if sim != nil {\n\t\t\tvar target string','if false && sim != nil {\n\t\t\tvar target string','./internal/server','TestNetworkScanDiscards','revoked scan result was ingested'),
 ('parallel-limit','internal/scans/worker.go','available := s.slots() - active','available := 32','./internal/server','TestFirstClassScan','parallel reservations must be two'),
]
results=[]
def run(package,test,path):
 p=subprocess.run(['go','test',package,'-run',test,'-count=1'],cwd=module,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=180)
 path.write_text(p.stdout);return p
for name,file,old,new,package,test,marker in cases:
 target=module/file;original=target.read_text()
 if original.count(old)!=1:raise SystemExit(f'{name}: expected one mutation site, got {original.count(old)}')
 baseline=run(package,test,evidence/(name+'-baseline.log'))
 if baseline.returncode:raise SystemExit(f'{name}: baseline failed')
 try:
  target.write_text(original.replace(old,new,1));mutant=run(package,test,evidence/(name+'-mutant.log'))
 finally:target.write_text(original)
 if mutant.returncode==0 or marker not in mutant.stdout:raise SystemExit(f'{name}: survived or failed for unrelated reason')
 restored=run(package,test,evidence/(name+'-restored.log'))
 if restored.returncode:raise SystemExit(f'{name}: restored failed')
 results.append({'name':name,'killed':True,'restored':True});print(name+': killed/restored',flush=True)
(evidence/'result.json').write_text(json.dumps(results,indent=2)+'\n')
print('All scan mutants killed and restored; '+str(evidence),flush=True)

