#!/usr/bin/env python3
"""Synthetic binary parser tests, not an actual Apple product or signature test."""
import importlib.util
from pathlib import Path
import struct
import unittest

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

    def test_malformed_count_size_and_build_tools(self):
        for offset, value in ((16, 0xffffffff), (16, 2), (20, 8), (36, 0),
                              (36, 16), (36, 25), (52, 1)):
            data = bytearray(binary())
            struct.pack_into('<I', data, offset, value)
            with self.subTest(offset=offset, value=value):
                self.reject(data)
        self.reject(binary([struct.pack('<II', 0x32, 8)]))


if __name__ == '__main__':
    unittest.main()
