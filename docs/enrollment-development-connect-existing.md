# Explicit one-shot save then connect, separate development candidate

Not accepted for normal startup, installation, a real device, publication or
production. This is a new opt-in workflow in the explicitly tagged disposable
command only. Original State, key, policy, completed journal and receipt remain
unchanged. There is no renewal, new approval, proof submission or OS interface.

## One flow, two processes

An operator first provides the same private temporary root, explicit marker,
local-only source/registrar policy and workflow-limits.json as existing disposable
setup. For the CLI network entry, its original timeout is three seconds.

```
mnet-setup-development --disposable-root ORIGINAL_ROOT --save-one-shot-connection --open-browser
# Original process exits after real approval/Return, proof and joining save.
mnet-setup-development --disposable-root ORIGINAL_ROOT --connect-existing-once --open-browser
```

The second command is explicit, never launched by product setup or inspection.
It is permitted only before the original receipt expires. A fresh current map,
mandatory injected in-memory device, exact peer pin and real encrypted TCP
challenge must succeed before a live network dashboard is shown. The reviewed
serialized/terminal dashboard is reused unchanged. Stop, original peer expiry,
source failure or request exhaustion ends the finite run and joins its owners.
The result remains historical, never an ongoing connection claim.

The actual fixture interval is about ten seconds, with fifteen seconds only a
configured maximum. Automation can exercise separate processes within it. A
human cannot reliably do so. This is orchestration evidence, not a usable
consumer session-duration policy. No deadline was extended to make it pass.

## Local workflow accounting is not authority

ReceiptV2 has no request quota. The new record explicitly identifies itself as a
LOCAL DEVELOPMENT WORKFLOW LIMIT, not a receipt quota. The declared local total
is16, matching the existing connected-development option ceiling. Historical
setup reserves2 before its first current request; a later explicit connection
spends ALL14 remaining requests before any HTTP, endpoint publication, device or
control server. Failed requests count, unused capacity is never returned.

Every connection operation shares one native client, maxConcurrent1/maxRequests14,
including the initial current map, endpoint publication and later map checks.
This does not change the separate proof/source workflow's own prior limits.
The finite network original expiry and current authority checks are independent.
No ledger can make an expired or denied receipt current.

The canonical private local-development-connection-v1.json binds exact SHA256s
of the saved State, original native policy, completed pending journal and original
workflow-limits bytes. The full original device/receipt/expiry are transitively
bound by the exact State. No secret/key/receipt blob is copied into the ledger.

Phases:
- setup-reserved: setup2 consumed, not eligible for connection. If setup dies,
  inspection only. No auto-completion or setup retry.
- ready: successful original setup/current-map result; one finite connection
  slice may be explicitly spent within the unchanged original receipt interval.
- connection-reserved: all14 consumed, even if no request/device ever started.
  Never eligible again. This is not evidence the network started or stopped.

Old roots with no original ledger refuse. There is no migration, initializer,
refund, reset, resume, replacement attempt, or terminal-note budget replenishment.
The implementation writes NO terminal note at all. Inspection/recovery merely
reports historical phase and original expiry and cannot launch a network.

## Ownership and crash semantics

A sole native daemon lease is held from validation through durable reservation
and final network teardown. Concurrent public callers fail while it is owned;
a later caller finds the whole connection slice spent. Exact inspection binds
saved source hashes, which the acquired owner and ledger must match again.

Publication writes a0600 temp file, syncs/closes it, publishes atomically and
syncs the parent directory. Initial creation uses an exclusive hardlink publish,
never an overwrite. A crash leaving an extra hardlink fails strict single-link
reads rather than inventing a recovered budget. Transitions use atomic rename.
Any failed stage freezes that in-memory ledger owner; no following HTTP/device
is allowed. After uncertain rename or directory sync, a fresh owner may inspect
only the actual whole image. A reserved image never reconnects. A still-ready
image after a pre-I/O failed reservation is safe to spend explicitly because no
connection I/O occurred in that failed owner. There is no post-I/O rollback.
A whole ready image after an uncertain setup-ready publication does not renew
anything: the setup slice was already durably reserved before setup I/O.

The record and locks provide cooperative single-host single-root accounting.
They do NOT protect against a hostile same-UID editor, copied credentials,
restored snapshots or another device. They are not global/server quota control.
Server-side admission persistence/abuse budgets are separate work.

## Evidence and limits

The companion tests/mnet-connect-existing-validation.sh builds the actual tagged
CLI with race instrumentation and drives the real system browser via Chrome/Xvfb
and dbus. It uses fresh local Multies SQLite/source + actual registrar Store.
The TEST wrapper runs setup to process exit then invokes connect explicitly.
It also hides only the optional ledger to test old-root refusal/no initialization,
checks source/registrar forbidden-window zero, hashes original State/policy/journal/
limits, and attempts a second public connect while the winning owner holds them.
Stop and original expiry are distinct runs. A terminal repeat cannot alter the
spent ledger. All connection requests are counted under their original reserved
phase and bounded by setup2/connection14; no source/proof renewal is introduced.

Crash mode uses a separate tagged TEST process calling the actual public Connect
API. Its emitter blocks after durable spend before HTTP/device. The external
wrapper SIGKILLs it, then actual CLI connect refuses and actual inspection opens
spent/no-controls historical output. Device/Run callbacks in that helper fail
if reached. Source and registrar counters independently assert no post-spend I/O.
This is not a new product environment-variable pause or crash hook.

Unit fault matrix injects failures before write/file-sync/publish and after
publication/before directory-sync. Same owner freezes. SIGKILL persistence,
second-owner contention, illegal refund/double-spend and uncertain setup-ready
transitions are tested. These are injected errors, not a claim to simulate
physical power loss or filesystem hardware behavior.

Earlier failures are retained in the review evidence: initial enhanced counter
used wrong key encoding (test-only, corrected to receipt STANDARD base64); a
later initial map returned unavailable before TCP with1request (cause not proved);
first crash-browser assertion lacked a tab-arrival wait although crash runtime
already passed. Subsequent passes do not erase those failures or prove universal
installed/browser timing stability.

Default workflows/normal CLI/local API/startup/OS TUN/RD/SharedDesk unchanged.
Operator root/trust/service/source/registrar provisioning, production ingress,
persistent global admission, useful duration/renewal/revocation policy and
physical-host acceptance remain unresolved. Broad companion TypeScript remains
unverified at this resource limit, as before. No readiness claim follows from
fixture, affected-race or browser success.
