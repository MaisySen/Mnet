# Native State ownership prerequisite, initial design and failing contracts

Preparation only. No native enrollment/account/tunnel activation. Shared paths
reserved by main, desktop owner remains in cmd/mnet UX files without dependency.

Planned files: internal/daemon/state.go plus new state_owner/state_publication
files, daemon.go/identity.go/mobile.go writer call sites; cmd/mnetd/main.go and
lock_unix/lock_other plus tests; mobile/mobile.go lifecycle only if required by
actual owner contract; cmd/mnetd/selftest.go/sessionproof.go callsites/tests. No
privatefs change unless a source-grounded shared primitive gap is demonstrated.
Existing stateowner is reused, not another duplicate platform-lock implementation.

Current normal runDaemon holds daemon.lock. Its sessProof branch precedes the
lock. New/mobile/standalone LoadState/Save have no retained library file owner.
State.Save is temp sync+rename only. State fields are exported mutable DTO data,
including a legacy AuthKey and key pair. No State snapshot may carry the daemon's
lease and accidentally close it. No private key is moved into the enrollment
journal, migrated, rotated or silently re-created from corrupt input.

Proposed direction: private daemon State owner retains stable daemon.lock through
actual library lifetime and serialized candidate commits, replacing the process
wrapper rather than stacking the same lock. Daemon.Close marks unusable/cancels,
finishes workers/teardown, then releases owner. New refuses a second owner even
before first state publication. Mobile/New/selftest/sessionproof use the same
library owner. Unknown publication freezes every native action that could use or
publish the uncertain identity/config; Close/reopen validates/resyncs whole disk
image. Existing raw State remains a detached DATA snapshot, not a lease handle.

Standalone legacy LoadState/Save need deliberate semantics: LoadState can read a
detached snapshot under a temporary shared primitive owner; Save must reacquire
that same exclusion, compare original file existence/byte identity against its
loaded revision, then publish, not blindly overwrite a newer state. A fresh
State.Save may only create absent state, not replace an existing identity without
provenance. Value copies retain old revision BY VALUE, never share a mutable
revision pointer that could make stale copies current. A live daemon owner blocks
these standalone writers. Daemon's own mutable working State uses its retained
private owner, not public Save that would reacquire/compete with its own lock.

Exact byte revision CAS is concurrency evidence, not cryptographic authenticity
against a hostile same-UID writer. File shape/links/key pair/config validation and
no-follow owner checks remain mandatory. Separate direct-save reacquisition is
fresh recovery, not resurrection of a faulted daemon object. All prepublish
failures preserve old working/disk image; ambiguous error-after-rename or dirsync
freezes retained owner, never rollback-and-continue old authority. Native State
identity/key publication and pending enrollment journal remain distinct files;
there is no claimed two-file atomic transaction or installed enrollment yet.

Initial opt-in native_state_ownership_contract tests use actual existing New,
Close, LoadState and Save APIs: second daemon even absent state refuses; standalone
Save cannot bypass live daemon; detached stale snapshot cannot overwrite a newer
save. These are deliberately failing current-source expectations, not production
acceptance. No tests from the stopped List review or old worktree are inspected.
Next: actual real-file failure seam/rollback/uncertainty/reopen and complete writer
migration, then ordinary affected regressions and actual mobile/selftest/session
proof coverage. Windows/macOS compile alone cannot establish native execution.
