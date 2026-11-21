# Legacy auth-key rejoin after delete or expiry

This is characterization of existing source behavior, not a policy change or a
registrar implementation. The new matrix runs actual `Store` methods without
HTTP, a daemon process, account, live device or Multies access. Production code
is unchanged. The stopped List lifecycle review is unrelated and excluded.

## Verified matrix

Each row starts with a real store network, auth key and approved legacy Join.
The device is then deleted or expired. Its old node cannot receive a map at
that point. Repeat both immediately and after reopening the persisted store.
The retry includes the old saved node ID/token, as a daemon rejoin would.

| Remaining authority | After DeleteNode | After ExpireNode |
| --- | --- | --- |
| Valid reusable auth key | New node ID, approved, map allowed | Same node ID, renewed approval, map allowed |
| Already consumed one-time auth key | Keyed retry refuses ErrBadAuthKey | Keyed retry refuses ErrBadAuthKey |
| Expired reusable auth key | Keyed retry refuses ErrBadAuthKey | Keyed retry refuses ErrBadAuthKey |
| Removed reusable auth key | Keyed retry refuses ErrBadAuthKey | Keyed retry refuses ErrBadAuthKey |
| Saved node credential only | New pending node, no map | Same node becomes pending, no map |

For each invalid-key row, the test also performs the daemon's fallback shape:
retry with the same saved credential and no auth key. That creates/retains a
pending node, not approved connectivity. This separates failure of the keyed
request from the subsequent unauthenticated-public pending join behavior.
All cases retain exactly one node after the retry and create no verified
principal/Multies approval provenance.

The20case matrix is in
`internal/control/legacy_rejoin_authority_test.go`. Thirty race repetitions pass
(600case executions). The suite uses deterministic past key expiry, not sleeps.
The two restart variants verify that deletion/expiry and the remaining key
condition persist through store reopen, not only in-memory mutation.

## What this establishes

`DeleteNode` removes the node/token/provenance but leaves independent reusable
auth keys intact. `Join` therefore treats the missing public-key record as a
new enrollment when a still-valid reusable key is supplied. With expiry the
record remains; the saved node credential authenticates that existing identity
and a valid reusable auth key supplies fresh approval. A public key alone does
not bypass the existing-identity credential check.

Thus delete or expiry is effective before retry, but is not durable withdrawal
of the independent reusable-key enrollment authority. A spent one-time key is
not equivalent to a still-valid reusable key. No blanket claim that every saved
auth key resurrects a device is supported.

This test does not run the actual background daemon retry loop, active relay,
direct WireGuard traffic or a fresh Multies assertion. It mirrors the existing
request/fallback shape against the real store and tests returned map authority.
Existing relay/expiry tests remain separate evidence; this is not an end-to-end
revocation measurement or security certification.

## Legacy versus future registrar implications

The legacy reusable key may intentionally represent continuing permission to
add approved nodes. Changing delete/expiry to override it would change existing
product/operator semantics. No such choice was made or implemented here.
Removing a reusable key stops the tested automatic approval path, but this
document does not prescribe a live operator action or claim that doing so alone
provides a measured peer-traffic revocation bound.

A registrar cannot equate ordinary legacy delete/expiry with permanent device
withdrawal. It needs explicit durable enrollment/tombstone/recovery semantics,
legacy-path isolation, and a selected rule for any still-valid legacy auth key.
A tombstone or fresh-consent override policy must be deliberate, reviewed and
tested across both registration routes and restarts. User/operator decisions
about reusable keys, fresh consent and account-event effects remain open.
No Multies enrollment permission, production mapping or activation is inferred.
