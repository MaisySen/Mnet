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
import threading
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
        (test / 'AndroidManifest.xml').write_text('''<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="au.fintex.mnet.fixture"><uses-sdk android:minSdkVersion="24" android:targetSdkVersion="36"/><uses-permission android:name="android.permission.INTERNET"/><uses-permission android:name="android.permission.ACCESS_NETWORK_STATE"/><application android:label="Mnet disposable fixture"/><instrumentation android:name="au.fintex.mnet.fixture.IndependentAppInstrumentation" android:targetPackage="au.fintex.mnet.fixture"/><instrumentation android:name="au.fintex.mnet.fixture.MeshInstrumentation" android:targetPackage="au.fintex.mnet"/></manifest>''')
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
        # Disposable emulator only: remove transition animation from evidence
        # capture, not from the shipping app or any personal device.
        for setting in ('window_animation_scale', 'transition_animation_scale', 'animator_duration_scale'):
            run(adb + ['shell', 'settings', 'put', 'global', setting, '0'])
            if run(adb + ['shell', 'settings', 'get', 'global', setting]).strip() not in ('0', '0.0'):
                raise RuntimeError('disposable animation setting not applied')
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
            # Keep this fixed summary even if a later recorder step fails.
            independent_summary_lines = [line for line in independent.splitlines() if line.startswith('INSTRUMENTATION_RESULT: summary=')]
            (root / 'independent-summary.txt').write_text('\n'.join(independent_summary_lines) + '\n')
            stage = 'actual-screen-projection'
            run(adb + ['shell', 'run-as', 'au.fintex.mnet', 'touch', 'files/fixture-peer-done'])
            def screen_flag(name):
                try:
                    return json.loads((root / 'screen-proof.json').read_text()).get(name) is True
                except (OSError, ValueError):
                    return False
            stage = 'consent-record-start'
            wait(lambda: subprocess.run(list(map(str, adb + ['shell', 'run-as', 'au.fintex.mnet', 'test', '-f', 'files/fixture-record-start'])), capture_output=True, timeout=10).returncode == 0 or instrument.poll() is not None, 'consent-record-start', 90)
            recording = None
            recorder_stop = threading.Event()
            recorded_frames = []
            if instrument.poll() is None:
                stage = 'consent-recorder-launch'
                # Sample actual pixels externally: Android screenrecord on these
                # AVDs can end early during Settings/projection transitions.
                # This is a sampled walkthrough, never a continuous video claim.
                def sample_consent():
                    while not recorder_stop.is_set():
                        try:
                            frame = subprocess.run(list(map(str, adb + ['exec-out', 'screencap', '-p'])), capture_output=True, timeout=10)
                            if frame.returncode == 0 and frame.stdout.startswith(b'\x89PNG\r\n\x1a\n'):
                                path = root / ('consent-%05d.png' % len(recorded_frames))
                                path.write_bytes(frame.stdout)
                                recorded_frames.append(path)
                        except (OSError, subprocess.TimeoutExpired):
                            pass
                        recorder_stop.wait(0.4)
                recording = threading.Thread(target=sample_consent, daemon=True)
                recording.start()
                time.sleep(1)
                run(adb + ['shell', 'run-as', 'au.fintex.mnet', 'touch', 'files/fixture-record-ready'])
            def input_target():
                value = subprocess.run(list(map(str, adb + ['exec-out', 'run-as', 'au.fintex.mnet', 'cat', 'files/fixture-input-target.json'])), capture_output=True, timeout=10)
                if value.returncode: return False
                point = json.loads(value.stdout)
                if set(point) != {'x', 'y', 'width', 'height'} or not all(isinstance(v, int) and v >= 0 for v in point.values()):
                    raise RuntimeError('invalid fixture target')
                (root / 'input-target.json').write_text(json.dumps(point))
                return True
            stage = 'remote-target-ready'
            wait(lambda: input_target() or instrument.poll() is not None, 'input-target', 90)
            stage = 'projection-ready'
            wait(lambda: subprocess.run(list(map(str, adb + ['shell', 'run-as', 'au.fintex.mnet', 'test', '-f', 'files/fixture-projection-ready'])), capture_output=True, timeout=10).returncode == 0 or instrument.poll() is not None, 'projection-ready', 90)
            (root / 'screen-start').touch()
            stage = 'peer-frame-ready'
            wait(lambda: screen_flag('decoded_frames') or instrument.poll() is not None, 'screen-frames', 100)
            if instrument.poll() is None:
                run(adb + ['shell', 'run-as', 'au.fintex.mnet', 'touch', 'files/fixture-screen-received'])
                stage = 'peer-stop-ready'
                wait(lambda: screen_flag('closed_after_stop') or instrument.poll() is not None, 'screen-stop', 75)
                run(adb + ['shell', 'run-as', 'au.fintex.mnet', 'touch', 'files/fixture-screen-closed'])
                wait(lambda: subprocess.run(list(map(str, adb + ['shell', 'run-as', 'au.fintex.mnet', 'test', '-f', 'files/fixture-record-stop'])), capture_output=True, timeout=10).returncode == 0 or instrument.poll() is not None, 'consent-record-stop', 15)
                if recording is not None:
                    # Only the single recorder this disposable fixture started.
                    stage = 'consent-recorder-stop'
                    recorder_stop.set()
                    recording.join(timeout=15)
                    if recording.is_alive() or not recorded_frames:
                        raise RuntimeError('controlled screenshot sampler unavailable')
                    stage = 'consent-walkthrough-encode'
                    run(['ffmpeg', '-hide_banner', '-loglevel', 'error', '-framerate', '2', '-i', root / 'consent-%05d.png', '-vf', 'scale=540:-2', '-c:v', 'libx264', '-pix_fmt', 'yuv420p', ARTIFACTS / 'consent.mp4'])
                stage = 'consent-recorder-acknowledge'
                run(adb + ['shell', 'run-as', 'au.fintex.mnet', 'touch', 'files/fixture-record-stopped'])
        stage = 'primary-instrumentation-exit'
        instrument.wait(timeout=90)
        output = (root / 'instrumentation.log').read_text()
        stage = 'launch-image-collection'
        screenshot = subprocess.run(list(map(str, adb + ['exec-out', 'run-as', 'au.fintex.mnet', 'cat', 'files/fixture-launch.png'])), capture_output=True, timeout=15)
        if screenshot.returncode or not screenshot.stdout.startswith(b'\x89PNG\r\n\x1a\n'):
            raise RuntimeError('sanitized launch screenshot unavailable')
        (ARTIFACTS / 'launch.png').write_bytes(screenshot.stdout)
        stage = 'disclosure-images-collection'
        privacy = subprocess.run(list(map(str, adb + ['exec-out', 'run-as', 'au.fintex.mnet', 'cat', 'files/fixture-privacy.png'])), capture_output=True, timeout=15)
        if privacy.returncode == 0 and privacy.stdout.startswith(b'\x89PNG\r\n\x1a\n'):
            (ARTIFACTS / 'privacy.png').write_bytes(privacy.stdout)
        consent = subprocess.run(list(map(str, adb + ['exec-out', 'run-as', 'au.fintex.mnet', 'cat', 'files/fixture-consent.png'])), capture_output=True, timeout=15)
        if consent.returncode == 0 and consent.stdout.startswith(b'\x89PNG\r\n\x1a\n'):
            (ARTIFACTS / 'consent.png').write_bytes(consent.stdout)
        vpn_disclosure = subprocess.run(list(map(str, adb + ['exec-out', 'run-as', 'au.fintex.mnet', 'cat', 'files/fixture-vpn-disclosure.png'])), capture_output=True, timeout=15)
        if vpn_disclosure.returncode == 0 and vpn_disclosure.stdout.startswith(b'\x89PNG'):
            (ARTIFACTS / 'vpn-disclosure.png').write_bytes(vpn_disclosure.stdout)
        stage = 'primary-summary-validation'
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
        stage = 'independent-summary-validation'
        second = [line for line in independent.splitlines() if line.startswith('INSTRUMENTATION_RESULT: summary=')]
        stage = 'fixture-app-stop'
        run(adb + ['shell', 'am', 'force-stop', 'au.fintex.mnet'])
        stage = 'independent-summary-validation'
        if len(second) != 1 or 'summary=PASS independent app unbound TCP' not in second[0] or 'INSTRUMENTATION_CODE: -1' not in independent:
            (ARTIFACTS / 'instrumentation.txt').write_text('\n'.join(safe + second) + '\n')
            raise RuntimeError('independent app mesh TCP failed')
        (ARTIFACTS / 'instrumentation.txt').write_text('\n'.join(safe + second) + '\n')
        (ARTIFACTS / 'summary.json').write_text(json.dumps({'result': 'pass', 'scope': 'actual target36 APK/JNI/VPN, OS/UDP/TCP DNS before and after reconnect and independent app UID, independent app unbound TCP, hash reconnect, accessibility/projection/viewer consent, remote mesh gesture, decoded frames and stop', 'physical_device': False, 'remote_capture': True, 'remote_input': True, 'play_protect_install': False}, indent=2))
        print('PASS actual emulator VPN/JNI and real Go peer; physical/capture/input/Play Protect proof remains open')
    except Exception as failure:
        # Preserve only the existing fixed summary if an orchestration wait
        # fails before the normal collection point. Never expose raw am output.
        for name in ('instrumentation',):
            path = root / (name + '.log')
            if path.exists():
                lines = [line for line in path.read_text(errors='replace').splitlines() if line.startswith('INSTRUMENTATION_RESULT: summary=')]
                if lines:
                    (ARTIFACTS / 'instrumentation.txt').write_text('\n'.join(lines) + '\n')
        independent_summary = root / 'independent-summary.txt'
        if independent_summary.exists():
            existing = ARTIFACTS / 'instrumentation.txt'
            existing.write_text((existing.read_text() if existing.exists() else '') + independent_summary.read_text())
        diagnostic = {'result': 'fail', 'stage': stage, 'error_type': type(failure).__name__}
        try:
            flags = json.loads((root / 'screen-proof.json').read_text())
            diagnostic['peer_screen'] = {name: flags.get(name) is True for name in ('viewer_connected', 'geometry_received', 'one_decoded_frame', 'decoded_frames', 'closed_after_stop')}
        except (OSError, ValueError):
            pass
        # Stage is written from fixture-controlled literal await labels only.
        allowed = {'hosting-activity-finish', 'main-activity-idle-after-hosting', 'final-stop-click', 'final-stop', 'complete', 'consent-record-ready', 'accessibility-disclosure', 'accessibility-system-enable', 'accessibility-return-host', 'screen-projection-consent', 'screen-viewer-consent', 'screen-decoded-by-peer', 'remote-mesh-gesture-click', 'screen-stop-closed-peer', 'consent-record-stopped'}
        if 'adb' in locals():
            try:
                probe = subprocess.run(list(map(str, adb + ['exec-out', 'run-as', 'au.fintex.mnet', 'cat', 'files/fixture-stage'])), capture_output=True, timeout=10)
                value = probe.stdout.decode(errors='replace').strip()
                if value in allowed:
                    diagnostic['android_stage'] = value
            except (OSError, subprocess.TimeoutExpired):
                diagnostic['android_stage_unavailable'] = True
        if 'instrument' in locals():
            diagnostic['instrument_process_exited'] = instrument.poll() is not None
            primary = (root / 'instrumentation.log').read_text(errors='replace')
            diagnostic['primary_summary_present'] = 'INSTRUMENTATION_RESULT: summary=' in primary
            diagnostic['primary_code_success'] = 'INSTRUMENTATION_CODE: -1' in primary
            diagnostic['primary_short_error'] = 'shortMsg=' in primary
            diagnostic['primary_process_crash'] = 'Process crashed' in primary or 'process has crashed' in primary
            diagnostic['primary_code_zero'] = 'INSTRUMENTATION_CODE: 0' in primary
        if 'adb' in locals():
            try:
                crash = subprocess.run(list(map(str, adb + ['logcat', '-b', 'crash', '-d', '-t', '300'])), capture_output=True, timeout=10).stdout.decode(errors='replace')
                diagnostic['native_signal'] = 'Fatal signal' in crash or 'signal 11' in crash
                diagnostic['go_panic'] = 'panic:' in crash or 'fatal error:' in crash
                diagnostic['java_fatal'] = 'FATAL EXCEPTION' in crash
                diagnostic['crash_oom'] = 'OutOfMemoryError' in crash
            except (OSError, subprocess.TimeoutExpired):
                diagnostic['crash_diagnostics_unavailable'] = True
        (ARTIFACTS / 'summary.json').write_text(json.dumps(diagnostic, indent=2))
        raise RuntimeError('Android fixture failed at ' + stage) from None
    finally:
        if 'recorder_stop' in locals():
            recorder_stop.set()
        if 'recording' in locals() and recording is not None:
            recording.join(timeout=15)
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
