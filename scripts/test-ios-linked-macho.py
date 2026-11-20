#!/usr/bin/env python3
"""Synthetic binary parser tests, not an actual Apple product or signature test."""
import importlib.util
from pathlib import Path
import struct
import unittest
import plistlib
import tempfile
from unittest import mock

spec = importlib.util.spec_from_file_location('linked', Path(__file__).with_name('check-ios-linked-product.py'))
linked = importlib.util.module_from_spec(spec)
spec.loader.exec_module(linked)


def build_command(platform=7):
    return struct.pack('<IIIIII', 0x32, 24, platform, 0x100000, 0x100000, 0)


def binary(commands=None, cpu=0x100000c):
    commands = [build_command()] if commands is None else commands
    data = b''.join(commands)
    return struct.pack('<IIIIIIII', 0xfeedfacf, cpu, 0, 2, len(commands), len(data), 0, 0) + data


class LinkedMachO(unittest.TestCase):
    def reject(self, data):
        with self.assertRaises(ValueError):
            linked.validate_unsigned_simulator_macho(data)

    def test_unsigned_arm64_simulator(self):
        linked.validate_unsigned_simulator_macho(binary())

    def test_other_platforms_and_duplicate(self):
        for platform in (0, 1, 2, 6, 8):
            with self.subTest(platform=platform):
                self.reject(binary([build_command(platform)]))
        self.reject(binary([]))
        self.reject(binary([build_command(), build_command()]))

    def test_signature_and_legacy_refused(self):
        for cmd in (0x1d, 0x24, 0x25, 0x2f, 0x30):
            self.reject(binary([build_command(), struct.pack('<II', cmd, 8)]))

    def test_truncated_at_every_byte(self):
        data = binary()
        for end in range(len(data)):
            with self.subTest(end=end):
                self.reject(data[:end])

    def test_wrong_architecture_and_magic(self):
        self.reject(binary(cpu=0x1000007))
        self.reject(b'FAT!' + binary()[4:])

    def test_objects_and_libraries_are_not_executables(self):
        for kind in (1, 6, 8):
            data = bytearray(binary())
            struct.pack_into('<I', data, 12, kind)
            self.reject(data)

    def test_malformed_count_size_and_build_tools(self):
        for offset, value in ((16, 0xffffffff), (16, 2), (20, 8), (36, 0),
                              (36, 16), (36, 25), (52, 1)):
            data = bytearray(binary())
            struct.pack_into('<I', data, offset, value)
            with self.subTest(offset=offset, value=value):
                self.reject(data)
        self.reject(binary([struct.pack('<II', 0x32, 8)]))


class LinkedProduct(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.app = Path(self.temp.name) / 'Mnet.app'
        self.extension = self.app / 'PlugIns/MnetTunnel.appex'
        for bundle, identifier, kind, executable in [
                (self.app, 'au.fintex.mnet', 'APPL', 'Mnet'),
                (self.extension, 'au.fintex.mnet.tunnel', 'XPC!', 'MnetTunnel')]:
            bundle.mkdir(parents=True, exist_ok=True)
            info = dict(CFBundleIdentifier=identifier, CFBundlePackageType=kind,
                        CFBundleExecutable=executable)
            if bundle == self.extension:
                info['NSExtension'] = dict(
                    NSExtensionPointIdentifier='com.apple.networkextension.packet-tunnel',
                    NSExtensionPrincipalClass='MnetTunnel.PacketTunnelProvider')
            (bundle / 'Info.plist').write_bytes(plistlib.dumps(info))
            (bundle / executable).write_bytes(binary())
        self.symbols = 'MobileNewNode crosscall2'

    def output(self, args, **kwargs):
        if args[0] == 'file':
            return 'Mach-O 64-bit executable arm64'
        if args[0] == 'nm':
            return self.symbols
        raise AssertionError('unexpected executable')

    def check(self):
        with mock.patch.object(linked.subprocess, 'check_output', side_effect=self.output), \
                mock.patch.object(linked.subprocess, 'run') as run, \
                mock.patch('builtins.print'):
            linked.main(self.app)
        self.assertEqual(run.call_count, 2)

    def mutate(self, bundle, key, value):
        path = bundle / 'Info.plist'
        info = plistlib.loads(path.read_bytes())
        info[key] = value
        path.write_bytes(plistlib.dumps(info))

    def test_valid_synthetic_packaging(self):
        self.check()

    def test_bundle_identity_package_and_executable_mismatch(self):
        for bundle in (self.app, self.extension):
            path = bundle / 'Info.plist'
            original = path.read_bytes()
            for key in ('CFBundleIdentifier', 'CFBundlePackageType', 'CFBundleExecutable'):
                with self.subTest(bundle=bundle.name, key=key):
                    self.mutate(bundle, key, 'other')
                    with self.assertRaises(ValueError):
                        self.check()
                    path.write_bytes(original)

    def test_extension_role_mismatch(self):
        path = self.extension / 'Info.plist'
        original = path.read_bytes()
        for key in ('NSExtensionPointIdentifier', 'NSExtensionPrincipalClass'):
            info = plistlib.loads(original)
            info['NSExtension'][key] = 'other'
            path.write_bytes(plistlib.dumps(info))
            with self.assertRaises(ValueError):
                self.check()

    def test_signature_directory_or_missing_executable(self):
        for bundle, executable in ((self.app, 'Mnet'), (self.extension, 'MnetTunnel')):
            signature = bundle / '_CodeSignature'
            signature.mkdir()
            with self.assertRaises(ValueError):
                self.check()
            signature.rmdir()
            (bundle / executable).unlink()
            with self.assertRaises(ValueError):
                self.check()
            (bundle / executable).write_bytes(binary())

    def test_missing_native_bridge_refuses(self):
        for symbols in ('', 'MobileNewNode', 'crosscall2'):
            self.symbols = symbols
            with self.assertRaises(ValueError):
                self.check()


if __name__ == '__main__':
    unittest.main()
