# Actual native State owner and publication checkpoint

Isolated prerequisite, not native enrollment activation. The default-off Multies
work is unchanged. No source journal is hooked into the daemon, no new enrollment
route/client is invoked by it, no native proof/permit/Store consumption exists,
and no account, release, signing, deployment or physical device action occurred.
Network, Remote Desktop and Shared Desk approvals remain separate.

## Actual owner, not another journal

Daemon.New now acquires the same stable `daemon.lock` in its dedicated state
directory before loading any State, including an absent state.json. It retains the
shared stateowner lease through library lifetime. Normal cmd/mnetd no longer wraps
New in a competing process lease. The old platform-specific process lock mechanics
were replaced with a compatibility probe wrapper over the SAME shared primitive;
production ownership is New/Close alone. Unix old command instances that cooperate
with the original daemon.lock flock are excluded by the same inode lock.

New/mobile.NewNode/selftest/sessionproof now converge on that library owner.
Daemon.Close is idempotent, marks closed/cancels and finishes Down/worker/device
teardown before releasing. Down/phone Stop keep ownership because the same daemon
is reusable. Session-proof success cleanup now Close rather than Down-only, and
failed Prepare/netstack/setup cleanup releases its created daemon. Selftest closes
its ChannelTUN when New fails. No mobile API shape or first-run UI was added.

The source directory is explicitly dedicated/private, not an arbitrary shared
folder. Final directory symlinks/non-directories refuse before creating a lock.
Parent aliases canonicalize through stateowner. Missing parents are CREATED AND
SYNCED by stateowner BEFORE privatefs protection can pre-create them via MkdirAll;
then the dedicated directory is protected/synced. This ordering also corrects the
previous disconnected pending-journal Open. No privatefs implementation changed.

Existing native identity file stays at state.json with the SAME exported JSON
fields and existing valid key pair. No migration, revision column, credential
rotation, default enrollment or silent recovery is written. Legacy metadata-only
objects remain readable; malformed key/enrollment data still refuses. The new
private in-memory revision fields never serialize. Existing saved identity pair
cannot be replaced through ordinary Save, even by a current detached snapshot.

## Detached State API and writer exclusion

State remains DATA, not a lease/capability. LoadState acquires the same exclusion,
validates/resyncs the actual file/parent, returns a detached snapshot and releases.
It now intentionally refuses while a live daemon owns that directory. Consumers
needing a current in-process diagnostic use daemon.State; independent recovery/
verification must Close before LoadState. Current production callers use the
retained daemon path; affected tests were migrated rather than weakening ownership.

Standalone Save reacquires the SAME owner and compares original exact byte SHA256,
existence and canonical file path, all held by value in that snapshot. A fresh
State may create absent state, not overwrite existing identity. A stale/value-copy
or cross-directory snapshot refuses. Saves advance ONLY that snapshot's revision,
not a shared revision pointer. Public EnsureKey rechecks an existing snapshot under
the same exclusion, including no-op cases, rather than returning an existing key
from a stale snapshot. The retained daemon uses private owner saves, never public
Save that would reacquire its own lock.

State is not hostile-process security. Exported DTO fields and old memory snapshots
can be copied by code with process access. State.String/GoString now redact secrets,
but JSON and field access necessarily contain them and must not be logged or sent.
A healthy closed daemon's State is a historical diagnostic snapshot for existing
API compatibility, not proof that its key may now be used by a native authority
adapter. A faulted daemon returns empty State. Future native proof must use an
internal checked owner operation, not a DATA snapshot/private-key getter as a permit.

## Actual files and outcomes

Open refuses links/nonregular/multiply linked/oversized State, strict duplicate/
unknown/case-aliased/null/malformed fields and invalid key/enrollment semantics.
Legacy optional fields may be absent. One-MiB file and4096route-array parser bounds
are local defensive engineering bounds, not a network admission or product policy.
Open never rewrites malformed contents. It syncs a valid loaded file/parent before
returning a recovered owner; that does not prove the previous write survived power
loss. Existing dedicated file/directory permissions may be protected during open.

