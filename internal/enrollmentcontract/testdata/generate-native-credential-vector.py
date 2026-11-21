#!/usr/bin/env python3
"""Independent disposable fixture generator. Never use fixture secrets as credentials."""
import base64, hashlib, json, pathlib, struct
p=pathlib.Path(__file__).resolve().parent
c=json.loads((p/'transcript-v4.json').read_text())['challenge']; b=c['binding']
enc=lambda v:base64.urlsafe_b64encode(v).decode().rstrip('=')
dec=lambda v:base64.urlsafe_b64decode(v+'=')
lp=lambda vals:b''.join(struct.pack('>I',len(v))+v for v in vals)
secret=enc(bytes([73])*32); b['credential_digest']=enc(hashlib.sha256(secret.encode()).digest())
f=[b'mnet/enrollment-key-possession/experimental-v4']
f += [dec(c[k]) for k in ['id','nonce','ephemeral_public_key']]
f += [b[k].encode() for k in ['principal_authority','principal_namespace','principal_id','session_binding','issuer_label','registrar_origin','audience','service_id','workspace_id','network_id','mapping_revision']]
f += [dec(b[k]) for k in ['handoff_id','attempt_id','reserve_id','descriptor_digest']]
f += [b'1']+[dec(b[k]) for k in ['device_public_key','credential_digest']]
f += [struct.pack('>Q',n) for n in [b['checked_at_ms'],b['reserve_deadline_ms'],b['access_until_ms'],c['expires_at_ms']]]
transcript=enc(hashlib.sha256(lp(f)).digest()); commit=b['checked_at_ms']+1; rid=enc(bytes([74])*32)
r={k:b[k] for k in ['registrar_origin','audience','service_id','workspace_id','mapping_revision','handoff_id','attempt_id','reserve_id','descriptor_digest','descriptor_encoding_version','network_id','credential_digest']}
r.update(issuer=b['issuer_label'],receipt_id=rid,node_id='node-fixture',device_public_key=base64.b64encode(dec(b['device_public_key'])).decode(),committed_at_ms=commit,access_expires_at_ms=b['access_until_ms'],challenge_id=c['id'],transcript_digest=transcript)
rf=[v.encode() for v in ['mnet/store-enrollment-receipt/v2',r['issuer'],r['registrar_origin'],r['audience'],r['service_id'],r['workspace_id'],r['mapping_revision'],r['handoff_id'],r['attempt_id'],r['reserve_id'],r['descriptor_digest'],'1',rid,r['node_id'],r['network_id'],r['device_public_key'],r['credential_digest']]]
rf += [struct.pack('>Q',commit),struct.pack('>Q',r['access_expires_at_ms']),c['id'].encode(),transcript.encode()]
raw=lp(rf);digest=enc(hashlib.sha256(raw).digest())
doc=dict(version=1,kind='registrar-native-experimental-v1',secret=secret,finalization_id=enc(bytes([75])*32),challenge=c,receipt=r,receipt_digest=digest)
(p/'native-credential-v1.json').write_text(json.dumps(doc,ensure_ascii=False,indent=2)+'\n')
(p/'receipt-v2.hex').write_text(raw.hex()+'\n')
print('fixture receipt bytes',len(raw),'digest',digest)
