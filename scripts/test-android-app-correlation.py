import importlib.util
import unittest
s=importlib.util.spec_from_file_location('trace','scripts/android-app-correlation.py');m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
class Parser(unittest.TestCase):
 def test_only_known_fields_and_signed_results(self):
  events=m.parse('worker-123 [001] .... 91.000001: mnet_app_udp: sk=0xffff1234 sport=40001 uid=10100 mark=0x1234 length=71 unrelated=abcdef\nworker-123 [001] .... 91.000002: mnet_app_udpret: result=-1\n')
  self.assertEqual(events[0][0],123);self.assertEqual(events[0][3]['sk'],0xffff1234);self.assertEqual(events[1][3]['result'],-1);self.assertNotIn('unrelated',events[0][3])
 def test_no_unmatched_line_becomes_event(self):self.assertEqual(m.parse('junk key=1'),[])
unittest.main()
