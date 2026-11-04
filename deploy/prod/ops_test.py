import json,os,tempfile,types,unittest
from pathlib import Path
from unittest import mock
import ops
from backup import record_status

@unittest.skipUnless(os.geteuid()==0,'Production setup assigns container UIDs as root')
class MetricsSetup(unittest.TestCase):
 def test_upgrade_preserves_credentials_and_is_idempotent(self):
  with tempfile.TemporaryDirectory() as directory:
   state=Path(directory);args=types.SimpleNamespace(state=state)
   for name in ['server.json','nmap.json','nuclei.json','ai.json']:ops.private(state/name,{'ORCHIS_EXISTING_SECRET':'unchanged'})
   ops.metrics_init(args);token=(state/'metrics_token').read_bytes();password=(state/'grafana_password').read_bytes()
   ops.metrics_init(args)
   self.assertEqual((state/'metrics_token').read_bytes(),token);self.assertEqual((state/'grafana_password').read_bytes(),password)
   for name in ['server.json','nmap.json','nuclei.json','ai.json']:
    path=state/name;data=json.loads(path.read_text());self.assertEqual(data['ORCHIS_EXISTING_SECRET'],'unchanged');self.assertEqual(data['ORCHIS_METRICS_TOKEN'],token.decode().strip());self.assertEqual(path.stat().st_mode&0o777,0o600);self.assertEqual(path.stat().st_uid,10001)
   self.assertEqual((state/'metrics_token').stat().st_uid,65534);self.assertEqual((state/'grafana_password').stat().st_uid,472)
 def test_symlink_token_is_rejected(self):
  with tempfile.TemporaryDirectory() as directory:
   state=Path(directory);target=state/'unrelated';ops.private(target,'x'*43,0);(state/'metrics_token').symlink_to(target)
   with self.assertRaises(RuntimeError):ops.metrics_init(types.SimpleNamespace(state=state))
   self.assertEqual(target.read_text().strip(),'x'*43)
 def test_status_contains_only_safe_fields_and_atomic_replacement(self):
  with tempfile.TemporaryDirectory() as directory:
   state=Path(directory);args=types.SimpleNamespace(state=state)
   record_status(args,False);old=(state/'ops-status/backup.json').stat().st_ino
   record_status(args,True);path=state/'ops-status/backup.json';data=json.loads(path.read_text())
   self.assertTrue(data['success']);self.assertNotEqual(old,path.stat().st_ino);self.assertEqual(set(data),{'success','timestamp','disk_free_bytes','disk_total_bytes'});self.assertEqual(path.stat().st_mode&0o777,0o644)

@unittest.skipUnless(os.geteuid()==0,'Production setup assigns container UIDs as root')
class GeneratedSecrets(unittest.TestCase):
 def test_fresh_setups_have_independent_private_credentials_and_never_replace_them(self):
  import contextlib,io,hashlib
  with tempfile.TemporaryDirectory() as directory:
   states=[]
   with mock.patch('ops.subprocess.check_output',return_value=''),contextlib.redirect_stdout(io.StringIO()):
    for name in ['first','second']:
     state=Path(directory)/name
     args=types.SimpleNamespace(state=state,project='orchis-test-'+name,port=18443,email='operator@example.invalid',command='init')
     ops.init(args)
     states.append(state)
     before=hashlib.sha256((state/'server.json').read_bytes()).digest()
     with self.assertRaises(SystemExit):ops.init(args)
     self.assertEqual(before,hashlib.sha256((state/'server.json').read_bytes()).digest())
   all_values=[]
   for state in states:
    server=json.loads((state/'server.json').read_text());initial=json.loads((state/'initial-admin.json').read_text())
    self.assertEqual(server['ORCHIS_ADMIN_PASSWORD'],initial['password'])
    self.assertGreaterEqual(len(initial['password']),32)
    self.assertNotEqual(initial['password'],'Orchis-Dev-Pass-1')
    for name in ['postgres_password','owner_password','app_password','maintenance_password','s3_password','renderer_token','ai-master.key','metrics_token','grafana_password','backup.key']:
     path=state/name
     self.assertEqual(path.stat().st_mode&0o777,0o600)
     value=path.read_text().strip()
     self.assertGreaterEqual(len(value),32)
     all_values.append(value)
    all_values.append(initial['password'])
    self.assertEqual((state/'initial-admin.json').stat().st_mode&0o777,0o600)
    self.assertEqual((state/'server.json').stat().st_mode&0o777,0o600)
   self.assertEqual(len(all_values),len(set(all_values)))

@unittest.skipUnless(os.geteuid()==0,'Production setup assigns container UIDs as root')
class AgentTrustSetup(unittest.TestCase):
 def test_fingerprint_comes_from_selected_caddy_and_preserves_private_config(self):
  import hashlib
  with tempfile.TemporaryDirectory() as directory:
   state=Path(directory);args=types.SimpleNamespace(state=state)
   ops.private(state/'server.json',{'existing':'kept','ORCHIS_AGENT_TLS_PIN':'old pin'})
   with mock.patch.object(ops,'compose',return_value=types.SimpleNamespace(stdout='certificate')) as compose, mock.patch.object(ops.ssl,'PEM_cert_to_DER_cert',return_value=b'CA DER'):
    ops.agent_trust(args)
   compose.assert_called_once_with(args,'exec','-T','caddy','cat','/data/caddy/pki/authorities/local/root.crt',capture=True)
   path=state/'server.json';value=json.loads(path.read_text())
   self.assertEqual(value,{'existing':'kept','ORCHIS_AGENT_CA_FINGERPRINT':hashlib.sha256(b'CA DER').hexdigest()})
   self.assertEqual(path.stat().st_mode&0o777,0o600);self.assertEqual(path.stat().st_uid,10001)
 def test_invalid_ca_leaves_configuration_unchanged(self):
  with tempfile.TemporaryDirectory() as directory:
   state=Path(directory);args=types.SimpleNamespace(state=state)
   ops.private(state/'server.json',{'existing':'kept'});before=(state/'server.json').read_bytes()
   with mock.patch.object(ops,'compose',return_value=types.SimpleNamespace(stdout='invalid')):
    with self.assertRaises(ValueError):ops.agent_trust(args)
   self.assertEqual((state/'server.json').read_bytes(),before)
