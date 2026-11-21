# Bootstrap bearer boundary, confirmation and atomic-consumption preflight

Isolated proposal/test checkpoint. No production bootstrap scheme, verifier,
server confirmation, native proof caller, transaction or authority route exists.
This explicitly corrects earlier shorthand: sending the native credential in an
Authorization header IS BEARER-ON-WIRE. Keeping it out of URLs/bodies does not
make it proof-of-possession authentication. No digest-keyed HMAC is proposed.

## Chosen disconnected transport threat boundary

For the first isolated fixture ONLY, retain the simpler bearer-over-authenticated
TLS proposal, name the exposure, and refuse to implement it as live authority
before deployment topology/logging/native expected-binding review. The alternative
challenge-response keyed by original secret cannot be verified by a registrar
that stores only its SHA256 digest. Keying HMAC with the public digest is forgeable
by anyone reading launch metadata. A PAKE/asymmetric bootstrap is a separate
protocol with extra provisioning/recovery, not an ad hoc hash fix. No such change
is silently adopted from a reviewer suggestion.

Native bearer secret is the existing pre-saved random256bit native control
credential, reused for this tightly scoped bootstrap proposal. Exposure to a TLS
terminator or header logger could impersonate bootstrap AND later native control
auth if such a route existed, so scope separation alone does not eliminate risk.
The current fixture/protocol has no credential control route. Production may
prefer a separately generated bootstrap credential bound in a new descriptor
version, or independently provisioned client key. That changes the existing
consent digest/provenance and needs explicit versioned native+Multies work. It
cannot be faked by treating credentialDigest as a bootstrap secret.

Chosen threat boundary is ordinary authenticated HTTPS to one independently
configured registrar origin, trusting that registrar endpoint and every TLS
terminator allowed to observe it. No static registrar signing/DH key or TLS
exporter/channel-bound value is in the current transcript. TLS terminator is
therefore inside trusted computing boundary, NOT an untrusted relay. A deployment
with untrusted TLS termination is unsupported by this proposal, not repaired by
HMAC transcript fields. Backend protection, exact trusted forwarding topology,
request/header/APM/crash-log redaction, cache policy and native no-redirect origin
validation require real acceptance before activation. Secret in Authorization
must be redacted at EVERY layer, not only application logs.

No connection/TLS-exporter binding is selected. A legitimate native retry may
use a new TLS connection to the SAME configured origin, but must authenticate
the same secret/attempt and pin the same owned challenge, not accept caller-
selected references. This supports response-loss retry at the cost of no defense
against a compromised trusted terminator or credential observer replay. We make
NO relay-resistance or origin-authentication claim from transcript/DH/MAC alone.

## Credential digest exact bytes and own PKCE binding

The existing native credentialDigest field is pinned compatibly with Multies
`hash(string)`: SHA256 of UTF8 canonical43ASCII unpadded base64url secret, then
canonical unpadded base64url digest. NOT SHA256(raw decoded32bytes), NOT prefixed
with a new domain. Call this `mnet-native-credential-text-sha256-v1` in protocol
notes, not a byte prefix. Public fixture and independent Python/Go test distinguish
text and raw-byte digests. Introducing domain-prefixed digest would require a
new explicit descriptor/launch version, not silent mutation of reserve-v1.
A future bearer parser must accept only the exact dedicated scheme/canonical
secret spelling; no whitespace/aliases/URL fallback or logs of attempted secret.

Registrar verifies candidate bearer digest ONLY against SAME immutable reserve
credentialDigest from its independently authenticated live evidence. Matching
an attacker-supplied digest does not establish ownership. Native computes that
digest from its OWN durable saved secret before launching its OWN PKCE request.
It saves original attempt/key/digest/challenge/state/verifier plus exact operator
allowed map. Code+verifier exchange returns exact same attempt binding and named
principal/sessionBinding selected by code-bound journal, from the independently
configured Multies origin. Native checks exact callback/state and launch echo,
current allowed config and digest BEFORE saving selected tuple/provenance or
contacting registrar. The future two responses must explicitly agree on the
same row's opaque sessionBinding. No OIDC subject inference; URL bytes may match
issuerLabel but namespace still distinguishes semantics. TLS-authenticated raw
response alone is not trusted config or native-owned attempt evidence.

## Separate server confirmation, not acceptance

After successful owned-challenge response-MAC verification, compute
HKDF-SHA256 with same DH shared/nonce salt but dedicated expand label
`mnet/enrollment-key-possession/experimental-v4/server-confirmation-key`,32bytes.
Do not reuse response-mac-key. Confirmation message is EACH uint32BE byte-length
prefixed field in order:

1. UTF8 `mnet/enrollment-key-possession/experimental-v4/server-confirmation`.
2. Raw32byte SHA256 of exact owned v4 transcript.
3. Raw32byte response MAC that was accepted for that SAME owned challenge.

