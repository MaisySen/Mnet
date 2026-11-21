# Actual Store/daemon connected-slice preparation

Status: read-only research plus one owning-Store characterization test. No
production Go code, credential schema, registrar API, proof adapter or daemon
consumer changed. Input source is Mnet701643f8; subsequent Multies vector guards
do not connect to this Store. Existing stopped List lifecycle work and held daemon
worktree remain excluded.

## Current source findings

- `internal/control/store.go`: Store has a per-instance sync.Mutex and persists
  the whole storeData JSON. Open does not acquire a path/process ownership lock.
  `cmd/mnet-control/main.go` calls Open directly, no surrounding store lease.
- `saveLocked` creates a private random temp file, writes, file-syncs, closes and
  renames it. No parent-directory sync or post-rename uncertainty result exists.
  `commitLocked` rolls back mutated memory for any returned error. Today rename
  is the final operation, so adding a directory-sync error naively would make
  that rollback inconsistent with a file that might already have been published.
- map version/notifications occur after commitLocked success. Node allocation,
  IP cursor and key-use rollback are centralized in Join, but its authority is
  legacy auth-key/approval, not a reviewed Multies proof. Join+Approve chaining
  cannot be an atomic registrar transaction. Do not use them as enrollment.
- NodeTokens is a plaintext legacy token map. AuthorizeNode compares it against
  caller token; Join returns a legacy token. A digest-only new credential variant
  needs an explicit tagged/typed implementation, not token prefix heuristics.
- validateLoadedShape checks required maps/non-null record shapes, not complete
  referential consistency, receipt uniqueness, monotonic IP cursor or a schema
  version. A new receipt map must be validated on load, not silently inferred.
- DeleteNode removes node/token/provenance but not reusable AuthKeys or any
  revocation tombstone. Earlier executed legacy matrix remains valid: valid
  reusable key can reenroll after deletion. An enrollment receipt/tombstone must
  prevent registrar identity recovery from entering that legacy authority path.
- `internal/daemon/state.go`: current State has key pair plus server/network/node/
  legacy token/AuthKey, no versioned pending enrollment intent. Save uses
  privatefs.WriteFile, which also ends at file-sync+rename without directory sync.
  EnsureKey clears generated in-memory identity after failed save. It cannot
  safely assume a future post-rename error means nothing was persisted.
- daemon.joinLocked saves received legacy node/token before constructing its
  control client; on failed save it leaves received credentials in memory and
  reports setup accepted but storage failed. Existing rejoin uses saved AuthKey,
  with known-node credential-only fallback. Neither behavior supplies the new
  pre-saved credential/idempotent receipt protocol.
- cmd/mnetd acquires a stable no-follow regular single-link flock for the private
  directory before daemon.New on Linux/macOS. The lock inode is never unlinked.
  Other-platform lock helper is explicitly a no-op. The internal daemon package
  itself is not the singleton owner. No cross-platform/store singleton claim.

## Executed new characterization, not desired acceptance

`internal/control/registrar_prerequisite_test.go` opens two actual Store objects
on the same path. First writes first-only network; stale second writes second-only
network; disk reopen loses the first network while the first process still
believes it exists. Ten race-enabled repetitions pass this characterization.
This is an executable counterexample to a cross-instance atomicity claim, NOT a
working registrar feature or a regression contract requiring lost updates forever.
The test says to revisit/remove its expectation once ownership is implemented.
No fake secondary registrar store was introduced.

## Concrete implementation boundary before a consumer

1. Establish owning Store path lock lifetime first, including Close/release and
   documented unsupported-platform refusal. Reuse proven lock principles, not
   necessarily daemon code copied into a different ownership domain. Account for
   file aliases, symlinks/hardlinks and lock-inode replacement. Verify second
   process cannot open writable state, crash releases lock, replacement is not a
   bypass, and test cleanup closes ownership. This changes existing Open callers
   and must be designed as one connected lifecycle, not hidden in a new commit
   method. No production behavior changed in this preparation.
2. Specify persistence publication outcome: pre-publish failure is rollback-safe;
   durable success requires file and supported parent-dir sync; uncertain after
   rename must freeze further authoritative mutation and require verified reopen/
   reconciliation, not roll memory back and continue. Version/watch publication
   cannot assert clean success on uncertainty. Inject open/write/file-sync/close/
   rename/dir-sync failures through a bounded owning persistence seam, not global
   filesystem monkey-patching. Test actual reopen/notification visibility. OS
   unsupported sync semantics remain explicit rather than mislabeled guarantees.
3. In that lifecycle, inactive typed enrollment transaction must atomically persist
   one node/address, native credential digest variant, qualified immutable receipt
   and consumed proof/descriptor metadata. One canonical descriptor/receipt scope,
   same attempt/reserve retry returns same receipt; changed fields refuse without
   IP allocation/key consumption/map bump. Public key or service ack alone never
   establishes possession. Do not accept caller-created proof=true booleans.
4. Add persistent registrar-origin tombstones and isolate legacy Join/Approve/
   rejoin for those identities before enabling any credential consumer. Node
   deletion cannot erase standing history to permit new authority. Current valid
   legacy keys remain independent authority unless a separately scoped deliberate
   policy changes them, not an accidental side effect of this work.
5. Only after persistence/receipt/revocation tests and design reconciliation may
   daemon pending intent/current allowed-map/selected tuple be implemented through
   its existing state owner. No source response may set a new trusted origin.
   Then reviewed exact proof descriptor/adapter and local installed end-to-end
   acceptance, followed by physical two-host/OS/revocation. No routes, tunneling,
   account changes, release or activation are part of this preparation.

This plan identifies load-bearing lifecycle work, not a promise that file rename
or a race-enabled unit suite proves power-loss safety. Production receipt scope,
lease/revocation/window semantics and native credential authority remain held.
