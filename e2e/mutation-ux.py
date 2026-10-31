#!/usr/bin/env python3
"""Mutasi logika UX di salinan terisolasi; checkout aktif tidak diubah."""
from pathlib import Path
import shutil, subprocess, tempfile
root=Path(__file__).resolve().parents[1]
sandbox=Path(tempfile.mkdtemp(prefix="ux-mutation-",dir=root/"work"))
web=sandbox/"web"
shutil.copytree(root/"web",web,ignore=shutil.ignore_patterns("node_modules","dist"))
(web/"node_modules").symlink_to(root/"web/node_modules",target_is_directory=True)
cmd=["npm","test","--","src/components/UX.test.tsx","src/components/AssetFindings.test.tsx"]
def run(name,success):
 result=subprocess.run(cmd,cwd=web,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
 (sandbox/(name+".log")).write_text(result.stdout)
 if (result.returncode==0)!=success:raise SystemExit("Unexpected mutation result: "+name+"; "+str(sandbox))
 if not success and ("AssertionError" not in result.stdout or "FAIL" not in result.stdout):raise SystemExit("Mutation did not reach an assertion: "+name)
 print("LULUS: "+name,flush=True)
run("baseline",True)
mutations=[
 ("asset-findings","src/components/AssetFindings.tsx","const items=q.data?.items??[];","const items=[];"),
 ("address-readiness","src/components/OrganisationSetup.tsx","v.authorized>0&&v.ready>0","v.authorized>0"),
 ("assessment-progress","src/components/OrganisationSetup.tsx","v.assessments>0","v.assets>0"),
 ("navigation-permission","src/components/navigation.ts","!n.perm || can(me, n.perm)","true"),
]
for name,file,old,new in mutations:
 path=web/file;original=path.read_text()
 if old not in original:raise SystemExit("Mutation anchor missing: "+name)
 try:
  path.write_text(original.replace(old,new,1));run(name,False)
 finally:path.write_text(original)
 run(name+"-restored",True)
print("Evidence: "+str(sandbox))