Confirmation = HMAC-SHA256(confirmationKey, above bytes), raw32bytes.
Native must compare against its outstanding exact transcript and response,
not arbitrary values echoed in a server response. No caller-selected proof
reference. Wrong MAC/ID/version has no confirmation. Constant-time comparisons,
strict bounded parser and secret clearing required by future implementation.

This proves peer knowledge of ephemeral DH output for transcript, NOT static
registrar identity, current eligibility or durable Store acceptance. A malicious
configured registrar can compute it without committing. On native, confirmation
alone never sets enrolled/approved/current credential or causes tunnel use.
Durable exact qualified receipt from authenticated same registrar after atomic
consumed-proof/node/credential/claim publication is still required, then native
must durably save final state and verify current access is not expired/revoked.
Late historical receipt cannot grant current access. Confirmation cannot repair
an ambiguous transaction or lack of native durable restart.

Python stdlib and Go TEST-ONLY vectors pin separate confirmation key/message/MAC,
response/transcript mutation and cross-purpose key separation. No production
confirmation/verifier helper exists. Current718/669/568/570byte contracts remain
unchanged; confirmation is a separate test data contract.

## Failed bootstrap work and replay ownership

Future ingress: before parsing bodies or database/network/DH work, enforce header
count/size, strict dedicated scheme/43char canonical secret, body size/content-type,
no Cookie/Origin, no redirects and explicit invalid-request/IP global rate budget.
Use a server-keyed ephemeral rate bucket identifier for failed credential probes,
NOT raw secret/public digest in logs or persisted metrics. Required fixture
settings have no default product values and no automatic cardinality-unbounded
map. Bounded rate storage full refuses work; never evicts a live challenge.

After cheap checks, resolve ONLY exact configured attempt/reserve tuple and
compare bearer digest constant-time to independently authenticated binding.
Do not let callers enumerate another principal's record, override service ID,
select arbitrary remote evidence URL or allocate capacity on a failed bootstrap.
Stable generic failure, no detailed ticket/attempt existence or MAC diagnostics.
Allow at most one bounded local lookup for established attempt; initial live
service evidence fetch needs separate budget/concurrency/timeout cap BEFORE
remote I/O. There is no claim that unauthenticated request can cause zero CPU;
cheap parse/hash and bounded lookup still cost work. Failed MAC after admission
uses per-authenticated-attempt/principal budget, retains legitimate challenge
unchanged and original expiry, and cannot evict it to make room.

Server owns exact live challenge selected by authenticated attempt/reserve.
Exact GET/retry returns original ID/nonce/ephemeral/deadline. Conflicting binding
refuses; client cannot request a chosen replacement ID. Correct response is
verified side-effect-free against that stored immutable object, then commit
attempt is serialized under actual Store owner. Same consumed proof retry returns
same historical receipt only. Concurrent/replayed different scope/MAC/descriptor
cannot mint second node/credential or renew lease. Prepublish error permits SAME
challenge response retry; ambiguous publication freezes and requires owning
reopen/history reconciliation; expired unpublished challenges refuse, no eviction
or new ticket. New ephemeral after restart is allowed only for SAME still-live
immutable reserve with proven absent durable history. No automatic consent loop.

## Actual Store preflight boundary still unimplemented authority

All tuple/scope/consumed-proof index helpers are now real private storage helpers;
actual4field scopekey was extracted from validator unchanged and tested against
independent byte-exact vectors. Same immutable GoJSON escaping contract remains.
No new canonicalization/schema migration in this checkpoint.

To implement the eventual connected transaction, first require authenticated
source+native bootstrap+reviewed verifier yielding a private inaccessible permit,
NOT a fixture trust constructor. Owner rechecks health/current config/clock/
exact prior receipt/claim/proof and permanent count+byte capacity before generating
node/address. Candidate contains everything including consumed proof in strict
version2 namespace. Proof verification remains side-effect-free until publication.
Test concurrency and every actual-file failure seam including deadline-crossing
rename. Do not return confirmation as commit success; no called entry point can
skip source verification by supplying matching public fields.

Capacity benchmark evidence remains workload only. Permanent history/principal
privacy, record hashing O(n), no-GC permanence and downgrade gate remain. No
production duration/retention/recovery/default limits selected. D1/full nativeOS/
daemon-save/current expected-binding/directmesh gaps still open. No live service,
release, new user grant or wider Multies edit performed.

Executed evidence for this checkpoint: ten focused race repetitions of response-
MAC, separate server confirmation, credential text-digest and actual scope/claim/
proof index helpers pass. Independent Python --check passes for new public
fixtures plus unchanged v4 transcript/receipt-v1/v2 goldens. Affected vet/diff
checks pass. Existing Multies source hash helper was read directly and confirmed
SHA256(TextEncoder UTF8 string) with unpadded base64url output; no Multies code
was edited. This evidence does not execute real TLS/bootstrap/server confirmation,
credential logging topology, full authority transaction or native acceptance.
