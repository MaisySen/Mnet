#!/usr/bin/env python3
"""Inspect the actual disposable linked app/embedded extension, never sign it."""
import pathlib
import plistlib
import subprocess
import struct
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
    # Inspect the actual executable load commands. codesign's failure text is
    # not a stable unsigned predicate (bundle validation may fail for other reasons).
    data = binary.read_bytes()
    assert data[:4] == b'\xcf\xfa\xed\xfe', 'expected thin little-endian Mach-O64'
    ncmds, sizeofcmds = struct.unpack_from('<II', data, 16)
    offset = 32
    assert offset + sizeofcmds <= len(data), 'invalid Mach-O command region'
    for _ in range(ncmds):
        cmd, size = struct.unpack_from('<II', data, offset)
        assert size >= 8 and offset + size <= 32 + sizeofcmds, 'invalid load command'
        assert cmd != 0x1d, 'unexpected LC_CODE_SIGNATURE, including ad-hoc signature'
        offset += size
    assert offset == 32 + sizeofcmds, 'load-command length mismatch'
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
