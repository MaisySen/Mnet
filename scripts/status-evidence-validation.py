#!/usr/bin/env python3
"""Actual CLI against a private Unix status fixture, not live network proof."""
import http.server
import json
import os
from pathlib import Path
import secrets
import socketserver
import subprocess
import sys
import tempfile
import threading

binary = str(Path(sys.argv[1]).resolve())

class Server(socketserver.UnixStreamServer):
    pass

class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass
    def do_GET(self):
        self.server.requests.append((self.path, self.headers.get('X-Mnet-Local-Token')))
        if self.path != '/localapi/status' or self.headers.get('X-Mnet-Local-Token') != self.server.token:
            self.send_response(403)
            self.end_headers()
            return
        payload = json.dumps(self.server.payload).encode()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

with tempfile.TemporaryDirectory(prefix='mnet-status-', dir='/private/tmp' if sys.platform == 'darwin' else None) as root:
    state = Path(root)
    token_path = state / 'local-api-token'
    token_path.write_text(secrets.token_hex(32))
    token_path.chmod(0o600)
    peers = []
    rows = []
    for name, handshake_state, age, online, endpoint, relay, status, path, handshake in [
        ('unknown', 'unavailable', -1, False, '', False, 'unknown', 'unknown', 'unavailable'),
        ('never', 'never', -1, False, '127.0.0.1:1234', False, 'offline', 'direct', 'never'),
        ('future', 'clock-skew', -1, False, '127.0.0.1:1234', False, 'unknown', 'direct', 'clock skew'),
        ('recent', 'recent', 0, True, '127.0.0.1:1234', False, 'online', 'direct', '0s ago'),
        ('stale', 'stale', 180, False, 'relay://fixture', True, 'offline', 'relay', '180s ago'),
        ('legacy', '', 1, True, '127.0.0.1:1234', False, 'online', 'direct', '1s ago'),
    ]:
        peer = dict(name=name, ipv4='100.64.0.2', endpoint=endpoint, relay=relay,
                    last_handshake_ago_sec=age, online=online, rx_bytes=4, tx_bytes=8)
        if handshake_state:
            peer['handshake_state'] = handshake_state
        peers.append(peer)
        rows.append((name + ' 100.64.0.2 ' + status + ' ' + path + ' ' + handshake + ' 4/8').split())
    with Server(str(state / 'mnetd.sock'), Handler) as server:
        os.chmod(state / 'mnetd.sock', 0o600)
        server.token = token_path.read_text()
        server.requests = []
        server.payload = dict(status='up', node_name='synthetic-fixture', network='fixture', ipv4='100.64.0.1', peers=peers)
        thread = threading.Thread(target=server.serve_forever)
        thread.start()
        try:
            def run(*args):
                return subprocess.run([binary, '--state', str(state), 'status', *args], capture_output=True, text=True, timeout=12)
            human = run()
            assert human.returncode == 0 and not human.stderr, 'actual human status failed'
            actual_rows = [line.split() for line in human.stdout.splitlines() if line.split() and line.split()[0] in {p['name'] for p in peers}]
            assert actual_rows == rows, 'actual human rows changed: ' + human.stdout
            machine = run('--json')
            assert machine.returncode == 0 and not machine.stderr, 'actual JSON status failed'
            decoded = json.loads(machine.stdout)
            assert decoded['status'] == 'up' and len(decoded['peers']) == len(peers)
            for got, expected in zip(decoded['peers'], peers):
                for key, value in expected.items():
                    assert got[key] == value, 'actual JSON changed ' + key
            assert len(server.requests) == 2 and all(path == '/localapi/status' and token == server.token for path, token in server.requests)
            # The ping executable is a disposable local stub. This proves CLI
            # selection/dispatch, not network reachability or real ICMP behavior.
            stub_dir = state / 'bin'
            stub_dir.mkdir(mode=0o700)
            marker = state / 'ping-args.json'
            stub = stub_dir / 'ping'
            stub.write_text('#!' + sys.executable + '\nimport json,os,sys\nfrom pathlib import Path\nPath(os.environ["MNET_PING_ARGS"]).write_text(json.dumps(sys.argv[1:]))\nprint("fixture ping dispatched")\n')
            stub.chmod(0o700)
            ping_env = os.environ | {'PATH': str(stub_dir) + os.pathsep + os.environ.get('PATH', ''), 'MNET_PING_ARGS': str(marker)}
            def ping(*args):
                marker.unlink(missing_ok=True)
                return subprocess.run([binary, '--state', str(state), 'ping', *args], capture_output=True, text=True, timeout=12, env=ping_env)
            first = dict(name='peer', fqdn='peer.fixture.mnet', ipv4='100.64.0.2', online=False)
            server.payload = dict(status='up', peers=[first])
            for selector in ('peer', 'peer.fixture.mnet', '100.64.0.2'):
                result = ping(selector)
                assert result.returncode == 0 and result.stdout == 'fixture ping dispatched\n' and not result.stderr, 'unique target did not dispatch'
                assert json.loads(marker.read_text()) == ['-c', '3', '-W', '2', '100.64.0.2'], 'OS ping arguments changed'
            cases = [
                (dict(status='up', peers=[first, dict(name='peer', ipv4='100.64.0.3')]), ('peer',)),
                (dict(status='up', peers=[first, dict(name='other', ipv4=first['ipv4'])]), ('peer',)),
                (dict(status='key-expired', peers=[first]), ('peer',)),
                (dict(status='up', peers=[dict(name='peer', ipv4='--help')]), ('peer',)),
                (dict(status='up', peers=[first]), ('missing',)),
                (dict(status='up', peers=[first]), ('peer', 'extra')),
            ]
            for payload, args in cases:
                server.payload = payload
                result = ping(*args)
                assert result.returncode != 0 and not result.stdout and not marker.exists(), 'refused target reached executable'
            assert all(path == '/localapi/status' and token == server.token for path, token in server.requests), 'status credential/path changed'
            request_count = len(server.requests)
            print('PASS actual built ping selector: unique name/FQDN/address dispatched only canonical IPv4 to local stub; ambiguous/address-collision/down/invalid/extra targets never executed; no ICMP/network proof')
            token_path.unlink()
            denied = run()
            assert denied.returncode != 0 and not denied.stdout and len(server.requests) == request_count, 'missing capability reached the status fixture'
            print('PASS actual built CLI: authenticated private Unix status, exact unknown/never/future/recent/stale/legacy rows, JSON evidence, missing-capability refusal')
        finally:
            server.shutdown()
            thread.join(timeout=3)
            assert not thread.is_alive(), 'fixture server did not join'
