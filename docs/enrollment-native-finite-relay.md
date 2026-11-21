# Finite development registrar-native encrypted relay

This isolated local change adds no production Server route, Up/relayLoop caller,
map RelayURL support, legacy token fallback, installer, account effect or physical
device activation. Network permission remains separate from screen/input access.
Separate independent review is required. This document is not acceptance.

## Connected evidence

The added actual crossapp relay-transport mode starts with actual Multies
approval/SQLite/own-PKCE/source reserve, actual Store proof consumption and atomic
receipt, and an independently running actual native owner. The native process
finalizes sole State from the same confirmed preparation, closes, normally
reopens, and correlates the same saved State/key/journal/policy. The native
private key never travels in fixture IPC. Both devices use their SAME actual
enrollment-proof keys in real WireGuard devices with in-memory TUNs.

The first actually enrolled fixture peer uses its actual retained receipt and
secret to dial a private TLS WebSocket. The native owner uses its retained finite
RegistrarClient and a private native-current wrapper, not a fabricated installer
or legacy join. Dedicated relay Authorization and canonical base64 receipt
reference headers replace query tokens. The reference is DATA, not authority:
server finds the actual retained record and strictly matches the full wire against
its original retained challenge, then checks the real secret/current credential.

Both transport participants explicitly advertise zero direct endpoints. Their
actual current approved maps select identities and allowed IPs. Native applyMap
selects the repository relay bind endpoint; the fixture peer configures only its
real mapped native peer over relay. Real WebSocket frames, scoped Hub forwarding,
repository Bind injection and real WireGuard encryption carry exact ICMP/IP bytes
in both directions. The controlled fixture makes native the sole handshake
initiator: the first test device suppresses its own type-1 initiation frames,
while still sending responses and encrypted data. No production code suppresses
those frames. Simultaneous-start/reconnect reliability is NOT established.
A scratch drop-all forwarding control fails the native
missing-decrypted-reverse-payload assertion. This is process-local encrypted relay
evidence, not NAT/public-internet/OS routing or installed-device evidence.

## Lifetimes, authorization and limits

The private relay handler is explicitly constructed from the SAME already
installed finite ingress gate and shares its request/concurrency budget and Store
Close/freeze lifetime. It is not registered by Server.Handler. Header grammar,
exact path, TLS GET/no body/query/ambient cookies, reference bytes and canonical
secret are checked before admission to real attachment. A second current check
runs after WebSocket upgrade. Failed requests spend finite admitted budget.

Source credential, destination current identity, network isolation, expiry and
ACL are checked under the owning Store lock on forwarding and again on queued
writing. The legacy RelayAllowed logic is extracted to a lock-required helper;
legacy behavior otherwise stays unchanged. Actual connected self-frame tests
verify Store Close, expiry and ACL withdrawal stop further permitted frames. A
scratch fresh-auth bypass makes expiry and ACL tests fail on forbidden delivery.

Hub.AttachScopedBounded is an explicitly selected finite primitive, not
permission. It requires a context deadline within one minute, one to 4096 data
frames and an authenticated caller callback. Malformed DATA frames spend slots;
control frames do not, but cannot extend the mandatory deadline. The existing
legacy AttachScoped path keeps its previous behavior. Deadline/cancel closes the
actual socket and cleans presence. No reconnect or lease renewal is installed.

Dedicated RegistrarClient relay operations share the retained client's request
and concurrency slots. The slot stays held until socket finish. Each socket has
a fixed timeout, total send/receive frame limit, bounded packet size and one
serialized data writer. Denial ends that client permanently, without legacy
rejoin. Proxy and redirects are not a fallback. Client Close/receipt expiry
cancels the socket. Native wrapper rechecks actual State/journal/policy and
captures one Bind generation around use; an old callback cannot target a newly
created Bind. Device-generation contention refuses a packet. It does not close
the session merely because a map/device lock is briefly busy.

Neither this handler nor the native socket path is a production ingress. Accepted
socket count, prehandler TLS/header limits, source-IP/rate limits, cross-restart
budgets, response-header sizing in the WebSocket client, broader retention and
production lease/revocation policy remain open. Store/pending synchronous work is
not forcibly preempted, even after context cancellation. Per-packet full history
validation is bounded by the configured gate but not constant cost. Finite
request exhaustion and a new gate budget on Store reopen remain explicit.

## Revocation evidence is directional and finite

After bidirectional decrypted payloads, actual Store DeleteNode revokes the first
peer, native fetches a fresh current map and real applyMap removes its public key
from WireGuard IPC. Five new packets are enqueued each way. In relay mode these
new raw packets carry a distinct last-byte marker so old in-flight plaintext is
not falsely classified as post-removal traffic. Only exact earlier baseline
bytes can be ignored during the observation window. Marked post-removal packets
must not decrypt. The marker is a test raw-packet discriminator, not a claim of
valid OS ICMP delivery after checksum change. Counts prove enqueues, not five
independently observed wire transmissions. The 20ms/200ms windows are heuristic,
not latency or continuous-revocation guarantees. Already-written encrypted
packets cannot be retroactively withdrawn. Fresh-map retrieval drives device
removal; there is no upstream account-event or polling guarantee.

