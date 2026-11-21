#!/usr/bin/env python3
"""Independent receipt-v1 fixture generator/checker, Python stdlib only.

Run from any directory with --check to verify committed golden files. These are
public fixture values, never credentials or a production authorization issuer.
"""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import struct

ROOT = Path(__file__).resolve().parents[2]
DEST = ROOT / 'internal' / 'control' / 'testdata'

def token(b):
    return base64.urlsafe_b64encode(bytes([b]) * 32).rstrip(b'=').decode('ascii')

def fixture():
    record = {
        'network_id': 'fixture-network',
        'device_public_key': base64.b64encode(bytes([4]) * 32).decode('ascii'),
        'node_id': 'node-fixture-123',
        'credential_digest': token(5),
        'phase': 'revoked',
        'scope': {
            'issuer': 'https://issuer.test/path',
            'registrar_origin': 'https://registrar.test',
            'audience': 'fixture-<>&-é-\u2028-\u2029',
            'service_id': 'fixture-service',
            'workspace_id': 'fixture-workspace',
            'mapping_revision': 'fixture-r1',
            'handoff_id': token(1),
            'attempt_id': token(3),
            'reserve_id': token(2),
        },
        'descriptor_digest': token(6),
        'descriptor_encoding_version': 1,
        'receipt_encoding_version': 1,
        'receipt_id': token(7),
        'receipt_digest': '',
        'committed_at_ms': 1795227600123,
        'access_expires_at_ms': 1795227660456,
    }
    s = record['scope']
    text = ['mnet/store-enrollment-receipt/v1', s['issuer'],
            s['registrar_origin'], s['audience'], s['service_id'],
            s['workspace_id'], s['mapping_revision'], s['handoff_id'],
            s['attempt_id'], s['reserve_id'], record['descriptor_digest'],
            '1', record['receipt_id'], record['node_id'], record['network_id'],
            record['device_public_key'], record['credential_digest']]
    fields = [v.encode('utf-8') for v in text]
    fields += [struct.pack('>Q', record[k]) for k in
               ('committed_at_ms', 'access_expires_at_ms')]
    transcript = b''.join(struct.pack('>I', len(v)) + v for v in fields)
    record['receipt_digest'] = base64.urlsafe_b64encode(
        hashlib.sha256(transcript).digest()).rstrip(b'=').decode('ascii')
    metadata = {'field_count': len(fields), 'byte_length': len(transcript),
                'sha256_hex': hashlib.sha256(transcript).hexdigest(),
                'record': record}
    return {
        'registrar-receipt-v1.hex': (transcript.hex() + '\n').encode('ascii'),
        'registrar-receipt-v1.json': (json.dumps(metadata, ensure_ascii=True,
                                              indent=2) + '\n').encode('utf-8'),
    }

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    for name, expected in fixture().items():
        path = DEST / name
        if args.check:
            if path.read_bytes() != expected:
                raise SystemExit('fixture mismatch: ' + name)
        else:
            DEST.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print('receipt v1 independent Python fixture ' +
          ('matches' if args.check else 'written'))

if __name__ == '__main__':
    main()
