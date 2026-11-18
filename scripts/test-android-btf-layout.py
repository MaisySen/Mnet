#!/usr/bin/env python3
import importlib.util
import struct
import unittest
spec=importlib.util.spec_from_file_location('btf','scripts/android-btf-layout.py')
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class Layout(unittest.TestCase):
    def test_named_and_anonymous_offsets(self):
        strings=b'\0u16\0inner\0number\0outer\0nested\0'
        def name(n):return strings.index(n.encode()+b'\0')
        def base(n,k,count,size):return struct.pack('<III',name(n) if n else 0,k<<24|count,size)
        def member(n,typ,bits):return struct.pack('<III',name(n) if n else 0,typ,bits)
        types=base('u16',1,0,2)+struct.pack('<I',16)
        types+=base('inner',4,1,8)+member('number',1,16)
        types+=base('',5,1,8)+member('nested',2,0)
        types+=base('outer',4,1,16)+member('',3,64)
        header=struct.pack('<HBBIIIII',0xeb9f,1,0,24,0,len(types),len(types),len(strings))
        b=m.BTF(header+types+strings)
        self.assertEqual(b.offset('outer','nested.number'),10)
        with self.assertRaises(KeyError):b.offset('outer','missing')
    def test_rejects_unknown_version(self):
        with self.assertRaises(ValueError):m.BTF(struct.pack('<HBBIIIII',0xeb9f,2,0,24,0,0,0,1)+b'\0')
unittest.main()
