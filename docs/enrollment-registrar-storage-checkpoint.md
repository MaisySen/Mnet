# Registrar storage and legacy-isolation checkpoint

Isolated groundwork following reviewed owning lifecycle eb932cfe. There is NO
transaction/permit constructor, node-creation API, proof caller, service client,
credential verifier/route, native state change or connected consumer. No wider
Multies edit, deployment, release, live account change or production policy.

The earlier transaction design remains a proposal except for the storage,
legacy isolation and receipt-byte sections implemented here. Its opt-in
failing-first tests have become enabled tests of the actual Store.

## Implemented in existing Store

Optional `registrar_state` member of storeData, version 1 with a records map.
Omission stays omission for legacy files. Present empty namespace stays present
through unrelated saves/reopen. Explicit null, unknown version/field, missing
field, duplicate members, case aliases, malformed record or semantic corruption
refuse Open without rewriting file contents. New namespace parsing is strict;
existing unknown legacy top-level compatibility is retained. Root duplicate or
case-aliased registrar_state also refuses rather than allowing a null override.

Each record permanently claims canonical [networkId, standard-base64 raw key]
and carries immutable qualified scope/descriptor/receipt/credential digest and
explicit commit/access timestamps. Both descriptor_encoding_version and the
added receipt_encoding_version must be 1. Digests are canonical unpadded base64url
32-byte values. Node references/network/key/expiry are checked, duplicate
node IDs/handoff/attempt/reserve identities refuse, and an alternate node cannot
share the claimed raw key. Active records need their matching approved node;
revoked records can outlive deletion. Claimed nodes cannot have legacy tokens or
approval provenance, routes/exit capability or legacy user labels. Before any
save the same namespace semantics are checked, not only at startup.

Legacy Join refuses a standing claim after resolving the exact network but
BEFORE saved-token/auth-key/IP handling. This applies to active/revoked/deleted
identities and valid reusable keys or no key. It neither consumes a legacy key
nor creates even a pending legacy node. Other keys and networks retain legacy
behavior, including reusable-key reenrollment after delete. There is no claim
creation method; test-only fixture setup is compiled only in tests.

All legacy approve variants refuse claimed IDs. Legacy AuthorizeNode refuses
claimed nodes even if an in-memory mixed legacy token is injected. Account
identity projection excludes them rather than inventing person provenance.
Map and relay gates deny revoked or access-expired records independently of a
stale Node.Approved flag. No new credential route can currently authenticate an
active claimed node; that absence is intentional, not an end-to-end enrollment.

ExpireNode marks the standing record revoked and expires the node in the SAME
save; DeleteNode revokes the claim and removes node/credential in the SAME save
without erasing history or creating an empty legacy token. Rollback restores
phase/node/token presence on prepublication errors. Postpublication uncertainty
freezes the owning Store, with no success notification. Receipt fields do not
change. Expiring an already elapsed grant cannot set a later deadline or inherit
the legacy 180-day default. Revoked raw-key reuse has no implemented recovery or
replacement-epoch path.

## Pinned receipt encoding v1

This is a NEW Store receipt encoding, not the existing Multies reserve JSON
encoding. Reserve19field/570byte platform-containing vector is untouched.

Receipt domain `mnet/store-enrollment-receipt/v1`, then 18 values, every value
prefixed with a uint32 big-endian BYTE length:

1. Domain, UTF-8.
2. Issuer.
3. Registrar origin.
4. Audience.
5. Service ID.
6. Workspace ID.
7. Mapping revision.
8. Handoff ID.
9. Native attempt ID.
10. Reserve ID.
11. Descriptor digest.
12. Descriptor encoding version as literal ASCII `1`.
13. Receipt ID.
14. Node ID.
15. Network ID.
16. Canonical standard-base64 device public key.
17. Canonical base64url credential digest.
18. Committed-at positive Unix milliseconds as uint64 BE, length 8.
19. Access-expiry positive Unix milliseconds as uint64 BE, length 8.

Receipt encoding version is fixed by the domain and checked as 1 in storage.
No JSON escaping/order, HTML rewriting, Unicode normalization or newline enters
the hash. SHA256 digest uses canonical unpadded base64url. Mutable active/revoked
phase is intentionally excluded so history remains immutable after revocation.
Digest is an integrity/binding value, NOT a signature, service authentication,
proof of possession, trusted permit or bearer credential. Local state-file
write access can manufacture such public digests and remains trusted local
storage authority, not an external authorization channel.

Independent Python stdlib generator/checker:
`scripts/qa/registrar_receipt_vector.py --check`.
Golden fixtures: internal/control/testdata/registrar-receipt-v1.{json,hex}.
Vector is 19 fields, 568 bytes, SHA256 hex
`194e1b1f89311142b806268e96820211a1cf89a8fb74724bf119c49bd3cc987d`.
Includes literal HTML-sensitive characters, non-ASCII and U+2028/U+2029. Actual Go
encoder is compared byte-for-byte to Python struct-packed golden bytes, checks
digest, each mutable/immutable-field behavior and field-boundary separation.

## Executed evidence

- Earlier failing namespace/tombstone baseline retained. Tests now enabled.
- Strict malformed/duplicate/case namespace, semantic corruption and no-rewrite
  tests; actual owning Close/reopen; node/token/provenance/attempt conflicts.
- Permanent deleted-key denial across valid reusable, consumed, expired,
  removed and absent legacy-key modes, with node/IP/key-use/version/disk checks.
- Claimed legacy join/auth/all approve rejection and unchanged unrelated legacy
  reusable-key policy across networks/keys and reopen.
- Expire/delete success, before-publish rollback and after-publish uncertainty,
  retained exact receipt and no fabricated token through actual disk reopen.
- Elapsed access cannot renew through manual approval or a later ExpireNode.
- Ten race-enabled repetitions of new registrar storage/transaction/vector tests
  passed. Full Linux go test ./... passed. Final affected race suite after the
  elapsed-access test passed; affected vet and diff checks passed.
- Windows amd64/macOS arm64 control test binaries cross-compiled after storage
  integration; no native OS execution. Owning lifecycle OS residuals unchanged.
- Python --check matches committed vector. No external service tests or security
  certification are implied by these local results.

## Still unavailable and not disguised by storage tests

No atomic authority-creating transaction or permit issuer exists. Current proof
v2 lacks attempt/credential/reserve-descriptor binding and consumes in memory;
current reserve response lacks verified subject/session provenance; historical
status is not live eligibility. Need reviewed source and revised transcript,
then atomic consumed-proof/permit identity plus node/receipt transaction, before
any consumer. No public constructor accepts proof=true or a matching digest.

A fresh reserve retry still races Store publication. A production authorization
lease/revocation contract, clock/pause/final-write boundary handling or stronger
coordination remains necessary before activation. Access duration, capacity,
privacy retention, reprovisioning epochs and account-event networking effects
are not selected by fixture timestamps or retained records. No pruning/renewal
logic or arbitrary production limits were introduced. Qualified receipt alone
never grants current access and late history cannot reactivate a node.

Storage downgrade to an older binary that ignores this namespace is unsafe:
rollback needs explicit compatibility checking and cannot be made safe by the
new parser inside an old binary. No release/rollback procedure is changed here.
Native Windows/macOS execution, hostile-same-UID/network-filesystem exclusions,
already-distributed direct traffic and daemon-privatefs limitations stay as
recorded in the lifecycle checkpoint. Full unmodified D1 failure remains open.
