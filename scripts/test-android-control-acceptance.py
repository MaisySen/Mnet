#!/usr/bin/env python3
"""Guard the separated acceptance boundary without launching a device/run."""
from pathlib import Path
import ast
import unittest
import yaml

ROOT = Path(__file__).resolve().parents[1]
RUNNER = ROOT / 'scripts/android-control-acceptance.py'
WORKFLOW = ROOT / '.github/workflows/android-control-acceptance.yml'
JAVA = ROOT / 'android/instrumentation/au/fintex/mnet/fixture/ControlAcceptance.java'

class ControlAcceptanceSafety(unittest.TestCase):
    def test_manual_standard_runner_only(self):
        data = yaml.safe_load(WORKFLOW.read_text())
        triggers = data.get('on', data.get(True))
        self.assertEqual({'workflow_dispatch': None}, triggers)
        job = data['jobs']['attended-control']
        self.assertEqual('ubuntu-latest', job['runs-on'])
        self.assertEqual([34, 36], job['strategy']['matrix']['api'])
        self.assertLessEqual(job['timeout-minutes'], 25)
        self.assertFalse(job['strategy']['fail-fast'])
    def test_no_combined_diagnostics(self):
        for source in [RUNNER.read_text(), JAVA.read_text(), WORKFLOW.read_text()]:
            for forbidden in ['DNSProof', 'hashRoundTrip(', 'verifyDNS', 'android-emulator-validation.py',
                              'android-kernel-observer', 'android-app-correlation', 'adb root',
                              'bindSocket(', 'bindProcessToNetwork', '4MiB']:
                self.assertNotIn(forbidden, source)
    def test_allowlisted_artifacts_only(self):
        data = yaml.safe_load(WORKFLOW.read_text())
        upload = data['jobs']['attended-control']['steps'][-1]
        paths = upload['with']['path'].splitlines()
        self.assertEqual(14, len(paths))
        for path in paths:
            self.assertNotIn('*', path)
            self.assertTrue(path.endswith(('.png', '/summary.json')))
            self.assertIn('/mnet-control-artifacts/', path)
    def test_disposable_guard_and_cleanup(self):
        s = RUNNER.read_text()
        ast.parse(s)
        for required in ["os.environ.get('GITHUB_ACTIONS') != 'true'", "Path('/dev/kvm').exists()",
                         "env.pop('MNET_RELEASE_KEYSTORE', None)", 'shutil.rmtree(root)',
                         "config.chmod(0o600)", '720', "verdict = 'passed' if passed else 'blocked'"]:
            self.assertIn(required, s)
    def test_both_real_orientations_and_no_grant_shortcut(self):
        s = JAVA.read_text()
        for required in ['hostLeg();viewerLeg();', 'startActivitySync', 'Allow viewing', 'Allow control',
                         'actual-accessibility-gesture', 'viewer-bound-input-received',
                         'viewing-after-revoke', 'stale_refused', 'unsupported-viewing-frame',
                         'viewer-large-font', 'injectInputEvent']:
            self.assertIn(required, s)
        self.assertNotIn('.inject(', s)
        self.assertNotIn('setHosting(true)', s)
        self.assertNotIn('getUiAutomation()', s)

if __name__ == '__main__':
    unittest.main()
