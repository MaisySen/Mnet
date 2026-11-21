# Actual enrolled native key and current map to real encrypted UDP

Opt-in disposable test delta atopc39fe62/Multies593afee. No non-test Go code,
public product entrypoint, registered route, deployment, physical OS interface,
live account, production policy or installed user device is changed. Separate
review is required; this document is evidence description, not acceptance.

## Connected path, not a legacy join surrogate

New cross-app `direct-transport` uses actual Multies approval/SQLite/own-PKCE,
actual configured source reserve, actual Store proof/atomic receipt and actual
separate daemon durable-key enrollment. The daemon finalizes its sole native
credential, closes and normally reopens. It publishes a real loopback UDP endpoint
through its dedicated native credential client and obtains the real approved map.
The daemon process alone reads its SAME owned durable private key via existing
startDevice. Its public key is checked against current-map Self. There is no
legacy join/auth key/node token, replacement transport key, injected peer or
caller-supplied receipt installer. Public fixture IPC carries lifecycle markers,
never that native private key or saved secret.

The first independently approved fixture peer starts another real WireGuard
device using its SAME actual enrollment-proof key. Its endpoint is published
through the owning Store credential operation. Each side uses the actual current
approved map to select the other identity and allowed IP. The native daemon uses
the real applyMap -> peerTrack -> device IPC path, not hand-authored fake peer
configuration. The control-side disposable peer verifies its map contains the
exact native key before configuring its own device.

Both devices use the repository bind implementation, real X25519/WireGuard packet
encryption and real loopback UDP sockets, with in-memory TUNs. Exact ICMP packet
bytes, including mesh source/destination, are checked after decryption in BOTH
directions. This is process-local network/crypto evidence, not OS routing, NAT,
physical two-computer traffic, public ingress, native UI or product onboarding.

## Current revocation to actual device removal and packet denial

After bidirectional payload receipt, actual Store DeleteNode removes the first
transport peer. The native owner fetches another current map, asserts the revoked
ID is absent and runs actual applyMap. Its real WireGuard IPC must no longer
contain the revoked public key. Test-only drain markers separate old already-
decrypted/in-flight packets from five newly requested outbound payloads AFTER
removal. The revoked device decrypts none during the post-removal observation
window. The remaining current map still has the other approved peer. Revoked
credentials remain denied by the actual current endpoint; final deletion leaves
three revoked retained records, zero nodes and zero legacy node tokens.

Removal is driven by obtaining a fresh current map. This is NOT a continuous
revocation guarantee while offline, a policy-selected polling interval, lease
renewal, immediate upstream account event delivery or a production activation.
Current map access and original receipt expiry keep their earlier semantics.

## Failure work and validation

Initial fixture hung because unbuffered test-TUN inbound writes could block
WireGuard RemovePeer while the test stopped reading to apply a map. The fixture
now continuously drains its TUN like an independent OS reader; it does not
relax RemovePeer or allow stale peers. An added diagnostic timeout read raced a
still-running child's output buffer. Its failure path now kills and Waits for
the child before reading, eliminating that fixture-only diagnostic race. Neither
change alters product device or transport code. Initial failing and corrected
logs are retained, not relabeled as successful.

Full two-file cross-app suite passes all ELEVEN cases with race-enabled Go
subprocesses, preserving prior ten including DATA-only expired history,
unexpired recovered finalization and prepared5/7 recovery. Full Go graph race,
build, vet and tagged fixture vet pass. Direct-transport checks are opt-in tagged
and were run through actual Multies, not counted as default untagged tests.
No new Windows/macOS runtime or crosscompile claim is made.

## Measured remaining ingress boundary and next work

The existing actual Store retained-history publication benchmark was run for
three iterations per size on this local Linux amd64 host. Results: one retained
record3421 serialized bytes,3.54ms/op;100 records323281 bytes,3.36ms/op;1000
records3232081 bytes,36.88ms/op with26.55MB allocated/op. These are tiny local
samples including actual publication, NOT tail-latency or production sizing.

The public ingress blocker is still real: historical/current lookups validate
all retained records, begin admission Marshals the whole Store under mutex and
source-pool budgets are process-local. This delta DOES NOT register handlers or
mistake a larger timeout for bounded service cost. Next coherent service work
must enforce explicit private fixture input/body/concurrency/attempt/state-size
admission BEFORE expensive owning Store work, preserve exact history at capacity,
and measure validation/marshal/publication separately. A future performance cache
must be revision-bound to actual owning Store mutation, never a stale node
snapshot treated as authority. Production rates/retention/leases/revocation policy
are not selected by a test limit. Registrar-native relay authentication is also
separate: the existing legacy relay token path is not borrowed for this evidence.
Physical native installation, UI/provisioning and activated pair evidence remain.
