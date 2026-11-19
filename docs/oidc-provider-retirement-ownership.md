# Provider-retirement ownership for node approval

This correction is confined to the existing Mnet control-service engineering login flow. It does not add a Multies authorization bridge, enroll any real device, change the configured provider or deploy the control service.

## Failure reproduced

Four isolated HTTP regressions showed that an old provider could approve a waiting node and persist its identity after the configured provider had been replaced or disabled during token exchange. Reinstalling the same old provider object could also revive a pending approval started before replacement.

Checking the provider only before the token exchange was insufficient. The exchange runs outside the server's provider lock, so its result needs a second authorization check at the point of persistence. Comparing provider pointers alone cannot distinguish a newly restored provider from its retired attempts.

## Correction

- A callback claims one state but retains its server-owned revocation handle through the exchange. Duplicate callbacks cannot enter the exchange or retire its owner.
- Replacing or disabling the provider clears pending and claimed approval states. Installing the same provider without a change remains idempotent.
- Selecting the provider and inserting a new approval state share one retirement scope, preventing a late start from inserting an old-provider state after replacement.
- Final approval checks the exact claimed, current and unexpired attempt, then persists identity while serialized with provider replacement. No provider network request runs under these locks, and the start handler releases them before writing the HTTP response.
- All completion and denial paths retire the claimed state. The pending-state bound includes active exchanges. Existing node credentials, admission rules and manual approvals are unchanged.

The ordering is deliberate: an approval persisted before provider replacement remains an approval. Replacing a provider is not a command to revoke already approved devices. A replacement that wins before final approval prevents the old callback from persisting it. A fresh approval through the current provider remains possible.

## Verification

The four old-source HTTP failures now pass. Additional cases preserve same-provider idempotence, reject replay and duplicates during exchange, and verify that a fresh current-provider approval persists. Exact-state tests reject retired, unclaimed, expired and changed node/provider attempts, including restore of the old provider object.

Twenty focused race-enabled repetitions, the full repository race suite, vet, the existing isolated real-Chrome account journey and tracked-source checks pass locally. These use disposable local stores, providers and nodes. Hosted checks are separate evidence, not a live deployment or native/device acceptance.

```sh
go test -race -count=20 -timeout=2m ./internal/control \
  -run '^TestOIDCApprovalRequires|^TestOIDCProviderReplacementRetires|^TestOIDCStateBounds'
go test -race -count=1 -timeout=5m ./...
go vet ./...
MNET_ACCOUNT_BROWSER=1 go test -race -count=1 -timeout=2m \
  ./internal/control -run '^TestAccountRealChrome$'
sh scripts/check-secrets.sh
```
