#!/usr/bin/env python3
"""Disposable root Linux CI: real kernel TUN and OS TCP, no host route changes."""
import hashlib
import http.client
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import uuid

ROOT = Path(__file__).resolve().parents[1]

def run(*args, **kwargs):
    return subprocess.run(args, check=True, text=True, capture_output=True, timeout=90, **kwargs).stdout


def wait_for(check, label, timeout=30):
    until = time.monotonic() + timeout
    last = None
    while time.monotonic() < until:
        try:
            result = check()
            if result:
                return result
        except (OSError, ValueError, http.client.HTTPException) as exc:
            last = type(exc).__name__
        time.sleep(0.1)
    raise RuntimeError(f'timeout: {label}; last error type {last}')


class UnixHTTP(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__('localhost', timeout=3)
        self.path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(str(self.path))


def local_api(state, path='status', body=None):
    conn = UnixHTTP(state / 'mnetd.sock')
    try:
        token = (state / 'local-api-token').read_text().strip()
        conn.request('GET' if body is None else 'POST', '/localapi/' + path,
                     None if body is None else json.dumps(body),
                     {'X-Mnet-Local-Token': token, 'Content-Type': 'application/json'})
        response = conn.getresponse()
        data = json.loads(response.read())
        if response.status != 200:
            raise RuntimeError(f'local API status {response.status}')
        return data
    finally:
        conn.close()


# Both sides compute their own payload/hash; no success by byte count alone.
PAYLOAD = bytes(range(256)) * 16384
DIGEST = hashlib.sha256(PAYLOAD).hexdigest()
SERVER = r'''
import hashlib,json,socket,sys
from pathlib import Path
with socket.socket() as server:
 server.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
 server.bind((sys.argv[1],42001));server.listen();server.settimeout(70)
 Path(sys.argv[2]).write_text('ready')
 for _ in range(3):
  conn,_=server.accept()
  with conn:
   conn.settimeout(60);h=hashlib.sha256();total=0
   while True:
    chunk=conn.recv(65536)
    if not chunk:break
    total+=len(chunk);h.update(chunk)
   conn.sendall(json.dumps({'bytes':total,'sha256':h.hexdigest()}).encode())
'''
CLIENT = r'''
import hashlib,json,socket,sys
payload=bytes(range(256))*16384
with socket.create_connection((sys.argv[1],42001),timeout=60) as s:
 s.settimeout(60);s.sendall(payload);s.shutdown(socket.SHUT_WR)
 data=b''
 while True:
  chunk=s.recv(4096)
  if not chunk:break
  data+=chunk
 result=json.loads(data)
 assert result=={'bytes':len(payload),'sha256':hashlib.sha256(payload).hexdigest()},result
 print(json.dumps(result))
'''


def main():
    if sys.platform != 'linux' or os.geteuid() != 0:
        raise RuntimeError('requires root in a disposable Linux runner')
    for command in ['ip', 'tc', 'ping']:
        if not shutil.which(command):
            raise RuntimeError(f'required command missing: {command}')
    if not Path('/dev/net/tun').exists():
        raise RuntimeError('kernel TUN unavailable')
    tmp = Path(tempfile.mkdtemp(prefix='mnet-kernel-'))
    os.chmod(tmp, 0o700)
    tag = uuid.uuid4().hex[:7]
    spaces = {x: 'mn' + tag + x for x in 'cab'}
    processes, logs, created, host_links, configs = [], [], [], [], []

    def ns(which, *args):
        return ('ip', 'netns', 'exec', spaces[which], *map(str, args))

    def start(which, args, label):
        log = (tmp / (label + '.log')).open('w')
        logs.append(log)
        proc = subprocess.Popen(ns(which, *args), stdout=log, stderr=subprocess.STDOUT)
        processes.append(proc)
        return proc

    try:
        # Build without changing root's Go environment: binaries are supplied
        # by CI before sudo, and only these temporary files are executed.
        binary_dir = Path(os.environ['MNET_KERNEL_BIN']).resolve()
        for x in 'cab':
            run('ip', 'netns', 'add', spaces[x]); created.append(spaces[x])
            run('ip', '-n', spaces[x], 'link', 'set', 'lo', 'up')
        run('ip', '-n', spaces['c'], 'link', 'add', 'br0', 'type', 'bridge')
        run('ip', '-n', spaces['c'], 'addr', 'add', '198.18.10.1/24', 'dev', 'br0')
        run('ip', '-n', spaces['c'], 'link', 'set', 'br0', 'up')
        for index, x in enumerate('ab', 2):
            left, right = 'v' + tag + x, 'p' + tag + x
            run('ip', 'link', 'add', left, 'type', 'veth', 'peer', 'name', right)
            host_links.extend([left, right])
            run('ip', 'link', 'set', left, 'netns', spaces[x])
            run('ip', 'link', 'set', right, 'netns', spaces['c'])
            run('ip', '-n', spaces[x], 'link', 'set', left, 'name', 'eth0')
            run('ip', '-n', spaces[x], 'addr', 'add', f'198.18.10.{index}/24', 'dev', 'eth0')
            run('ip', '-n', spaces[x], 'link', 'set', 'eth0', 'up')
            run('ip', '-n', spaces['c'], 'link', 'set', right, 'master', 'br0')
            run('ip', '-n', spaces['c'], 'link', 'set', right, 'up')
            # Resolve STUN names locally in this isolated fixture only.
            config = Path('/etc/netns') / spaces[x]
            config.mkdir(parents=True); configs.append(config)
            (config / 'hosts').write_text('127.0.0.1 localhost\n198.18.10.1 stun.l.google.com stun1.l.google.com stun.cloudflare.com\n')
        start('c', [binary_dir / 'mnet-control', '-listen', '198.18.10.1:8080',
                    '-base-url', 'http://198.18.10.1:8080', '-data', tmp / 'control.json',
                    '-admin-password', 'kernel-ci-fixture-only'], 'control')
        # Control administration runs inside the isolated namespace. Never
        # print the fixture's auth key or retain its store as a CI artifact.
        admin = tmp / 'admin.py'
        admin.write_text(r'''
import http.cookiejar,json,sys,urllib.request
opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
def post(path,data):
 req=urllib.request.Request('http://198.18.10.1:8080'+path,json.dumps(data).encode(),{'Content-Type':'application/json'})
 return json.load(opener.open(req,timeout=3))
post('/admin/api/login',{'password':'kernel-ci-fixture-only'})
n=post('/admin/api/networks',{'name':'kerneltest'})
k=post('/admin/api/authkeys',{'network_id':n['id'],'reusable':True})
print(json.dumps(k))
''')
        # Wait for the listener without mutating network/key state on retry.
        for _ in range(100):
            try:
                run(*ns('c', sys.executable, '-c', "import socket; socket.create_connection(('198.18.10.1',8080),1).close()"))
                break
            except subprocess.CalledProcessError:
                time.sleep(0.1)
        else:
            raise RuntimeError('control did not listen')
        key = json.loads(run(*ns('c', sys.executable, admin)))['key']
        states = {}
        for x in 'ab':
            state = tmp / x; state.mkdir(mode=0o700); states[x] = state
            start(x, [binary_dir / 'mnetd', '-state', state], x)
            wait_for(lambda: local_api(state), 'local API ' + x)
            local_api(state, 'up', {'server': 'http://198.18.10.1:8080', 'network':'kerneltest',
                                   'name': x, 'auth_key': key})
            wait_for(lambda: local_api(state)['status'] == 'up', 'kernel TUN up ' + x)
        wait_for(lambda: (s if len(s.get('peers', [])) == 1 else None)
                          if (s := local_api(states['a'])) else None, 'peer map')
        ips = {x: local_api(states[x])['ipv4'] for x in 'ab'}
        for x in 'ab':
            link = json.loads(run('ip', '-n', spaces[x], '-j', 'addr', 'show', 'dev', 'mnet0'))[0]
            if not any(a['local'] == ips[x] for a in link['addr_info']):
                raise RuntimeError('mesh address missing from actual kernel TUN')
        for attempt in range(30):
            try:
                run(*ns('a', 'ping', '-c', '1', '-W', '1', ips['b']))
                break
            except subprocess.CalledProcessError:
                time.sleep(0.2)
        else:
            raise RuntimeError('kernel mesh ping failed')
        print('PASS actual kernel TUN addresses/routes and encrypted mesh ping', flush=True)
        for profile in ['normal', 'loss-1pct-delay-20ms']:
            if profile != 'normal':
                for x in 'ab':
                    run(*ns(x, 'tc', 'qdisc', 'add', 'dev', 'eth0', 'root', 'netem',
                            'delay', '20ms', 'loss', '1%', 'limit', '1000'))
            ready = tmp / ('ready-' + profile)
            server = start('b', [sys.executable, '-c', SERVER, ips['b'], ready], profile)
            wait_for(ready.exists, 'OS TCP receiver')
            for _ in range(3):
                result = json.loads(run(*ns('a', sys.executable, '-c', CLIENT, ips['b'])))
                if result != {'bytes': len(PAYLOAD), 'sha256': DIGEST}:
                    raise RuntimeError('independent TCP hash check failed')
            if server.wait(timeout=5) != 0:
                raise RuntimeError('TCP server exited with failure')
            print(f'PASS OS TCP {profile}: 3 x 4MiB, receiver hash and sender ACK', flush=True)
        for x in 'ab':
            local_api(states[x], 'down', {})
            if local_api(states[x])['status'] != 'down':
                raise RuntimeError('daemon did not report down')
            links = json.loads(run('ip', '-n', spaces[x], '-j', 'link', 'show'))
            if any(link['ifname'] == 'mnet0' for link in links):
                raise RuntimeError('kernel interface survived Down')
            routes = json.loads(run('ip', '-n', spaces[x], '-j', 'route', 'show'))
            if any(r.get('dst') == '100.64.0.0/10' for r in routes):
                raise RuntimeError('mesh route survived Down')
        print('PASS Down removes actual kernel TUN interfaces and mesh routes', flush=True)
    except Exception:
        for path in tmp.glob('*.log'):
            # Daemon logs can contain fixture approval URLs. Keep diagnostics
            # local to this disposable test rather than uploading its state.
            print(f'--- {path.name} ---\n{path.read_text()[-6000:]}', file=sys.stderr)
        raise
    finally:
        for proc in reversed(processes):
            if proc.poll() is None:
                proc.terminate()
        for proc in processes:
            try: proc.wait(timeout=5)
            except subprocess.TimeoutExpired: proc.kill(); proc.wait()
        for log in logs: log.close()
        for space in reversed(created):
            run('ip', 'netns', 'delete', space)
        for link in host_links:
            # A link may still be on the host only if setup failed early.
            subprocess.run(['ip', 'link', 'delete', link], capture_output=True)
        for config in configs: shutil.rmtree(config)
        shutil.rmtree(tmp)


if __name__ == '__main__':
    main()
