#!/usr/bin/env python3
"""Actual home refuses absent readable fonts and opens with declared X11 fonts."""
import os
import pathlib
import subprocess
import tempfile
import time

os.chdir(pathlib.Path(__file__).resolve().parent.parent)
out = pathlib.Path(tempfile.mkdtemp(prefix='mnet-font-prerequisite.'))
print(out, flush=True)
subprocess.run(['go', 'build', '-race', '-o', str(out / 'mnet'), './cmd/mnet'], check=True)
env = os.environ.copy()
env.pop('WAYLAND_DISPLAY', None)
env.pop('XDG_SESSION_TYPE', None)
for absent in (True, False):
    name = 'builtins-only' if absent else 'declared-fonts'
    server = client = None
    try:
        display = out / (name + '.display')
        with display.open('w') as fd, (out / (name + '-x.log')).open('w') as log:
            server = subprocess.Popen(['Xvfb', '-noreset', '-nolisten', 'tcp', '-displayfd', str(fd.fileno()), '-screen', '0', '1024x768x24', *(['-fp', 'built-ins'] if absent else [])], pass_fds=(fd.fileno(),), stdout=log, stderr=log)
        deadline = time.monotonic() + 6
        while not display.read_text().strip() and time.monotonic() < deadline:
            time.sleep(.02)
        assert display.read_text().strip(), 'X server did not start'
        env['DISPLAY'] = ':' + display.read_text().strip()
        with (out / (name + '-client.log')).open('w') as log:
            client = subprocess.Popen([str(out / 'mnet'), '--state', str(out / 'absent-state'), 'open'], env=env, stdout=log, stderr=log)
        if absent:
            assert client.wait(timeout=6) != 0
            assert 'home readable fonts unavailable' in (out / (name + '-client.log')).read_text()
            assert not (out / 'absent-state').exists()
            print('PASS actual built-ins-only X server refuses home without readable-font prerequisite; no setup/state fallback', flush=True)
        else:
            window = None
            deadline = time.monotonic() + 6
            while time.monotonic() < deadline:
                found = subprocess.run(['xdotool', 'search', '--onlyvisible', '--name', '^Mnet: three tools'], env=env, text=True, capture_output=True)
                if found.returncode == 0 and found.stdout.strip():
                    window = found.stdout.strip().splitlines()[-1]
                    break
                assert client.poll() is None, (out / (name + '-client.log')).read_text()
                time.sleep(.02)
            assert window, 'actual home not opened with declared fonts'
            subprocess.run(['import', '-display', env['DISPLAY'], '-window', window, str(out / 'font-ready-home.png')], check=True)
            subprocess.run(['xdotool', 'windowfocus', window, 'key', 'Escape'], env=env, check=True)
            assert client.wait(timeout=3) == 0
            assert not (out / 'absent-state').exists()
            print('PASS actual declared-font X server opens and closes readable home with no daemon/state', flush=True)
    finally:
        if client is not None and client.poll() is None:
            client.terminate()
            client.wait(timeout=3)
        if server is not None:
            server.terminate()
            server.wait(timeout=3)
