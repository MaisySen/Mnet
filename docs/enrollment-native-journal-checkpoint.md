# Disconnected native pending-intent journal checkpoint

This implements only the first enrollment-specific journal slice proposed in
`enrollment-native-journal-direction.md`. It does not integrate the daemon,
existing key owner, browser/listener, source HTTPS client, registrar proof,
control Store transaction, credential installation or tunnel. No runtime caller
exists outside this package's tests. No tests use the user's credentials.

## Exact implemented boundary

`internal/enrollmentpending` uses the extracted stateowner primitive for one
`enrollment-pending-v1.json` plus stable sibling lock in an explicitly supplied
DEDICATED private directory. It must not be pointed at an arbitrary shared folder:
privatefs.EnsureDir protects the dedicated directory. The old daemon State and
cmd/mnetd lock are not silently replaced. No WireGuard private key is duplicated.

Begin accepts the independently owned existing public key, listener callback,
platform, installed principal authority, original allowed mappings and explicit
start/deadline. These arguments remain DATA. It does not prove key durability or
listener ownership. It generates independent native secret, verifier, attempt
and callback state with crypto/rand, computes existing text-secret SHA256 and
PKCE SHA256 digests, and saves the exact launch/config/material before returning
public launch DATA. On failure it returns no launch. Existing intent, even expired,
is never silently replaced, evicted, reset or renewed. There is no clear/delete/
abandon/reconsent method in this slice. No duration default is chosen.

The exact local phases are prepared, callback-recorded,
exchange-may-have-started, exchanged-data. RecordCallback checks saved attempt,
state, canonical code and original lifetime, but the future HTTP listener must
still independently enforce method/host/path/Origin/unique query and own socket.
A second callback is refused. Reopening prepared data produces runtime status
prepared-listener-not-owned and refuses RecordCallback. A saved port does not
restore listener ownership. A recorded code may explicitly advance within its
original lifetime, without constructing another listener.

MarkExchangeStarted publishes the may-have-started state BEFORE any future
network call. The package has no network call, no private secret/verifier getter
and no exchange retry method. A remotely consumed/lost exchange response remains
unknown; it cannot be repaired by journal fsync or automatically starting another
consent. RecordExchange strictly checks source DATA against saved own launch,
installed authority, original allowed map and independently supplied current map,
and saves only after the marker. It does not authenticate HTTP or assert that an
actual own-PKCE request took place. Its resulting phase intentionally says DATA.
Exported mutable source structs and private journal methods do not create permits.

No secrets are returned in Status, String, GoString or errors. Value AND pointer
formatting redact. The private serialized file necessarily contains native secret,
verifier, accepted code and source ticket; neither its bytes nor memory dumps may
be logged or attached for real users. Tests generate only temporary native test
material and report no values. Go string allocations are not claimed to be
cryptographically zeroed; Close drops the record reference and ends use, not a
memory-erasure guarantee. No history/retention/product duration policy is supplied.

## Real ownership and persistence

Open requires a regular single-link bounded file, exact required JSON fields,
no duplicate/unknown/case alias/null, strict phase/material consistency and valid
original launch/config. Saved credential and verifier must hash to the exact saved
digests. Opening existing data resyncs the file and parent under the retained owner
before returning it. Malformed contents are refused without rewriting contents.
Existing dedicated directory/file permissions may be protected by privatefs.

Each commit validates an immutable candidate, bounded at 64KiB, creates a private
temp beside the target, checks full write, sync, close and retained owner, then
renames and syncs parent, rechecking ownership. Before-publication failure keeps
old live/file state, removes temp and allows exact retry. A rename error is
retry-safe only if both original target identity/absence AND source temp identity
are demonstrably unchanged. Error-after-rename or directory-sync error freezes
local use until Close/reopen. After reopen the whole current image is validated
and resynced. This local publication uncertainty is different from a durable
exchange-may-have-started record, which means remote consumption may be unknown.
No uncertain outcome returns launch/normal phase success.

The shared owner supplies the existing Unix and Windows file primitives, not
new Store semantics. Journal alone owns its transition validation, candidate and
freeze behavior. Same-UID hostile writes and remote filesystem lock/durability
semantics remain outside the supported cooperative single-host boundary. Native
Windows/macOS execution remains required; compile evidence is not that execution.

## Executed evidence and open gates

Actual temporary-file race tests cover original material persistence/reopen,
original/current config, one-shot callback, original deadline equality/backward
time, no launch on failed Begin, distinct local/remote uncertainty, failed create/
write/short write/sync/close/rename, error AFTER real rename, directory-sync error,
exact retry, no temp leak, malformed load without rewrite, secret-safe formatting,
concurrent callbacks/Close, second process and abrupt process death, parent/lock
replacement and permanent freeze even after the path is restored. Test fixtures
build disconnected DATA, never a proof permit. There is no browser/UI acceptance.

Still open: all native actual key and State writer ownership/durability gates;
owned listener and first-run UX; original/current versioned config integration;
strict HTTPS client with no redirects/cookies/ambient credentials/body logging;
source-authenticated result inaccessible to public DATA constructors; one-shot
response-loss product recovery; clock/staleness/lease policy; bootstrap/possession/
confirmation/admission and actual atomic control Store consumption. No live source,
account, daemon, release or desktop action was taken by this checkpoint.
