# Outgoing file snapshot and session ownership

SendFile previously hashed an open source to EOF without checking session closure, then later sent to EOF again rather than stopping at the offered size. A file growing after selection could therefore cause extra data reads/queued bytes before receiver size/digest enforcement. A source failure after offering also released the local stream without promptly attempting a peer reset.

The sender now hashes and transfers only the original regular-file size. It checks the owning mux's terminal state before opening, between file reads and after a read returns, so late read data is not forwarded once closure is observed. Source-size changes after hashing or transfer fail. A file that shrinks fails instead of silently treating early EOF as the advertised complete content. A file that grows does not send bytes beyond the original advertised range. Resume offset is still validated and only the remaining offered range is sent.

A failed offered transfer now attempts a nonblocking stream reset before local release. Reset delivery remains best-effort when the outbound queue or session is unavailable; receiver idle cleanup is still needed. Success requires the existing receiver digest acknowledgement. Same-size source edits are not an atomic filesystem snapshot and can still be transmitted as changed content, but cannot be reported successful unless the receiver's digest matches the original offer. This change does not promise immutable source contents or secure erasure.

The copy loop cannot interrupt an OS filesystem Read or writer already in progress. It stops before another read/write after observing closure; a race after a check is not rollback. This bounds cancellation work to read boundaries rather than a hard latency. Existing incoming destination, size ceiling, file approval, overwrite and retention policy is unchanged. No real user file was read or sent in this work.

## Evidence

Three focused tests pass30fresh race repetitions. They cover exact original-size copying with appended bytes left unread, canceled-before-read, shrink/zero-progress handling, blocked-read cancellation with no late forwarding, and actual framed loopback offers followed by grow/shrink/same-size source mutations. Failure resets are observed and no appended bytes exceed the offer. Removing the post-read close check reproduces late-byte forwarding in the regression. Fresh full repository uncached race tests and vet pass locally. Exact-revision general/Desktop CI must be collected separately.

These tests do not establish consumer file consent, production/live transfer, filesystem snapshots, public-network performance or physical-platform acceptance. Multies/file-policy decisions, closed Android networking diagnostics, signing, account/device changes and release remain outside this source correction.
