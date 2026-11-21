# Test-only B-enforces-A original-expiry spike

No non-test source changed. Accepted Mnet 5f8eebd and Multies 8b9296f remain
immutable. This fixture adds neither grant/renewal authority nor a duration,
carrier, missing-expiry/capability policy, normal startup or OS interface.

Two real Multies browser/session authorizations are coordinated through actual
application/DB endpoints. Each distinct native daemon owns its own State key,
callback, source exchange, real proof, confirmed receipt and sole native
finalization. A finalizes first; B starts two seconds later. The stagger is test
scheduling only, never an edited observation, receipt, challenge or deadline.
The unchanged original challenge/max-lease profile is 10000ms/15000ms and each
actual access deadline remains clipped by the original initial-authority rules.

Each owned key starts a real userspace WireGuard device with proofnet TCP, no
OS device. A is deliberately a bare probe: no expiryLoop is run on A, so A's
physical ability to send after its deadline is not a claim of valid authority or
A-side enforcement. B uses the actual daemon applyMap/refreshPaths/expiryLoop.
No installed connection session is used because it retires the whole network
on peer expiry and would not establish survivor enforcement.

Before A expiry, authenticated maps supply exact peers, real endpoints and
original expiries. Both directions establish persistent TCP connections, send
fresh random32-byte application challenges, and receive the exact acknowledgement
from a receiver whose acceptance counter advances once. Local Write success
alone is not treated as delivery.

The coordinator SIGSTOPs the separate disposable registrar process, verifies
/proc process state T, then acknowledges the cut. B attempts a genuine native
CurrentMap call while the service is stopped; it returns no map. This is a
stopped-service/TLS-request outage, not a claim that an already authenticated
in-flight HTTP map handler was paused. B's expiryLoop and device keep running.

At A's original absolute expiry:

- B removes A's actual native peer public key and allowed IP, checked through
  WireGuard IPC, not only a map/status list.
- Fresh challenges on both preexisting TCP streams fail to receive valid acks.
  Both receiver acceptance counters stay at one, checked again after B expiry.
- A's bare device still has B's allowed IP and remains physically alive, so the
  negative result is not explained by A self shutdown.
- B remains up with a live device under its own still-valid original receipt.
- Applying the same old authenticated map and refreshing paths cannot reinstall
  A. No newer map or successful control response is needed for removal.
- B then becomes key-expired at its own unchanged original deadline. Receiver
  and expiry-loop cleanup is joined; final owner/device cleanup is explicit.

This proves B-side native enforcement only. It does not prove both endpoints
comply, immediate source revocation, renewal, rollback defense, mixed-version
refusal or product-ready timing. TCP stream objects are explicitly cleaned up at
fixture end; peer-key removal/failed delivery is not claimed to auto-close every
application stream at A's deadline.

The final exact run measured original A=2026-11-21T16:52:14.448Z and
B=2026-11-21T16:52:16.713Z, survivor margin2265ms, receiver counters1/1 before and
after. Before final extra assertions, earlier runs passed with margins2273/2301ms.
No runtime failure was retried into a pass. ESLint initially found one unused
copied import, fixed before the integration execution; no assertion was relaxed.
Five targeted existing daemon expiry regressions pass with -race, and tagged
daemon vet and the new coordinator ESLint pass.

The margin is a harness limit. The fixture reports explicit harness-inconclusive
failures when positive traffic or postexpiry probes cannot fit the original
windows. It does not extend time or retry-until-pass. Run this finite-time test
without concurrent CPU-heavy suites. A successful run is not an all-load claim.

## Reproduce

Use accepted Multies dependencies and this source pair, plus Go, OpenSSL and Linux
SIGSTOP/proc process-state support. No browser binary or real account is needed;
actual authenticated application test sessions perform the two explicit decisions.

    MNET_NATIVE_REPO=/absolute/mnet node node_modules/vitest/vitest.mjs run tests/mnet-peer-expiry-survivor.test.ts

The coordinator builds the unchanged V2 command and a tagged race daemon test
binary, then selects TestPeerExpirySurvivorFixture. An unconfigured direct Go
invocation skips rather than manufacturing receipts. Existing targeted boundary:

    go test -race -tags multies_native_fixture ./internal/daemon -run 'Test(LocalExpiryWithoutControlResponse|DaemonExpiryWithBlockedControlAndRestart|ApplyMapRejectsExpiredAuthorization|ExpiryGuardRenewsAndWakesFromNoDeadline|ExpiryEqualityAndNoDeadlineCompatibility)$' -count=1
    go vet -tags multies_native_fixture ./internal/daemon

Independent review is pending. No broader readiness or execution authority follows
from this packet.
