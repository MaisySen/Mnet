# Inactive actual Store-owned source lifecycle

New private reserveRegistrarSource has no production caller or HTTP route. It connects the existing strict bounded DATA pool to the retained actual Store owner, not to a public client-pointer/verified-evidence constructor. Source DATA cannot create a node, consume a proof, write a receipt or change native/account/network authority.

## Explicit config boundary

The experimental private config path is exact canonical Store ownedPath plus .registrar-sources-v1.json. Different Store state files in one directory use different source configs/owners, not one parent-global filename. No startup read, installer, implicit environment path, URL discovery, v1/v2 identity conversion or default config exists. Missing config on first explicit invocation means off, creates no config/client and leaves the existing Store alone.

Private explicit schema version1 requires revision, global/per-source concurrency and permanent attempt cap, finite sources. Each source requires exact alias/source origin/local principal authority/key ID/service/issuer label/registrar origin/audience, separate canonical bearer secret, transport timeout/connections and full allowed mappings. All nested fields are required/non-null/case-exact, unknown/duplicate/trailing JSON and nonintegral numbers refuse. Finite32-entry slice and65536-byte bounds apply. Validation uses the existing strict configured client/pool, without HTTP. Secret is service-only, not native/node credential. Real secret provisioning/rotation is NOT implemented or authorized.

The file must be regular/single-linked/bounded; platform private-file protection is applied before open; path/descriptor identity and final named file are checked around bounded read. Exact bytes are fingerprinted. These are cooperative owner/current-image checks, not malicious same-UID isolation or a claim that finding a file establishes operator consent.

## One source lifecycle per actual owner

First explicit call under Store.mu checks actual retained owner health, parses/pins one image and creates ONE private pool. Subsequent handles reuse it and its spent-attempt memory. Every request checks owner/current exact file before and after actual remote I/O, with actual time.Now and no Store mutex held over network. Source alias selects ONLY configured scope. Result stays strict observed DATA; no grant type is returned.

After pin, missing/malformed/changed config permanently retires this source subsystem and closes its pool, without clearing attempt history. Restoring exact bytes does not reactivate it or allocate a new pool. This prevents config edits/repeated handles from resetting admission in that Store lifetime. Unrelated existing Store network authority is not revoked by source-policy failure. No automatic reload/key rotation/lease/retry/renewal exists. Source-file change is noticed only on explicit checks, not by watch/continuous lease. Admitted work may have remote effects before a later local check fails; it returns remote-unknown DATA outcome, not permission to retry.

Actual Store freeze or Close retires/cancels source pool BEFORE file-owner release. Source admission remains bounded across concurrent private requests through this one owner. Restart/reopen history is NOT durable here: pool history is process/owner lifetime only. Future durable reservation/challenge reconciliation is still mandatory; never reconstruct a fresh pool to claim an ambiguous prior attempt absent.

## Evidence

Strict recursive missing/null and grammar/config tests; real two-Store same-parent independence; inactive startup with invalid config; absent-off/no synthesis; reused same-owner pool; changed/missing/links/directory/oversize image retirement and restored-byte refusal with unrelated Store Health still valid. Isolated fresh test-process verified localTLS exercises actual Store/pool/client success, config change during I/O, actual Store Close/freeze and lost response, one-shot refusal and unchanged Store bytes/no node/registrar record. Root cert is disposable process input, no production transport/TLS bypass.

No installed-policy authority, bootstrap authenticator/challenge/DH consumer, source staleness/lease policy, ingress abuse limiter, atomic consumed-proof/node/receipt transaction, native credential finalization, browser/installer UX, live service, push/deployment or native OS execution acceptance is claimed. Windows/macOS checks are compile-only.
