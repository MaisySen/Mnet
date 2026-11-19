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

class AppNotVisible(Exception):
    pass


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
    kid, issuer = os.environ['ASC_KEY_ID'].strip(), os.environ['ASC_ISSUER_ID'].strip()
    assert re.fullmatch(r'[A-Z0-9]{10}', kid)
    assert re.fullmatch(r'[a-fA-F0-9]{8}(?:-[a-fA-F0-9]{4}){3}-[a-fA-F0-9]{12}', issuer)
    key = key_directory / 'app-store-connect.p8'
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
    if not rows:
        raise AppNotVisible
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



def inspect():
    stage = 'KEY_FORMAT'
    try:
        credentials()
        stage = 'KEY_DECODE'
        install_key()
        stage = 'KEY_SIGN'
        jwt()
        stage = 'APP_LOOKUP'
        owned = app_id()
        stage = 'BUILD_HISTORY'
        rows = list(builds(owned))
        stage = 'BUILD_NUMBER_RULE'
        choose_number(rows)
        return 'APP_AND_BUILD_SEQUENCE_VISIBLE'
    except AppNotVisible:
        return 'APP_NOT_VISIBLE'
    except urllib.error.HTTPError as error:
        return stage + '_HTTP_' + str(error.code if error.code in (400, 401, 403, 404, 429, 500, 502, 503) else 'OTHER')
    except Exception:
        return stage + '_FAILED'


if __name__ == '__main__':
    # The key never persists across steps; this verifier makes only App Store Connect GET requests.
    with tempfile.TemporaryDirectory(prefix='mnet-apple-preflight.', dir=os.environ['RUNNER_TEMP']) as work:
        key_directory = Path(work)
        outcome = inspect()
        print('MNET_APPLE_PREFLIGHT=' + outcome, flush=True)
        sys.exit(0 if outcome == 'APP_AND_BUILD_SEQUENCE_VISIBLE' else 1)
