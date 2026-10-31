import base64,json,os,tempfile,types,unittest
from pathlib import Path
from unittest import mock
import ops,rekey
from backup import backup

@unittest.skipUnless(os.geteuid()==0,'Production rotation assigns container UIDs as root')
class MasterRotation(unittest.TestCase):
 def setup_state(self,directory):
  state=Path(directory);old=base64.b64encode(b'o'*32)+b'\n';new=base64.b64encode(b'n'*32)+b'\n'
  rekey.atomic(state/'ai-master.key',old,10001);rekey.atomic(state/'selected-new.key',new,0)
  return types.SimpleNamespace(state=state,key_file=state/'selected-new.key'),old,new
 def test_committed_database_failure_rolls_forward_without_starting_wrong_key(self):
  with tempfile.TemporaryDirectory() as directory:
   args,old,new=self.setup_state(directory);calls=[];committed=False
   def compose(*values,**options):
    nonlocal committed
    calls.append(values[1:]);result=0 if committed else 2
    if 'run' in values:committed=True
    return types.SimpleNamespace(stdout=json.dumps({'reencrypted_policies':result}))
   real_atomic=rekey.atomic
   def fail_promotion(path,data,uid):
    if path.name=='ai-master.key':raise OSError('simulated filesystem failure after DB commit')
    return real_atomic(path,data,uid)
   with mock.patch.object(rekey,'container',return_value={'State':{'Running':True}}),mock.patch.object(rekey,'assert_idle'),mock.patch.object(rekey,'atomic',side_effect=fail_promotion):
    with self.assertRaises(OSError):rekey.rekey(args,compose)
   self.assertTrue(committed);self.assertTrue((args.state/'ai-rekey.json').exists());self.assertEqual((args.state/'ai-master.key').read_bytes(),old);self.assertFalse(any('up' in c for c in calls))
   with self.assertRaises(RuntimeError):ops.up(args)
   with self.assertRaises(RuntimeError):backup(args,compose)
   with mock.patch.object(rekey,'assert_idle'),mock.patch.object(rekey,'wait_services'):
    self.assertEqual(rekey.rekey(args,compose),0)
   self.assertEqual((args.state/'ai-master.key').read_bytes(),new);self.assertEqual((args.state/'ai-master.previous.key').read_bytes(),old);self.assertFalse((args.state/'ai-rekey.json').exists());self.assertTrue(any('up' in c for c in calls))
 def test_pending_rotation_refuses_another_new_key(self):
  with tempfile.TemporaryDirectory() as directory:
   args,old,new=self.setup_state(directory)
   rekey.atomic(args.state/'ai-rekey.json',b'{"new_fingerprint":"different","services":[]}',0)
   with self.assertRaises(RuntimeError):rekey.rekey(args,lambda *a,**k:self.fail('must not stop or mutate on different pending key'))
   self.assertEqual((args.state/'ai-master.key').read_bytes(),old)
 def test_active_work_is_rejected_before_stopping(self):
  with tempfile.TemporaryDirectory() as directory:
   args,_,_=self.setup_state(directory)
   with mock.patch.object(rekey,'assert_idle',side_effect=RuntimeError('active work')):
    with self.assertRaises(RuntimeError):rekey.rekey(args,lambda *a,**k:self.fail('must not stop active work'))
   self.assertFalse((args.state/'ai-rekey.json').exists())
