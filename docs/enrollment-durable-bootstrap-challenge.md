# Durable journal-owned bootstrap challenge DATA

Explicit inactive BootstrapChallengeDATA on existing pending Journal. No daemon/UI/startup caller, durable WireGuard key access, crypto response submission, node/receipt transaction, credential installation, source authority or finalization. Network client is the separately reviewed experimental exact-origin bootstrap client. No secret getter or caller HTTP/transport override is introduced.

## SAME durable intent before I/O

Only current live record2/exchanged-data from SAME journal can admit. Reopened journals refuse automatic admission, even exchanged-data; no restart resume policy is invented. Initial supplied full binding must match SAME saved own-PKCE launch/key/credential digest/attempt, actual earlier saved source-row principal namespace/authority/ID/session binding, selected original mapping, exact service and recomputed reserve descriptor for supplied reserve ID. Original and current explicit policy must still allow SAME observed principal/service/mapping. Current source exchange expiry, original journal deadline and actual transport's checked-at/access/reserve limits must pass. Merely sharing a network/key string is insufficient.

Reserve ID and checked-at/deadline/access times remain proposed DATA inputs. This method does not authenticate a service reservation, source clock or a live eligibility/lease decision. It cannot issue a permit. An arbitrary internally consistent binding must never feed a durable-key signing method; no such method is called here.

After syntax/bounds constructor preflight (no connection), SAME file enrollment-pending-v1.json durably transitions to record3/bootstrap-may-have-started. New bootstrap_json contains exact expected binding, original started time and explicit transport/lifetime bounds, with empty challenge string. Only after marker commit succeeds does it construct the request using journal's private saved secret, one fresh client, and one exact-origin request. No private token is returned or logged. File/owner never switches to an alternate journal to escape existing pending intent.

After response, method rechecks health, current original policy/SAME principal/mapping, own exchange current validity, elapsed and caller-clock bounds, cancellation, SAME phase/binding and challenge deadline. It saves exact pinned ChallengeWire DATA in record3/bootstrap-challenge-data before returning. After durable save it rechecks current config/clock again outside journal mutex; failure returns historical-data-but-no-longer-current, preserving saved image. No lock is held across HTTP or current-config callbacks. Journal Close cancels in-flight request and suppresses returned late DATA.

## Compatibility and crash semantics

Existing record2 encoding omits bootstrap_json and remains byte-shape unchanged. Parser explicitly rejects even an empty bootstrap_json on record2. Existing record2 Open/Status does not migrate or add files. Only explicit admitted method creates record3. Historical record1 stays refused by existing code. New record3 requires exact phase/payload/shape and consistent saved own exchange/descriptor/principal/service; missing/malformed fields refuse without rewriting. The previously reviewed exact record2 decoder did not know the added field and would refuse its new shape. This source-level fact is not broad historical binary/version-floor or downgrade acceptance.

| Failure | Durable outcome |
|---|---|
| Invalid binding/current policy/constructor | No marker or request, original v2 unchanged |
| Marker temp/write/sync/close/rename rollback-safe failure | Original v2 unchanged, zero sends; no success |
| Marker postrename/parent-sync uncertainty | Owner freezes, zero sends; owning reopen finds complete v3 may-have-started |
| TLS/network/status/loss/current-context change after admission | Spent v3 may-have-started retained, remote-unknown; no retry/refund |
| Challenge pin rollback-safe failure | v3 may-have-started retained, remote-unknown; no DATA returned |
| Challenge pin postrename/parent-sync uncertainty | Owner freezes, remote-unknown; owning reopen finds complete pinned challenge image |
| Pin durable then principal/policy/current clock fails | Historical pinned challenge retained, ErrNoLongerCurrent, no returned challenge |
| Close/restart | No automatic listener/request/challenge resume; retained phase/secret/attempt unchanged |

No request retry, challenge renewal, fresh secret, key, ticket or reconsent is created on failure. One-shot client reconstruction is inside an already-durable journal admission; callers cannot repeat from marker/pinned phase. Future explicit recovery must reconcile SAME exact remote attempt and current own policy, not reset record/version or assume a lost response meant absent remote work.

## Evidence

Fresh subprocess uses production bootstrap client constructor and isolated SSL_CERT_FILE/SSL_CERT_DIR trust for httptest's standard embedded localhost certificate. This is not a fresh per-run CA or production TLS bypass. Server inspects actual on-disk record3/may-have-started and SAME saved secret BEFORE returning challenge. Covers success/durable pin, zero-send marker failure, TCP loss, pin failure, policy/principal revocation in flight, post-save principal revocation, Close, no repeat after admission/reopen, unchanged secret, exact saved binding negatives, and original v2 byte shape/no migration. Full temp/create/write/short/sync/close/rename/postrename/dirsync fault matrix runs separately at marker and pin, checking whole-image owning reopen and zero-send versus remote-unknown semantics. Tests initially used identical fixture bytes for session/key negative control and caught the invalid control; final mutation uses distinct attempt bytes and reruns. Test fixtures establish no real source/receipt provenance.

No native State writer, key operation, active route, current-control/relay wiring, installed account/device or physical/release acceptance is added. Actual server challenge owner/source permit/atomic proof-consumption and native usable State/two-file finalization remain unbuilt. Timing policy is not selected by fixture arithmetic.
