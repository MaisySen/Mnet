# Owning Store lifecycle implemented checkpoint

Isolated development prerequisite only. No enrollment transaction, receipt,
proof adapter, native consumer, deployment or activation was added. This
supersedes preparation-only wording in the preceding design documents; their
failing baseline log is retained separately for comparison.

## Implemented behavior

- Actual control.Open acquires a retained OS ownership lease before reading or
  returning absent state. Every Open participates. Absolute canonical parent
  aliases converge on a stable per-state sibling lock; existing shared-parent
  mode/DACL is not changed. New directories are created privately and their
  parent entries synced. No lock is unlinked at Close.
- Linux/macOS and BSD use nonblocking flock plus no-follow regular/single-link
  inspection. Windows uses native nonblocking exclusive LockFileEx and an open
  handle that does not share delete access. Windows is not a silent no-op.
  Unimplemented target platforms return an explicit unsupported error.
- State and lock links are rejected, lock and parent identities are retained
  and checked, and replacement freezes authority rather than silently acquiring
  a new ownership inode. Existing lock/state privacy protection remains in use.
- Close is idempotent, permanently marks that object closed before lease release,
  clears relay-index authority and wakes map waiters. It does not increment the
  map version. All restarted tests now close the old real Store before reopening;
  ordinary fixture owners register cleanup. No mutex-containing Store is copied
  and no test bypass reopens unowned state.
- Real persistence has a private per-instance filesystem seam. Full writes,
  file sync/close, rename and directory publication are distinct operations.
  Before-publish failures roll back the provisional mutation with no key-use,
  IP-cursor, notification or disk-image change. Temp files are cleaned.
- If rename returned an error, rollback is allowed only when both its original
  source inode and its old destination (or absence) are demonstrably unchanged.
  An error after a real successful rename therefore freezes, rather than undoing
  memory as if publication did not happen. Unclassifiable results also freeze.
- After-publication sync/identity failures retain candidate state privately,
  return errors.Is(ErrStoreUncertain), and freeze authority. No successful join
  response or map bump is returned. Existing waiters wake to recheck health.
  A new owning Open resyncs the loaded image before serving it, not a claim that
  the earlier write survived power loss.
- Health gates cover existing and no-op mutations, credential/status/map/account
  reads, alternate snapshots and per-packet RelayAllowed, including the queued
  delivery callback already used by the relay. Touch does nothing while frozen.
  HTTP root middleware returns 503/no-store on subsequent unhealthy requests,
  including healthz, rather than advertising healthy empty state. The mutation
  request that first discovers an I/O failure still returns its existing error
  response, never success. A request already linearized before freeze can finish.

## Executed evidence

- Original five desired lifecycle tests first failed against the actual Store.
  They are now enabled in the ordinary supported-platform graph and pass.
- Added real subprocess ownership and killed-child lock-release/reopen tests.
  Ten race-enabled focused repetitions passed, including the initial publication
  matrix. No stale PID logic.
- Eight-stage actual-file failure matrix passes: create temp, write, short write,
  file sync, file close, unchanged rename failure, error after real rename and
  error after real directory sync. It compares real JSON/disk images, key uses,
  node count, map version, waiter visibility, cleanup and owned reopen.
- Frozen/closed account, token, status, map, snapshot, key, approval, membership
  no-op and HTTP gates are exercised. Lock-path replacement and non-revival are
  exercised on Linux. Close unblocks a registered long map wait.
- Full Linux go test ./... passed after lifecycle integration. Final affected
  go test -race -count=1 ./internal/control ./cmd/mnet-control passed after the
  extra error-after-rename case; affected go vet and diff checks passed.
- Final control test binaries cross-compile for Windows amd64 and macOS arm64.
  This is compilation evidence only, NOT native execution or OS acceptance.

## Residual limits and review focus

Windows publication uses MoveFileEx(REPLACE_EXISTING|WRITE_THROUGH), plus the
already file-synced temporary image, instead of POSIX parent-directory fsync.
The Windows parent check is not labeled a directory flush. This preserves a
native Windows control implementation without claiming identical filesystem or
power-loss semantics. Windows LockFileEx/DACL/rename/process-death tests and macOS
flock/directory-sync/process tests still require native execution before shipping.
No claim of full cross-platform durable acceptance is made by cross-compilation.

Advisory locking assumes cooperative Store writers and a supported local
filesystem. Same-UID hostile mutation, network-filesystem/multi-host lock
semantics, malicious storage and universal hardware power-loss guarantees are
not covered. Lease/path validation is an integrity guard, not an adversarial
filesystem transaction. Existing privatefs daemon saving is unchanged by this
control-only batch. In-flight already copied maps and already distributed direct
mesh traffic cannot be revoked by a later control freeze.

The legacy rejoin authority matrix is preserved, including valid reusable-key
reenrollment after delete/expiry. This prerequisite does NOT repair that policy
for registrar-created identities: no such credential/identity variant exists yet.
Persistent registrar tombstones, digest credentials, immutable receipt schema,
proof/service scopes and native pending intent remain unimplemented. No new
consumer can be connected until those authority rules are separately implemented
and reviewed. Full unmodified D1 migration failure remains unrelated and open.
