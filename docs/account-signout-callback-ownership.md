# Account sign-out callback ownership

The existing Mnet control-service account view is still a separate, test-only member view. This change does not implement or deploy Multies sign-in, enroll devices, change membership, release an app or touch the production control service.

## Correction

Previously, sign-out revoked an existing account-session cookie but left the browser's pending account login usable. A provider callback could finish after sign-out and issue a new session. The same problem occurred when sign-out happened while the provider token exchange was blocked.

The callback now claims its one-use state without removing the server's revocation handle during exchange. Session issuance checks that exact claimed, unexpired attempt under the same account lock used by sign-out. Sign-out retires the originating browser proof's pending attempts and any newly issued session associated with it, then clears both account cookies. This also covers a callback that has minted a token but whose response has not reached the browser yet.

Starting a replacement login retires the prior browser proof. An older blocked callback therefore cannot restore access after the replacement login is signed out. Other browsers' pending logins and active sessions remain separate. Provider identity, explicit current membership, browser binding, expiration, one-use callback behavior, pending-state/session caps and same-origin JSON write checks remain in place.

## Evidence

Actual HTTP regressions first failed against the old source for pending, in-flight exchange and superseded-login cases. The fixed tests cover those cases, fresh login after sign-out, and isolation of another browser's pending login and active session.

A separate race regression runs 1,000 session-issuance/sign-out interleavings per repetition, checking that neither lock order leaves a usable token or retained flow. Exact-attempt tests reject retired, unclaimed, expired and changed browser/network/provider snapshots. The focused family passes 20 race-enabled repetitions, including 20,000 issuance/sign-out interleavings.

The full repository race test, vet, tracked-source credential/style check and existing isolated real-Chrome account journey pass locally. Normal hosted checks are tracked separately. Chrome uses a disposable signed-provider fixture, not a live user account or production identity provider. No installed app, native authentication bridge, Multies integration or live deployment acceptance is implied.

## Reproduction

```sh
go test -race -count=20 -timeout=2m ./internal/control \
  -run '^TestAccountCallbackSession|^TestAccountLogout|^TestAccountNewLogin'
go test -race -count=1 -timeout=5m ./...
go vet ./...
MNET_ACCOUNT_BROWSER=1 go test -race -count=1 -timeout=2m \
  ./internal/control -run '^TestAccountRealChrome$'
sh scripts/check-secrets.sh
```
