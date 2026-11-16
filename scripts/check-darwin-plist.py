#!/usr/bin/env python3
"""Native Darwin read-only CLI/plutil verification; never loads a launchd job."""
from pathlib import Path
import plistlib
import subprocess
import sys
import tempfile

if sys.platform != 'darwin':
    raise RuntimeError('requires native macOS for actual CLI/plutil checks')
binary = str(Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix='mnet-plist-check-') as root:
    state = Path(root) / 'not-initialized'
    base = [binary, '-state', str(state), '-ifname', 'utun9', '-service', 'print-plist']
    result = subprocess.run(base, check=True, capture_output=True)
    expected = {'Label': 'au.fintex.mnet.mnetd',
                'ProgramArguments': [binary, '-state', str(state), '-ifname', 'utun9', '-up'],
                'RunAtLoad': True, 'KeepAlive': {'SuccessfulExit': False},
                'ThrottleInterval': 10, 'Umask': 63}
    assert plistlib.loads(result.stdout) == expected, 'plist keys/argv/recovery contract mismatch'
    assert not state.exists() and not result.stderr, 'generation changed state or printed diagnostics'
    plist = Path(root) / 'au.fintex.mnet.mnetd.plist'
    plist.write_bytes(result.stdout)
    subprocess.run(['/usr/bin/plutil', '-lint', str(plist)], check=True)
    auto = subprocess.run([binary, '-state', str(state), '-service', 'print-plist'], check=True, capture_output=True)
    expected['ProgramArguments'] = [binary, '-state', str(state), '-ifname', 'utun', '-up']
    assert plistlib.loads(auto.stdout) == expected, 'default automatic utun argv mismatch'
    assert not state.exists(), 'automatic generation initialized state'
    for extra in [['-auth-key', 'fixture-private-value'], ['-network', 'fixture'],
                  ['-ifname', 'mnet0'], ['-service', 'install'],
                  ['-service', 'remove'], ['-service', 'run']]:
        rejected = subprocess.run(base + extra, capture_output=True)
        assert rejected.returncode != 0 and not rejected.stdout, 'unsupported generation accepted'
        assert b'fixture-private-value' not in rejected.stderr, 'secret echoed'
        assert not state.exists(), 'rejected action initialized state'
    for number, name in enumerate(['mnet0', 'utun01', 'utun7junk', 'utun65536']):
        invalid_state = Path(root) / ('invalid-' + str(number))
        rejected = subprocess.run([binary, '-state', str(invalid_state), '-ifname', name,
                                   '-up', '-server', 'http://127.0.0.1:1', '-network', 'must-not-enroll'],
                                  capture_output=True, timeout=5)
        assert rejected.returncode != 0, 'invalid kernel request started'
        assert not (invalid_state / 'state.json').exists(), 'invalid request created device identity'
        assert not (invalid_state / 'local-api-token').exists(), 'invalid request initialized API'
        assert not (invalid_state / 'mnetd.sock').exists(), 'invalid request opened API socket'
print('actual native macOS CLI: plist exact argv/keys, plutil syntax, no state writes and rejected actions passed; no job loaded')
