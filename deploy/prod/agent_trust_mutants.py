from pathlib import Path
import shutil,subprocess,tempfile,py_compile
root=Path(__file__).resolve().parent;source=(root/'ops.py').read_text()
mutants=[('wrong CA fingerprint','fingerprint=hashlib.sha256(ssl.PEM_cert_to_DER_cert(pem)).hexdigest()',"fingerprint='0'*64"),('world-readable server configuration','private(temporary,value)\n        os.replace(temporary,path)','private(temporary,value)\n        os.chmod(temporary,0o644)\n        os.replace(temporary,path)')]
for name,before,after in mutants:
 assert source.count(before)==1,(name,source.count(before))
 with tempfile.TemporaryDirectory(prefix='orchis-trust-mutant-') as d:
  dest=Path(d)
  for p in root.glob('*.py'):shutil.copy2(p,dest/p.name)
  (dest/'ops.py').write_text(source.replace(before,after));py_compile.compile(str(dest/'ops.py'),doraise=True)
  result=subprocess.run(['python3','-m','unittest','ops_test.AgentTrustSetup'],cwd=dest,capture_output=True,text=True)
  assert result.returncode!=0 and 'FAIL:' in result.stderr and 'ImportError' not in result.stderr,(name,result.stderr)
  print('BUILT AND KILLED: '+name)
assert source==(root/'ops.py').read_text()
print('Source unchanged; 2/2 mutants built and killed')
