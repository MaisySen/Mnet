# Disposable registrar-only local service candidate

This is an isolated development candidate, not a deployed service or a consent verifier. The default control Server, daemon, flags, environment configuration and legacy root readers do not select it. Build the separate command only with `mnet_enrollment_development`. No service installation, release signing, hosting, real account activation, native OS networking, remote desktop or input control is included.

## Process and scope

`go run -tags mnet_enrollment_development ./cmd/mnet-registrar-development -parent <temporary-parent>` creates a NEW private random root and network and listens only on an ephemeral IPv4 loopback port. It first writes a JSON discovery line containing root, network ID, HTTPS origin and certificate path. Send the exact `sources` and `workflow` configuration objects on stdin followed by EOF. Neither service bearer nor native secret is logged. This is initial-only configuration into that new root. It does not validate a human owner, grant, workspace administrator or independently verified operator authority.

Both configured origins must be numeric HTTPS loopback origins. Configuration must match the new network and the source/policy scope and mappings exactly. The accepted development maximums cannot exceed the existing fixture profile: challenge 10,000ms, lease cap 15,000ms, source observation age 20,000ms, clock uncertainty 1ms, four live attempts/principal, four proof attempts, four live total, eight attempts/records and 1MiB workflow state. Source caps are two concurrent, one per source, eight attempts, 3s timeout and one connection. There is no longer-duration grant, renewal, durable admission or hosted policy.

One retained finite ingress per Store lifecycle covers the real strict protocol router and relay. It retains 64 requests, four concurrent, 16KiB request bodies, 4MiB overall state, eight entries and 5s request deadlines, plus the existing finite relay frame cap. Malformed or rejected ingress requests may spend RAM work allowance. These counters, source attempts and pending proof attempts reset when a Store/process restarts. This is NOT a durable quota, cross-process rate limit, anti-copy defense or a persistent admission grant. Reopening durable receipt history does not recreate pending proof/confirmation ownership or extend original receipt times.

The command is initial-run only. Its ephemeral TLS private key is memory-only; there is no command-line resume/legacy-import mode. The tagged internal development owner can reopen a valid marked root and actual envelope for inspection/tests. That is not a turnkey server restart workflow or continuity promise. Temporary roots are retained for explicit inspection and deletion, not silently cleaned up by the command. Tests own and delete their temporary roots.

## Storage compatibility fence and lock scope

Normal `Open` calls the same loader with a nil per-instance codec and has unchanged identity serialization. There is no codec flag, config or environment selector. Only the tagged dedicated constructor sets a non-nil codec before the first publication in a NEW root. Tagged reopen requires a private correct root marker and an existing correct prefixed image; missing images and legacy JSON are refused, never converted.

The actual Store file is `registrar-only.store`, beginning with `MNET-DISPOSABLE-REGISTRAR-ONLY-STORE-V1` plus newline, then the normal bounded JSON Store payload. All Store writes encode before the existing temp/write/file-sync/rename/directory-sync pipeline. The prefix is never appended after a JSON publication. Reopen decodes before the existing schema/registrar validation and reestablishes file/directory sync without rewriting the image. Tagged owner startup also refuses legacy nodes, tokens, auth keys, identities, memberships or foreign network namespaces. Legitimate registrar `NextIP` advancement is retained.

The stable lock is on the actual FILE path, not a directory-wide ownership lock. Legacy `Open` refuses this actual prefixed image because it is not JSON. It can still create a DIFFERENT absent filename in the same directory, including a normal legacy-named state file. The root marker does not change legacy behavior. Tests deliberately demonstrate this limit. Do not point a legacy process at this root and call the directory protected against it.

The header and marker are a format incompatibility fence only. They do not authenticate the source, encrypt the payload, enforce human authority, prevent rollback/snapshot copying or defend against hostile code with the same OS identity. Copying marker/header text does not create verified permission. No mixed-network migration is implemented.

## Protocol and fixture extraction

