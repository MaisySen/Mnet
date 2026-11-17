#!/usr/bin/env python3
"""Fixture guard/diagnostic source checks, not an emulator execution."""
import importlib.util
from pathlib import Path
import sys
import unittest
ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('android_fixture', ROOT / 'scripts/android-emulator-validation.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
class Guards(unittest.TestCase):
    def test_child_failure_does_not_echo_captured_data(self):
        with self.assertRaisesRegex(RuntimeError, '^fixture child command failed$'):
            module.run([sys.executable, '-c', "raise SystemExit('synthetic-private-marker')"])
    def test_subprocess_output_is_captured(self):
        self.assertEqual('fixture-result\n', module.run([sys.executable, '-c', "print('fixture-result')"]))
    def test_gate_requires_disposable_runner(self):
        code = (ROOT / 'scripts/android-emulator-validation.py').read_text()
        self.assertIn("os.environ.get('GITHUB_ACTIONS') != 'true'", code)
        self.assertIn("env.pop('MNET_RELEASE_KEYSTORE', None)", code)
        self.assertIn("shutil.rmtree(root)", code)
    def test_artifact_allowlist_excludes_installable_and_state(self):
        workflow = (ROOT / '.github/workflows/android-emulator.yml').read_text()
        outputs = workflow.split('path: |')[1]
        self.assertNotIn('*', outputs)
        self.assertEqual({'summary.json', 'instrumentation.txt', 'launch.png'}, {x.strip().split('/')[-1] for x in outputs.splitlines() if x.strip()})
        self.assertIn('runs-on: ubuntu-latest', workflow)
    def test_real_consent_and_no_protection_disable(self):
        source = (ROOT / 'android/instrumentation/au/fintex/mnet/fixture/MeshInstrumentation.java').read_text()
        self.assertIn('com.android.vpndialogs', source)
        self.assertIn('if (!consentClicked)', source)
        self.assertIn('root.findAccessibilityNodeInfosByViewId("android:id/button1")', source)
        self.assertNotIn('package_verifier_enable', source)
        self.assertNotIn('verifier_verify_adb_installs', source)
    def test_boot_diagnostics_are_fixed_categories_only(self):
        source = (ROOT / 'scripts/android-emulator-validation.py').read_text()
        self.assertIn("'missing_shared_library':", source)
        self.assertIn("'emulator_exited': emulator.poll() is not None", source)
        self.assertIn("'pre_boot_free_mib': free_mib", source)
        self.assertIn("'not enough space'", source)
        self.assertIn("'-partition-size', '2048'", source)
    def test_avd_creation_and_boot_share_explicit_home(self):
        source = (ROOT / '.github/workflows/android-emulator.yml').read_text()
        self.assertIn('ANDROID_AVD_HOME=$RUNNER_TEMP/mnet-avd', source)
        self.assertIn('--path "$ANDROID_AVD_HOME/mnet-fixture.avd"', source)
        self.assertIn('test -s "$ANDROID_AVD_HOME/mnet-fixture.ini"', source)
        self.assertIn('-list-avds', source)
    def test_current_target_and_actual_runtime_matrix(self):
        source = (ROOT / 'scripts/android-emulator-validation.py').read_text()
        workflow = (ROOT / '.github/workflows/android-emulator.yml').read_text()
        test = (ROOT / 'android/instrumentation/au/fintex/mnet/fixture/MeshInstrumentation.java').read_text()
        self.assertIn('api: [34, 36]', workflow)
        self.assertIn('targetSdkVersion != 36', test)
        self.assertIn('first-layout', test)
        self.assertIn('summary=PASS target36 runtimeAPI=', source)
    def test_hash_before_and_after_real_reconnect(self):
        source = (ROOT / 'android/instrumentation/au/fintex/mnet/fixture/MeshInstrumentation.java').read_text()
        self.assertEqual(2, source.count('            hashRoundTrip();'))
        self.assertIn('MessageDigest.getInstance("SHA-256")', source)
        self.assertIn('worker-exit-before-reconnect', source)
        self.assertLess(source.index('takeScreenshot()'), source.index('set(R.id.auth_key'))
if __name__ == '__main__': unittest.main()
