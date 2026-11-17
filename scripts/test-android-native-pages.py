#!/usr/bin/env python3
"""Synthetic ELF layout checks. These are not Android runtime tests."""
import importlib.util
from pathlib import Path
import struct
import unittest

spec = importlib.util.spec_from_file_location('pages', Path(__file__).with_name('check-android-native-pages.py'))
pages = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pages)


def elf(bits=64, alignment=16384, fileoff=0, address=0):
    data = bytearray(256)
    data[:6] = b'\x7fELF' + bytes([2 if bits == 64 else 1, 1])
    if bits == 64:
        struct.pack_into('<Q', data, 32, 64)
        struct.pack_into('<HH', data, 54, 56, 1)
        struct.pack_into('<IIQQQQQQ', data, 64, 1, 4, fileoff, address, address, 1, 1, alignment)
    else:
        struct.pack_into('<I', data, 28, 64)
        struct.pack_into('<HH', data, 42, 32, 1)
        struct.pack_into('<IIIIIIII', data, 64, 1, fileoff, address, address, 1, 1, 4, alignment)
    return data


class Pages(unittest.TestCase):
    def test_64bit_aligned(self): pages.check_elf(elf())
    def test_32bit_aligned(self): pages.check_elf(elf(32))
    def test_4k_rejected(self):
        for bits in [32, 64]:
            with self.assertRaises(ValueError): pages.check_elf(elf(bits, 4096))
    def test_incongruent_rejected(self):
        with self.assertRaises(ValueError): pages.check_elf(elf(address=4096))
    def test_congruent_nonzero(self): pages.check_elf(elf(fileoff=123, address=16384 + 123))
    def test_not_elf(self):
        with self.assertRaises(ValueError): pages.check_elf(b'invalid')
    def test_no_load(self):
        data = elf()
        struct.pack_into('<H', data, 56, 0)
        with self.assertRaises(ValueError): pages.check_elf(data)


if __name__ == '__main__': unittest.main()
