#!/usr/bin/env python3
"""Source XML policy tests, not cloud backup or OEM restore execution."""
from pathlib import Path
import unittest
import xml.etree.ElementTree as ET
ROOT=Path(__file__).resolve().parents[1]
APP=ROOT/'android/app/src/main'
A='{http://schemas.android.com/apk/res/android}'
DOMAINS={'root','file','database','sharedpref','external','device_root','device_file','device_database','device_sharedpref'}
class BackupPolicy(unittest.TestCase):
    def test_explicit_opt_out_and_rules(self):
        app=ET.parse(APP/'AndroidManifest.xml').getroot().find('application')
        self.assertEqual('false',app.get(A+'allowBackup'))
        self.assertEqual('@xml/backup_rules',app.get(A+'fullBackupContent'))
        self.assertEqual('@xml/data_extraction_rules',app.get(A+'dataExtractionRules'))
    def exclusions(self,section):
        self.assertIsNotNone(section)
        self.assertEqual([],section.findall('include'))
        self.assertEqual(DOMAINS,{x.get('domain') for x in section.findall('exclude') if x.get('path')=='.'})
        self.assertEqual(len(DOMAINS),len(section))
    def test_legacy_all_domains(self):
        rules=ET.parse(APP/'res/xml/backup_rules.xml').getroot()
        self.assertEqual('full-backup-content',rules.tag)
        self.exclusions(rules)
    def test_cloud_and_device_transfer(self):
        rules=ET.parse(APP/'res/xml/data_extraction_rules.xml').getroot()
        self.assertEqual('data-extraction-rules',rules.tag)
        for mode in ('cloud-backup','device-transfer'):
            with self.subTest(mode=mode):self.exclusions(rules.find(mode))
if __name__=='__main__':unittest.main()