## Validation and failure history

The actual two-file crossapp suite contains twelve cases, eight workflow modes
and four separate native regressions. Direct mode remains separately named and
is not relabeled relay. Supporting real local WebSocket tests cover exact
credential/reference/header refusal, connected expiry/ACL denial, Store Close,
frame limits, malformed frame spending, timeout/cancel cleanup, client permanent
denial, and absent/busy native generation refusal. Full Go race, build, vet and
tagged fixture vet are run independently from the tagged crossapp fixtures.

Initial wrapper code took deviceMu's write lock inside a synchronous WireGuard
Send callback invoked under applyMap's read lock. A retained SIGQUIT stack exposed
that self-deadlock. The wrapper now uses nonblocking generation read acquisition;
a deterministic held-reader regression covers it. Initial first-peer WebSocket
writes were not serialized and raced handshake/data sends; the writer mutex now
covers deadline and data write. Initial Store per-packet TryLock could drop packets under a legitimate
concurrent map read. Missing forward payload failures occurred with that code,
but the exact dropped frame was not retained. Fresh per-packet checks now
serialize on Store.mu,
retaining the explicit nonpreemption limitation rather than extending leases.

The first drain-only relay denial check misclassified pre-removal in-flight
packets. New-generation raw markers fix that evidence boundary. An intermediate
extra-send loop affected the direct test; it is now relay-mode only. Finally,
fixture JSON publication could be observed empty while os.WriteFile was still
running. Public fixture handoffs now write a private temporary name and rename.
Initial/fixed failures, race logs and deliberate-mutant outputs are retained.
No failed run is relabeled successful and no timeout change masks those failures.

Repeated full-suite runs after one initial twelve-case pass exposed a further
limit. Diagnostic per-kind counters showed both initiation/response messages
forwarded and both native ownership callbacks accepted, but verbose WireGuard
logs showed both responses rejected after simultaneous initiation. Retry was at
5s, beyond the deliberately fixed 3s native socket. The test now explicitly
chooses one handshake initiator rather than increasing socket/receipt policy.
This is a test topology restriction, not a general startup fix. Diagnostic
counters/logging were removed from final code; their logs/source are retained.
Explicit socket/map-ready and first-peer-configured barriers keep early synthetic
fixture endpoint publication from being mistaken for the zero-endpoint phase.

## Focused review corrections

Transient valid Store admission contention was incorrectly returned as403,
which permanently denies the retained client. It now returns503 through a private
service-unavailable sentinel. It still spends an admitted finite request, and
neither failure nor continuation refunds budget. A valid dial under held Store.mu
refuses without queuing, then succeeds after unlock. A client receiving503 can
successfully dial again with the same retained client and correct spent budget;
real401/403 still permanently denies. Gate/Store state-capacity unavailability
also uses503 rather than falsely declaring a secret invalid.

A true two-node destination matrix keeps the registrar-native source valid and
uses an explicitly legacy-joined/approved second fixture destination. A real
baseline socket frame succeeds; destination unapproval/expiry/deletion/network
move/ACL-pair withdrawal each denies the new frame. This is cross-type pair
authorization evidence, not a claim of a second finalized native owner. A fresh
pair-check bypass mutant fails all five cases on forbidden delivery.

Queued-only sensitivity uses a real local WebSocket with a test-owned connection
wrapper: hold the first already-authorized destination write, enqueue a second
frame while allowed, revoke permission, then release the writer. Only the first
frame may drain. Removing ONLY the queued writer's p.permitted check makes the
second queued-before-revoke frame arrive and fails that exact assertion. Admission
is deliberately independent in this test, so a source admission refusal cannot
manufacture its pass. This check does not revoke a write already in progress.

### Actual handler queued-callback wiring

The separate Hub queued test did not cover omission of the control handler's
ForwardScopedChecked callback argument. The actual-handler regression now holds
the first destination socket write, sends a second destination frame through the
real credential-authenticated TLS handler, then sends a FIFO source-self barrier.
Receiving that barrier proves the second frame has passed real handler admission
and enqueue while permission remains valid. Only then does the test expire the
source, unapprove the destination, or withdraw the ACL pair, before releasing the
destination write. The already-authorized first write completes, but the second
queued frame must not arrive. Destination attachment is explicitly test-owned
Hub plumbing; it does not substitute a test callback for the handler's source
forwarding or queued authorization. A mutant replacing ONLY the handler's queued
callback argument with nil fails all three cases on second-frame delivery.

Mutant results are specific to their exact edits. Our earlier destination mutant
bypasses the entire allowed callback and fails all five destination modes. The
independent review used a different destination-check bypass, with three state
failures plus a credential case. Those counts are not equivalent evidence.
