#!/usr/bin/env python3
"""Wiring guards, not native execution. Host Swift tests own state behavior."""
from pathlib import Path
import unittest
ROOT = Path(__file__).resolve().parents[1]

class ProviderOwnership(unittest.TestCase):
    def setUp(self):
        self.source = (ROOT / 'ios/PacketTunnelProvider.swift').read_text()

    def test_permanent_owner_and_no_mutable_node_slot(self):
        self.assertIn('TunnelLifecycle<MobileNode, NEPacketTunnelNetworkSettings>', self.source)
        self.assertIn('close: { $0.close() }', self.source)
        self.assertNotIn('private var node:', self.source)
        self.assertNotIn('node?.stop()', self.source)
        self.assertIn('lifecycle.stop(completion: completionHandler)', self.source)

    def test_no_swallowed_setup_errors(self):
        self.assertNotIn('try?', self.source)
        self.assertIn('try node.setUseExit(exitNode)', self.source)
        self.assertIn('error: &prepareError)', self.source)
        self.assertIn('if let error = prepareError { throw error }', self.source)
        self.assertIn('try node.start(Flow(', self.source)
        self.assertNotIn('if !exitNode.isEmpty {\n            try', self.source)

    def test_target_contains_coordinator(self):
        project = (ROOT / 'ios/project.yml').read_text()
        self.assertIn('- path: ContentView.swift', project)
        self.assertIn('- path: TunnelLifecycle.swift', project)
        checks = (ROOT / 'scripts/test-ios-source.sh').read_text()
        self.assertIn('TunnelLifecycleTest.swift', checks)
        self.assertIn('"$ROOT/ios/PacketTunnelProvider.swift"', checks)

if __name__ == '__main__':
    unittest.main()
