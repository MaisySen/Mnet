#!/usr/bin/env python3
"""Inactive proof-v3 public transcript, independent struct-packed bytes.
No signing, credentials, network or Store authority. --check verifies goldens.
"""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import struct

DEST = Path(__file__).resolve().parents[2] / 'internal/enrollmentcontract/testdata'
def token(n):
    return base64.urlsafe_b64encode(bytes([n]) * 32).decode().rstrip('=')
def fixture():
    binding = dict(
        principal_authority='https://multies.test',
        principal_namespace='multies-local-user-v1',
        principal_id='local-person-<>&-é-\u2028-\u2029-end', session_id='local-session-123',
        issuer_label='https://configured-issuer.test/path',
        registrar_origin='https://registrar.test', audience='fixture-enrollment',
        service_id='fixture-service', workspace_id='fixture-workspace',
        network_id='fixture-network', mapping_revision='fixture-r1',
        handoff_id=token(4), attempt_id=token(5), reserve_id=token(6),
        descriptor_digest=token(7), descriptor_encoding_version=1,
        device_public_key=token(8), credential_digest=token(9),
        checked_at_ms=1795227600123, reserve_deadline_ms=1795227660123,
        access_until_ms=1795227650123)
    challenge = dict(binding=binding, id=token(1), nonce=token(2),
                     ephemeral_public_key=token(3), expires_at_ms=1795227640123)
    fields = [b'mnet/enrollment-key-possession/experimental-v3']
    fields += [base64.urlsafe_b64decode(challenge[k] + '=') for k in
               ('id', 'nonce', 'ephemeral_public_key')]
    fields += [binding[k].encode('utf-8') for k in (
        'principal_authority', 'principal_namespace', 'principal_id', 'session_id',
        'issuer_label', 'registrar_origin', 'audience', 'service_id',
        'workspace_id', 'network_id', 'mapping_revision')]
    fields += [base64.urlsafe_b64decode(binding[k] + '=') for k in
               ('handoff_id', 'attempt_id', 'reserve_id', 'descriptor_digest')]
    fields += [b'1']
    fields += [base64.urlsafe_b64decode(binding[k] + '=') for k in
               ('device_public_key', 'credential_digest')]
    fields += [struct.pack('>Q', binding[k]) for k in
               ('checked_at_ms', 'reserve_deadline_ms', 'access_until_ms')]
    fields += [struct.pack('>Q', challenge['expires_at_ms'])]
    data = b''.join(struct.pack('>I', len(field)) + field for field in fields)
    meta = dict(field_count=len(fields), byte_length=len(data),
                sha256_hex=hashlib.sha256(data).hexdigest(), challenge=challenge)
    return {'transcript-v3.hex': (data.hex() + '\n').encode(),
            'transcript-v3.json': (json.dumps(meta, ensure_ascii=True, indent=2)
                                   + '\n').encode()}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    for name, data in fixture().items():
        if args.check:
            if (DEST / name).read_bytes() != data:
                raise SystemExit('mismatch: ' + name)
        else:
            DEST.mkdir(parents=True, exist_ok=True)
            (DEST / name).write_bytes(data)
    print('inactive transcript v3 ' + ('matches' if args.check else 'written'))
if __name__ == '__main__':
    main()
