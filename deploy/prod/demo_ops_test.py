import sys,tempfile,unittest
from unittest import mock
import ops
class DemoOpsSurface(unittest.TestCase):
 def test_explicit_demo_commands_are_delegated_to_selected_project(self):
  for command in ['demo-seed','demo-purge','demo-status']:
   with self.subTest(command=command),tempfile.TemporaryDirectory() as state:
    with mock.patch.object(sys,'argv',['ops.py',command,'--state',state,'--demo-seed','7']),mock.patch.object(ops,'compose') as compose:
     ops.main()
    args=compose.call_args.args
    self.assertEqual(str(args[0].state),state)
    self.assertEqual(args[1:],('run','--rm','--no-deps','-e','DEMO_SEED=7','server',command))
 def test_bad_demo_seed_has_no_side_effect(self):
  with tempfile.TemporaryDirectory() as state:
   with mock.patch.object(sys,'argv',['ops.py','demo-seed','--state',state,'--demo-seed','1000001']),mock.patch.object(ops,'compose') as compose:
    with self.assertRaises(RuntimeError):ops.main()
    compose.assert_not_called()
