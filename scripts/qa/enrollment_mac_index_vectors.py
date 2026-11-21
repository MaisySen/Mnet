#!/usr/bin/env python3
"""Public fixture-only KDF/MAC and byte-exact Go JSON index vectors.
No production authenticator/verifier or private credentials.
"""
import argparse
import hashlib
import hmac
import json
import struct
from pathlib import Path
ROOT = Path(__file__).resolve().parents[2]

def go_key(fields):
    # Explicit independent implementation of the versioned Go []string shape.
    # No generic-JSON equivalence assumption. ensure_ascii=False preserves UTF8.
    s = json.dumps(fields, ensure_ascii=False, separators=(',', ':'))
    for char, escaped in [('<', '\\u003c'), ('>', '\\u003e'), ('&', '\\u0026'),
                          ('\u2028', '\\u2028'), ('\u2029', '\\u2029')]:
        s = s.replace(char, escaped)
    return s

def vectors():
    data = bytes.fromhex((ROOT / 'internal/enrollmentcontract/testdata/transcript-v4.hex').read_text().strip())
    shared = bytes(range(32))  # PUBLIC arbitrary KDF input, not X25519 output.
    nonce = bytes([2]) * 32
    info = b'mnet/enrollment-key-possession/experimental-v4/response-mac-key'
    prk = hmac.new(nonce, shared, hashlib.sha256).digest()
    key = hmac.new(prk, info + b'\x01', hashlib.sha256).digest()
    mac = hmac.new(key, data, hashlib.sha256).digest()
    kdf = dict(shared_hex=shared.hex(), nonce_hex=nonce.hex(), info=info.decode(),
               prk_hex=prk.hex(), response_key_hex=key.hex(), response_mac_hex=mac.hex())
    special = 'a<>&é\u2028\u2029"\\\n\t\x00-end'
    arrays = [['fixture-network', 'fixture-key'], [special, 'key/with/slashes'],
              ['https://authority.test', 'https://registrar.test', special],
              [special, 'https://registrar.test', 'attempt', 'id'],
              ['ab', 'c'], ['a', 'bc']]
    index = [dict(fields=a, encoded_utf8_hex=go_key(a).encode().hex()) for a in arrays]
    confirm_info = b'mnet/enrollment-key-possession/experimental-v4/server-confirmation-key'
    confirm_key = hmac.new(prk, confirm_info + b'\x01', hashlib.sha256).digest()
    fields = [b'mnet/enrollment-key-possession/experimental-v4/server-confirmation',
              hashlib.sha256(data).digest(), mac]
    confirmation_bytes = b''.join(struct.pack('>I', len(v)) + v for v in fields)
    confirmation = dict(info=confirm_info.decode(), key_hex=confirm_key.hex(),
                        message_hex=confirmation_bytes.hex(),
                        mac_hex=hmac.new(confirm_key, confirmation_bytes, hashlib.sha256).hexdigest())
    # Public fixture secret, never real native credentials. Existing consent
    # digest semantics are SHA256 UTF8 canonical43char secret, without prefix.
    secret = 'AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE'
    credential = dict(secret=secret, digest=__import__('base64').urlsafe_b64encode(hashlib.sha256(secret.encode()).digest()).decode().rstrip('='))
    return {
        ROOT / 'internal/enrollmentcontract/testdata/response-mac-v4.json': kdf,
        ROOT / 'internal/control/testdata/registrar-index-keys.json': index,
        ROOT / 'internal/enrollmentcontract/testdata/server-confirmation-v4.json': confirmation,
        ROOT / 'internal/enrollmentcontract/testdata/native-credential-digest-v1.json': credential,
    }

def main():
    p = argparse.ArgumentParser()
    p.add_argument('--check', action='store_true')
    args = p.parse_args()
    for path, value in vectors().items():
        raw = (json.dumps(value, ensure_ascii=True, indent=2) + '\n').encode()
        if args.check:
            if path.read_bytes() != raw:
                raise SystemExit('mismatch: ' + path.name)
        else:
            path.write_bytes(raw)
    print('public MAC/index fixtures ' + ('match' if args.check else 'written'))
if __name__ == '__main__':
    main()
