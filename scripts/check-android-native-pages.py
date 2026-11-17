#!/usr/bin/env python3
"""Reject Android AAR native load segments that cannot map on 16KiB pages."""
import argparse
from pathlib import Path
import struct
import zipfile


def check_elf(data):
    if data[:4] != b'\x7fELF' or data[5] != 1:
        raise ValueError('expected little-endian ELF')
    if data[4] == 2:
        offset = struct.unpack_from('<Q', data, 32)[0]
        size, count = struct.unpack_from('<HH', data, 54)
        fmt = '<IIQQQQQQ'
    elif data[4] == 1:
        offset = struct.unpack_from('<I', data, 28)[0]
        size, count = struct.unpack_from('<HH', data, 42)
        fmt = '<IIIIIIII'
    else:
        raise ValueError('unsupported ELF class')
    loads = 0
    for i in range(count):
        fields = struct.unpack_from(fmt, data, offset + size * i)
        if fields[0] != 1:
            continue
        loads += 1
        fileoff, address, alignment = (fields[2], fields[3], fields[7]) if data[4] == 2 else (fields[1], fields[2], fields[7])
        if alignment < 16384 or fileoff % 16384 != address % 16384:
            raise ValueError('native LOAD segment is not 16KiB-aligned')
    if not loads:
        raise ValueError('ELF contains no LOAD segments')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('aar', type=Path)
    args = parser.parse_args()
    with zipfile.ZipFile(args.aar) as archive:
        libs = [n for n in archive.namelist() if n.startswith('jni/') and n.endswith('.so')]
        if not libs:
            raise ValueError('AAR contains no native libraries')
        for name in libs:
            try:
                check_elf(archive.read(name))
            except (ValueError, struct.error, IndexError) as error:
                raise ValueError(name + ': invalid 16KiB ELF layout') from error
            print('PASS 16KiB ELF:', name)


if __name__ == '__main__':
    main()
