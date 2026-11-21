# Explicit local-v2 policy pinned in disconnected native journal

The inactive pending journal now requires LocalPolicyV2 at Begin. It durably stores the full defensive local policy snapshot, including expected service identity and workspace-specific local principal IDs, before returning launch DATA. The legacy duplicate origin/authority/mapping fields must exactly agree with that policy. No response field becomes an expected identity.

## Version and recovery

The experimental record is now version 2. The filename intentionally stays enrollment-pending-v1.json, and uses its existing lock. Existing incomplete version-1 data is refused in place, not migrated, removed, replaced, or bypassed through a new filename. Explicit recovery remains unimplemented. This change is not a claim of compatibility with prior experimental journal data.

## Checks

SourceConfigFor supplies a syntax-checked immutable policy snapshot, not proof of installation/currentness. Existing SourceConfig fields must agree exactly with it. Before I/O at least one original/current mapping and principal must still overlap, and source/authority/namespace/service must agree with the saved policy. A response's observed local principal must be allowed by BOTH the original and current policy on its exact full mapping. Current policy cannot add a principal to an existing intent that did not originally allow it.

Checks repeat after remote I/O and after local result publication. Failure after I/O retains remote-may-have-started uncertainty. Failure after successful durable result publication retains historical exchanged DATA and returns ErrNoLongerCurrent, not remote unknown. The standalone RecordExchange DATA setter applies the same saved/current policy checks; it still authenticates no HTTP execution.

Tests exercise preflight zero sends, original/current principal restrictions, service changes during I/O and publication, current-policy removal, aliased snapshot mutation, actual reopen persistence, and legacy refusal with exact bytes preserved. Existing TLS/file/concurrency/failure tests continue through the changed API. Fuzz seed is updated to the new explicit record shape.

## Limits

This is a disconnected journal/client prerequisite. It creates no daemon policy file, installer, loader, watch, current-policy owner, challenge authority, checked native key permit, browser, or loopback listener. The caller-supplied current callback is still untrusted DATA. An actual daemon-owned installed/current policy and key/journal two-file outcome handling remain prerequisites. No enrollment activation, account effect, source request against a live service, network/screen/input grant, push, signing, or deployment is performed.
