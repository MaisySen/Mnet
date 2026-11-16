#!/usr/bin/env python3
"""Reject dynamic/private values and stack traces in Android VPN log calls."""
from pathlib import Path
import re
import unittest

PATTERN = re.compile(r'\bLog\.(?:v|d|i|w|e|wtf)\s*\((.*?)\)\s*;', re.S)
FIXED = re.compile(r'\s*TAG\s*,\s*"[^"\\]*"\s*\Z', re.S)

def unsafe_calls(source):
    return [call for call in PATTERN.findall(source) if not FIXED.fullmatch(call)]

class LogPolicyTests(unittest.TestCase):
    def test_fixed(self):
        self.assertFalse(unsafe_calls('Log.i(TAG, "mesh started");'))
    def test_private(self):
        for value in ['Log.i(TAG, node.status());', 'Log.i(TAG, "up " + node.status());',
                      'Log.e(TAG, "failed", e);', 'Log.e(TAG, e.getMessage());',
                      'Log.i(TAG, "server " + server);', 'Log.w(TAG, "key " + authKey);']:
            self.assertTrue(unsafe_calls(value))

if __name__ == '__main__':
    result = unittest.TextTestRunner().run(unittest.defaultTestLoader.loadTestsFromTestCase(LogPolicyTests))
    if not result.wasSuccessful():
        raise SystemExit(1)
    source = (Path(__file__).resolve().parents[1] / 'android/app/src/main/java/au/fintex/mnet/MnetVpnService.java').read_text()
    if unsafe_calls(source):
        raise SystemExit('Android VPN log call contains nonliteral values or stack trace')
    print('PASS Android VPN general logs use fixed messages only; runtime log capture remains unverified')
