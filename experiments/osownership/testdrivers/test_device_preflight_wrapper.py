import ast
import unittest
from pathlib import Path
import device_preflight_wrapper as w

class WrapperOffline(unittest.TestCase):
    def test_exact_lifetime_not_name(self):
        r={'pid':17,'tgid':17,'exe':'/fixed','start_ticks':77}
        self.assertTrue(w.same_lifetime(r,dict(r)))
        for k in r:
            q=dict(r);q[k]='different';self.assertFalse(w.same_lifetime(r,q))
        self.assertFalse(w.same_lifetime(r,{'pid':17,'observation_error':'gone'}))
    def test_no_top_level_launch(self):
        tree=ast.parse(Path(w.__file__).read_text())
        for n in tree.body:
            self.assertNotIsInstance(n,ast.Expr) if not(isinstance(n,ast.Expr) and isinstance(n.value,ast.Constant)) else None
    def test_fixed_scope_source(self):
        s=Path(w.__file__).read_text()
        for part in ['--device-open-preflight','WNOHANG | os.WNOWAIT','driver_reaped','pending_custody','checked_kill(driver, signal.SIGKILL)','implicit_effects_inventory']:
            self.assertIn(part,s)
        self.assertNotIn('killpg',s)
        self.assertNotIn('--bootstrap-readonly',s)
if __name__=='__main__': unittest.main()

class SignalIsolated(unittest.TestCase):
    def setUp(self):
        from unittest.mock import patch
        self.kill_patch=patch.object(w.os,'kill')
        self.mock_kill=self.kill_patch.start()
        self.addCleanup(self.kill_patch.stop)

class SimulatedRun(SignalIsolated):
    def test_complete_mock_launch_no_process(self):
        import tempfile
        import hashlib
        from unittest.mock import patch, MagicMock
        with tempfile.TemporaryDirectory() as td:
            b=Path(td)/'binary';b.write_bytes(b'fixture')
            digest=hashlib.sha256(b'fixture').hexdigest()
            ident={'pid':17,'tgid':17,'exe':str(b),'start_ticks':77}
            p=MagicMock();p.pid=17;p.wait.return_value=0
            real_read=Path.read_bytes
            def contents(path):
                if str(path)=='/proc/17/exe':return b'fixture'
                return real_read(path)
            with patch.object(w.os,'getuid',return_value=1000),patch.object(w,'namespace',return_value={'device':7,'inode':1}),patch.object(w.subprocess,'Popen',return_value=p) as launch,patch.object(w,'identity',return_value=ident),patch.object(w,'snapshot',return_value={'records':[ident]}),patch.object(w.os,'waitid',return_value=object()),patch.object(Path,'read_bytes',contents):
                m=w.run_once(b,digest,Path(td)/'output')
            self.mock_kill.assert_not_called()
            self.assertEqual(m['invocations'],1);self.assertTrue(m['driver_reaped'])
            self.assertEqual(launch.call_args.args[0],[str(b),'--device-open-preflight'])
            self.assertEqual(m['classification'],'unreviewed-output')
            self.assertNotIn('same process-local pidfd feature check before real client',m['implicit_effects_inventory']['resident'])
    def test_signal_refuses_changed_identity(self):
        from unittest.mock import patch
        r={'pid':17,'tgid':17,'exe':'/fixed','start_ticks':77}
        with patch.object(w,'identity',return_value={**r,'start_ticks':78}),patch.object(w.os,'kill') as k:
            self.assertFalse(w.checked_kill(r,9));k.assert_not_called()

    def run_timeout_fixture(self, changed=False, unknown_reap=False, bad_hash=False):
        import tempfile,hashlib
        from unittest.mock import patch,MagicMock
        with tempfile.TemporaryDirectory() as td:
            b=Path(td)/'binary';b.write_bytes(b'fixture');digest=hashlib.sha256(b'fixture').hexdigest()
            old={'pid':17,'tgid':17,'exe':str(b),'start_ticks':77}
            now={**old,'start_ticks':78} if changed else old
            p=MagicMock();p.pid=17
            if unknown_reap:p.wait.side_effect=w.subprocess.TimeoutExpired('fixed',2)
            else:p.wait.return_value=-9
            real_read=Path.read_bytes
            def contents(path):
                if str(path)=='/proc/17/exe':return b'wrong' if bad_hash else b'fixture'
                return real_read(path)
            with patch.object(w.os,'getuid',return_value=1000),patch.object(w,'namespace',return_value={'device':7,'inode':1}),patch.object(w.subprocess,'Popen',return_value=p) as launch,patch.object(w,'identity',side_effect=[old,now,now]),patch.object(w,'snapshot',return_value={'records':[old]}) as snap,patch.object(w.os,'waitid',return_value=None),patch.object(w.time,'monotonic',side_effect=[0,11]),patch.object(Path,'read_bytes',contents):
                result=w.run_once(b,digest,Path(td)/'out')
            launch.assert_called_once();snap.assert_called_once();p.wait.assert_called_once_with(timeout=2)
            return result
    def test_timeout_exact_owned_kill_only_mocked(self):
        m=self.run_timeout_fixture();self.mock_kill.assert_called_once_with(17,w.signal.SIGKILL)
        self.assertTrue(m['driver_kill_sent']);self.assertTrue(m['driver_reaped'])
    def test_timeout_identity_mismatch_never_signals(self):
        m=self.run_timeout_fixture(changed=True);self.mock_kill.assert_not_called();self.assertFalse(m['driver_kill_sent'])
    def test_timeout_reap_unknown_preserves_pending(self):
        m=self.run_timeout_fixture(unknown_reap=True);self.mock_kill.assert_called_once_with(17,w.signal.SIGKILL)
        self.assertFalse(m['driver_reaped']);self.assertTrue(m['pending_custody'])
    def test_postlaunch_hash_mismatch_only_mocked_kill(self):
        m=self.run_timeout_fixture(bad_hash=True);self.mock_kill.assert_called_once_with(17,w.signal.SIGKILL)
        self.assertFalse(m['driver_exe_sha_verified']);self.assertTrue(m['timeout_or_identity_failure'])

