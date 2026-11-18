#!/usr/bin/env python3
"""One manual disposable control-journey matrix leg. No DNS/bulk/tracing tests."""
from pathlib import Path
import http.cookiejar
import json
import os
import shutil
import subprocess
import tempfile
import time
import urllib.request
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
SCREENSHOTS = ['host-view-only', 'host-decline', 'host-allow', 'host-paused', 'host-revoked',
               'viewer-available', 'viewer-pending', 'viewer-declined', 'viewer-allowed',
               'viewer-paused', 'viewer-revoked', 'viewer-unsupported', 'viewer-large-font']


def run(args, timeout=90, **kwargs):
    p = subprocess.run(list(map(str, args)), capture_output=True, timeout=timeout, **kwargs)
    if p.returncode:
        raise RuntimeError('bounded fixture child failed')
    return p.stdout


def wait(probe, seconds=90):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        try:
            v = probe()
            if v:
                return v
        except (OSError, ValueError, RuntimeError):
            pass
        time.sleep(.3)
    raise RuntimeError('bounded prerequisite failed')


def main():
    if os.environ.get('GITHUB_ACTIONS') != 'true' or not Path('/dev/kvm').exists():
        raise RuntimeError('manual disposable public CI only')
    sdk = Path(os.environ['ANDROID_HOME'])
    bt = sdk / 'build-tools/36.0.0'
    android = sdk / 'platforms/android-36/android.jar'
    artifacts = Path(os.environ['MNET_CONTROL_ARTIFACTS'])
    artifacts.mkdir(parents=True, exist_ok=True)
    root = Path(tempfile.mkdtemp(prefix='mnet-control-acceptance-'))
    root.chmod(0o700)
    processes, handles = [], []
    stage, verdict = 'prerequisite-build', 'blocked'
    checks=[]
    adb = [sdk / 'platform-tools/adb', '-s', 'emulator-5554']

    def start(args, label, **kwargs):
        handle = (root / (label + '.private')).open('wb')
        handles.append(handle)
        p = subprocess.Popen(list(map(str, args)), stdout=handle, stderr=subprocess.STDOUT, **kwargs)
        processes.append(p)
        return p

    def exists(name):
        return subprocess.run(list(map(str, adb + ['shell', 'run-as', 'au.fintex.mnet', 'test', '-f', 'files/' + name])), capture_output=True, timeout=5).returncode == 0

    try:
        staged = root / 'source'
        staged.mkdir()
        archive = run(['git', 'archive', 'HEAD'])
        subprocess.run(['tar', '-x', '-C', str(staged)], input=archive, check=True)
        manifest = staged / 'android/app/src/main/AndroidManifest.xml'
        tree = ET.parse(manifest)
        ns = '{http://schemas.android.com/apk/res/android}'
        tree.getroot().find('application').set(ns + 'debuggable', 'true')
        tree.getroot().find('application').set(ns + 'usesCleartextTraffic', 'true')
        ET.SubElement(tree.getroot(), 'uses-permission', {ns + 'name': 'android.permission.ACCESS_NETWORK_STATE'})
        tree.write(manifest, encoding='utf-8', xml_declaration=True)
        env = os.environ.copy()
        env.pop('MNET_RELEASE_KEYSTORE', None)
        env.pop('MNET_RELEASE_PASS_FILE', None)
        run(['sh', staged / 'android/build.sh', os.environ['MNET_CONTROL_AAR']], timeout=180, env=env, cwd=staged)
        out = staged / 'android/out'
        test = root / 'test'
        for sub in ['classes', 'dex']:
            (test / sub).mkdir(parents=True, exist_ok=True)
        (test / 'AndroidManifest.xml').write_text('''<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="au.fintex.mnet.controlfixture"><uses-sdk android:minSdkVersion="24" android:targetSdkVersion="36"/><uses-permission android:name="android.permission.INTERNET"/><application android:label="Disposable control acceptance"/><instrumentation android:name="au.fintex.mnet.fixture.ControlAcceptance" android:targetPackage="au.fintex.mnet"/></manifest>''')
        run([bt / 'aapt2', 'link', '-o', test / 'base.apk', '-I', android, '--manifest', test / 'AndroidManifest.xml'])
        run(['javac', '-source', '11', '-target', '11', '-cp', f'{android}:{out}/classes:{out}/aar/classes.jar', '-d', test / 'classes', staged / 'android/instrumentation/au/fintex/mnet/fixture/ControlAcceptance.java',staged / 'android/instrumentation/au/fintex/mnet/fixture/ControlAcceptanceState.java'])
        run(['jar', 'cf', test / 'target.jar', '-C', out / 'classes', '.'])
        run([bt / 'd8', '--min-api', '24', '--lib', android, '--classpath', test / 'target.jar', '--classpath', out / 'aar/classes.jar', '--output', test / 'dex', *sorted((test / 'classes').rglob('*.class'))])
        shutil.copy(test / 'base.apk', test / 'unaligned.apk')
        run(['zip', '-q', test / 'unaligned.apk', 'classes.dex'], cwd=test / 'dex')
        run([bt / 'zipalign', '-f', '4', test / 'unaligned.apk', test / 'unsigned.apk'])
        run([bt / 'apksigner', 'sign', '--ks', staged / 'android/debug.keystore', '--ks-pass', 'pass:android', '--key-pass', 'pass:android', '--ks-key-alias', 'mnet-debug', '--out', test / 'fixture.apk', test / 'unsigned.apk'])
        base = 'http://127.0.0.1:8080'
        binaries = Path(os.environ['MNET_CONTROL_BIN'])
        start([binaries / 'mnet-control', '-listen', '127.0.0.1:8080', '-base-url', base, '-data', root / 'control.json', '-admin-password', 'disposable-control-only'], 'server')
        wait(lambda: urllib.request.urlopen(base + '/healthz', timeout=2).status == 200)
        opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

        def admin(path, body):
            return json.load(opener.open(urllib.request.Request(base + path, json.dumps(body).encode(), {'Content-Type': 'application/json'}), timeout=3))
        admin('/admin/api/login', {'password': 'disposable-control-only'})
        network = admin('/admin/api/networks', {'name': 'control-acceptance'})
        key = admin('/admin/api/authkeys', {'network_id': network['id'], 'reusable': True})['key']
        config = root / 'peer.json'
        config.write_text(json.dumps({'Server': base, 'Network': 'control-acceptance', 'Key': key, 'Root': str(root)}))
        config.chmod(0o600)
        peer_env = env.copy()
        peer_env['MNET_CONTROL_CONFIG'] = str(config)
        start([binaries / 'control-peer', '-test.run', '^TestControlAcceptancePeer$', '-test.timeout', '16m'], 'peer', env=peer_env)
        peer = wait(lambda: json.loads((root / 'ready.json').read_text()))
        stage = 'prerequisite-emulator'
        emulator = start([sdk / 'emulator/emulator', '-avd', 'mnet-control', '-port', '5554', '-no-window', '-no-audio', '-no-boot-anim', '-no-snapshot', '-gpu', 'swiftshader', '-partition-size', '2048', '-memory', '2048', '-cores', '2'], 'emulator')
        wait(lambda: emulator.poll() is None and run(adb + ['shell', 'getprop', 'sys.boot_completed'], timeout=6).strip() == b'1', 300)
        # Disposable layout targets only; no root, routing, DNS or debug tracing.
        run(adb + ['shell', 'wm', 'size', '320x800'])
        run(adb + ['shell', 'wm', 'density', '160'])
        for setting in ['window_animation_scale', 'transition_animation_scale', 'animator_duration_scale']:
            run(adb + ['shell', 'settings', 'put', 'global', setting, '0'])
        run(adb + ['shell', 'input', 'keyevent', '82'])
        for port in ['8080', '8081']:
            run(adb + ['reverse', 'tcp:' + port, 'tcp:' + port])
        run(adb + ['install', '-r', out / 'mnet.apk'])
        run(adb + ['install', '-r', test / 'fixture.apk'])
        stage = 'prerequisite-native-join'
        instrument = start(adb + ['shell', 'am', 'instrument', '-w', '-r', '-e', 'server', base, '-e', 'key', key, '-e', 'peer', peer['ipv4'], 'au.fintex.mnet.controlfixture/au.fintex.mnet.fixture.ControlAcceptance'], 'instrumentation')
        deadline = time.monotonic() + 720
        mirrored = font = False
        while instrument.poll() is None and time.monotonic() < deadline:
            if not mirrored and exists('control-host-ready'):
                point = json.loads(run(adb + ['exec-out', 'run-as', 'au.fintex.mnet', 'cat', 'files/control-target.json']))
                if set(point) != {'x', 'y', 'width', 'height'} or not all(isinstance(v, int) and v >= 0 for v in point.values()):
                    raise RuntimeError('invalid synthetic coordinate')
                (root / 'target.json').write_text(json.dumps(point))
                run(adb + ['shell', 'run-as', 'au.fintex.mnet', 'touch', 'files/control-target-ready'])
                mirrored = True
            if not font and exists('control-font-ready'):
                run(adb + ['shell', 'settings', 'put', 'system', 'font_scale', '1.3'])
                run(adb + ['shell', 'run-as', 'au.fintex.mnet', 'touch', 'files/control-font-applied'])
                font = True
            try:
                value=run(adb + ['exec-out','run-as','au.fintex.mnet','cat','files/fixture-stage'],timeout=5).decode().strip()
                if value and all(c.isalnum() or c=='-' for c in value):
                    stage=value
                    verdict='blocked' if stage.startswith(('prerequisite','os-vpn','target-mirrored','button-Allow-viewing','host-decoded-frames','viewer-Ask-the-host')) else 'failed'
            except (OSError,RuntimeError,subprocess.SubprocessError):
                pass
            time.sleep(.5)
        if instrument.poll() is None:
            raise RuntimeError('acceptance deadline')
        text = (root / 'instrumentation.private').read_text(errors='replace')
        import re
        match = re.search(r'INSTRUMENTATION_RESULT: stage=([A-Za-z0-9-]+)', text)
        stage = match.group(1) if match else 'instrumentation-result'
        check_match=re.search(r'INSTRUMENTATION_RESULT: checks=(\[[^\n]+\])',text)
        if check_match:
            candidates=json.loads(check_match.group(1))
            checks=[v for v in candidates if isinstance(v,str) and all(c.isalnum() or c=='-' for c in v)]
        passed = 'INSTRUMENTATION_RESULT: acceptance=passed' in text
        verdict = 'passed' if passed else 'blocked' if stage.startswith(('prerequisite', 'os-vpn', 'target-mirrored', 'button-Allow-viewing', 'host-decoded-frames','viewer-Ask-the-host')) else 'failed'
        for name in SCREENSHOTS:
            p = subprocess.run(list(map(str, adb + ['exec-out', 'run-as', 'au.fintex.mnet', 'cat', 'files/control-' + name + '.png'])), capture_output=True, timeout=10)
            if p.returncode == 0 and p.stdout.startswith(b'\x89PNG\r\n\x1a\n'):
                (artifacts / (name + '.png')).write_bytes(p.stdout)
        if passed:
            stage = 'complete'
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError):
        pass
    finally:
        summary = {'verdict': verdict, 'stage': stage, 'passed_checks':checks, 'api': os.environ['MNET_CONTROL_API'], 'commit': os.environ['GITHUB_SHA'], 'scope': 'disposable emulator control journey only; no DNS/bulk/tracing or physical/OEM proof'}
        (artifacts / 'summary.json').write_text(json.dumps(summary, indent=2) + '\n')
        print(json.dumps(summary))
        for p in reversed(processes):
            if p.poll() is None:
                p.terminate()
        for p in processes:
            try:
                p.wait(timeout=10)
            except subprocess.TimeoutExpired:
                p.kill()
                p.wait()
        for handle in handles:
            handle.close()
        shutil.rmtree(root)
    if verdict != 'passed':
        raise SystemExit(1)


if __name__ == '__main__':
    main()
