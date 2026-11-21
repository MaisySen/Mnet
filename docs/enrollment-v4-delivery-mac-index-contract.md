# V4 proof delivery/MAC and storage-key contract, disconnected

Supersedes derived-mac-key label and underspecified challenge delivery in prior
notes. No verifier, request handler, bootstrap authenticator, permit constructor
or authority transaction is implemented. Domain/transcript v4 bytes unchanged.
This is a reviewable exact protocol proposal, not production approval.

## Device-initiated owned attempt, not an unsolicited signing oracle

Native opens only its independently configured exact registrar HTTPS origin,
with normal TLS hostname/certificate validation, no redirects, inherited cookies,
HTTP downgrade, caller URL, browser proxy or arbitrary local proof request. It
has one pre-saved attempt, selected allowed mapping and native-generated private
credential. It has already authenticated its own code+PKCE exchange to configured
Multies and privately saved the namespaced principal/per-handoff session binding.
A challenge received from the browser, a local untrusted caller, another origin,
server push or an inbound request is never passed to the durable WireGuard key.
No mere matching transcript can substitute for this initiation/transport gate.

Proposed bootstrap request uses a new EXACT enrollment-bootstrap authorization
scheme over that device-initiated HTTPS connection. It presents the already
presaved native256bit credential in a dedicated Authorization header, never URL,
body JSON, cookie, browser or log. Registrar may check it ONLY against the exact
credential digest in the independently authenticated SAME native attempt/reserve
binding. Public digest is NOT accepted as secret. Ticket remains a separate
one-use consent input and is not that credential. Bounded strict parser and
constant-time digest comparison required. Invalid/bootstrap authentication must
not allocate a live challenge slot or generate DH work. Scope is challenge
retrieval/submission/receipt-history for one immutable enrollment attempt, NOT
legacy node tokens, membership/admin, existing mesh APIs or screen/input.
No such scheme or endpoint exists yet. Native never reuses its secret at an
origin merely selected by a response; restart checks current installed config.

Registrar first authenticates exact owned attempt from the secret and configured
service/evidence, then selects that attempt's one live server challenge. Client
is not allowed to select another challenge by arbitrary supplied challengeId.
A response submission contains only the expected challengeId echo and raw32byte
MAC in a strict bounded body; no caller-supplied Binding/expiry/server key or
reference resolver determines verification input. Registrar fetches its own
immutable stored challenge selected by authenticated attempt, checks echoed ID
matches it, reconstructs exact bytes from that owned object, compares MAC and
verifies current local/evidence/deadline limits. A mismatching ID cannot consume,
replace or reveal a different challenge. Same exact attempt retry returns the
same challenge and original deadline, not another challenge or renewed evidence.

Native accepts the response only for its own outstanding exact-attempt request.
It compares all v4 binding fields against the independent saved context before
computing MAC. Registrar ephemeral/id/nonce/deadline become pinned on first
accepted response and exact retry must match them. Lost request state/restart
requires current exact-attempt reconciliation, never unsolicited challenge
acceptance. Normal TLS authenticates the configured registrar; transcript does
not establish channel origin or defeat a malicious trusted registrar relaying
its own power. A credential compromise, TLS trust compromise, local privileged
compromise or configured registrar compromise is outside the relay-prevention
claim. Never call the MAC alone relay-proof.

## Exact response-MAC key schedule

For EACH newly admitted challenge, registrar generates with CSPRNG:
independent32byte nonzero challengeId, independent32byte nonzero nonce and fresh
X25519 ephemeral private key. Collision with ANY retained live or consumed owned
identity refuses/retries internal generation within a bounded generation attempt
limit; it never evicts/replaces that identity. No caller chooses ID/nonce/private
key; deterministic test vectors are NOT generation policy. Crypto APIs reject
invalid/low-order peer keys or all-zero shared output. Both DH sides bind their
own independently checked public keys and the transcript's server ephemeral.

`shared = X25519(server_ephemeral_private, durable_device_public)` and conversely
`X25519(durable_device_private, server_ephemeral_public)`.

HKDF-SHA256 extract salt = raw32byte nonce; IKM = raw32byte shared output.
Dedicated expand info = UTF8
`mnet/enrollment-key-possession/experimental-v4/response-mac-key`.
Output length32bytes. NOT derived-mac-key, NOT a key used for another purpose.
`response = HMAC-SHA256(key, exact v4 transcript bytes)`, raw32bytes.
If later server confirmation or channel binding is added, it needs a different
explicit KDF label and versioned transcript semantics; never reuse response key.
Constant-time equal-length MAC comparison and best-effort temporary-secret clear.

The added Python stdlib/Go TEST-ONLY vector starts with a fixed PUBLIC shared
fixture value to pin extract/expand/MAC bytes independently of a DH library.
It tests wrong info label, nonce and transcript produce different MAC. It does
NOT demonstrate ECDH correctness/low-order handling, cryptographic review,
authenticated delivery, proof verification or any durable authority. Production
code still has no MAC/verify API or callable trust-bypass constructor.

