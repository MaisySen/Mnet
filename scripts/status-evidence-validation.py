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
            token_path.unlink()
            denied = run()
            assert denied.returncode != 0 and not denied.stdout and len(server.requests) == 2, 'missing capability reached the status fixture'
            print('PASS actual built CLI: authenticated private Unix status, exact unknown/never/future/recent/stale/legacy rows, JSON evidence, missing-capability refusal')
        finally:
            server.shutdown()
            thread.join(timeout=3)
            assert not thread.is_alive(), 'fixture server did not join'
