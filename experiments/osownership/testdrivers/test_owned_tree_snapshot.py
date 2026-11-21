import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch
import owned_tree_snapshot as s

class OfflineSnapshots(unittest.TestCase):
    def test_fixture_identity_and_timestamps(self):
        with tempfile.TemporaryDirectory() as td:
            root=Path(td);p=root/'17';(p/'task'/'17').mkdir(parents=True)
            values=['S','9']+['0']*17+['77']+['0']*4
            (p/'stat').write_text('17 (fixed) '+' '.join(values))
            (p/'status').write_text('Tgid:\t17\n')
            (p/'cmdline').write_bytes(b'/fixed\0--fixed-mode\0')
            (p/'task'/'17'/'children').write_text('')
            with patch.object(s.os,'readlink',return_value='/fixed'):
                r=s.snapshot(17,root)
            a=r['records'][0]
            self.assertEqual(a['tgid'],17);self.assertEqual(a['start_ticks'],77)
            self.assertEqual(a['cmdline'],['/fixed','--fixed-mode'])
            self.assertLessEqual(a['read_start_ns'],a['read_end_ns'])
            self.assertLessEqual(r['collection_start_ns'],r['collection_end_ns'])
    def test_unreadable_preserved_not_absence(self):
        with tempfile.TemporaryDirectory() as td:
            r=s.snapshot(17,Path(td));self.assertIn('observation_error',r['records'][0])
    def test_implicit_actor_inventory(self):
        self.assertIn('clone0x5100 transient child',s.IMPLICIT_EFFECTS['driver_first_StartProcess'])
        self.assertIn('nonspawning',s.IMPLICIT_EFFECTS['fixed_client'])
if __name__=='__main__':unittest.main()
