# Proposed owning Store lifecycle contract

Preparation only, no production implementation. Applies to the actual control
Store and every existing writer/authority reader before a registrar transaction
can be safe. This is not another registrar storage implementation.

## Ownership and compatibility

Preferred contract: Open is writable and exclusive from its first successful
return, including absent state. Store.Close is idempotent, marks closed under
Store.mu before releasing the lease and blocks later authority/mutations. Reopen
is a new object reading disk, never resurrection of the old object. Make callers
close explicitly, including tests that currently Open the same file twice while
old objects remain live. cmd/mnet-control owns Close at shutdown. An inspect-only
future reader needs a separately named snapshot operation, not a second writable
Open. No new Open bypass for tests or registrar.

Resolve an absolute cleaned path and its existing parent aliases before opening
one stable sibling lock for that state name. Create missing parents conservatively
without chmod of an existing shared parent. Reject final state/lock symlinks,
nonregular files and multiply linked files; use no-follow file descriptors and
verify identity against path after acquisition. Do not lock the state file inode,
which rename replaces. Keep the stable sibling lock inode, never unlink on Close.
Separate state basenames in one parent need distinct locks, not a global parent
flock. Parent aliases must converge or refuse; aliases are not permission to open
another writer. Validate retained parent/lock identity before each publication,
and fail closed if replaced. Existing shared-parent permission preservation test
must still pass. A private state filename does not justify chmod of its parent.

Limit honestly: a same-UID hostile actor able to replace the lock/parent can also
change the state and binary. Cooperative single-writer and filesystem integrity
checks are the supported boundary, not hostile-same-UID exclusion. Remote/multi-
host network filesystem advisory-lock and durability semantics are not covered.
Unsupported platforms/filesystems refuse this future owned writable mode rather
than treating a no-op lock as exclusive. Linux first; macOS behavior requires
execution evidence, Windows needs a native owner implementation before this
contract can replace its currently working control lifecycle.

Actual unresolved compatibility tradeoff: replacing Open immediately with a
Linux/macOS-only lock would regress current Windows control use. Prefer an
explicit unsupported owned-mode refusal ONLY when the new enrollment capability
is requested until Windows ownership is supplied, while clearly retaining legacy
Open as non-registrar-safe. This cannot create mixed owned/unowned writers on one
state: owned-mode needs ALL opens to participate in the same exclusion on the
supported platforms; feature-only locking is insufficient. Therefore either
implement Windows ownership in the prerequisite batch, or isolate the build/
supported platform release boundary. Do not silently claim a universal guarantee.

## Persistence outcomes and immutable candidate

Owning Store persistence gets a per-instance filesystem-operations seam, initialized
with real OS calls by Open. No exported runtime test hooks and no global monkey
patch. Operations: create-private-temp, write, sync-file, close-file, rename,
open-directory, sync-directory, close-directory, inspect path/lease identity.
Tests wrap the real operations and inject one precise failure; they still write
and reopen the actual Store image. Short writes are failures. No fake receipt
store. Record operation ordering and cleanup.

Three outcomes:

- notPublished: error before rename succeeds. Current rollback closure restores
  live image; no version bump or watcher wake claiming success. Temp cleaned.
- durable: full write, file sync and close, rename, parent directory sync succeed.
  Only this returns successful mutation/receipt and normal map publication.
- uncertain: rename succeeded but directory sync/open or verification failed, or
  publication result itself cannot be classified. Do not roll the candidate back
  into a working old-authority Store. Freeze all authority and mutations. Keep
  diagnostic cause but return a typed persistence-uncertain error without receipt
  success. Close/reopen under ownership is required. No in-process automatic retry
  to allocate another identity. Reopen validates the current disk image; it does
  not retroactively prove the previous write survived power loss. Recovery must
  re-sync the loaded file/parent under lock before calling its receipt durable.

No uncertain-commit notification should be labeled a normal map change. Wake
waiters separately to recheck health and return unavailable. Closed/frozen
AuthorizeNode, JoinStatus, Map, IdentityDevices and operator mutation endpoints
refuse; RelayAllowed returns false (including the per-packet queued delivery
callback). Touch becomes no-op. Network/Networks/Snapshot must not provide an
alternate enrollment or authorization source while unhealthy. WaitForChange's
int64 API cannot express health; keep wake-as-recheck and require the following
Map gate, rather than pretending version increment repairs health. Health must be
checked BEFORE idempotent/no-op mutation returns, not merely inside commitLocked:
SetNetworkMember allow-existing and deny-absent currently return before persistence.
SetNetworkMember has no map bump by design; membership-only revocation is not
existing device revocation and this prerequisite must not silently change that.

Availability tradeoff: freeze includes existing legacy peers, not just enrollment,
because an uncertain image can contain ACL/device/key/membership changes. Keeping
old data serving could restore revoked authority. Prefer process unhealthy/503 and
packet denial rather than continuing from a rollback snapshot. This costs control
availability on a storage error. It does not revoke already distributed mesh
maps or direct peer traffic, which need their own lease/revocation protocol.

## Failing-first checkpoint

`registrar_lifecycle_contract_test.go` is opt-in with build tag
`registrar_lifecycle_contract` on Linux/macOS. Five real Store tests fail now:
second writer before first write, parent alias, hardlinked state, missing Close/
authority shutdown, independent subprocess writer. No production implementation
or consumer added. The ordinary test graph excludes the desired failing contract.
The separate untagged lost-update test remains a hazard characterization and must
be removed/replaced when ownership changes. A passing hazard test is not feature
acceptance. Baseline expected failure log is retained separately.

Next tests, once exact seam contract is reviewed: each prepublication failure
preserves image/key uses/IP/version; postrename sync failure freezes node/account/
map/relay/no-op membership gates and wakes pending map waits without a success;
reopen under lease yields one whole old/new image and same immutable receipt, no
new allocation; Close races and process death release; replaced lock/parent
refuses write; all existing reopen tests migrate to Close. Receipt and tombstone
schema is not invented in these prerequisite tests.
