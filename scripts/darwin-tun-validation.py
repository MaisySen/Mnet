#!/usr/bin/env python3
"""Disposable native root macOS CI: real utun setup/cleanup, not peer VPN proof."""
import http.client
import http.cookiejar
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request

if sys.platform != 'darwin' or os.geteuid() != 0 or os.environ.get('GITHUB_ACTIONS') != 'true':
    raise RuntimeError('requires root on a disposable native GitHub Actions macOS runner')
binary = Path(sys.argv[1]).resolve()
control_binary = Path(sys.argv[2]).resolve()
root = Path(tempfile.mkdtemp(prefix='mnet-tun-', dir='/private/tmp'))
state = root / 'state'
processes, logs = [], []

def run(*args):
    return subprocess.run(args, check=True, capture_output=True, text=True, timeout=15).stdout

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

def start(args, label):
    log = (root / (label + '.log')).open('w')
    logs.append(log)
    p = subprocess.Popen(list(map(str, args)), stdout=log, stderr=subprocess.STDOUT)
    processes.append(p)
    return p

class UnixHTTP(http.client.HTTPConnection):
    def __init__(self):
        super().__init__('localhost', timeout=2)
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(2)
        self.sock.connect(str(state / 'mnetd.sock'))

def api(path='status', body=None):
    conn = UnixHTTP()
    try:
        conn.request('GET' if body is None else 'POST', '/localapi/' + path,
                     None if body is None else json.dumps(body),
                     {'Content-Type': 'application/json', 'X-Mnet-Local-Token': (state / 'local-api-token').read_text()})
        response = conn.getresponse()
        value = json.loads(response.read())
        assert response.status == 200, 'local API rejected fixture request'
        return value
    finally:
        conn.close()

def interfaces():
    return run('/sbin/ifconfig', '-l').split()

def route_for_mesh():
    result = subprocess.run(['/sbin/route', '-n', 'get', '100.64.0.2'], capture_output=True, text=True)
    return result.stdout

# Refuse any existing CGNAT route or selected interface rather than replacing it.
# A plain default-route lookup is expected; a selected /10-specific entry is not.
initial_routes = run('/usr/sbin/netstat', '-rn', '-f', 'inet')
if any(line.split() and (line.split()[0].startswith('100.64') or line.split()[0] == '100.0/10') for line in initial_routes.splitlines()):
    raise RuntimeError('refusing preexisting mesh-range route in disposable runner')
name = next(('utun' + str(i) for i in range(30, 200) if 'utun' + str(i) not in interfaces()), None)
if name is None:
    raise RuntimeError('no free explicit utun fixture interface')
try:
    with socket.socket() as probe:
        probe.bind(('127.0.0.1', 0))
        port = probe.getsockname()[1]
    base = 'http://127.0.0.1:' + str(port)
    start([control_binary, '-listen', '127.0.0.1:' + str(port), '-base-url', base,
           '-data', root / 'control.json', '-admin-password', 'native-tun-fixture-only'], 'control')
    wait_for(lambda: urllib.request.urlopen(base + '/healthz', timeout=2).status == 200, 'disposable control health')
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    def admin(path, body):
        request = urllib.request.Request(base + path, json.dumps(body).encode(), {'Content-Type': 'application/json'})
        return json.load(opener.open(request, timeout=3))
    admin('/admin/api/login', {'password': 'native-tun-fixture-only'})
    network = admin('/admin/api/networks', {'name': 'native-utun'})
    key = admin('/admin/api/authkeys', {'network_id': network['id'], 'reusable': True})['key']
    daemon = start([binary, '-state', state, '-ifname', name], 'daemon')
    wait_for(api, 'authenticated native daemon API')
    api('up', {'server': base, 'network': 'native-utun', 'name': 'native-utun', 'auth_key': key})
    def up():
        value = api()
        if value.get('last_error'):
            raise RuntimeError('native setup failed: ' + value['last_error'])
        return value if value['status'] == 'up' else None
    status = wait_for(up, 'actual macOS utun Up')
    address = status['ipv4']
    info = run('/sbin/ifconfig', name)
    assert 'mtu 1280' in info and 'inet ' + address in info, 'kernel interface address/MTU mismatch'
    assert 'interface: ' + name in route_for_mesh(), 'mesh route not bound to actual utun'
    saved = json.loads((state / 'state.json').read_text())
    identity = {k: saved[k] for k in ['node_id', 'public_key', 'node_token']}
    print('PASS actual macOS kernel utun address/MTU/mesh route and authenticated Up; single node, not peer traffic')
    api('down', {})
    wait_for(lambda: name not in interfaces(), 'utun removed after Down')
    assert 'interface: ' + name not in route_for_mesh(), 'mesh route survived Down'
    api('up', {})
    wait_for(up, 'saved-state utun Up')
    daemon.terminate()
    assert daemon.wait(timeout=15) == 0, 'daemon clean stop failed'
    wait_for(lambda: name not in interfaces(), 'utun removed after process stop')
    assert not (state / 'mnetd.sock').exists()
    daemon = start([binary, '-state', state, '-ifname', name, '-up'], 'restarted')
    status = wait_for(up, 'fresh process saved-state automatic utun Up')
    assert status['ipv4'] == address
    saved = json.loads((state / 'state.json').read_text())
    assert all(saved[k] == v for k, v in identity.items()), 'saved native device identity changed'
    assert 'interface: ' + name in route_for_mesh()
    api('down', {})
    wait_for(lambda: name not in interfaces(), 'final native utun cleanup')
    assert 'interface: ' + name not in route_for_mesh()
    print('PASS actual macOS utun Down/Up, clean process restart/saved identity and final interface/route teardown; no peer VPN/boot proof')
except Exception:
    # Fixture-only logs may hold enrollment URLs. Keep credential-bearing
    # files local; report safe current daemon error only, never saved state.
    try:
        print('native daemon status/error: ' + json.dumps({k: api().get(k) for k in ['status', 'last_error']}), file=sys.stderr)
    except Exception:
        pass
    raise
finally:
    for p in reversed(processes):
        if p.poll() is None:
            p.terminate()
    for p in processes:
        try:
            p.wait(timeout=15)
        except subprocess.TimeoutExpired:
            p.kill()
            p.wait()
    for log in logs:
        log.close()
    if name in interfaces() or 'interface: ' + name in route_for_mesh():
        raise RuntimeError('native fixture interface/route cleanup incomplete')
    shutil.rmtree(root)
