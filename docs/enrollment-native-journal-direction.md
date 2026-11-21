# Native enrollment journal direction and shared ownership extraction

Status: preparation, no callable native enrollment. No daemon/State/privatefs/
cmd/mnetd edits and no change to installed or mobile behavior. Desktop owner is
working in separate cmd/mnet files. Source parser remains DATA-only.

## Actual source, not a second generic Store

The existing daemon State has WireGuard private/public keys, server/network,
node ID/token and legacy AuthKey. It has no pending enrollment secret/PKCE intent.
`PrepareIdentity` serializes with daemon lifecycle and mu, then calls EnsureKey.
LoadState/Save/EnsureKey themselves do not retain a process-wide file owner.
State.Save uses privatefs.WriteFile: private temp, file sync, close, rename. It
has no directory sync, retained parent/lock identity check or ambiguous publish
outcome. Current joinLocked stores remotely accepted credentials in memory before
Save and returns a save-failed diagnostic on error. A new enrollment path cannot
pretend that this is a durable native pending-intent transaction.

Linux/macOS cmd/mnetd ALREADY takes daemon.lock before creating the daemon and
holds it through main. This prevents cooperative second cmd/mnetd instances.
The lock is not acquired by daemon.New/mobile/direct State writers, and other
platform entry uses the current no-op lock_other. These are exact scope limits,
not a claim that existing Linux cmd/mnetd runs are wholly unlocked. Changing the
process owner and all State writers needs a coordinated actual lifecycle change,
not a second lock arbitrarily added by enrollment.

The implemented control Store already has a stronger local filesystem owner.
This checkpoint extracts ONLY that path/lock/platform primitive to stateowner;
control retains its private wrapper/API, public Close/health/fault policy and all
serialization/publication/rollback logic. Existing test fixture lease path stays
available through that private wrapper. No global Store type is reused for a
native intent. No generic authority/transaction helper is invented.

stateowner.Acquire canonicalizes parent aliases, preserves existing parent
permissions, makes/syncs new parent entries, owns the stable basename.lock,
refuses unsafe links/nonregular locks and checks held parent/lock identity.
Platform mechanics are moved, not rewritten: flock on supported Unix,
LockFileEx/no-delete-share and MoveFileEx WRITE_THROUGH on Windows, unsupported
platform refusal. Check/Close/SingleLink/SyncDir/Rename are mechanical operations,
not a durable journal. Close is idempotent, never unlinks the lock. Callers must
serialize their own state, retain permanent freeze after Check fails, validate
state file links/shape and resync a reopened image before using it. Linux
execution plus Windows/macOS cross-compilation is not native OS acceptance.

## Proposed enrollment-specific journal, before integration

Prefer a separate versioned enrollment journal under the dedicated private
state directory, owned by the future daemon lifecycle. It must not duplicate
WireGuard private key or silently replace the existing daemon identity. It pins
the exact existing durable public key and independently computed text-credential
digest; daemon must re-read/check its owned actual key before each signing/DH step.
Until shared State writer ownership/durability is fixed, preparing this separate
journal does not make that precondition true.

The journal carries native-only verifier, callback state and a new pre-saved
credential secret, plus exact attempt/launch, originally installed config snapshot
and explicit local principal authority/namespace. Secret generation precedes
public launch but successful durable publication precedes browser open. Never
accept a caller-supplied launch as proof of a saved secret. New secret's canonical
text digest must match the existing Multies launch-v1 hash semantics. No new
bootstrap scheme or extra raw credential transmission is approved here.

A bounded single active intent is enough for first integration, not a product
limit chosen for all users. Terminal/history retention and recovery epoch need
an explicit separate policy; neither reuses consent nor mints new identity.
No attempt is silently evicted on capacity pressure. Separate enrollment attempts
must have separate random secret/state/verifier/attempt, not shared session data.

Suggested narrow states:

- prepared: exact native launch + secret/verifier/config durable, no browser
  callback accepted and no credential installed.
- callback-recorded: exact code+saved state/attempt accepted once from owned
  listener; save BEFORE own-PKCE request. Callback code is private and short-lived.
- exchange-started: persist uncertainty BEFORE sending one-shot PKCE. A crash or
  response loss cannot safely repeat it as a fresh handoff. Existing source does
  not redeliver a consumed exchange response. Repeating the exact request can
  only return a result if the server did not consume it, otherwise remain blocked.
- exchanged: save exact checked own-PKCE response and chosen tuple/provenance
  before contacting registrar. Ticket remains private. Current installed config
  and original snapshot must both still allow this exact tuple.
- proof-in-flight: pin exact owned challenge/transcript/MAC; no authority implied.
- accepted-history: exact qualified durable registrar receipt saved, still not
  sufficient to activate the tunnel without current unexpired access validation.
- abandoned/expired/uncertain: no network authority. No automatic browser reopen,
  secret regeneration, identity replacement, approval or new consent loop.

This state proposal is not an enum shipped in the daemon. In particular source
exchange is one-shot: local storage cannot make its consumed response recoverable.
A product recovery path must be explicit and cannot be solved by automatically
starting another consent request after failure. Journal progress should distinguish
not attempted, may have been consumed, and response durably stored.

On process restart the exact loopback socket no longer exists. Do not reconstruct
listener ownership from a persisted port string or silently bind a new port into
the old launch. A prepared intent with no recorded callback is suspended/abandoned
for user-visible recovery. A recorded callback may attempt the same request only
within original lifetime and config, with uncertainty preserved. No new callback
or verifier is accepted for an old attempt. Monotonic time readings do not survive
restart as trusted elapsed time; fresh source and explicit wall-clock assumptions
are required for new authority, never extend original deadlines.

## Journal persistence and integration gates

Use stateowner for journal-only exclusive ownership and exact per-instance real
filesystem seams. Keep immutable candidate state separate from working live
state. File size/schema/version/duplicates/keys/secrets/key-digest/config transitions
validate before publication. No omitted malformed value may become a default.
Private file permissions/DACL, single-link state, bounded bytes, full writes,
fsync+close+rename+parent sync and retained ownership checks precede success.
Prepublish errors leave original state and allow exact retry. Unknown rename or
postpublish error freezes the journal and prevents any browser/network/credential
use until owning reopen validates/resyncs the disk image. Do not delete or rotate
an uncertain intent to make the UI proceed. Close marks unusable before releasing.

Required tests use actual files and fault wrappers: every stage failure and short
write, error-after-rename, directory sync failure, strict reopen, original identity
unchanged, same saved secret/PKCE across exact restart, ambiguous one-shot response
loss, callbacks/old listener restart, second process/death, alias/link/parent/lock
replacement, Close races, deadlines/config removal, no tunnel/credential install
from historical/uncertain states. No fixture-created journal object can produce a
trusted native or Store permit. Affected daemon library/mobile/process owner paths
must join the final design before integration; their existing behavior remains
untouched in this extraction.

Follow-up correction: original sourceOrigin is now a separate required exact HTTPS
field in the unshipped journal and Begin input, not inferred from principalAuthority.
See enrollment-native-source-transport-checkpoint.md for rationale and missing-field
refusal. Tests pin distinct source and principal-authority origins.
