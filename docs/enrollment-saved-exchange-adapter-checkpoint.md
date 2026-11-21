# Saved-intent source exchange adapter checkpoint

Isolated journal/client integration only. No daemon, State writer, actual durable
WireGuard key owner, browser/loopback listener, native UI, service-reserve client,
Store permit/transaction or network authority is activated. The only application
caller of the native source transport is now this unhooked pending-journal method.
Older checkpoint statements that there is no journal HTTP adapter are superseded.

## Save, send once, recheck, save DATA

ExchangeSaved requires callback-recorded in the live journal, original lifetime,
explicit current SourceConfig callback and clock callback, and explicit transport
limits. SourceConfig is caller DATA, not installed-policy proof. The future daemon
must own that actual config/time source and existing key separately. Current source
origin/authority must exactly match the saved independent originals, with a valid
original/current mapping intersection. No private secret/verifier getter is added.

It publishes exchange-may-have-started before admitting the source call, snapshots
its private saved code/verifier/launch/config, then releases journal lock for I/O.
A private per-instance sender seam defaults to the real NewNative/Exchange client;
no runtime setter, environment bypass or exported fake-trust constructor exists.
Only package tests replace that seam for deterministic real-file ordering/failure
checks. The default client integration also has a real local TLS test below.

Before saving returned DATA, it reads current config again outside the journal
lock, checks unchanged source/authority and that selected mapping is still in the
original/current scope, checks original local lifetime and source ticket deadline,
and includes measured monotonic I/O elapsed time in supplied now. Backward clock
refuses. It validates the result through the strict source DATA parser, then saves
exchanged-data. Another public DATA setter is refused while the owned request is
active. No result is reported before successful local publication.

Close cancels the admitted request, marks the shared journal closed and ends its
owner before returning. Cancellation is best effort, not proof of no remote
consumption. A response/Close race returns no ticket/result, joins closed with
remote-outcome-unknown, and preserves the durable may-have-started marker. No
retry/new consent or secret replacement follows. A final post-publication clock
check distinguishes durable DATA that became stale during save: it remains saved
as historical exchanged-data, returns no result and a no-longer-current error,
not false success or a claim the remote result is unknown.

Failure to save after a source response joins local cause with remote-outcome-
unknown. Prepublish failure leaves the original may-have-started image; ambiguous
local publication also freezes through the existing local uncertainty contract.
These are distinct from notPublished failure of the initial marker, which causes
ZERO private sends and leaves callback-recorded safe for an explicit exact retry.

## Two additional correctness tightenings

Journal is now a copy-safe handle around one private shared backing object. Value
copies share mutex/state/closed/cancel/owner rather than retaining separate mutable
images under one lease. Tests copy a handle, mutate through it, then close it and
verify the original sees the same phase and closed state. Formatting remains
secret-safe for value and pointer handles. This tightens the earlier b31 handle
shape before integration, not a claim that shared native State is now owned.

The native transport now explicitly pins fresh HTTP/1.1 connections, disables
keepalive/HTTP2 and removes Request.GetBody rewind. The earlier implementation
had no application retry loop, but Go's Transport can retry certain pooled
connection/HTTP2 stream failures. Actual installed Go transport source was read
and this path removed rather than calling a no-loop assertion a no-retry proof.
TLS remains verified/system-root TLS1.2+, no redirects/proxy/cookies/ambient auth.
This deliberately gives up pooling/multiplexing for the one-shot exchange. Future
HTTP2/pooling needs its own review; there is no silent enablement. Wire tests
assert HTTP1 and those constructor settings, plus consumed-body response-loss
remains one request. No live source/deployment compatibility claim follows.

## Executed evidence

Actual-file adapter tests inspect the persisted may-have-started record from the
sender before seeing any private request, and match exact saved verifier/code/
source. Initial marker publication failure makes zero sends. Tests cover lost
response, changed current source/authority/mapping, backward clock, final expiry,
response-save failure, public DATA setter racing active request, one-shot no-retry,
Close cancellation and whole-image reopen with the original remote marker.

The DEFAULT real journal-to-native-client path also ran against a disposable local
TLS server. Test-only process system-root input trusts only its generated test
certificate, without an exported custom client transport/TLS bypass. The handler
checks durable marker via Journal.Status before checking received exact own saved
code/verifier and returning strict response DATA. The journal rechecks and durably
saves that response. This is a real journal/client/TLS/file test, NOT the actual
Multies app nor actual native key/listener end-to-end acceptance. Producer/parser
handler parity has its separate source evidence. No real user credentials were
captured; output logs contain only test names/status and no generated material.

Remaining gates: coherent independent owner+journal/client review; actual daemon
State/key/all-writer ownership and durability; real loopback/browser/first-run;
actual config-owner/time assumptions and response-loss product recovery; service
source client and inaccessible verified-source evidence; bootstrap/possession/
confirmation/admission and atomic Store consumption; native OS execution. No
shared daemon/privatefs/mnetd/desktop file changed in this adapter checkpoint.
