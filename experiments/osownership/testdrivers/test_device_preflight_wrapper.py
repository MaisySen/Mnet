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

class SimulatedRun(unittest.TestCase):
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
            self.assertEqual(m['invocations'],1);self.assertTrue(m['driver_reaped'])
            self.assertEqual(launch.call_args.args[0],[str(b),'--device-open-preflight'])
            self.assertEqual(m['classification'],'unreviewed-output')
            self.assertNotIn('same process-local pidfd feature check before real client',m['implicit_effects_inventory']['resident'])
    def test_signal_refuses_changed_identity(self):
        from unittest.mock import patch
        r={'pid':17,'tgid':17,'exe':'/fixed','start_ticks':77}
        with patch.object(w,'identity',return_value={**r,'start_ticks':78}),patch.object(w.os,'kill') as k:
            self.assertFalse(w.checked_kill(r,9));k.assert_not_called()

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