class RecoveredLifetimeOffline(unittest.TestCase):
    def test_recover_unsampled_resident_and_unknown(self):
        import tempfile,json
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as td:
            root=Path(td);(root/'stdout.json').write_text(json.dumps({'verdict':{'lifecycle':{'pid':938,'start_ticks':325}}}))
            records={};self.assertEqual(w.recover_verdict_lifetimes(root,records),'recovered')
            self.assertIn((938,325),records)
            with patch.object(w,'identity',return_value={'pid':938,'observation_error':'unreadable-or-gone'}):
                r=w.lifetime_postcheck(records[(938,325)]);self.assertIsNone(r['same_lifetime']);self.assertEqual(r['state'],'unreadable-unknown')
    def test_survivor_absence_and_reuse(self):
        from unittest.mock import patch
        old={'pid':17,'start_ticks':77,'source':'bounded-owner-verdict'}
        for current,state,same in [({'pid':17,'start_ticks':77},'survivor',True),({'pid':17,'start_ticks':78},'different-lifetime',False),({'pid':17,'observation_error':'not-found'},'absent-at-read',False),({'pid':17,'observation_error':'partial-read-process-present'},'unreadable-unknown',None)]:
            with patch.object(w,'identity',return_value=current):
                r=w.lifetime_postcheck(old);self.assertEqual(r['state'],state);self.assertIs(r['same_lifetime'],same)

    def test_duplicate_keys_refuse_no_recovery(self):
        import tempfile
        with tempfile.TemporaryDirectory() as td:
            root=Path(td);(root/'stdout.json').write_text('{"verdict":{"lifecycle":{"pid":17,"pid":18,"start_ticks":77}}}')
            records={};self.assertEqual(w.recover_verdict_lifetimes(root,records),'unavailable-or-invalid');self.assertEqual(records,{})
    def test_overflow_refuse_no_recovery(self):
        import tempfile
        with tempfile.TemporaryDirectory() as td:
            root=Path(td);(root/'stdout.json').write_bytes(b' '*131073)
            records={};self.assertEqual(w.recover_verdict_lifetimes(root,records),'output-overflow');self.assertEqual(records,{})
