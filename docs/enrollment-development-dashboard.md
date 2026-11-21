# Disposable live network dashboard

This remains an explicit tagged development command. No normal startup, service, OS interface, screen or input path is added.

Build `cmd/mnet-setup-development` with `mnet_enrollment_development`. Supply the same authoritative disposable private root, source policy and finite limits as the existing setup command. Select `--disposable-network-dashboard`, not `--disposable-network-tcp`. The selectors are mutually exclusive. No implicit default activation occurs.

After actual joining/finalization and fresh encrypted userspace TCP verification, the same owner opens a local dashboard. Joining saved is separate from Network active. Peers come from the validated native snapshot, with source checks still bounded by the original request budget and interval. The page is not independent proof of instantaneous remote revocation.

Stop requests the same network owner to stop. The page hides peers while stopping and cannot show ended until the network function and its device/resources have returned, plus the Stop worker has joined. The HTTP listener remains only for a three-second post-join display grace and then shuts down, closes connections and joins handlers. A browser that misses that grace shows Live status unavailable, not a false confirmation. Reloading after shutdown does not resurrect a session. A tab already showing ended is historical.

The listener binds literal 127.0.0.1 on an ephemeral port. A single-use 256-bit bootstrap capability is exchanged for a per-run uniquely named host-only HttpOnly SameSiteStrict cookie, followed by a redirect to the clean local URL. Cookie names must differ because cookies are not port-scoped. Exact Host and loopback remote are required. Stop requires an exact Origin, per-run CSRF header, JSON content type and fixed empty-object body. CSP forbids framing, external resources and arbitrary connections; referrers and caching are disabled. The bootstrap and cookie are not put in HTML. The CSRF token is intentionally present only in the authorized page. No registrar secret/private key is exposed.

No generic unauthenticated control endpoint, reconnect, renewal, OS device, physical-device activation, screen/input authority or production configuration is added. The development caller's stdout carries the short-lived bootstrap URL as a normal local UI event and must remain private.

## Validation scope

Actual installed browser joining/approval/return, actual enrolled same-key WireGuard/proofnet TCP, active current-peer page, clicked Stop, stopped owner, ended page and original selected-peer expiry were exercised. Active and ended pixels were inspected. Capability/origin/CSRF/bootstrap replay/cross-port cookie/host-rebinding/requested-versus-joined state checks pass under race count 3. The existing 12 cross-app cases pass. Exact final Stop/expiry browser replays and tagged vet/build pass.

The attempted full default Go race command hit its 120-second outer execution limit after reporting passes through enrollmentidentity; later packages and the chained default vet/build did not complete in that attempt. This is not a full default-suite pass. A separate final default build passed. Broad TypeScript remains unpassed from earlier memory limits. Original failed browser launches and test-configuration errors are preserved as failed evidence, not retroactive passes. The UI remains engineering-facing, not a finished Multies consumer experience.