## Byte-exact existing storage keys, immutable semantics

Current registrar namespace v1/v2 index strings intentionally preserve Go
encoding/json Marshal([]string) semantics. This is NOT generic JSON equality.
No new writer may rebuild the key by parsing and reserializing arbitrary JSON.
Exact compact array of strings, no spaces/BOM/trailing newline, exact UTF8 input,
quote/backslash/control escapes as Go JSON strings, HTML-sensitive '<', '>', '&'
encoded as lowercase \\u003c, \\u003e, \\u0026; U+2028/U+2029 escaped as lowercase
\\u2028/\\u2029. Slash is not escaped, other valid nonASCII remains UTF8. No Unicode
normalization, URL case folding or canonical-port rewriting. Inputs have already
passed namespace/value validation. This contract is PINNED by independent Python
encoder and actual Go claim/proof key byte tests including HTML, quotes, backslash,
control characters, nonASCII, Unicode separators and field-boundary shifts.
Some test strings intentionally exceed production identity syntax to stress the
encoding helper only, not to allow such origins/keys into valid stored authority.

Permanent claim key [networkId,canonicalStandardBase64DeviceKey].
Consumed proof key [principalAuthority,registrarOrigin,challengeId].
Existing scope-uniqueness key [issuerLabel,registrarOrigin,kind,id], kind one of
handoff/attempt/reserve. The latter remains issuer+origin scoped, not globally
unique. No schema/canonicalization change is made here. A future nonGo writer
needs these exact vectors, strict schema validation and shared ownership before
writing; otherwise introduce an explicitly versioned migration, not silent repair.

## Admission and storage costs still gate production

There is no new challenge/authority creation path, so no production admission
limit is invented. Future coherent transaction must reserve capacity BEFORE
challenge creation for both live challenge ownership and resulting permanent
record/index bytes. Required explicit fixture limits: live global/principal/
reserve counts, permanent records/consumed index count, serialized state byte
budget and per-entry input bytes. Full refuses new authority, never prunes live
challenges or permanent history. Exact historical retry/revoke/delete preserves
access to existing evidence even when new-admission budget is exhausted. No
retention/epoch policy is selected by these limits.

Per-save validation now rehashes all records. Before production, benchmark actual
valid Store images at chosen fixture limits, include marshal/private-temp sync/
rename/parent-sync and uncertainty latency, and bound lock hold time plus state
file bytes. Source-grounded count/byte admission belongs in owning Store and the
authenticated challenge owner together, not an isolated exported 'capacity passed'
boolean that could manufacture permission. Storage stress evidence cannot prove
network-D1 latency, cryptographic throughput or product safety. No unsupported
claim of bounded permanent growth or production scalability is made here.

Remaining: exact Multies session-binding/provenance responses and tests; native
own-PKCE/presaved secret/restart/transport; reviewed DH+MAC verifier; real owner
challenge admission; actual atomic consumed-proof/node/receipt/claim transaction;
source time/revocation semantics and final-publish gates; compatibility procedure;
D1/full nativeOS/daemon/directmesh. No activation/release/spend or product policy.

## Executed disconnected evidence

Python stdlib HKDF extract/expand and response-MAC golden matches Go crypto/hkdf
and crypto/hmac in TEST-ONLY code. Wrong purpose labels (including the superseded
derived-mac-key), nonce and transcript changes differ. Six independent Python
Go-JSON key vectors match actual claim/proof helpers byte-for-byte. Ten focused
race repetitions of both groups passed; affected vet and diff checks pass.
No production MAC or verifier was added by these tests.

Actual owning Store retained-history publication benchmark includes strict
all-record transcript/receipt validation, JSON marshal, private temp creation,
write, file-sync/close, rename and directory sync. Ten measured writes per fixture
on this Linux environment, not latency promises:

| Revoked retained records | State file bytes | Mean time/write | Allocated bytes/write |
| --- | ---: | ---: | ---: |
| 1 | 3,421 | 0.191ms | 28,776 |
| 100 | 323,281 | 3.329ms | 2,341,332 |
| 1000 | 3,232,081 | 34.504ms | 30,790,699 |

`BenchmarkRegistrarRetainedHistoryPublication` builds syntactically/semantically
valid consumed-history fixtures only in tests, not through a fake registrar or
callable permit API. Records remain revoked, no node authority created. Same
actual Store lease and persistence are used for all writes. Results expose O(n)
CPU/memory/file growth and do not justify unbounded admission. Ten iterations on
one local filesystem are too few for tail latency/power-loss/production sizing;
count values are workload fixtures, NOT product capacities, retention policy or
an approval to evict histories. Actual bounded authenticated admission still
needs implementation as part of a coherent real transaction owner.
