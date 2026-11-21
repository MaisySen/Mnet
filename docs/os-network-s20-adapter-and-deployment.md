# S20 inert adapter session and ownership architecture

## Current facts and limits

S19 actual sandbox node open denied EACCES13. No retry, workaround, permission
change, install, privileged helper or alternate machine probe. This blocks this
runner's real TUN create/conformance path. User deployment capabilities remain
UNKNOWN, not inferred from runner0600 node or reviewer machine. Successful S11
read-only bootstrap did not prove OS networking; S19 valid denial is not TUN
support. Earlier failed runs and accidental namespace mutation incident retained.

## New inert source component

adapter_session.go contains canonical creation handle+baseline/phase bookkeeping,
no syscalls/process/device callbacks and no Kernel interface implementation.
No caller/entry imports or constructs it outside future tests. Pure changes
only, existing reviewed preflight candidate live code unchanged.

Canonical handle bound once from exclusive creation evidence, never caller
name/ifindex adoption. Caller cannot promote TagConfirmed; stale caller tag may
lag canonical confirmation but identity/CreationLink fields must match. Exact
opaque mutation builder validates requested operation, separately reviewed
IPv6 writer still required. ACK and exact inventory readback BOTH required before
promoting tag/address phase. Ambiguous mutation error poisons forward progression,
never interprets error as no effect. Original ownership still recorded for future
read-only inspection and reviewed cleanup design. Close intent one-shot, close
success distinct from exact whole baseline absence. No cross-run hashing or
speculative volatility filtering.

This is NOT a full adapter: Snapshot/Create/Apply/Close effects remain held,
cleanup after poison needs a separate narrowly validated cleanup path, owned
raw schemas still strict/synthetic. No claim of comprehensive operation ordering
beyond listed phase gates. Source is not yet tested or released to run tests.
Do not wire it into Transaction until ACK/readback/cleanup model tests reviewed.

## Supported portable ownership design proposal

Platform-neutral contract: networking, Remote Desktop and shared input are
THREE distinct tools/authorities. Network owner grants interface/address/route
and lease scope only, never screen/input consent. Cross-platform transport and
trusted pairing can share foundations, not implicit authority.

Linux supported model: minimal dedicated network owner keeps original TUN FD
and exact namespace custody, client app zero caps/NNP/no TUN FD, precise per-peer
routes, no host default route changes. Namespace-only proof first on a genuinely
suitable explicitly authorized environment, no silent host fallback. If installed
host networking ultimately needed, separate least-privilege service and install
consent/design review, exact resource journal/reconciliation and teardown. Device
node access prerequisite separate from CAP_NET_ADMIN target namespace authority.
No in-run privilege escalation to bypass denied device access.

macOS supported design candidate: platform network-extension owner controls
packet tunnel through platform API and OS approval, signed app/extension,
entitlements/provisioning and lifecycle managed there. Windows candidate:
explicitly installed signed virtual-network driver/service ownership, OS access
control and approved install/update/uninstall. These are proposals requiring
current vendor/SDK/toolchain verification before implementation, not availability
or entitlement promises. Existing cross-platform UI alone cannot prove either.

Portable session contract should describe assigned own/peer identities, exact
routes, expiry without extension, resource ownership tokens, capability failure
as a supported blocked state, and independent screen/input grants. Persistent
records must say creating/active/stopping/pending cleanup, never active merely
because a device opened or transport request returned200. Reopen should require
resource+lease revalidation, never adopt same-name objects automatically.

## Next unblocked product slice

1. Pure canonical session/adapter tests using raw fixtures+fake operations:
   stale/promoted handle refusal, alias ACK+readback, ambiguous address add,
   exact cleanup, close failure and same-number/ifindex replacement. No launch
   dependencies/tags changed, review list before resuming local tests.
2. Implement adapter interface with INJECTED inert effects only, no real binding.
   Bind exclusive creation token to canonical ledger and update transaction
   integration explicitly. Cleanup must work conservatively after poisoned
   forward setup without granting new forward work. Existing Compose raw owned
   schema must not be widened without real platform fixtures and source review.
3. Product capability/status projection from evidence states: runner blocked
   node access, install/setup required only when independently grounded, paired
   permission expired, or cleanup pending. No phantom connected/ready state.
4. Installer/privileged owner/platform APIs are distinct later product gates,
   not another sandbox preflight repeat. Need genuinely new authorized environment
   evidence before any real TUN create. No existing approval implies that gate.

No test or new device/namespace/process action occurred for S20. Parent owns
next slice/review choice, no automatic continuation into live effects.
