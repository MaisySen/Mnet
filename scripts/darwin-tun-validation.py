#!/usr/bin/env python3
"""Disposable native root macOS CI: real utun setup/cleanup, not peer VPN proof."""
import datetime
import hashlib
import http.client
import http.cookiejar
import ipaddress
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
import urllib.request
import uuid

if sys.platform != 'darwin' or os.geteuid() != 0 or os.environ.get('GITHUB_ACTIONS') != 'true':
    raise RuntimeError('requires root on a disposable native GitHub Actions macOS runner')
binary = Path(sys.argv[1]).resolve()
control_binary = Path(sys.argv[2]).resolve()
peer_binary = Path(sys.argv[3]).resolve()
root = Path(tempfile.mkdtemp(prefix='mnet-tun-', dir='/private/tmp'))
state = root / 'state'
processes, logs = [], []
label = 'au.fintex.mnet.ci.tun.' + uuid.uuid4().hex
target = 'system/' + label
launchd_attempted = False

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

def start(args, label, env=None):
    log = (root / (label + '.log')).open('w')
    logs.append(log)
    p = subprocess.Popen(list(map(str, args)), stdout=log, stderr=subprocess.STDOUT, env=env)
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
def has_mesh_route(routes):
    for line in routes.splitlines():
        fields = line.split()
        if not fields or not fields[0][0].isdigit():
            continue
        destination = fields[0]
        address, _, prefix = destination.partition('/')
        address += '.0' * (4 - len(address.split('.')))
        try:
            network = ipaddress.ip_network(address + '/' + (prefix or '32'), strict=False)
        except ValueError:
            continue
        if network.prefixlen and network.overlaps(ipaddress.ip_network('100.64.0.0/10')):
            return True
    return False

if has_mesh_route(initial_routes):
    raise RuntimeError('refusing preexisting mesh-range route in disposable runner')
name = next(('utun' + str(i) for i in range(30, 200) if 'utun' + str(i) not in interfaces()), None)
if name is None:
    raise RuntimeError('no free explicit utun fixture interface')
