#!/usr/bin/env python3
"""Fresh-process Linux singleton, active API preservation and crash release."""
import http.client
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time

binary = str(Path(sys.argv[1]).resolve())

class UnixHTTP(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__('localhost', timeout=2)
        self.path = str(path)
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(2)
        self.sock.connect(self.path)

def ready(state, process):
    end = time.monotonic() + 10
    while time.monotonic() < end:
        if process.poll() is not None:
            raise AssertionError('daemon exited before authenticated API readiness')
        conn = UnixHTTP(state / 'mnetd.sock')
        try:
            token = (state / 'local-api-token').read_text()
            conn.request('GET', '/localapi/status', headers={'X-Mnet-Local-Token': token})
            response = conn.getresponse()
            response.read()
            if response.status == 200:
                return
        except (OSError, http.client.HTTPException):
            pass
        finally:
            conn.close()
        time.sleep(.02)
    raise AssertionError('daemon API did not become ready')

with tempfile.TemporaryDirectory(prefix='mnet-owner-check-') as root:
    state = Path(root)
    processes = []
    def start():
        p = subprocess.Popen([binary, '-state', str(state)], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        processes.append(p)
        return p
    try:
        first = start()
        ready(state, first)
        sock = (state / 'mnetd.sock').stat()
        token = (state / 'local-api-token').read_bytes()
        lock = (state / 'daemon.lock').stat()
        for _ in range(5):
            duplicate = start()
            out, err = duplicate.communicate(timeout=5)
            assert duplicate.returncode != 0 and not out, 'duplicate daemon did not fail'
            assert b'another daemon already owns this state directory' in err
            assert first.poll() is None, 'active owner was stopped'
            assert (state / 'mnetd.sock').stat().st_ino == sock.st_ino, 'active API socket replaced'
            assert (state / 'local-api-token').read_bytes() == token, 'active API credential changed'
            ready(state, first)
        first.terminate()
        first.communicate(timeout=5)
        assert first.returncode == 0 and not (state / 'mnetd.sock').exists()
        second = start()
        ready(state, second)
        assert (state / 'local-api-token').read_bytes() == token
        second.kill()
        second.communicate(timeout=5)
        third = start()
        ready(state, third)
        assert (state / 'local-api-token').read_bytes() == token
        assert os.path.samestat(lock, (state / 'daemon.lock').stat()), 'lock inode replaced'
        third.terminate()
        third.communicate(timeout=5)
        assert third.returncode == 0 and not (state / 'mnetd.sock').exists()
        # Simultaneous empty-state contenders cannot both initialize/bind.
        for attempt in range(5):
            race_state = state / ('race-' + str(attempt))
            contenders = [subprocess.Popen([binary, '-state', str(race_state)], stdout=subprocess.PIPE, stderr=subprocess.PIPE) for _ in range(2)]
            processes.extend(contenders)
            end = time.monotonic() + 10
            while all(p.poll() is None for p in contenders) and time.monotonic() < end:
                time.sleep(.02)
            winners = [p for p in contenders if p.poll() is None]
            assert len(winners) == 1, 'simultaneous owners both survived or both failed'
            winner = winners[0]
            ready(race_state, winner)
            loser = next(p for p in contenders if p is not winner)
            out, err = loser.communicate(timeout=5)
            assert loser.returncode != 0 and b'another daemon already owns this state directory' in err
            winner.terminate()
            winner.communicate(timeout=5)
            assert winner.returncode == 0 and not (race_state / 'mnetd.sock').exists()
        print('actual Linux processes: duplicate owners rejected; API/token preserved; clean stop and crash release permit restart')
    finally:
        for process in processes:
            if process.poll() is None:
                process.kill()
            process.communicate(timeout=5)
