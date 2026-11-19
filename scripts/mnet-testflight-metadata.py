"""Read build numbers, verify the IPA, and observe only Mnet's TestFlight upload."""
import base64
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import plistlib
import re
import stat
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile

API = 'https://api.appstoreconnect.apple.com'
BUNDLE = 'au.fintex.mnet'
TUNNEL = BUNDLE + '.tunnel'
TEAM = 'B85KQRQZW7'

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise ValueError('Unexpected redirect from App Store Connect')


def encode(b):
    return base64.urlsafe_b64encode(b).decode('ascii').rstrip('=')


def unpack_es256(signature):
    # P-256 SEC signatures use DER integers; JWT ES256 expects fixed-width r || s.
    assert len(signature) <= 74 and signature[0] == 0x30 and signature[1] == len(signature) - 2
    pos, parts = 2, []
    for _ in range(2):
        assert signature[pos] == 2
        size = signature[pos + 1]
        assert 1 <= size <= 33 and pos + 2 + size <= len(signature)
        integer = signature[pos + 2:pos + 2 + size]
        assert not integer[0] & 128
        if len(integer) > 1 and integer[0] == 0:
            assert integer[1] & 128
            integer = integer[1:]
        assert any(integer) and len(integer) <= 32
        parts.append(integer.rjust(32, b'\0'))
        pos += 2 + size
    assert pos == len(signature)
    return b''.join(parts)


def credentials():
    kid, issuer = os.environ['ASC_KEY_ID'], os.environ['ASC_ISSUER_ID']
    assert re.fullmatch(r'[A-Z0-9]{10}', kid)
    assert re.fullmatch(r'[a-fA-F0-9]{8}(?:-[a-fA-F0-9]{4}){3}-[a-fA-F0-9]{12}', issuer)
    key = Path.home() / 'private_keys' / ('AuthKey_' + kid + '.p8')
    return kid, issuer, key


def install_key():
    _, _, key = credentials()
    data = base64.b64decode(''.join(os.environ['ASC_KEY_BASE64'].split()), validate=True)
    assert data.startswith(b'-----BEGIN PRIVATE KEY-----') and 100 < len(data) < 4096
    fd = os.open(str(key), os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'wb') as f:
        f.write(data)


def jwt():
    kid, issuer, key = credentials()
    fmt = lambda x: encode(json.dumps(x, separators=(',', ':')).encode())
    clock = int(time.time())
    value = fmt({'alg': 'ES256','kid': kid,'typ': 'JWT'}) + '.' + fmt({'iss': issuer,'iat': clock,'exp': clock+360,'aud': 'appstoreconnect-v1'})
    result = subprocess.run(['/usr/bin/openssl','dgst','-sha256','-sign',str(key)],input=value.encode(),capture_output=True,check=True,timeout=6)
    return value + '.' + encode(unpack_es256(result.stdout))


def get(path_or_next, limit_path, params=None):
    url = API + path_or_next if path_or_next.startswith('/') else path_or_next
    part = urllib.parse.urlsplit(url)
    assert part.scheme == 'https' and part.netloc == 'api.appstoreconnect.apple.com' and part.path == limit_path and not part.fragment
    if params:
        assert not part.query
        url += '?' + urllib.parse.urlencode(params)
    req = urllib.request.Request(url,headers={'Authorization':'Bearer '+jwt(),'Accept':'application/json'})
    client = urllib.request.build_opener(NoRedirect())
    with client.open(req,timeout=10) as r:
        b = r.read(1_600_001)
        assert r.status == 200 and len(b) <= 1_600_000
        z = json.loads(b)
        assert type(z.get('data')) is list
        return z


def app_id():
    response = get('/v1/apps','/v1/apps',{'filter[bundleId]':BUNDLE,'fields[apps]':'bundleId','limit':'3'})
    rows = response['data']
    assert len(rows) == 1 and rows[0]['attributes']['bundleId'] == BUNDLE and rows[0]['type'] == 'apps'
    a = rows[0]['id']
    assert re.fullmatch(r'[0-9]{3,20}', a)
    return a


def builds(app, number=None):
    query = {'filter[app]':app,'fields[builds]':'version,processingState','limit':'100'}
    if number is not None:
        query['filter[version]'] = number
    limit = '/v1/builds'
    result = get(limit, limit, query)
    for i in range(12):
        for row in result['data']:
            assert row['type'] == 'builds'
            yield row.get('attributes') or {}
        next_url = (result.get('links') or {}).get('next')
        if next_url is None:
            return
        result = get(next_url, limit)
    raise ValueError('Build history must be verified without truncation')


def choose_number(rows):
    versions = [str(r['version']) for r in rows]
    assert all(re.fullmatch(r'[1-9][0-9]{0,3}(?:\.[0-9]{1,2}){0,2}', v) for v in versions), 'Existing build sequence needs exact review'
    value = 1 + max((int(v.split('.')[0]) for v in versions), default=0)
    assert 1 <= value <= 9999
    return str(value)


