# Explicit retained-attempt inspection and recovery

This is a separate disposable development entry point. Nothing is installed,
activated on a real account, started by the normal daemon, or connected on startup.
Screen, keyboard and mouse access are outside this command.

Build explicitly:

```
go build -tags mnet_enrollment_development -o /tmp/mnet-retained-cli ./cmd/mnet-setup-development
/tmp/mnet-retained-cli --disposable-root /tmp/EXISTING-PRIVATE-ROOT --inspect-retained
/tmp/mnet-retained-cli --disposable-root /tmp/EXISTING-PRIVATE-ROOT --recover-retained
```

The original private root must already have the disposable marker, exact local
HTTPS source/registrar policy, finite limits, existing native State/key, and
existing native/journal ownership locks. These commands cannot create a missing
attempt or resume a lost browser listener. Inspection opens existing locks
without create, chmod or sync, and returns a sanitized local projection only.
It does not make an HTTP request or change the inspected tree. A held owner,
missing State/journal/lock, malformed or noncanonical journal, wrong key, changed
policy or saved-document/finalization mismatch is refused, not repaired.

Recovery is explicitly requested and finite: at most the existing three-second,
two-attempt same-attempt history policy. The retained journal budget is not
reset by reopening. It never submits another proof, exchanges another source
code, generates a replacement key, starts a tunnel, renews a receipt or performs
auto-resume. An already saved result is only inspected, without another lookup.
A prepared original-State recovery uses the existing private recovery owner and
can publish only the exact original joining within its original access interval.
A lost-submit history recovery returns authenticated historical delivery only;
it does not install a credential or manufacture proof confirmation. Expired
original access cannot finalize. A historical receipt is not a current
revocation check or proof of an active network.

`--open-browser` optionally opens a private temporary historical HTML file
outside the inspected root. No result page has network controls. Opening the
page does not change joining success or failure. There is no generic HTTP
control endpoint in this feature. The page and JSON always distinguish saved
joining or recovered history from a currently verified network. All network
selectors are mutually exclusive with these retained-attempt selectors.

## Reproduction and evidence scope

The companion Multies tests require their lockfile dependencies, Go/toolchain,
OpenSSL and the fixture TLS trust setup. For real CLI inspection/prepared recovery:

```
MNET_NATIVE_REPO=/path/to/mnet \
MNET_DEVELOPMENT_RECOVERY_FIXTURE=1 \
MNET_RETAINED_CLI_BINARY=/tmp/mnet-retained-cli \
node node_modules/vitest/vitest.mjs run tests/mnet-registrar-workflow-cross-app.test.ts \
  -t 'workflow (prepared-finalize|prepared-history-finalize|history-recovery)$'
```

The expired lost-submit case uses the exported recovery adapter and real
source/registrar coordinator, including its exactly-one-history-request and
unchanged-original-State assertions. Prepared proof and prepared historical
provenance cases invoke the actual CLI and retain the coordinator's original
receipt/current-peer assertions. All three invoke the real inspection CLI.
They are not physical-device tests.

For the actual installed command and optional system-browser path:

```
bash tests/mnet-retained-validation.sh /path/to/mnet /path/to/multies
```

This uses an isolated HOME, Chrome profile and MIME handler, Xvfb and dbus. It
executes actual joining, then CLI saved inspection and expired saved-idempotent
recovery, checks there are no controls, and captures screenshots. The expired
saved case performs no history lookup. These pixels do not prove the distinct
expired-history or prepared-finalization display branches. New local tests
cover read-only tree preservation, original-owner conflict, missing/malformed
State/journal, changed valid key, and changed installed policy. Real prepared
fixtures also reject a well-shaped saved original-State revision mismatch.
Existing full daemon tests separately cover original-access expiry, history
budget retention, ownership and failure transitions of the reused recovery core.

No consumer-ready, full-repository race, broad TypeScript, production account,
service, release or deployment claim follows from these selected tests.

## Exact already-saved bookkeeping recovery

The preceding immutable6fb14ff checkpoint refused published-State/prepared-
bookkeeping images. This separate increment admits ONLY the exact already-saved
native document matching its original prepared journal, original-State revision,
current policy revision and device key. Inspection is still nonmutating and
reports `joiningSaved:true`, `bookkeepingPending:true` and
`recovery:saved-bookkeeping-pending`, never a verified network.

Explicit `--recover-retained` reacquires/binds the same existing State/policy/
journal image and calls the EXISTING exact `reconcileNativeFinalization` /
`CorrelateFinalization` bookkeeping transition. It does not construct authority,
publish State, contact source/registrar, fetch a map or renew access. Output is
`bookkeepingCompleted:true` distinctly from `joiningFinalized:false` and
`historicalRecovered:false`, always `networkVerified:false`. Expired documents
may complete bookkeeping only; their original expiry and current-access refusal
remain unchanged. A second call is a saved historical no-op, not another repair.
No other corrupt/incomplete image or normal-startup auto-repair is admitted.

New actual fixture injects a failure only AFTER the real State rename, closes
the original owner, inspects that crash image without writes, invokes the real
CLI, checks byte-identical State/key/document/receipt/expiry, and checks exact
prepared-to-completed bookkeeping plus idempotence. During a marker-delimited
inspection/recovery window the independent real registrar and source HTTP
handlers count requests and must both see zero. Normal proof/source traffic
outside that window remains expected; this is not a claim of zero setup traffic.
Malformed/missing State/journal variants are refused without repair.

Run real unexpired post-rename composition from the companion repository:

```
MNET_NATIVE_REPO=/path/to/mnet MNET_RETAINED_CLI_BINARY=/tmp/mnet-retained-cli \
MNET_DEVELOPMENT_RECOVERY_FIXTURE=1 MNET_DEVELOPMENT_BOOKKEEPING_FIXTURE=1 \
node node_modules/vitest/vitest.mjs run tests/mnet-registrar-workflow-cross-app.test.ts \
  -t 'workflow success$'
```

Add `MNET_BOOKKEEPING_EXPIRED=1` for the explicit expired variant. That variant
waits the ORIGINAL deadline, checks the real registrar still refuses current
access, and uses a distinct expired-bookkeeping outcome, not a live-peer claim.
For actual CLI/system-browser pending and completed expired pages:

```
MNET_BOOKKEEPING_EXPIRED=1 bash tests/mnet-bookkeeping-validation.sh /path/to/mnet /path/to/multies
```

The isolated integration also includes the separately reviewed27eb931 dashboard
correction. Its exact product diff is a cherry-pick, not a new authority path.
Combined six exact-client and three real recovery cases, actual Stop/expiry and
saved historical-page regression are separate evidence, not sibling assumptions.
No physical-device, installer, startup or complete general crash-repair claim.

The first integrated retained-browser rerun failed before retained CLI work: the
coordinator refused a current-peer count differing from the historical setup
count. That failed run lacked detailed receipt clocks; timing is a plausible
cause, not a conclusively measured attribution. The opt-in retained-browser
fixture now uses the reviewed dashboard's readiness anchoring too, without
changing any lease. The failed log is retained alongside the subsequent pass.
The integrated candidate has affected-package full race/vet/defaultbuild passes,
not a fresh full30-default-package race claim for this newer bookkeeping head.
The earlier corrected-only27eb931 full30-package evidence stays separately scoped.
Broad TypeScript remains unverified after a resource-related SIGKILL with no
source diagnostics; this is not a typecheck pass.
