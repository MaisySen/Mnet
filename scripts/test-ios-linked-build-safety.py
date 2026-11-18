#!/usr/bin/env python3
"""Safety/source contract only; actual linked-product check runs on macOS."""
from pathlib import Path
import plistlib
import unittest
ROOT = Path(__file__).resolve().parents[1]

class UnsignedBuildContract(unittest.TestCase):
    def test_no_signing_provisioning_or_launch(self):
        script = (ROOT / 'scripts/test-ios-linked-build.sh').read_text()
        for required in ['CODE_SIGNING_ALLOWED=NO', 'CODE_SIGNING_REQUIRED=NO',
                         'CODE_SIGN_IDENTITY= DEVELOPMENT_TEAM=', '-no_adhoc_codesign',
                         '-target=iossimulator/arm64', '-sdk iphonesimulator',
                         'hashlib.sha256(data).hexdigest()', 'check-ios-linked-product.py']:
            self.assertIn(required, script)
        for forbidden in ['allowProvisioning', 'simctl', 'notarytool', 'altool', 'archivePath', 'codesign --sign']:
            self.assertNotIn(forbidden, script)

    def test_real_app_depends_on_embedded_extension(self):
        project = (ROOT / 'ios/project.yml').read_text()
        self.assertIn('- target: MnetTunnel\n        embed: true\n        codeSign: false', project)
        self.assertIn('- framework: Mnet.xcframework\n        embed: false', project)
        self.assertNotIn('path: ios', project)
        self.assertNotIn('framework: ios/', project)
        self.assertNotIn('path: .', project)

    def test_extension_package_and_identity(self):
        with (ROOT / 'ios/MnetTunnel-Info.plist').open('rb') as stream:
            info = plistlib.load(stream)
        self.assertEqual('XPC!', info['CFBundlePackageType'])
        self.assertEqual('com.apple.networkextension.packet-tunnel', info['NSExtension']['NSExtensionPointIdentifier'])
        script = (ROOT / 'scripts/check-ios-linked-product.py').read_text()
        for required in ['MobileNewNode', 'crosscall2', 'PlugIns/MnetTunnel.appex', 'not signed']:
            self.assertIn(required, script)

if __name__ == '__main__':
    unittest.main()
