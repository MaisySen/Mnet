# Peer status uses handshake evidence

`mnet status` and `mnet status --json` describe the local daemon. A map entry is not proof that a peer is reachable. This change fixes misleading handshake output without changing routing, authentication, enrollment or transport behavior.

## Human output

The peer table keeps its existing columns. The status and handshake columns now distinguish missing evidence from a known old or absent handshake:

| Device evidence | STATUS | LAST HANDSHAKE |
| --- | --- | --- |
| No device statistics or no statistics entry for this peer | unknown | unavailable |
| Statistics entry with no recorded handshake | offline | never |
| Handshake timestamp ahead of this machine's clock | unknown | clock skew |
| Handshake age from 0 through 179 seconds | online | age in seconds |
| Handshake age of 180 seconds or more | offline | age in seconds |

`online` is the existing recent-handshake heuristic. It is not an active reachability check, application health check, authorization decision or guarantee of a working route. `offline` does not prove a machine is powered off. A clock step can invalidate the age calculation; future timestamps are not treated as recent.

PATH is `unknown` when no endpoint was returned by the device. With an endpoint it remains `direct` or `relay`, describing that endpoint's type, not a successful traffic probe. RX/TX keeps the device's existing byte counters. Missing statistics still leave numeric counters at zero; zero alone does not prove that no traffic has ever passed.

## JSON compatibility

The existing `last_handshake_ago_sec`, `online`, endpoint and counter fields remain. The additive `handshake_state` field is one of:

- `unavailable`: no device/stat entry for this map peer.
- `never`: a device/stat entry reports no handshake.
- `clock-skew`: its timestamp is ahead of the local wall clock.
- `recent`: a nonfuture handshake is younger than 180 seconds.
- `stale`: a nonfuture handshake is at least 180 seconds old.

The age is `-1` for unavailable, never and clock-skew states. `online` is false for those states. Consumers should use `handshake_state` to distinguish unknown evidence from a known absence. The CLI still renders the older age/sentinel format when an older daemon omits this field.

Unusable map public keys are omitted from this diagnostic peer table rather than sliced blindly or presented as usable peers. This is not new map admission or a repair to control-server validation. The private local API still requires its existing capability.

Node identity, network and peer identities are taken from the same immutable map snapshot. Device counters are read later. A diagnostic read does not hold the map-application barrier over device IPC, and it is not a transaction across map and device generations.

## Acceptance and limits

Failing-first daemon tests reproduced two existing defects: missing statistics were rendered as a zero-second handshake, and a short malformed map key panicked the status call. Regressions cover both, missing/zero/future/extreme timestamps, the 179/180-second boundary, unchanged source data and counters, capability-protected HTTP JSON and exact human table rows. A concurrent status regression checks map identity consistency while device-stat access waits, without preventing publication of a later authorization-map snapshot.

A separate built-CLI process test uses a private Unix-socket status fixture to verify exact human rows, JSON states, legacy compatibility and missing-capability refusal. Its daemon payload is synthetic, not live device evidence.

Local validation includes repeated race tests, the full uncached Go race suite, vet, builds and Darwin/Windows CLI cross-compilation. Hosted checks for each new source commit are accounted separately in project status. These are diagnostic correctness checks, not proof of a new live network deployment or consumer readiness. Multies, production accounts, signed releases and physical-device acceptance are unchanged.
