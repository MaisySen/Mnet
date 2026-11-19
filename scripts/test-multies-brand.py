#!/usr/bin/env python3
"""Source/provenance checks, not visual acceptance or runtime integration."""
from pathlib import Path
import hashlib,json,re,unittest,xml.etree.ElementTree as ET
ROOT=Path(__file__).resolve().parents[1]
class BrandTests(unittest.TestCase):
 def test_copied_brand_assets_remain_exact(self):
  root=ROOT/'brand/multies';provenance=json.loads((root/'provenance.json').read_text())
  for name,digest in provenance['copied_sha256'].items():
   self.assertEqual(hashlib.sha256((root/name).read_bytes()).hexdigest(),digest)
  for name,digest in provenance['android_artwork_sha256'].items():
   self.assertEqual(hashlib.sha256((ROOT/'android/app/src/main/res/drawable'/name).read_bytes()).hexdigest(),digest)
 def test_default_multies_palette_not_independent_teal(self):
  tokens=json.loads((ROOT/'brand/multies/tokens.json').read_text())
  self.assertEqual(tokens['color']['light']['accent'],'#0066CC')
  self.assertEqual(tokens['color']['light']['background'],'#F5F5F7')
  native=json.loads((ROOT/'brand/multies/provenance.json').read_text())['native_mapping']
  self.assertEqual(native['background'],'#F2F2F7')
  self.assertEqual(native['card_radius'],18)
  self.assertEqual(native['control_radius'],12)
  for name in ['internal/control/account_page.go','internal/control/dashboard.go']:
   text=(ROOT/name).read_text()
   for color in ['#0066cc','#0058b0','#f5f5f7','#1d1d1f','#5a5a5f','#e8e8ed']:
    self.assertIn(color,text)
   for retired in ['#086963','#f4f7f6','#b66022','#4d6269']:
    self.assertNotIn(retired,text)
 def test_android_mark_geometry_preserved(self):
  tree=ET.parse(ROOT/'android/app/src/main/res/drawable/multies_mark.xml')
  ns='{http://schemas.android.com/apk/res/android}'
  group=tree.getroot().find('group');self.assertEqual(group.attrib[ns+'translateY'],'0.75')
  m,spark=group.findall('path')
  self.assertEqual(m.attrib[ns+'pathData'],'M20,79 L20,39 L50,67 L80,39 L80,79')
  self.assertEqual(m.attrib[ns+'strokeWidth'],'21')
  self.assertEqual(m.attrib[ns+'strokeLineCap'],'round')
  self.assertEqual(m.attrib[ns+'strokeLineJoin'],'round')
  self.assertEqual(m.attrib[ns+'strokeColor'],'#1D1D1F')
  self.assertEqual(spark.attrib[ns+'fillColor'],'#FF6A1A')
 def test_orange_confined_to_logo_geometry(self):
  for name in ['internal/control/account_page.go','internal/control/dashboard.go']:
   text=(ROOT/name).read_text()
   without_svg=re.sub(r'<svg\b.*?</svg>','',text,flags=re.S)
   self.assertNotIn('#ff6a1a',without_svg.lower())
 def test_native_tokens_and_mark_not_runtime_auth(self):
  android=(ROOT/'android/app/src/main/java/au/fintex/mnet/AppUi.java').read_text()
  for token in ['Color.rgb(29, 29, 31)','Color.rgb(0, 102, 204)','Color.rgb(242, 242, 247)','g.setCornerRadius(dp(c, 12))','R.drawable.multies_mark']:
   self.assertIn(token,android)
  swift=(ROOT/'ios/MnetConnectionPanel.swift').read_text()
  self.assertIn('MultiesBrandMark().frame(width: 40, height: 40)',swift)
  self.assertIn('StrokeStyle(lineWidth: 21, lineCap: .round, lineJoin: .round)',swift)
  self.assertNotIn('UserDefaults',swift)
if __name__=='__main__':unittest.main()
