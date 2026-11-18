#!/usr/bin/env python3
"""Source-only event/callback wiring, no system notification or preference IO."""
from pathlib import Path
import unittest
ROOT = Path(__file__).resolve().parents[1]

class StatusEvents(unittest.TestCase):
    def setUp(self):
        self.view = (ROOT / 'ios/ContentView.swift').read_text()
        self.connect = self.view.split('private func connect()', 1)[1].split('private func disconnect()', 1)[0]
        self.disconnect = self.view.split('private func disconnect()', 1)[1].split('private func refresh()', 1)[0]
        self.refresh = self.view.split('private func refresh()', 1)[1]

    def test_events_not_polling(self):
        for required in ['.NEVPNStatusDidChange', '.NEVPNConfigurationChange', '.onChange(of: scenePhase)', '.onDisappear { statusReads.invalidate() }']:
            self.assertIn(required, self.view)
        self.assertNotIn('Timer.', self.view)
        self.assertNotIn('asyncAfter', self.view)

    def test_reads_owned_and_explicit_actions_invalidate(self):
        self.assertIn('let ticket = statusReads.begin()', self.refresh)
        self.assertIn('guard statusReads.isCurrent(ticket), !busy else { return }', self.refresh)
        for action in [self.connect, self.disconnect]:
            self.assertLess(action.index('statusReads.invalidate()'), action.index('manager(createIfMissing:'))
        self.assertLess(self.refresh.index('guard !busy'), self.refresh.index('manager(createIfMissing:'))

    def test_request_snapshot_and_no_newer_key_erasure(self):
        self.assertLess(self.connect.index('let opts:'), self.connect.index('manager(createIfMissing:'))
        self.assertIn('let submittedKey = authKey', self.connect)
        self.assertIn('if authKey == submittedKey { authKey = "" }', self.connect)
        successful = self.connect.split('try manager.connection.startVPNTunnel(options: opts)', 1)[1]
        self.assertLess(successful.index('busy = false'), successful.index('refresh()'))

if __name__ == '__main__':
    unittest.main()
