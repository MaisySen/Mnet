#!/usr/bin/env python3
"""No preference access. Source contract for transient enrollment form input."""
from pathlib import Path
import unittest
ROOT = Path(__file__).resolve().parents[1]

class EnrollmentInput(unittest.TestCase):
    def test_no_app_preference_secret_load_or_save(self):
        view = (ROOT / 'ios/ContentView.swift').read_text()
        self.assertIn('@State private var authKey = ""', view)
        self.assertNotIn('string(forKey: "authKey")', view)
        self.assertNotIn('set(authKey', view)
        self.assertIn('EnrollmentPreferences.removeLegacyKey(from: .standard)', view)
        self.assertIn('EnrollmentPreferences.saveNonSecretSettings(', view)

    def test_key_clears_only_after_successful_handoff(self):
        view = (ROOT / 'ios/ContentView.swift').read_text()
        connect = view.split('private func connect()', 1)[1].split('private func disconnect()', 1)[0]
        self.assertIn('"authKey": authKey as NSObject', connect)
        self.assertLess(connect.index('try manager.connection.startVPNTunnel(options: opts)'), connect.index('authKey = ""'))
        self.assertNotIn('authKey = ""', connect.split('try manager.connection.startVPNTunnel(options: opts)', 1)[0])

    def test_helper_has_no_secret_read_or_persistence_api(self):
        helper = (ROOT / 'ios/EnrollmentPreferences.swift').read_text()
        self.assertIn('removeObject(forKey: "authKey")', helper)
        self.assertNotIn('string(forKey:', helper)
        self.assertNotIn('authKey: String', helper)
        self.assertNotIn('print(', helper)
        self.assertIn('- path: EnrollmentPreferences.swift', (ROOT / 'ios/project.yml').read_text())

if __name__ == '__main__':
    unittest.main()
