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
