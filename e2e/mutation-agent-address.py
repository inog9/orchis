#!/usr/bin/env python3
"""Mutasi terisolasi: identitas agent bukan persetujuan target admin (Run, Claim, frozen chain, upgrade)."""
from pathlib import Path
import shutil,subprocess,time,json
root=Path(__file__).resolve().parents[1];evidence=root/'work'/('agent-address-mutations-'+str(time.time_ns()));module=evidence/'src';module.mkdir(parents=True)
for folder in ['internal','migrations','docs']:shutil.copytree(root/folder,module/folder)
for name in ['go.mod','go.sum']:shutil.copy2(root/name,module/name)
cases=[
 ('authorized-address-edit','internal/assets/service.go','if in.Addresses != nil && authorized && !authz.Has(p.Permissions, authz.AssetAuthorize) {','if false && in.Addresses != nil && authorized && !authz.Has(p.Permissions, authz.AssetAuthorize) {','TestAuthorizedAssetAddressChangeRequiresAuthorizer','non-authorizer replaced approved target'),
 ('enrollment-target','internal/agents/agents.go','// Hostname hanya identitas agent; alamat scan harus ditetapkan admin pada aset.', '''// Mutant: trust agent identity as scan address.
 if _,err:=tx.Exec(ctx,`INSERT INTO asset_addresses(id,tenant_id,asset_id,kind,value) VALUES($1,$2,$3,'hostname',$4)`,ids.New(),s.Tenant,assetID,in.Hostname);err!=nil{return err}''','TestEnrollmentTokenIsAdminOnlyValidatedAndSingleUse','aset dari agent:'),
 ('run-unconfirmed','internal/execution/service.go','AND scan_approved ORDER BY','ORDER BY','TestAgentAddressNeedsAdministratorConfirmationAtRunAndClaim','unconfirmed agent address allowed at Run'),
 ('claim-unconfirmed','internal/execution/runtime.go','if why == "" && !onAgent && (scanProof == nil || scanProof.CIDR == "") {','if false && why == "" && !onAgent && (scanProof == nil || scanProof.CIDR == "") {','TestAgentAddressNeedsAdministratorConfirmationAtRunAndClaim','unconfirmed frozen agent address allowed at Claim'),
 ('frozen-chain','internal/simulation/engine.go','else if proof == nil || proof.CIDR == "" {','else if false && (proof == nil || proof.CIDR == "") {','TestFrozenChainAgentAddressIsRecheckedBeforeNextStep','unconfirmed frozen chain target reached next step'),
 ('migration-preserves-admin','migrations/00029_confirmed_agent_addresses.sql','AND d.created_at=a.created_at','AND true','TestLegacyAutomaticAgentHostnameMigrationPreservesDataAndRevokesQueue','legacy auto=false admin edit=false'),
]
results=[]
def run(test,path):
 p=subprocess.run(['go','test','./internal/server','-run','^'+test+'$','-count=1'],cwd=module,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=240);path.write_text(p.stdout);return p
for name,file,old,new,test,marker in cases:
 target=module/file;original=target.read_text();assert original.count(old)==1,(name,original.count(old))
 assert run(test,evidence/(name+'-baseline.log')).returncode==0,name+': baseline failed'
 try:
  target.write_text(original.replace(old,new,1));mutant=run(test,evidence/(name+'-mutant.log'))
 finally:target.write_text(original)
 assert mutant.returncode and marker in mutant.stdout,name+': survived or unrelated failure\n'+mutant.stdout
 assert run(test,evidence/(name+'-restored.log')).returncode==0,name+': restore failed'
 results.append({'name':name,'killed':True,'restored':True});print(name+': killed/restored',flush=True)
(evidence/'result.json').write_text(json.dumps(results,indent=2)+'\n');print('Agent address mutants killed/restored: '+str(evidence),flush=True)