Per-instance private filesystem seams wrap real create/write/sync/close/rename/
directory-sync operations. Writes validate the whole candidate and unchanged
original disk revision before creating a private temp beside the destination.
Short write is failure. Full write, file sync/close, retained owner check, rename,
parent sync and owner recheck precede successful state publication.

Before-publication failure preserves original disk and normal working config/key
and cleans temp, permitting explicit exact retry. Unknown/error-after-rename or
parent-sync error permanently freezes the retained object, cancels current work,
clears usable control client/map/peer admission and schedules Down/device teardown
outside the state mutex. PrepareIdentity/SetRouting/SetTUN/Prepare/Up/new Join,
map application, control-loop clients and session-peer admission reject unhealthy
owner. Client reads are held locally before use rather than re-fetching a pointer
that could become nil during freeze. State-unavailable status persists through Down.
Restoring a replaced lock path cannot resurrect a faulted object. Close/reopen is
required. This is not synchronous per-packet revocation: already-running I/O/device
teardown and previously distributed direct-mesh state have their own timing.

Remote legacy Join acceptance remains distinct from local config publication.
If a server has accepted credentials but a provably prepublish save fails, the
existing diagnostic/in-process exact-retry behavior keeps that candidate credential,
NOT a usable control client, and explicitly reports accepted-but-not-saved. It
cannot promise crash recovery or reverse remote acceptance. Local unknown publish
still freezes and refuses retry in place. This does not connect the new Multies
enrollment protocol or silently solve cross-system atomicity.

## Executed evidence

The initial three failing actual API tests from f4e7e4d are now ordinary untagged
regressions: second owner before first write, direct Save bypass, stale snapshot
lost update. Their recorded red log remains in the review packet.

New real-file tests cover create/write/short-write/sync/close/rename failure,
error AFTER real rename, directory-sync failure, unchanged existing/generated
identity, first-key publication failure and uncertain first-key no-regeneration,
whole candidate reopen, no temp leak, exact retry, stale value-copy and cross-dir
refusal, no-op existing key owner/revision check, external disk change at save,
unsafe JSON/link target without content rewrite, replaced lock permanent freeze,
concurrent Close/mutation and abrupt independent-process death/reacquire. No real
credential is captured or included in reports.

Actual mobile Node tests verify Stop retains exclusion and Close releases. Actual
selfTestDaemon and failed sessionproof Prepare tests verify shared library owner
and error cleanup. Existing affected daemon/cmd/mnetd/mobile/keypossession tests
now Close before independent LoadState. Full source `go test ./...` passes after
that migration; affected race and vet pass. Cross-compiles are reported separately,
not native Windows/macOS execution. These tests do not exercise power loss, kernel
filesystem guarantees, real mobile VPN/OS UI, actual enrollment authority or a
live release. A new native State fault does not retroactively revoke copies of
keys/maps that left this process before the fault.

## Remaining gates

Independent coherent native-owner review; native Windows/macOS/mobile execution;
old-binary/downgrade discipline (old Windows/no-op/noncooperating writers do not
become safe because this version locks); supported single-host filesystem semantics;
actual installed config/time owner, owned loopback/browser, one-shot lost-response
recovery; journal/native-key integration with explicit two-file outcome handling;
service source adapter with private evidence, reviewed bootstrap/possession/
confirmation/admission and actual atomic Store consumption. No live account or
physical native acceptance claim follows from this isolated source checkpoint.

Final hardening: a missing private State owner is unhealthy, never a production
fixture bypass. Existing synthetic routing/session/path/STUN tests now attach a
real temporary native file owner through a test-only helper. That helper is not
in production builds and constructs no authority permit. Current detached Save
also refuses replacing an already saved key pair. These changes are covered by
the final exact-source affected race and whole test graph below.
