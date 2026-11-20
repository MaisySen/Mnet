#!/usr/bin/env python3
"""Inspect the actual disposable linked app/embedded extension, never sign it."""
import pathlib
import plistlib
import subprocess
import struct
import sys

def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate_unsigned_simulator_macho(data):
    """Bounded thin arm64 simulator validation; no host tools or signing access."""

    require(len(data) >= 32, 'truncated Mach-O64 header')
    require(data[:4] == b'\xcf\xfa\xed\xfe', 'expected thin little-endian Mach-O64')
    cpu, = struct.unpack_from('<I', data, 4)
    require(cpu == 0x100000c, 'expected arm64 CPU')
    filetype, = struct.unpack_from('<I', data, 12)
    require(filetype == 2, 'expected MH_EXECUTE product, not an object or library')
    ncmds, sizeofcmds = struct.unpack_from('<II', data, 16)
    end = 32 + sizeofcmds
    require(end <= len(data), 'invalid Mach-O command region')
    require(ncmds <= sizeofcmds // 8, 'impossible load-command count')
    offset = 32
    platforms = []
    for _ in range(ncmds):
        require(offset + 8 <= end, 'truncated load-command header')
        cmd, size = struct.unpack_from('<II', data, offset)
        require(size >= 8 and size % 8 == 0 and offset + size <= end,
                'invalid load command')
        require(cmd != 0x1d, 'unexpected LC_CODE_SIGNATURE, including ad-hoc signature')
        if cmd == 0x32:  # LC_BUILD_VERSION
            require(size >= 24, 'truncated LC_BUILD_VERSION')
            platform, _, _, ntools = struct.unpack_from('<IIII', data, offset + 8)
            require(size == 24 + 8 * ntools, 'invalid build-version tools')
            platforms.append(platform)
        # A legacy minimum-version command cannot prove this arm64 binary is
        # an iOS simulator product rather than a device or macOS product.
        require(cmd not in (0x24, 0x25, 0x2f, 0x30), 'legacy platform command')
        offset += size
    require(offset == end, 'load-command length mismatch')
    require(platforms == [7], 'expected exactly one iOS simulator build platform')


def main(app):
    extension = app / 'PlugIns/MnetTunnel.appex'
    for bundle, identifier, kind, executable in [
        (app, 'au.fintex.mnet', 'APPL', 'Mnet'),
        (extension, 'au.fintex.mnet.tunnel', 'XPC!', 'MnetTunnel'),
    ]:
        with (bundle / 'Info.plist').open('rb') as stream:
            info = plistlib.load(stream)
        require(info['CFBundleIdentifier'] == identifier, 'bundle identifier mismatch')
        require(info['CFBundlePackageType'] == kind, 'bundle package type mismatch')
        require(info['CFBundleExecutable'] == executable, 'bundle executable mismatch')
        binary = bundle / executable
        require(binary.is_file(), 'linked executable missing')
        file_result = subprocess.check_output(['file', str(binary)], text=True)
        require('Mach-O' in file_result and 'arm64' in file_result, 'file did not identify arm64 Mach-O')
        require(not (bundle / '_CodeSignature').exists(), 'unexpected signature directory')
        # Inspect the actual executable load commands. codesign's failure text is
        # not a stable unsigned predicate (bundle validation may fail for other reasons).
        validate_unsigned_simulator_macho(binary.read_bytes())
        subprocess.run(['otool', '-L', str(binary)], check=True)
    with (extension / 'Info.plist').open('rb') as stream:
        ext_info = plistlib.load(stream)['NSExtension']
    require(ext_info['NSExtensionPointIdentifier'] == 'com.apple.networkextension.packet-tunnel', 'extension point mismatch')
    require(ext_info['NSExtensionPrincipalClass'] == 'MnetTunnel.PacketTunnelProvider', 'extension principal mismatch')
    # Native Go core must be linked into the extension, not merely a Swift API stub.
    symbols = subprocess.check_output(['nm', str(extension / 'MnetTunnel')], text=True)
    require('MobileNewNode' in symbols, 'Go Objective-C entry point not linked')
    require('crosscall2' in symbols, 'Go runtime bridge not linked')
    print('PASS unsigned arm64 simulator app and embedded packet-tunnel executable linked with current Go core; no launch/signing/device/entitlement acceptance')


if __name__ == '__main__':
    main(pathlib.Path(sys.argv[1]))
