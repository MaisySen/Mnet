#!/usr/bin/env python3
"""Lock the prepared document picker/output behind absent production routes.

This is a source-boundary check, not OS component/permission enforcement proof.
"""
from pathlib import Path
import re
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
APP = ROOT / 'android/app/src/main'
A = '{http://schemas.android.com/apk/res/android}'


def check(manifest, sources):
    failures = []
    for component in ET.fromstring(manifest).iter():
        if 'DocumentSaveActivity' in component.get(A + 'name', ''):
            failures.append('document picker must remain absent from manifest')
    for name, text in sources.items():
        if name not in ('DocumentSaveActivity.java', 'DocumentExport.java'):
            if re.search(r'\b(?:DocumentSaveActivity|DocumentExport)\b', text):
                failures.append('production document picker/output caller added: ' + name)
    return failures


def regression():
    empty = '<manifest xmlns:android="http://schemas.android.com/apk/res/android"><application/></manifest>'
    assert not check(empty, {'MainActivity.java': 'class MainActivity {}'})
    enabled = empty.replace('<application/>', '<application><activity android:name=".DocumentSaveActivity"/></application>')
    assert check(enabled, {})
    for name in ('DocumentSaveActivity', 'DocumentExport'):
        assert check(empty, {'MainActivity.java': name + '.prepare();'})
    assert not check(empty, {'DocumentSaveActivity.java': 'DocumentExport output;'})


if __name__ == '__main__':
    regression()
    sources = {p.name: p.read_text() for p in (APP / 'java').rglob('*.java')}
    failures = check((APP / 'AndroidManifest.xml').read_text(), sources)
    if failures:
        raise SystemExit('\n'.join(failures))
    print('PASS disabled document picker/output source routes and guard mutations; no installed permission/OS proof')
