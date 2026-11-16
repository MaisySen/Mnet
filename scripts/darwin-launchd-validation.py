#!/usr/bin/env python3
"""Disposable native macOS CI only: actual launchd, offline local API, no VPN."""
import http.client
import json
import os
from pathlib import Path
import plistlib
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import uuid

if sys.platform != 'darwin' or os.geteuid() != 0 or os.environ.get('GITHUB_ACTIONS') != 'true':
    raise RuntimeError('requires root on a disposable native GitHub Actions macOS runner')

binary = Path(sys.argv[1]).resolve()
root = Path(tempfile.mkdtemp(prefix='mnet-launchd-', dir='/private/tmp'))
state = root / 'state'
label = 'au.fintex.mnet.ci.' + uuid.uuid4().hex
target = 'system/' + label
loaded = False
attempted = False

def run(*args):
    return subprocess.run(args, check=True, capture_output=True, text=True, timeout=30).stdout

def wait_for(fn, what, seconds=40):
    end = time.monotonic() + seconds
    while time.monotonic() < end:
        try:
            value = fn()
            if value:
                return value
        except (OSError, ValueError, http.client.HTTPException, subprocess.CalledProcessError):
            pass
        time.sleep(.1)
    raise RuntimeError('timeout: ' + what)

class UnixHTTP(http.client.HTTPConnection):
    def __init__(self):
        super().__init__('localhost', timeout=2)
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(2)
        self.sock.connect(str(state / 'mnetd.sock'))

def api_ready():
    conn = UnixHTTP()
    try:
        token = (state / 'local-api-token').read_text()
        conn.request('GET', '/localapi/status', headers={'X-Mnet-Local-Token': token})
        response = conn.getresponse()
        value = json.loads(response.read())
        return response.status == 200 and value['status'] == 'down'
    finally:
        conn.close()

def pid():
    # Test-only exact argv match, not parsing unstable launchctl print output.
    expected = ' '.join([str(binary), '-state', str(state), '-ifname', 'utun9', '-up'])
    matches = []
    for line in run('/bin/ps', '-ww', '-axo', 'pid=,command=').splitlines():
        parts = line.strip().split(None, 1)
        if len(parts) == 2 and parts[1] == expected:
            matches.append(int(parts[0]))
    return matches[0] if len(matches) == 1 else None

def stopped():
    return not (state / 'mnetd.sock').exists() and pid() is None

try:
    # The only generated change is a unique disposable label. No production
    # label or /Library configuration is touched, enabled or disabled.
    original = run(str(binary), '-state', str(state), '-ifname', 'utun9', '-service', 'print-plist')
    config = plistlib.loads(original.encode())
    config['Label'] = label
    plist = root / (label + '.plist')
    plist.write_bytes(plistlib.dumps(config))
    os.chmod(plist, 0o600)
    run('/usr/bin/plutil', '-lint', str(plist))
    assert subprocess.run(['/bin/launchctl', 'print', target], capture_output=True).returncode != 0
    attempted = True
    run('/bin/launchctl', 'bootstrap', 'system', str(plist))
    loaded = True
    wait_for(api_ready, 'launchd RunAtLoad offline authenticated API')
    first = wait_for(pid, 'unique launchd daemon PID')
    token = (state / 'local-api-token').read_bytes()
    conn = UnixHTTP()
    try:
        conn.request('GET', '/localapi/status')
        response = conn.getresponse()
        response.read()
        assert response.status == 401, 'unauthenticated API allowed'
    finally:
        conn.close()
    print('PASS actual launchd generated argv starts offline authenticated API; no enrollment/VPN')
    run('/bin/launchctl', 'kill', 'SIGKILL', target)
    wait_for(lambda: (p if p and p != first else None) if (p := pid()) else None,
             'automatic launchd failure-only recovery new PID', 60)
    wait_for(api_ready, 'recovered authenticated API')
    assert (state / 'local-api-token').read_bytes() == token, 'API token changed on recovery'
    print('PASS actual launchd automatic forced-crash recovery with new PID and same API token')
    run('/bin/launchctl', 'bootout', target)
    loaded = False
    wait_for(stopped, 'launchd unload process/socket teardown')
    run('/bin/launchctl', 'bootstrap', 'system', str(plist))
    loaded = True
    wait_for(api_ready, 'launchd reload authenticated API')
    assert (state / 'local-api-token').read_bytes() == token
    run('/bin/launchctl', 'bootout', target)
    loaded = False
    wait_for(stopped, 'final launchd unload cleanup')
    assert subprocess.run(['/bin/launchctl', 'print', target], capture_output=True).returncode != 0
    print('PASS actual launchd unload/reload/unload removes process/socket/job; not macOS VPN or OS reboot proof')
finally:
    # A partially successful bootstrap can still have registered the job.
    if attempted and (loaded or subprocess.run(['/bin/launchctl', 'print', target], capture_output=True).returncode == 0):
        run('/bin/launchctl', 'bootout', target)
        wait_for(stopped, 'failure cleanup of disposable launchd job')
    shutil.rmtree(root)
