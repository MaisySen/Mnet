#!/usr/bin/env python3
"""Shell verifier regression with fixed test programs, no release credentials."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
ROOT = Path(__file__).resolve().parents[1]

class SignatureGate(unittest.TestCase):
    def verify(self, body):
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            signer = temp / 'jarsigner'
            signer.write_text('#!/bin/sh\n' + body)
            signer.chmod(0o700)
            env = dict(os.environ, PATH=str(temp) + os.pathsep + os.environ['PATH'])
            return subprocess.run(['sh', str(ROOT/'scripts/verify-android-aab.sh'), str(temp/'fixture.aab'), str(temp/'verification.txt')], env=env, text=True, capture_output=True)
    def test_signed_confirmation_passes(self):
        self.assertEqual(0, self.verify("printf '%s\\n' 'jar verified.'\n").returncode)
    def test_unsigned_zero_exit_rejected(self):
        self.assertNotEqual(0, self.verify("printf '%s\\n' 'jar is unsigned.'\nexit 0\n").returncode)
    def test_nonzero_not_hidden_by_output(self):
        self.assertNotEqual(0, self.verify("printf '%s\\n' 'jar verified.'\nexit 7\n").returncode)
    def test_blank_success_rejected(self):
        self.assertNotEqual(0, self.verify('exit 0\n').returncode)

if __name__ == '__main__':
    unittest.main()
