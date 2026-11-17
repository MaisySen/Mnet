#!/usr/bin/env python3
"""Disposable standard GitHub Linux runner: actual Android VPN/JNI + Go peer.
No production credentials or services. No APK/state/logcat artifact exports.
"""
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
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
SDK = Path(os.environ.get('ANDROID_HOME', ''))
ADB = SDK / 'platform-tools/adb'
ARTIFACTS = Path(os.environ.get('MNET_ANDROID_ARTIFACTS', '/tmp/mnet-android-artifacts'))


def run(args, **kwargs):
    value = subprocess.run(list(map(str, args)), text=True, capture_output=True, timeout=kwargs.pop('timeout', 90), **kwargs)
    if value.returncode:
        # Never echo instrument arguments/config, logs or private fixture keys.
        raise RuntimeError('fixture child command failed')
    return value.stdout


def wait(probe, label, seconds=90):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        try:
            result = probe()
            if result:
                return result
        except (OSError, ValueError, RuntimeError):
            pass
        time.sleep(0.25)
    raise RuntimeError('bounded fixture readiness failed: ' + label)


def main():
    if os.environ.get('GITHUB_ACTIONS') != 'true' or sys.platform != 'linux':
        raise RuntimeError('disposable GitHub Linux runner only')
    if not Path('/dev/kvm').exists() or not ADB.exists():
        raise RuntimeError('emulator acceleration/platform tools unavailable')
    ARTIFACTS.mkdir(parents=True, exist_ok=True)
    root = Path(tempfile.mkdtemp(prefix='mnet-android-fixture-'))
    os.chmod(root, 0o700)
    processes, logs = [], []
    stage = 'prepare'

    def start(args, name, **kwargs):
        log = (root / (name + '.log')).open('w')
        logs.append(log)
        process = subprocess.Popen(list(map(str, args)), stdout=log, stderr=subprocess.STDOUT, **kwargs)
        processes.append(process)
        return process

    try:
        # No production manifests/keystores are modified. Cleartext loopback and
        # debuggable exist only in this throwaway engineering APK, never shipped.
        stage = 'fixture-build'
        staged = root / 'source'
        staged.mkdir()
        archive = subprocess.Popen(['git', 'archive', 'HEAD'], cwd=ROOT, stdout=subprocess.PIPE)
        run(['tar', '-x', '-C', staged], stdin=archive.stdout)
        archive.stdout.close()
        if archive.wait() != 0:
            raise RuntimeError('source archive failed')
        manifest = staged / 'android/app/src/main/AndroidManifest.xml'
        tree = ET.parse(manifest)
        app = tree.getroot().find('application')
        namespace = '{http://schemas.android.com/apk/res/android}'
        app.set(namespace + 'usesCleartextTraffic', 'true')
        app.set(namespace + 'debuggable', 'true')
        # Read OS VPN publication in instrumentation, not a production permission.
        ET.SubElement(tree.getroot(), 'uses-permission', {namespace + 'name': 'android.permission.ACCESS_NETWORK_STATE'})
        tree.write(manifest, encoding='utf-8', xml_declaration=True)
        env = os.environ.copy()
        env.pop('MNET_RELEASE_KEYSTORE', None)
        env.pop('MNET_RELEASE_PASS_FILE', None)
        run(['sh', staged / 'android/build.sh', os.environ['MNET_ANDROID_AAR']], cwd=staged, env=env, timeout=180)
        appout = staged / 'android/out'
        bt = SDK / 'build-tools' / os.environ.get('MNET_ANDROID_BUILD_TOOLS', '36.0.0')
        android = SDK / ('platforms/android-' + os.environ.get('MNET_ANDROID_COMPILE_API', '36')) / 'android.jar'
        test = root / 'test'
        test.mkdir()
        (test / 'classes').mkdir()
        (test / 'dex').mkdir()
        (test / 'AndroidManifest.xml').write_text('''<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="au.fintex.mnet.fixture"><uses-sdk android:minSdkVersion="24" android:targetSdkVersion="36"/><uses-permission android:name="android.permission.INTERNET"/><application android:label="Mnet disposable fixture"/><instrumentation android:name="au.fintex.mnet.fixture.IndependentAppInstrumentation" android:targetPackage="au.fintex.mnet.fixture"/><instrumentation android:name="au.fintex.mnet.fixture.MeshInstrumentation" android:targetPackage="au.fintex.mnet"/></manifest>''')
        run([bt / 'aapt2', 'link', '-o', test / 'base.apk', '-I', android, '--manifest', test / 'AndroidManifest.xml'])
        run(['javac', '-source', '11', '-target', '11', '-cp', str(android) + ':' + str(appout / 'classes') + ':' + str(appout / 'aar/classes.jar'), '-d', test / 'classes', *sorted((staged / 'android/instrumentation').rglob('*.java'))])
        run(['jar', 'cf', test / 'target.jar', '-C', appout / 'classes', '.'])
        run([bt / 'd8', '--min-api', '24', '--lib', android, '--classpath', test / 'target.jar', '--classpath', appout / 'aar/classes.jar', '--output', test / 'dex', *sorted((test / 'classes').rglob('*.class'))])
        shutil.copy(test / 'base.apk', test / 'unaligned.apk')
        run(['zip', '-q', test / 'unaligned.apk', 'classes.dex'], cwd=test / 'dex')
        run([bt / 'zipalign', '-f', '4', test / 'unaligned.apk', test / 'unsigned.apk'])
        run([bt / 'apksigner', 'sign', '--ks', staged / 'android/debug.keystore', '--ks-key-alias', 'mnet-debug', '--ks-pass', 'pass:android', '--key-pass', 'pass:android', '--out', test / 'fixture.apk', test / 'unsigned.apk'])

        # Bind only loopback. Emulator uses adb reverse for this endpoint.
        stage = 'isolated-control'
        with socket.socket() as check:
            check.bind(('127.0.0.1', 8080))
        base = 'http://127.0.0.1:8080'
        binaries = Path(os.environ['MNET_ANDROID_BIN'])
        start([binaries / 'mnet-control', '-listen', '127.0.0.1:8080', '-base-url', base, '-data', root / 'control.json', '-admin-password', 'android-fixture-only'], 'control')
        wait(lambda: urllib.request.urlopen(base + '/healthz', timeout=2).status == 200, 'control')
        opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

        def admin(path, body):
            req = urllib.request.Request(base + path, json.dumps(body).encode(), {'Content-Type': 'application/json'})
            return json.load(opener.open(req, timeout=3))

        admin('/admin/api/login', {'password': 'android-fixture-only'})
        network = admin('/admin/api/networks', {'name': 'android-emulator'})
        key = admin('/admin/api/authkeys', {'network_id': network['id'], 'reusable': True})['key']
        peer_config = root / 'peer.json'
        peer_config.write_text(json.dumps({'Server': base, 'Network': 'android-emulator', 'Key': key, 'Root': str(root)}))
        os.chmod(peer_config, 0o600)
        peer_env = os.environ.copy()
        peer_env['MNET_NATIVE_PEER_CONFIG'] = str(peer_config)
        start([binaries / 'native-peer', '-test.run', '^TestNativeKernelPeerFixture$', '-test.timeout', '20m'], 'peer', env=peer_env)
        ready = root / 'peer-ready.json'
        peer = wait(lambda: json.loads(ready.read_text()) if ready.exists() else None, 'real-go-peer')

        stage = 'emulator-boot'
        free_mib = shutil.disk_usage(root).free // (1024 * 1024)
        emulator = start([SDK / 'emulator/emulator', '-avd', 'mnet-fixture', '-port', '5554', '-no-window', '-no-audio', '-no-boot-anim', '-no-snapshot', '-gpu', 'swiftshader', '-partition-size', '2048', '-memory', '2048', '-cores', '2'], 'emulator')
        adb = [ADB, '-s', 'emulator-5554']
        def booted():
            if emulator.poll() is not None:
                raise ChildProcessError('emulator exited before boot')
            return run(adb + ['shell', 'getprop', 'sys.boot_completed'], timeout=8).strip() == '1'
        try:
            wait(booted, 'emulator-boot', 300)
        except (RuntimeError, ChildProcessError):
            # Only fixed classifications, never raw logs, paths or user data.
            text = (root / 'emulator.log').read_text(errors='replace').lower()
            categories = {
                'missing_shared_library': 'error while loading shared libraries' in text,
                'disk_space': any(marker in text for marker in ('not enough disk', 'not enough space', 'no space left', 'insufficient disk')),
                'avd_missing': 'unknown avd name' in text or 'cannot find avd' in text,
                'unsupported_gpu': 'invalid gpu mode' in text or 'invalid gpu option' in text,
                'cpu_feature': 'requires avx' in text or 'requires x86_64 cpu' in text,
                'avd_lock': 'multiple emulators' in text or 'another emulator' in text,
                'image_missing': 'broken avd system path' in text or 'cannot find system' in text,
                'acceleration_unavailable': 'kvm is not installed' in text or 'kvm permission denied' in text,
                'gpu_initialization': 'failed to initialize' in text and ('vulkan' in text or 'opengl' in text),
                'emulator_exited': emulator.poll() is not None,
            }
            (ARTIFACTS / 'instrumentation.txt').write_text(json.dumps({'boot_diagnostics': categories, 'emulator_exit_code': emulator.poll(), 'pre_boot_free_mib': free_mib}) + '\n')
            raise
        run(adb + ['shell', 'input', 'keyevent', '82'])
        run(adb + ['reverse', 'tcp:8080', 'tcp:8080'])
        run(adb + ['install', '-r', appout / 'mnet.apk'])
        run(adb + ['install', '-r', test / 'fixture.apk'])
        stage = 'actual-instrumentation'
        instrument = start(adb + ['shell', 'am', 'instrument', '-w', '-r', '-e', 'server', base, '-e', 'key', key, '-e', 'peer', peer['ipv4'], 'au.fintex.mnet.fixture/au.fintex.mnet.fixture.MeshInstrumentation'], 'instrumentation')
        def peer_ready():
            if instrument.poll() is not None:
                return True  # collect the safe failure below, not a raw log
            check = subprocess.run(list(map(str, adb + ['shell', 'run-as', 'au.fintex.mnet', 'test', '-f', 'files/fixture-peer-ready'])), capture_output=True, timeout=10)
            return check.returncode == 0
        wait(peer_ready, 'independent-app-ready', 300)
        independent = ''
        if instrument.poll() is None:
            stage = 'independent-app-unbound-tcp'
            independent = run(adb + ['shell', 'am', 'instrument', '-w', '-r', '-e', 'peer', peer['ipv4'], 'au.fintex.mnet.fixture/au.fintex.mnet.fixture.IndependentAppInstrumentation'], timeout=150)
            run(adb + ['shell', 'run-as', 'au.fintex.mnet', 'touch', 'files/fixture-peer-done'])
        instrument.wait(timeout=90)
        output = (root / 'instrumentation.log').read_text()
        screenshot = subprocess.run(list(map(str, adb + ['exec-out', 'run-as', 'au.fintex.mnet', 'cat', 'files/fixture-launch.png'])), capture_output=True, timeout=15)
        if screenshot.returncode or not screenshot.stdout.startswith(b'\x89PNG\r\n\x1a\n'):
            raise RuntimeError('sanitized launch screenshot unavailable')
        (ARTIFACTS / 'launch.png').write_bytes(screenshot.stdout)
        safe = [line for line in output.splitlines() if line.startswith('INSTRUMENTATION_RESULT: summary=')]
        if len(safe) != 1 or 'actual APK/JNI/VPN consent' not in safe[0] or 'summary=PASS self-bound target36 runtimeAPI=' not in safe[0] or 'INSTRUMENTATION_CODE: -1' not in output:
            # Only explicit whitelist summary is exported, never the whole am
            # output (which can include private runner arguments on a crash).
            if safe:
                # Fixed booleans only, not a blind export of a private file.
                try:
                    packets = json.loads((root / 'peer-packets.json').read_text())
                    names = ('peerInboundTCP', 'peerOutboundTCP', 'peerInboundSYN', 'peerOutboundSYNACK')
                    safe[0] += ''.join(' ' + name + '=' + str(packets.get(name) is True).lower() for name in names)
                except (OSError, ValueError):
                    safe[0] += ' peerDiagnosticsUnavailable=true'
                (ARTIFACTS / 'instrumentation.txt').write_text('\n'.join(safe) + '\n')
            raise RuntimeError('actual Android integration not passed')
        second = [line for line in independent.splitlines() if line.startswith('INSTRUMENTATION_RESULT: summary=')]
        run(adb + ['shell', 'am', 'force-stop', 'au.fintex.mnet'])
        if len(second) != 1 or 'summary=PASS independent app unbound TCP' not in second[0] or 'INSTRUMENTATION_CODE: -1' not in independent:
            (ARTIFACTS / 'instrumentation.txt').write_text('\n'.join(safe + second) + '\n')
            raise RuntimeError('independent app mesh TCP failed')
        (ARTIFACTS / 'instrumentation.txt').write_text('\n'.join(safe + second) + '\n')
        (ARTIFACTS / 'summary.json').write_text(json.dumps({'result': 'pass', 'scope': 'actual disposable emulator APK/JNI/VPN, 4MiB hash both directions twice, manual disconnect/reconnect', 'physical_device': False, 'remote_capture_input': False, 'play_protect_install': False}, indent=2))
        print('PASS actual emulator VPN/JNI and real Go peer; physical/capture/input/Play Protect proof remains open')
    except Exception as failure:
        (ARTIFACTS / 'summary.json').write_text(json.dumps({'result': 'fail', 'stage': stage, 'error_type': type(failure).__name__}, indent=2))
        raise RuntimeError('Android fixture failed at ' + stage) from None
    finally:
        for process in reversed(processes):
            process.terminate()
        for process in reversed(processes):
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
        for log in logs:
            log.close()
        shutil.rmtree(root)


if __name__ == '__main__':
    main()
