#!/usr/bin/env python3
"""Source wiring only. Swift compile/tests run separately on macOS CI."""
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[1]


class IosManagerWiring(unittest.TestCase):
    def setUp(self):
        self.text = (ROOT / 'ios/ContentView.swift').read_text()
        self.manager = self.text.split('    private func manager(', 1)[1].split('    private func connect()', 1)[0]
        self.connect = self.text.split('    private func connect()', 1)[1].split('    private func disconnect()', 1)[0]
        self.disconnect = self.text.split('    private func disconnect()', 1)[1].split('    private func refresh()', 1)[0]
        self.refresh = self.text.split('    private func refresh()', 1)[1]

    def test_exact_policy_selects_returned_index(self):
        self.assertIn('TunnelManagerPolicy.selectedIndex(in: ids)', self.manager)
        self.assertIn('cb(.success(managers[index]))', self.manager)
        self.assertNotIn('managers?.first', self.text)
        self.assertNotIn('managers.first', self.text)
        self.assertIn('as? NETunnelProviderProtocol', self.manager)

    def test_reads_never_save_or_stop(self):
        for text in [self.manager, self.refresh]:
            for effect in ['saveToPreferences', 'startVPNTunnel', 'stopVPNTunnel', '.isEnabled =']:
                self.assertNotIn(effect, text)
        self.assertNotIn('saveToPreferences', self.disconnect)
        self.assertNotIn('startVPNTunnel', self.disconnect)

    def test_creation_only_explicit_connect(self):
        self.assertEqual(1, self.text.count('manager(createIfMissing: true)'))
        self.assertIn('manager(createIfMissing: true)', self.connect)
        for read in [self.refresh, self.disconnect]:
            self.assertIn('manager(createIfMissing: false)', read)
        self.assertLess(self.manager.index('guard createIfMissing'), self.manager.index('NETunnelProviderManager()'))

    def test_load_error_cannot_become_absent_configuration(self):
        self.assertLess(self.manager.index('if let error = error'), self.manager.index('let managers = managers ?? []'))
        self.assertIn('cb(.failure(error)); return', self.manager)
        self.assertIn('catch { cb(.failure(error)) }', self.manager)

    def test_save_reload_start_and_error_returns(self):
        self.assertLess(self.connect.index('saveToPreferences'), self.connect.index('loadFromPreferences'))
        self.assertLess(self.connect.index('loadFromPreferences'), self.connect.index('startVPNTunnel'))
        self.assertIn('if let error = saveError', self.connect)
        self.assertIn('if let error = loadError', self.connect)
        self.assertNotIn('connected = true', self.connect)

    def test_no_automatic_removal_or_signing(self):
        self.assertNotIn('removeFromPreferences', self.text)
        self.assertIn('DispatchQueue.main.async', self.manager)
        self.assertIn('connected = manager.connection.status', self.refresh)


if __name__ == '__main__':
    unittest.main()
