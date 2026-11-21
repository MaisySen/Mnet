# Inactive native-owned enrollment context substrate

New private daemon-only methods capture and revalidate policy plus native identity under d.mu and the retained actual native State owner. No production path calls them. New, Up, existing local HTTP routes, mobile, browser and source clients remain unchanged. This is not native enrollment activation or a public authority permit.

## Evidence captured

The fixed experimental filename is enrollment-policy-v2.json in the canonical retained native owner's directory, never a current mutable Config path. Missing policy means off; no policy, key, enrollment, directory alternative or migration is created by capture. The key must already be in the owner's durable validated State; detached or unsaved working State does not count. Exact State byte revision is re-read and compared, not just the exported DTO or an existing in-memory key string.

Policy is bounded, strictly parsed as explicit local v2, and checked with regular-file/single-link/path-descriptor identity checks before and after read. State is checked again after policy parsing. The context stores its exact policy-byte digest, native State-byte revision, existing public key, private owner pointer and immutable parsed policy. It contains no private-key/credential getter. Its formatters redact all fields.

Revalidation repeats these actual file and lifecycle checks under the same native owner. A foreign owner, Close, owner fault/release, modified policy pointer, changed State/key, policy removal/malformed data, symlink/hardlink/directory, oversize or any policy byte change invalidates the capture. Ordinary saved State changes invalidate old captures without rotating the existing key.

## Semantics and limits

A context is private checked point-in-time snapshot evidence, not enrollment authority. There is no installer or production caller. Any future policy installer must coordinate through the retained native owner and preserve explicit user/operator authority. A local filename alone does not establish consent to enrollment. Syntax-checked data, including a caller-provided policy pointer, cannot manufacture this owner-bound capture.

There is no watch, continuous lease, global revocation promise, ABA history, source eligibility proof, finalization, or two-file native-State/journal transaction. Cooperative operations can be serialized under d.mu/the owner; these filesystem checks do not isolate against a malicious same-UID writer that can change files between checks. Invalid enrollment policy blocks this capture, not an unrelated existing network connection. Policy reading does not install node credentials or change account/network/screen/input access.

Tests use actual New/retained owner/state publication and real local policy/state files. They cover inactive startup despite malformed policy, missing policy/default refusal, no key generation, current/foreign/forged/closed captures, exact file changes, links and malformed/oversize input, working/disk State mutation, released/faulted owner, and ordinary native State revision changes. Race/vet and compile-only OS results are separate from native Windows/macOS/mobile execution acceptance.
