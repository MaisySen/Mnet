# Active mobile session authorization repair

## Defect and scope

The standalone mobile session listener authenticated the peer's current mesh name, source IP and full public key, including a second check after local consent. An already admitted session did not recheck that authorization. A local TCP regression admitted a session, waited for capture to start, removed the peer from the current map and found the connection still open. This failed on the previous implementation before the repair.

This repair uses the existing daemon `SessionPeers` contract. That snapshot excludes a stopped/not-up daemon, an unapproved or expired local node, and unapproved or expired peers. No Multies identity, sharing, recovery or offline lease policy is selected.

## Implementation

- Pin the admitted canonical name, source address and full key for the session's lifetime.
- Recheck that identity before capture, input injection, chat delivery, received-file completion notification and host-side chat sending.
- Recheck after platform capture returns, dropping the result if authorization ended meanwhile.
- Close the session mux on an observed denial. An invalidated session cannot resume when a later map restores the peer. A fresh session and consent are required.
- Independently check the local snapshot at the existing 8 Hz screen cadence, so a blocked platform capture or backpressured screen cannot keep the revoked connection alive. This is an in-process snapshot read, not a network request. It is not an SLA for observing administrative changes.
- Keep connection ownership visible to Stop during host initialization, and run application geometry outside the lifecycle lock. Closing a mux from a worker never waits for that worker through `Host.Stop`.

## Evidence

The before-repair live-session removal regression failed because the admitted connection remained open. Fresh race-detector mobile repetitions pass after the repair, including removal, key replacement, rename, duplicate source and duplicate name. Additional tests cover capture that outlives authorization, terminal invalidation, input callback suppression, connection closure during blocked capture, one session-end callback after capture returns, and reentrant Stop during geometry initialization.

The full Go race suite is run separately with uncached execution. General CI is collected after commit; no native Android reliability inference follows from these loopback and Go checks.

## Limits

A snapshot can only enforce state the daemon currently knows. An offline administrator change that has not reached the daemon remains unknown. The guard does not establish an offline grace period, workspace role policy, server freshness lease or instant revocation guarantee.

Checks are boundaries, not preemption: a platform input/gesture already dispatched may finish, and arbitrary application callbacks cannot be forcibly interrupted. The independent watcher can close the transport while capture is stuck, but host cleanup and its end callback wait for cooperative platform code to return. Geometry already being computed may finish. A file already being written may leave partial local data; suppressing its completion callback is not file rollback. Bytes already queued or delivered cannot be recalled. Scheduling and map lookup add latency, so the 8 Hz check must not be represented as a hard revocation deadline.

The Android DNS/bulk/reconnect reliability gate remains unresolved. The closed tracing and minimal-baseline investigations are not reopened by this repair.