auto_interface = os.environ.get('MNET_AUTO_UTUN') == '1'
interface_args = [] if auto_interface else ['-ifname', name]
requested_name = 'utun' if auto_interface else name
print('native kernel interface mode: ' + ('automatic default utun' if auto_interface else 'explicit utunN'))
try:
    with socket.socket() as probe:
        probe.bind(('127.0.0.1', 0))
        port = probe.getsockname()[1]
    base = 'http://127.0.0.1:' + str(port)
    control = start([control_binary, '-listen', '127.0.0.1:' + str(port), '-base-url', base,
           '-data', root / 'control.json', '-admin-password', 'native-tun-fixture-only'], 'control')
    wait_for(lambda: urllib.request.urlopen(base + '/healthz', timeout=2).status == 200, 'disposable control health')
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    def admin(path, body):
        request = urllib.request.Request(base + path, json.dumps(body).encode(), {'Content-Type': 'application/json'})
        return json.load(opener.open(request, timeout=3))
    admin('/admin/api/login', {'password': 'native-tun-fixture-only'})
    network = admin('/admin/api/networks', {'name': 'native-utun'})
    key = admin('/admin/api/authkeys', {'network_id': network['id'], 'reusable': True})['key']
    daemon = start([binary, '-state', state] + interface_args, 'daemon')
    wait_for(api, 'authenticated native daemon API')
    api('up', {'server': base, 'network': 'native-utun', 'name': 'native-utun', 'auth_key': key})
    def up():
        global name
        value = api()
        if value.get('last_error'):
            raise RuntimeError('native setup failed: ' + value['last_error'])
        if value['status'] != 'up':
            return None
        actual = value.get('interface', '')
        assert actual.startswith('utun') and actual[4:].isdigit(), 'missing actual kernel utun status'
        if auto_interface:
            name = actual
        else:
            assert actual == name, 'explicit kernel interface name changed'
        return value
    status = wait_for(up, 'actual macOS utun Up')
    address = status['ipv4']
    info = run('/sbin/ifconfig', name)
    assert 'mtu 1280' in info and 'inet ' + address in info, 'kernel interface address/MTU mismatch'
    assert 'interface: ' + name in route_for_mesh(), 'mesh route not bound to actual utun'
    saved = json.loads((state / 'state.json').read_text())
    identity = {k: saved[k] for k in ['node_id', 'public_key', 'node_token']}
    print('PASS actual macOS kernel utun address/MTU/mesh route and authenticated Up; single node, not peer traffic')
    peer_config = root / 'peer-config.json'
    peer_config.write_text(json.dumps({'Server': base, 'Network': 'native-utun', 'Key': key, 'Root': str(root)}))
    os.chmod(peer_config, 0o600)
    start([peer_binary, '-test.run=^TestNativeKernelPeerFixture$', '-test.timeout=8m'], 'userspace-peer',
          {**os.environ, 'MNET_NATIVE_PEER_CONFIG': str(peer_config)})
    def peer_ready():
        path = root / 'peer-ready.json'
        return json.loads(path.read_text()) if path.exists() else None
    peer = wait_for(peer_ready, 'proof-only userspace encrypted peer listener')
    assert peer['ipv4'] != address
    wait_for(lambda: len(api().get('peers', [])) == 1, 'native kernel device sees the userspace peer')
    payload = bytes(range(256)) * (4 * 1024 * 1024 // 256)
    digest = hashlib.sha256(payload).digest()
    def transfer():
        with socket.create_connection((peer['ipv4'], peer['port']), timeout=5) as conn:
            conn.settimeout(30)
            conn.sendall(payload)
            received = b''
            while len(received) < 32:
                chunk = conn.recv(32 - len(received))
                if not chunk:
                    break
                received += chunk
            assert received == digest, 'encrypted peer receiver hash ACK mismatch'
            reverse = bytearray()
            while len(reverse) < len(payload):
                chunk = conn.recv(min(65536, len(payload) - len(reverse)))
                if not chunk:
                    raise RuntimeError('truncated reverse kernel TCP payload')
                reverse.extend(chunk)
            expected_reverse = (b'reverse-native-peer-proof' * (len(payload) // 25 + 1))[:len(payload)]
            assert reverse == expected_reverse, 'reverse kernel TCP payload mismatch'
            conn.sendall(hashlib.sha256(reverse).digest())
            ack = b''
            while len(ack) < 2:
                chunk = conn.recv(2 - len(ack))
                if not chunk:
                    break
                ack += chunk
            assert ack == b'OK', 'reverse kernel receiver hash not acknowledged'
        return True
    wait_for(transfer, 'native OS TCP over actual utun to encrypted userspace peer', 60)
    print('PASS actual macOS kernel utun OS TCP 4MiB receiver SHA256 ACK to proof-only userspace WireGuard peer; same host, not two-kernel-host proof')
    api('down', {})
    wait_for(lambda: name not in interfaces(), 'utun removed after Down')
    assert 'interface: ' + name not in route_for_mesh(), 'mesh route survived Down'
    api('up', {})
    wait_for(up, 'saved-state utun Up')
    wait_for(transfer, 'kernel peer TCP after Down/Up', 60)
    daemon.terminate()
    assert daemon.wait(timeout=15) == 0, 'daemon clean stop failed'
    wait_for(lambda: name not in interfaces(), 'utun removed after process stop')
    assert not (state / 'mnetd.sock').exists()
    daemon = start([binary, '-state', state, '-up'] + interface_args, 'restarted')
    status = wait_for(up, 'fresh process saved-state automatic utun Up')
    assert status['ipv4'] == address
    wait_for(transfer, 'kernel peer TCP after saved process restart', 60)
    saved = json.loads((state / 'state.json').read_text())
    assert all(saved[k] == v for k, v in identity.items()), 'saved native device identity changed'
    assert 'interface: ' + name in route_for_mesh()
    api('down', {})
    wait_for(lambda: name not in interfaces(), 'final native utun cleanup')
    assert 'interface: ' + name not in route_for_mesh()
    daemon.terminate()
    assert daemon.wait(timeout=15) == 0
    assert not (state / 'mnetd.sock').exists()
    print('PASS actual macOS utun Down/Up, clean process restart/saved identity and final interface/route teardown; no peer VPN/boot proof')
    def pid():
        expected = ' '.join([str(binary), '-state', str(state), '-ifname', requested_name, '-up'])
        matches = []
        for line in run('/bin/ps', '-ww', '-axo', 'pid=,command=').splitlines():
            parts = line.strip().split(None, 1)
            if len(parts) == 2 and parts[1] == expected:
                matches.append(int(parts[0]))
        return matches[0] if len(matches) == 1 else None

    def kernel_ready():
        status = up()
        if not status:
            return None
        assert status['ipv4'] == address
        current = json.loads((state / 'state.json').read_text())
        assert all(current[k] == v for k, v in identity.items())
        info = run('/sbin/ifconfig', name)
        assert 'mtu 1280' in info and 'inet ' + address in info
        assert 'interface: ' + name in route_for_mesh()
        return status

    def stopped():
        return (pid() is None and not (state / 'mnetd.sock').exists()
                and name not in interfaces() and 'interface: ' + name not in route_for_mesh())

    # Generate unchanged saved-state production argv/recovery settings. Only
    # the unique test label differs; no /Library or persistent enable setting.
    config = plistlib.loads(run(str(binary), '-state', str(state), *interface_args,
                               '-service', 'print-plist').encode())
    config['Label'] = label
    plist = root / (label + '.plist')
    plist.write_bytes(plistlib.dumps(config))
    os.chmod(plist, 0o600)
    assert subprocess.run(['/bin/launchctl', 'print', target], capture_output=True).returncode != 0
    token = (state / 'local-api-token').read_bytes()
    launchd_attempted = True
    run('/bin/launchctl', 'bootstrap', 'system', str(plist))
    wait_for(kernel_ready, 'launchd saved-state actual kernel utun Up')
    wait_for(transfer, 'kernel peer TCP after launchd bootstrap', 60)
    first = wait_for(pid, 'launchd enrolled daemon PID')
    assert (state / 'local-api-token').read_bytes() == token
    print('PASS actual launchd generated job restores saved one-node kernel utun address/MTU/route and identity')
    run('/bin/launchctl', 'kill', 'SIGKILL', target)
    wait_for(lambda: (p if p and p != first else None) if (p := pid()) else None,
             'launchd utun forced-crash recovery new PID', 60)
    wait_for(kernel_ready, 'launchd crash recovery actual kernel utun Up')
    wait_for(transfer, 'kernel peer TCP after launchd forced-crash recovery', 60)
    assert (state / 'local-api-token').read_bytes() == token
    run('/bin/launchctl', 'bootout', target)
    wait_for(stopped, 'launchd utun unload process/socket/interface/route cleanup')
    run('/bin/launchctl', 'bootstrap', 'system', str(plist))
    wait_for(kernel_ready, 'launchd reload actual kernel utun Up')
    wait_for(transfer, 'kernel peer TCP after launchd reload', 60)
    print('PASS actual macOS utun OS TCP 4MiB hash/ACK restored after Down/Up, saved process restart and launchd bootstrap/crash/reload against same-host proof peer')
    print('PASS bidirectional actual macOS utun OS TCP 4MiB each way with both receiver SHA256 ACKs after all recovery stages')
    # Stop the disposable control before editing its private snapshot. Force
    # a received authorization denial without changing saved device secrets.
    control.terminate()
    control.wait(timeout=10)
    snapshot_path = root / 'control.json'
    snapshot = json.loads(snapshot_path.read_text())
    node = next(n for n in snapshot['nodes'].values() if n['id'] == identity['node_id'])
    node['approved'] = False
    snapshot_path.write_text(json.dumps(snapshot))
    control = start([control_binary, '-listen', '127.0.0.1:' + str(port), '-base-url', base,
                     '-data', snapshot_path, '-admin-password', 'native-tun-fixture-only'], 'control-denied')
    def denied():
        value = api()
        return value['status'] == 'waiting-approval' and not value.get('peers')
    wait_for(denied, 'received denial withdraws actual macOS kernel peers', 60)
    # Connection failure, not only status text, is required before reapproval.
    for _ in range(3):
        try:
            conn = socket.create_connection((peer['ipv4'], peer['port']), timeout=2)
        except OSError:
            continue
        conn.close()
        raise RuntimeError('received denial left fresh kernel TCP connection reachable')
    current = json.loads((state / 'state.json').read_text())
    assert all(current[k] == v for k, v in identity.items()), 'denial changed native identity'
    assert (state / 'local-api-token').read_bytes() == token
    admin('/admin/api/login', {'password': 'native-tun-fixture-only'})
    admin('/admin/api/nodes/approve', {'node_id': identity['node_id']})
    def reapproved():
        value = api()
        return value['status'] == 'up' and not value.get('approval_url') and len(value.get('peers', [])) == 1
    wait_for(reapproved, 'reapproval restores native kernel peer without stale approval link', 60)
    wait_for(transfer, 'bidirectional kernel TCP after received denial and reapproval', 60)
    current = json.loads((state / 'state.json').read_text())
    assert all(current[k] == v for k, v in identity.items()), 'reapproval changed native identity'
    assert api()['ipv4'] == address and (state / 'local-api-token').read_bytes() == token
    print('PASS received control denial removes actual macOS kernel peers and blocks three fresh TCP connects; reapproval restores bidirectional 4MiB hash/ACK with same identity/IP/token')
    # Reload the generated job against a known short future key deadline.
    # Fresh Up must consume this map before we take control/relay offline.
    run('/bin/launchctl', 'bootout', target)
    wait_for(stopped, 'native kernel teardown before known-expiry reload')
    control.terminate()
    control.wait(timeout=10)
    snapshot = json.loads(snapshot_path.read_text())
    expiry = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(seconds=45)
    node = next(n for n in snapshot['nodes'].values() if n['id'] == identity['node_id'])
    node['key_expiry'] = expiry.isoformat().replace('+00:00', 'Z')
    snapshot_path.write_text(json.dumps(snapshot))
    control = start([control_binary, '-listen', '127.0.0.1:' + str(port), '-base-url', base,
                     '-data', snapshot_path, '-admin-password', 'native-tun-fixture-only'], 'control-known-expiry')
    wait_for(lambda: urllib.request.urlopen(base + '/healthz', timeout=2).status == 200, 'known-expiry control health')
    run('/bin/launchctl', 'bootstrap', 'system', str(plist))
    wait_for(kernel_ready, 'fresh launchd Up consumes known future key expiry')
    def direct():
        transfer()
        peers = api().get('peers', [])
        return (len(peers) == 1 and peers[0].get('online') and not peers[0].get('relay')
                and peers[0].get('endpoint') and not peers[0]['endpoint'].startswith('relay'))
    wait_for(direct, 'actual native peer direct UDP path before stopping control/relay', 60)
    control.terminate()
    control.wait(timeout=10)
    # Fresh sockets after the process exits cannot use its WebSocket relay.
    for _ in range(3):
        transfer()
    print('PASS actual macOS kernel bidirectional TCP 3 x 4MiB each way hash/ACK with control/relay process stopped; same-host direct UDP fixture only')
    assert datetime.datetime.now(datetime.timezone.utc) < expiry, 'offline transfer missed known expiry boundary'
    def locally_expired():
        value = api()
        return value['status'] == 'key-expired' and not value.get('peers')
    wait_for(locally_expired, 'known native key expiry enforced with control/relay offline', 70)
    assert control.poll() is not None, 'control unexpectedly alive at local expiry'
    for _ in range(3):
        try:
            conn = socket.create_connection((peer['ipv4'], peer['port']), timeout=2)
        except OSError:
            continue
        conn.close()
        raise RuntimeError('known local expiry left fresh kernel TCP reachable while control offline')
    current = json.loads((state / 'state.json').read_text())
    assert all(current[k] == v for k, v in identity.items())
    print('PASS known future key expiry removes actual macOS kernel peers and blocks three fresh TCP connects while control/relay remains stopped; same saved identity')
    assert (state / 'local-api-token').read_bytes() == token
    run('/bin/launchctl', 'bootout', target)
    wait_for(stopped, 'final launchd utun unload cleanup')
    assert subprocess.run(['/bin/launchctl', 'print', target], capture_output=True).returncode != 0
    print('PASS actual launchd one-node kernel utun forced-crash recovery/reload/unload; no peer VPN or OS boot proof')
except Exception:
    # Fixture-only logs may hold enrollment URLs. Keep credential-bearing
    # files local; report safe current daemon error only, never saved state.
    try:
        print('native daemon status/error: ' + json.dumps({k: api().get(k) for k in ['status', 'last_error']}), file=sys.stderr)
    except Exception:
        pass
    raise
finally:
    if launchd_attempted and subprocess.run(['/bin/launchctl', 'print', target], capture_output=True).returncode == 0:
        run('/bin/launchctl', 'bootout', target)
        wait_for(stopped, 'failure cleanup of launchd utun job')
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