def reserve(work):
    install_key()
    owned = app_id()
    build = choose_number(list(builds(owned)))
    (work/'app-id').write_text(owned+'\n')
    (work/'build-number').write_text(build+'\n')
    print('Verified exact App Store Connect app and allocated build '+build,flush=True)


def export_options(work):
    settings={'method':'app-store-connect','destination':'export','signingStyle':'automatic','teamID':TEAM,'testFlightInternalTestingOnly':True,'manageAppVersionAndBuildNumber':False}
    (work/'export-options.plist').write_bytes(plistlib.dumps(settings))


def validate_ipa(work):
    name=work/'ipa/Mnet.ipa'
    assert name.is_file() and 10000<name.stat().st_size<900_000_000
    opt=plistlib.loads((work/'export-options.plist').read_bytes())
    assert opt['testFlightInternalTestingOnly'] is True and opt['teamID']==TEAM and opt['manageAppVersionAndBuildNumber'] is False
    number=(work/'build-number').read_text().strip()
    with zipfile.ZipFile(name) as z:
        entries=z.infolist()
        assert len(entries)<10000 and sum(x.file_size for x in entries)<1_200_000_000
        for item in entries:
            p=PurePosixPath(item.filename)
            assert not p.is_absolute() and '..' not in p.parts and '\\' not in item.filename and not stat.S_ISLNK(item.external_attr >> 16)
        app='Payload/Mnet.app/'
        d=plistlib.loads(z.read(app+'Info.plist'))
        assert d['CFBundleIdentifier']==BUNDLE and d['CFBundleVersion']==number and 'iPhoneOS' in d['CFBundleSupportedPlatforms'] and str(d['DTSDKName']).startswith('iphoneos26.')
        plugins={n.split('/')[2] for n in z.namelist() if n.startswith(app+'PlugIns/') and n.endswith('.appex/Info.plist')}
        allpl=[n for n in z.namelist() if n.startswith(app+'PlugIns/') and n.endswith('.appex/Info.plist')]
        assert len(allpl)==1 and plugins=={'PlugIns'}
        ext=plistlib.loads(z.read(allpl[0]))
        assert ext['CFBundleIdentifier']==TUNNEL and ext['CFBundleVersion']==number
        assert d['CFBundleShortVersionString']==ext['CFBundleShortVersionString']
    with tempfile.TemporaryDirectory(dir=work) as tmp:
        subprocess.run(['/usr/bin/ditto','-x','-k',str(name),tmp],check=True,timeout=40,capture_output=True)
        check=Path(tmp)/'Payload/Mnet.app'
        sig=subprocess.run(['/usr/bin/codesign','--verify','--strict','--deep',str(check)],check=True,timeout=20,capture_output=True)
        met=subprocess.run(['/usr/bin/codesign','-dv','--verbose=4',str(check)],check=True,timeout=12,capture_output=True,text=True)
        assert ('TeamIdentifier='+TEAM) in met.stderr and 'Authority=Apple Distribution:' in met.stderr
    with name.open('rb') as f:
        sha=hashlib.file_digest(f,'sha256').hexdigest()
    (work/'verified.json').write_text(json.dumps({'bundle_id':BUNDLE,'version':d['CFBundleShortVersionString'],'build':number,'ipa_sha256':sha,'internal_only':True},indent=2)+'\n')
    print('Verified Mnet app, VPN extension, distribution signature and iOS SDK for build '+number,flush=True)


def observe(work):
    v=json.loads((work/'verified.json').read_text())
    assert v['bundle_id']==BUNDLE and v['internal_only'] is True
    number=v['build']
    owned=(work/'app-id').read_text().strip()
    assert owned == app_id()
    for i in range(13):
        states={str(row.get('processingState')) for row in builds(owned,number) if str(row.get('version'))==number}
        if 'VALID' in states or 'PROCESSING' in states:
            label='VALID' if 'VALID' in states else 'PROCESSING'
            outcome=f'Mnet {v["version"]} build {number} confirmed in App Store Connect: {label} (internal TestFlight only).'
            print(outcome,flush=True)
            with open(os.environ['GITHUB_STEP_SUMMARY'],'a') as f:
                f.write(outcome+'\n')
            return
        assert not states, 'App Store Connect reported an invalid or failed build'
        if i<12:
            time.sleep(7)
    raise ValueError('Upload command accepted the file; Apple build record not yet observed. Reconcile before retrying.')


def main():
    mode, work = sys.argv[1], Path(sys.argv[2]).resolve()
    assert work.parent==Path(os.environ['RUNNER_TEMP']).resolve() and work.name=='mnet-testflight' and work.is_dir()
    try:
        {'reserve':reserve,'export-options':export_options,'verify-ipa':validate_ipa,'observe':observe}[mode](work)
    except urllib.error.HTTPError as e:
        raise SystemExit('App Store Connect metadata request returned HTTP '+str(e.code)) from None
    except (ValueError,AssertionError,KeyError,subprocess.SubprocessError) as e:
        raise SystemExit('Mnet TestFlight check stopped: '+type(e).__name__) from None


if __name__=='__main__':
    main()
