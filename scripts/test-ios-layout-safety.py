#!/usr/bin/env python3
"""Render boundary checks. They are not a substitute for SwiftUI pixels."""
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[1]

class LayoutSafety(unittest.TestCase):
    def test_renderer_links_only_production_layout_and_harness(self):
        shell = (ROOT / 'scripts/render-swiftui-layouts.sh').read_text()
        self.assertIn('ios/MnetConnectionPanel.swift', shell)
        self.assertIn('ios-tests/MnetLayoutRender.swift', shell)
        for excluded in ['ContentView.swift', 'MnetApp.swift', 'NetworkExtension', 'EnrollmentPreferences.swift', 'simctl', 'xcodebuild', 'codesign', 'security ', 'sudo ']:
            self.assertNotIn(excluded, shell)

    def test_pure_component_has_no_runtime_effects(self):
        panel = (ROOT / 'ios/MnetConnectionPanel.swift').read_text()
        renderer = (ROOT / 'ios-tests/MnetLayoutRender.swift').read_text()
        for text in [panel, renderer]:
            for effect in ['import NetworkExtension', 'UserDefaults', 'startVPNTunnel', 'saveToPreferences', 'loadAllFromPreferences', 'URLSession', 'NETunnelProviderManager']:
                self.assertNotIn(effect, text)
        self.assertIn('action: { fatalError(', renderer)
        self.assertIn('MnetConnectionPanel(', renderer)
        self.assertIn('NSHostingView(rootView: panel)', renderer)
        self.assertIn('host.cacheDisplay(in: host.bounds, to: bitmap)', renderer)
        self.assertNotIn('ContentView(', renderer)
        self.assertIn('SecureField(', panel)
        self.assertIn('.disabled(busy)', panel)

    def test_production_uses_same_component_and_explicit_action(self):
        source = (ROOT / 'ios/ContentView.swift').read_text()
        body = source.split('var body:', 1)[1].split('private func save()', 1)[0]
        self.assertIn('MnetConnectionPanel(server: $server', body)
        self.assertIn('save()\n                    connected ? disconnect() : connect()', body)
        self.assertNotIn('startVPNTunnel', body)
        self.assertIn('- path: MnetConnectionPanel.swift', (ROOT / 'ios/project.yml').read_text())
        self.assertIn('"$ROOT/ios/MnetConnectionPanel.swift"', (ROOT / 'scripts/test-ios-source.sh').read_text())

if __name__ == '__main__':
    unittest.main()
