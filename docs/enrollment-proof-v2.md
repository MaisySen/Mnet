# Inactive enrollment proof v2 contract

This document specifies the isolated experiment only. There is no HTTP route,
Multies enrollment extension, daemon proof endpoint, registrar store transaction
or production caller. A passing proof is key possession, not enrollment consent,
identity authentication, membership or permission to see/control a screen.

## Exact identity and mapping

`internal/enrollmentidentity` is the shared syntax definition used by the offline
operator checker and the experiment. Origins require exact HTTPS origin syntax,
with no path/query/fragment/userinfo. Issuers may have an exact path. No network
lookup, redirects, case folding or URL normalization establishes identity.
Explicit ports remain part of the exact identifier, including explicit443;
operators must use one identical spelling at every boundary. Plain HTTP,
including loopback, is no longer accepted by the isolated proof experiment.
The existing List bridge's separate development transport is unchanged.

Every proof binding now requires a mapping revision. Altered or omitted revision
refuses as do other permission tuple changes. This binds bytes supplied by the
caller; it does not prove that they are the current operator mapping. A future
registrar must independently fetch/recheck that revision at commit.

## Binary transcript

The protocol and HKDF domain is now
`mnet/enrollment-key-possession/experimental-v2`. There is no v1 fallback.
Every field is uint32 big-endian byte length followed by that many exact bytes.
The ordered fields are:

1. Domain bytes in UTF-8.
2. Challenge ID,32raw bytes.
3. Nonce,32raw bytes.
4. Server ephemeral public key,32raw bytes.
5. Registrar origin, UTF-8.
6. Purpose audience, UTF-8.
7. Exact issuer, UTF-8.
8. Subject, UTF-8.
9. Person session ID, UTF-8.
10. Workspace ID, UTF-8.
11. Network ID, UTF-8.
12. Mapping revision, UTF-8.
13. Enrollment authorization ID, UTF-8.
14. Device public key,32raw bytes.
15. Expiry,8bytes containing unsigned big-endian Unix milliseconds.

No BOM, JSON escaping, string normalization or trailing newline appears. Each
fixed-size value still receives its length prefix. Valid future dates in the
experiment use positive Unix milliseconds. The HKDF uses SHA256, the32byte nonce
as salt, the X25519 agreement as input key material, and the domain with
`/derived-mac-key` appended as info. HMAC-SHA256 covers the full encoded tuple.
This defines bytes, not approval of cryptographic suitability for routed use.

`internal/keypossession/testdata/transcript-v2.hex` is a355byte deterministic
vector independently generated with Python struct packing. It includes raw
sequential key/nonce bytes and subject/purpose strings containing HTML-sensitive
characters and non-ASCII UTF-8. Its SHA256 is
`5504f35c0696ffe297af16b7704bf19ae67b6508d4fe6a5de95f720af081330c`.
The Go test compares all bytes, not only a locally generated digest. Another
case proves field-boundary shifts produce different bytes. No vector key is a
real device secret.

## Required future adapter contract, not an implementation

A production adapter must create `expected` independently of all challenge
fields. Never pass `challenge.Binding` as `expected` merely because the challenge
came from an HTTPS endpoint. Source each value as follows:

| Value | Required independent source |
| --- | --- |
| Registrar origin, audience, issuer | Explicit trusted operator configuration |
| Workspace/network/revision | Current exact operator mapping, rechecked at commit |
| Subject/person-session/authorization | Authenticated enrollment-only consent result bound to this native attempt |
| Device key | Existing durable daemon identity, not browser-provided replacement |
| Challenge ID/nonce/ephemeral key/expiry | Exact challenge from the authenticated configured registrar for this attempt |

A malformed, absent, expired, canceled or conflicting input must refuse without
requesting a proof or replacing an identity. Only one in-flight locally owned
attempt may use the daemon key. A future local endpoint needs existing local
capability authentication, bounded request shape/capacity and per-attempt
ownership. The daemon authenticates the configured registrar directly, with no
redirects, arbitrary remote URL or browser cookie inheritance. The private key
stays inside the daemon. UI/browser metadata cannot create consent or supply
trusted expected fields.

The current unit tests demonstrate equality checking of independently supplied
expected bindings, not a working trusted adapter. No adapter-sourced test can
substitute for the missing authentication/consent implementation. That adapter
must receive its own adversarial tests and review before any wiring.

## Review reconciliation and remaining decisions

The source assessment supplied by main is reasoning, not tested certification
or authority. Its later qualifications matter: an uncertain distributed commit
must never become reusable solely on timeout; hash-only credential storage
cannot reproduce an old secret; response recovery needs a reviewed encrypted
redelivery or authenticated atomic reissue/rotation design. Neither recovery
window/count nor ticket/lease durations are selected here. Session logout,
workspace removal, device withdrawal and unlink must have separately chosen
product effects, not implicit global node deletion.

Current source also supports a narrower legacy concern: still-valid reusable
auth keys can grant approval after node deletion or expiry, whereas consumed
one-time keys cannot. No legacy behavior is changed by this contract fix. A
new registrar's tombstone/re-enrollment semantics still need explicit design,
regressions and independent review. No revocation SLA is established.

Unresolved high-impact work remains authenticated enrollment assertions,
check/commit race handling, atomic durable proof/authority/node receipt commit,
crash recovery and measured peer/relay revocation. The possible stopped List
lifecycle test edit is excluded. These source fixes neither revisit it nor
create permission for the pending Multies extension.
