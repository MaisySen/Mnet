#!/usr/bin/env python3
"""Actual private-prefix installation, desktop launch and conservative removal."""
import json
import os
import pathlib
import signal
import subprocess
import tempfile
import time
from gi.repository import Gio, GLib
from PIL import Image

REPO = pathlib.Path(__file__).resolve().parent.parent
os.chdir(REPO)
out = pathlib.Path(tempfile.mkdtemp(prefix='mnet-installed.'))
print(out, flush=True)
prefix = out / 'Engineering space dollar$ quote" tick` slash\\'
installer = REPO / 'scripts/linux-local-install.py'

def invoke(action, path=prefix, okay=True):
    result = subprocess.run(['python3', str(installer), action, str(path)], text=True, capture_output=True)
    (out / (action + '-last.log')).write_text(result.stdout + result.stderr)
    assert (result.returncode == 0) == okay, result.stdout + result.stderr
    return result


def poll(check, label):
    deadline = time.monotonic() + 8
    while time.monotonic() < deadline:
        result = check()
        if result:
            return result
        time.sleep(.02)
    raise AssertionError('deadline: ' + label)


xserver = None
daemon = None
pids = []
old_environment = dict(os.environ)
try:
    home = out / 'empty-home'
    home.mkdir(mode=0o700)
    invoke('install', out / 'percent%refused', okay=False)
    assert not (out / 'percent%refused').exists()
    invoke('install')
    manifest = json.loads((prefix / 'INSTALLATION.json').read_text())
    assert manifest['engineering_only'] is True
    assert manifest['source_commit'] == subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    assert set(manifest['files']) == {'bin/mnet', 'bin/mnetd', 'share/applications/mnet-engineering.desktop', 'README.txt'}
    desktop = prefix / 'share/applications/mnet-engineering.desktop'
    subprocess.run(['desktop-file-validate', str(desktop)], check=True)
    for binary, argument in (('mnet', 'version'), ('mnetd', '-version')):
        subprocess.run([str(prefix / 'bin' / binary), argument], check=True, env={'PATH': os.environ['PATH'], 'HOME': str(home)})
    before = (prefix / 'bin/mnet').read_bytes()
    invoke('install', okay=False)
    assert (prefix / 'bin/mnet').read_bytes() == before
    # Both extra files and changed content must refuse removal before any
    # existing installed file is deleted. No recursive cleanup is used.
    (prefix / 'extra').write_text('keep this unrelated file')
    invoke('uninstall', okay=False)
    assert (prefix / 'bin/mnet').read_bytes() == before
    (prefix / 'extra').unlink()
    readme = prefix / 'README.txt'
    original = readme.read_bytes()
    readme.write_bytes(original + b'local change\n')
    invoke('uninstall', okay=False)
    assert (prefix / 'bin/mnet').read_bytes() == before and readme.read_bytes() != original
    readme.write_bytes(original)
    linked = out / 'linked-prefix'
    linked.symlink_to(prefix, target_is_directory=True)
    invoke('uninstall', linked, okay=False)
    assert (prefix / 'bin/mnet').read_bytes() == before
    shared = out / 'shared'
    shared.mkdir(mode=0o700)
    shared.chmod(0o777)
    invoke('install', shared / 'new', okay=False)
    assert not (shared / 'new').exists()
    with (out / 'display').open('w') as display, (out / 'x.log').open('w') as log:
        xserver = subprocess.Popen(['Xvfb', '-noreset', '-nolisten', 'tcp', '-displayfd', str(display.fileno()), '-screen', '0', '1024x768x24'], pass_fds=(display.fileno(),), stdout=log, stderr=log)
    display_name = ':' + poll(lambda: (out / 'display').read_text().strip(), 'X server')
    os.environ.clear()
    os.environ.update({'PATH': old_environment['PATH'], 'HOME': str(home), 'DISPLAY': display_name, 'XDG_CONFIG_HOME': str(home / '.config'), 'XDG_DATA_HOME': str(home / '.local/share'), 'XDG_CACHE_HOME': str(out / 'launcher-cache'), 'DBUS_SESSION_BUS_ADDRESS': 'unix:path=' + str(out / 'no-session-bus'), 'GIO_USE_VFS': 'local'})
    app = Gio.DesktopAppInfo.new_from_filename(str(desktop))
    assert app is not None
    def launched(info, pid, data):
        pids.append(pid)
    assert app.launch_uris_as_manager([], None, GLib.SpawnFlags.DO_NOT_REAP_CHILD, None, None, launched, None)
    assert len(pids) == 1
    def visible():
        value = subprocess.run(['xdotool', 'search', '--onlyvisible', '--name', '^Mnet: three tools'], text=True, capture_output=True)
        return value.stdout.strip().splitlines()[-1] if value.returncode == 0 and value.stdout.strip() else None
    window = poll(visible, 'installed desktop entry home')
    actual = pathlib.Path('/proc') / str(pids[0]) / 'exe'
    assert actual.resolve() == (prefix / 'bin/mnet').resolve(), 'wrong executable launched'
    screenshot = out / 'installed-home.png'
    subprocess.run(['import', '-display', display_name, '-window', window, str(screenshot)], check=True)
    image = Image.open(screenshot).convert('RGB')
    assert image.size == (900, 560) and image.getpixel((20, 110)) == (28, 42, 59)
    assert not list(home.iterdir()), 'opening installed home wrote user state'
    invoke('uninstall', okay=False)
    assert (prefix / 'bin/mnet').read_bytes() == before, 'running install removed'
    def find_window(pattern):
        value = subprocess.run(['xdotool', 'search', '--onlyvisible', '--name', pattern], text=True, capture_output=True)
        return value.stdout.strip().splitlines()[-1] if value.returncode == 0 and value.stdout.strip() else None
    def click(wid, x, y):
        subprocess.run(['xdotool', 'windowfocus', wid, 'mousemove', '--window', wid, str(x), str(y), 'click', '1'], check=True)
    def close_child(wid):
        subprocess.run(['xdotool', 'windowfocus', wid, 'key', 'Escape'], check=True)
        return poll(visible, 'fresh home after setup state')
    def capture(wid, name):
        subprocess.run(['import', '-display', display_name, '-window', wid, str(out / (name + '.png'))], check=True)
    click(window, 100, 250)
    chooser = poll(lambda: find_window(r'^Mnet Remote Desktop: choose \[unavailable\]'), 'missing capability guidance')
    capture(chooser, 'missing-local-setup')
    assert not list(home.iterdir()), 'missing setup journey wrote user state'
    window = close_child(chooser)
    # Explicit fixture-only daemon startup. No Up, control-plane URL, network,
    # session-host flag, enrollment or real interface is requested.
    state = home / '.config/mnet'
    with (out / 'daemon.log').open('w') as log:
        daemon = subprocess.Popen([str(prefix / 'bin/mnetd'), '-state', str(state), '-fake-tun'], stdout=log, stderr=log)
    poll(lambda: (state / 'mnetd.sock').exists(), 'actual installed daemon local API')
    token_path = state / 'local-api-token'
    poll(lambda: token_path.exists(), 'actual installed local capability')
    token_path.chmod(0o644)
    denied = subprocess.run([str(prefix / 'bin/mnet'), '--state', str(state), 'status', '--json'], text=True, capture_output=True)
    assert denied.returncode != 0 and token_path.read_text() not in denied.stdout + denied.stderr, 'unsafe token mode accepted or disclosed'
    token_path.chmod(0o600)
    accepted = subprocess.run([str(prefix / 'bin/mnet'), '--state', str(state), 'status', '--json'], text=True, capture_output=True)
    assert accepted.returncode == 0 and json.loads(accepted.stdout)['status'] == 'down', 'restored private capability did not recover'
    click(window, 100, 250)
    chooser = poll(lambda: find_window(r'^Mnet Remote Desktop: choose \[unavailable\]'), 'actual daemon down guidance')
    capture(chooser, 'device-not-connected')
    window = close_child(chooser)
    click(window, 100, 140)
    devices = poll(lambda: find_window(r'^Mnet devices: read-only \[down\]'), 'actual installed daemon down snapshot')
    capture(devices, 'actual-daemon-down')
    window = close_child(devices)
    daemon.terminate()
    assert daemon.wait(timeout=5) == 0
    daemon = None
    state_before = {str(path.relative_to(state)): path.read_bytes() for path in state.rglob('*') if path.is_file() and not path.is_symlink()}
    click(window, 100, 250)
    chooser = poll(lambda: find_window(r'^Mnet Remote Desktop: choose \[unavailable\]'), 'stopped daemon guidance')
    capture(chooser, 'local-daemon-not-reached')
    window = close_child(chooser)
    subprocess.run(['xdotool', 'windowfocus', window, 'key', 'Escape'], check=True)
    pid = pids.pop()
    assert os.waitpid(pid, 0)[1] == 0
    assert not visible(), 'installed home survived closing'
    assert state_before == {str(path.relative_to(state)): path.read_bytes() for path in state.rglob('*') if path.is_file() and not path.is_symlink()}, 'failure journey altered daemon state'
    os.environ.clear()
    os.environ.update(old_environment)
    invoke('uninstall')
    assert not prefix.exists()
    assert state_before == {str(path.relative_to(state)): path.read_bytes() for path in state.rglob('*') if path.is_file() and not path.is_symlink()} and linked.is_symlink(), 'removal touched unrelated state'
    print('PASS actual clean-prefix binary install, desktop-file parser and launched native home from special-character path; no initial Mnet state/service/enrollment; actual installed daemon down/missing/stopped recovery; preserved fixture state; exact close; changed/extra/link refusals; unchanged removal', flush=True)
finally:
    os.environ.clear()
    os.environ.update(old_environment)
    for pid in pids:
        try:
            os.kill(pid, signal.SIGTERM)
            os.waitpid(pid, 0)
        except ProcessLookupError:
            pass
    if daemon is not None:
        daemon.terminate()
        daemon.wait(timeout=5)
    if xserver is not None:
        xserver.terminate()
        xserver.wait(timeout=3)
    print('Artifacts retained: ' + str(out), flush=True)
