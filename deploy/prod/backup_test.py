import io, os, tempfile, tarfile, types, unittest
from pathlib import Path
from unittest.mock import patch
import backup

class ArchiveSafety(unittest.TestCase):
    def test_active_or_paused_work_is_not_silently_lost_with_broker(self):
        with patch.object(backup,'pg',return_value='1'):
            with self.assertRaisesRegex(RuntimeError,'active scans'):backup.assert_idle(None,None)
        with patch.object(backup,'pg',return_value='0'):backup.assert_idle(None,None)

    def test_nonseekable_zip_stream_remains_readable(self):
        target=io.BytesIO()
        with backup.zipfile.ZipFile(backup.BoundedWriter(target,8192),'w') as archive:archive.writestr('proof.txt',b'local fixture')
        with backup.zipfile.ZipFile(io.BytesIO(target.getvalue())) as archive:self.assertEqual(archive.read('proof.txt'),b'local fixture')

    def test_streaming_backup_stops_at_disk_budget(self):
        target=io.BytesIO();writer=backup.BoundedWriter(target,5)
        writer.write(b'1234')
        with self.assertRaises(RuntimeError):writer.write(b'56')
        self.assertEqual(target.getvalue(),b'1234')

    def test_volume_archive_rejects_paths_links_and_limits(self):
        with tempfile.TemporaryDirectory() as folder:
            for name,kind,size,valid in [('normal/file',tarfile.REGTYPE,3,True),('../escape',tarfile.REGTYPE,0,False),('/absolute',tarfile.REGTYPE,0,False),('link',tarfile.SYMTYPE,0,False),('hard',tarfile.LNKTYPE,0,False),('large',tarfile.REGTYPE,10,False)]:
                with self.subTest(name=name):
                    path=Path(folder)/'volume.tar'
                    with tarfile.open(path,'w') as out:
                        item=tarfile.TarInfo(name);item.type=kind;item.size=size;item.linkname='../escape';out.addfile(item,io.BytesIO(b'x'*size) if kind==tarfile.REGTYPE else None)
                    if valid:backup.validate_tar(path,5)
                    else:
                        with self.assertRaises(RuntimeError):backup.validate_tar(path,5)
    def test_authentication_failure_never_creates_target_state(self):
        binary=Path(os.environ.get('ORCHIS_BACKUP_SEAL_BIN',Path(__file__).resolve().parents[2]/'bin/backup-seal'))
        if not binary.exists():self.fail('Build bin/backup-seal before running archive safety tests')
        with tempfile.TemporaryDirectory() as folder:
            root=Path(folder);cipher=root/'broken.age';cipher.write_bytes(b'age-encryption.org/v1\nmalformed encrypted archive')
            key=root/'backup.key';key.write_text('ab'*32);key.chmod(0o600)
            state=root/'new-state';args=types.SimpleNamespace(state=state,project='fixture-restored',archive=cipher,key_file=key,max_bytes=1<<20)
            init_calls=[]
            with patch.object(backup.subprocess,'check_output',return_value=b''),patch.object(backup,'seal_command',return_value=[str(binary),'-key-file',str(key),'-decrypt']):
                with self.assertRaisesRegex(RuntimeError,'authentication failed'):
                    backup.restore(args,None,lambda args:init_calls.append(args),None,None)
            self.assertFalse(state.exists());self.assertEqual(init_calls,[])
if __name__=='__main__':unittest.main()

class HackbotBackup(unittest.TestCase):
    def test_private_engine_archive_has_read_access_and_excludes_seeded_package_link(self):
        command=backup.volume_read('fixture-image','private-home','project',True,True)
        self.assertIn('DAC_READ_SEARCH',command)
        self.assertIn('--exclude=./.pi/agent/node_modules',command)
        self.assertIn('none',command)
        self.assertIn('--read-only',command)