The tagged inner protocol handler calls the existing owning begin, submit, immutable-history, current map and endpoints operations. It uses the existing strict historical receipt decoder pinned to the original retained challenge. Nested receipt syntax is retained as RawMessage rather than being round-tripped through a ReceiptV2 struct, so duplicate/case/unknown keys remain available to the existing strict decoder. The wrapper remarshal DOES normalize whitespace and JSON escapes and may expand strings near the decoder size cap, causing a stricter size refusal. Authority is the pinned strict decoded shape, struct equality and digest, not raw wire-byte identity. The original request-body bound remains unchanged; this is not an unconditional claim of identical wire acceptance. The router contains no fixture environment reads, request counters, test modes or fault responses. Legacy/account/OIDC/admin/raw map/legacy relay paths have no fallback. Raw-path, escaped and query aliases are refused.

The existing cross-app test bridge now decorates this handler only: it observes requests, delays/deletes in test scenarios and injects lost response/map response faults outside the effect handler. It still has direct test-only Store inspection/setup elsewhere. It is not being promoted into a production server. Its mid-journey Store reopen must select the current Store's router, not retain an old closed instance.

## Evidence and limits

- A deterministic fixed legacy mutation produces exact equal bytes under accepted baseline 7785ded and this candidate. Both SHA-256 hashes: `86b7b04f7989f1a1f550e0058a4b4f3f758b97de999de8b37987527d8f593de3`.
- Default full control race regression and tagged full control plus setup command race regressions pass. Vet and default/tagged command builds pass. A wider `go test -race ./...` was stopped at the 120s execution cap after some packages passed; do not describe the whole repository as passing from that run.
- Eight injected persistence error stages distinguish prepublication unchanged old envelope from complete postpublication new envelope. Six real SIGKILL boundaries separately assert OS-reported SIGKILL and complete old/new envelope with no reopen rewrite. SIGKILL is not power-loss testing, and it does not prove hardware directory durability.
- Actual Multies/source cross-app success, recovery/finalize variants, direct transport, relay transport and source denial run through the extracted router. Existing installed setup browser success, map response failure and lost response run through it too.
- A NEW standalone registrar binary and SEPARATE native client test process use actual Multies approval/SQLite/source and two own-key journal-owned proof/confirmation flows, current maps/endpoints, bidirectional real peers and unchanged original expiry. That new process test does NOT install native State, run a real device, demonstrate encrypted payload transport or prove durable quotas. Earlier existing direct/relay fixture tests are separate evidence, not proof the new standalone journey exercised those transports.
- New service process test configuration uses disposable generated local fixture credentials only. This candidate does not provision or activate real operator/service credentials.

Development failures retained in the review logs: initial strict native envelope helper lacked int64 handling and was replaced by retaining raw nested syntax for the existing pinned decoder (with wrapper remarshal normalization as described above); first extracted fixture closure kept a closed pre-reopen Store and was corrected; initial installed regression invocations omitted the required owned browser profile and refused before the browser workflow. No authority interval was extended to get a passing run.

## Reproduction

From Mnet: run `go test -race -tags mnet_enrollment_development ./internal/control ./cmd/mnet-registrar-development ./cmd/mnet-setup-development -count=1 -timeout=60s`.

From the paired Multies checkout with dependencies: set `MNET_NATIVE_REPO` to this checkout and run `npx vitest run tests/mnet-disposable-registrar-service.test.ts tests/mnet-registrar-workflow-cross-app.test.ts`. The separate process test compiles the actual tagged binary and independent test client itself.

For installed browser regressions use the paired `tests/mnet-disposable-registrar-regression-validation.sh` with both source paths and `MNET_DEVELOPMENT_SETUP_CASE=success`, `map-failure` or `lost-response`. It uses a NEW private desktop/browser profile and dynamic debug endpoint; never attach it to an existing profile. No user browser/account is needed.

Review correction: earlier reports described original raw bytes as untouched. That was inaccurate because json.Marshal on a RawMessage wrapper compacts/escapes it. Source comments and this note correct that claim without changing runtime behavior.
