#!/usr/bin/env python3
"""Inspect the actual disposable linked app/embedded extension, never sign it."""
import pathlib
import plistlib
import subprocess
import sys

app = pathlib.Path(sys.argv[1])
extension = app / 'PlugIns/MnetTunnel.appex'
for bundle, identifier, kind, executable in [
    (app, 'au.fintex.mnet', 'APPL', 'Mnet'),
    (extension, 'au.fintex.mnet.tunnel', 'XPC!', 'MnetTunnel'),
]:
    with (bundle / 'Info.plist').open('rb') as stream:
        info = plistlib.load(stream)
    assert info['CFBundleIdentifier'] == identifier
    assert info['CFBundlePackageType'] == kind
    assert info['CFBundleExecutable'] == executable
    binary = bundle / executable
    assert binary.is_file(), 'linked executable missing'
    file_result = subprocess.check_output(['file', str(binary)], text=True)
    assert 'Mach-O' in file_result and 'arm64' in file_result, file_result
    assert not (bundle / '_CodeSignature').exists(), 'unexpected signature directory'
    signed = subprocess.run(['codesign', '--verify', str(bundle)], capture_output=True, text=True)
    assert signed.returncode != 0 and 'not signed' in signed.stderr, 'unsigned-only boundary not established'
    subprocess.run(['otool', '-L', str(binary)], check=True)
with (extension / 'Info.plist').open('rb') as stream:
    ext_info = plistlib.load(stream)['NSExtension']
assert ext_info['NSExtensionPointIdentifier'] == 'com.apple.networkextension.packet-tunnel'
assert ext_info['NSExtensionPrincipalClass'] == 'MnetTunnel.PacketTunnelProvider'
# Native Go core must be linked into the extension, not merely a Swift API stub.
symbols = subprocess.check_output(['nm', str(extension / 'MnetTunnel')], text=True)
assert 'MobileNewNode' in symbols, 'Go Objective-C entry point not linked'
assert 'crosscall2' in symbols, 'Go runtime bridge not linked'
print('PASS unsigned arm64 simulator app and embedded packet-tunnel executable linked with current Go core; no launch/signing/device/entitlement acceptance')
