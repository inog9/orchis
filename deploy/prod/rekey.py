"""Fail-closed AI key rotation. Pending journal blocks startup/backup until roll-forward."""
import base64,hashlib,json,os,secrets
from pathlib import Path
from backup import container,assert_idle,wait_services
SERVICES=['server','nmap','nuclei','ai-worker','renderer','caddy']

def read_key(path):
 if path.is_symlink() or not path.is_file() or path.stat().st_mode&0o077 or path.stat().st_size>128:raise RuntimeError('AI key must be a private regular file')
 raw=path.read_bytes().strip()
 try:key=base64.b64decode(raw,validate=True)
 except ValueError:raise RuntimeError('Invalid AI master key') from None
 if len(key)!=32:raise RuntimeError('Invalid AI master key')
 return raw+b'\n'

def atomic(path,data,uid):
 temporary=path.with_name(path.name+'.'+secrets.token_hex(4)+'.partial')
 fd=os.open(temporary,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 try:
  with os.fdopen(fd,'wb') as out:out.write(data);out.flush();os.fsync(out.fileno())
  os.chown(temporary,uid,uid);os.replace(temporary,path)
  directory=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY)
  try:os.fsync(directory)
  finally:os.close(directory)
 finally:temporary.unlink(missing_ok=True)

def rekey(args,compose):
 if os.geteuid()!=0:raise RuntimeError('AI rotation requires the root operator')
 if args.key_file is None:raise RuntimeError('Select the new private base64 AI key with --key-file')
 new=read_key(args.key_file);fingerprint=hashlib.sha256(new).hexdigest()
 journal=args.state/'ai-rekey.json';next_key=args.state/'ai-master.next.key';master=args.state/'ai-master.key'
 resuming=journal.exists()
 if resuming:
  if journal.is_symlink() or journal.stat().st_mode&0o077 or journal.stat().st_size>4096:raise RuntimeError('Invalid AI rotation journal')
  state=json.loads(journal.read_text())
  if state.get('new_fingerprint')!=fingerprint or not isinstance(state.get('services'),list) or any(s not in SERVICES for s in state['services']):raise RuntimeError('Resume pending rotation with the same new key')
 else:
  assert_idle(args,compose)
  running=[s for s in SERVICES if container(args,compose,s)['State']['Running']]
  state={'new_fingerprint':fingerprint,'services':running}
  atomic(journal,(json.dumps(state)+'\n').encode(),0)
 atomic(next_key,new,10001)
 # On any failure leave writers stopped and journal intact. A retry can decrypt
 # rows under either key, but only installs the new key after the DB transaction.
 compose(args,'stop','--timeout','30',*SERVICES)
 try:assert_idle(args,compose)
 except BaseException:
  if not resuming:
   next_key.unlink(missing_ok=True);journal.unlink(missing_ok=True)
   if state['services']:compose(args,'up','-d','--no-deps','--force-recreate',*state['services'])
  raise
 old=read_key(master)
 if old!=new:atomic(args.state/'ai-master.previous.key',old,10001)
 result=json.loads(compose(args,'run','--rm','--no-deps','-e','ORCHIS_AI_REKEY_NEW_FILE=/run/secrets/ai-next.key','-v',str(next_key)+':/run/secrets/ai-next.key:ro','server','ai-rekey',capture=True).stdout)
 atomic(master,new,10001)
 next_key.unlink(missing_ok=True);journal.unlink()
 if state['services']:
  compose(args,'up','-d','--no-deps','--force-recreate',*state['services'])
  wait_services(args,compose,state['services'])
 print('AI key rotation committed; reencrypted policies: '+str(result['reencrypted_policies']))
 return result['reencrypted_policies']
