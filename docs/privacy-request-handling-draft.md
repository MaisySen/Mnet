# Mnet privacy-request handling - operator review draft

Updated2026-11-17. Proposed handling steps, not an adopted SLA, new retention policy or authorization to delete a real person's data. The owner confirmed Fintex as operator and contact@fintex.au as the monitored support/deletion route. Confirm the actual requester's identity, scope, current deployment and authority before changes.

## Before touching data

1. Receive the request through the approved support route. Keep the request private. Do not ask for enrollment keys, node private keys or passwords by email.
2. Verify the person and exact affected account/network/devices through existing authenticated records. A matching display name or possession of a forwarded message is insufficient. Return an unresolved identity/scope question privately instead of disclosing a device list to an unverified sender.
3. Inventory current records through the actual operator account: nodes and their credentials/approval associations, enrollment keys, account memberships, application audit files, provider logs, backups and any separately retained support records. Check the deployed revision, not merely latest source.
4. Explain any loss of access and records that cannot immediately be removed before committing. Do not reset a shared network, delete unrelated devices, revoke shared enrollment keys or wipe the whole service as a shortcut.

## Distinguish access revocation from erasure

| Action in current source | Effect and boundary |
| --- | --- |
| Disconnect or stop hosting | Ends the local connection/session. Does not delete saved identity, server records, support mail or another endpoint's copy. |
| Delete an exact node through the operator API | Removes its stored node record, token and saved approval-identity association after persistence succeeds; detaches relay connection. Produces a new node-delete audit event. Not a purge of all related records. |
| Delete an exact enrollment key | Removes that key. It does not automatically remove devices enrolled earlier, and shared-key impact needs separate review. |
| Revoke an exact account membership | Removes that principal's explicit query grant for the selected network. Not mesh transport revocation or deletion of every identity-provider record. |
| Remove app data on a phone | Local app-storage action, separate from server/provider/support records and other endpoint copies. Do not remotely wipe or claim the user did this without verification. |
| Service restart or idle spin-down | Current free host loses local files. This is not a verified privacy erasure of provider logs/backups and must not be used as a deletion workflow. |

## Verify and close only what happened

Read the current operator state after each authorized mutation. A failed/uncertain response is not completion. Record a minimal private handling result without duplicating keys or full data exports. Separately verify audit/provider/backup/support records under the actual retention policy. Current source has no general per-person audit purge, automatic audit rotation or implemented all-data self-service deletion.

Tell the requester which verified actions completed, which records remain and why, and what follow-up is needed. Never promise all copies are erased or a deadline that the operator has not adopted. Other session endpoints can retain content; do not claim control over them.

## Gaps before adopting this procedure

- Actual operator/provider log-retention settings and backup/export inventory.
- A decision on audit/support retention and lawful exceptions, implemented tooling and the responsible human process.
- An approved way to authenticate a request without creating a new secret-disclosure channel.
- A tested all-record handling walkthrough on disposable data, matched to the deployed revision, before representing it as an operational capability.

See [privacy notice review draft](android-privacy-notice-draft.md) and [source privacy evidence](android-privacy-evidence.md). No live privacy request has been executed by writing this document.
