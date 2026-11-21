# Actual dedicated current client and owning Store fixture

Test-only integration in registrar_client_fixture_test.go, no production code change. A real production-constructor RegistrarClient runs in a fresh subprocess, uses isolated fixture certificate trust via SSL_CERT_FILE/SSL_CERT_DIR (httptest uses Go's standard embedded localhost certificate, not a freshly generated per-run CA), and talks to actual retained Store same-lock current operations. Test fixture supplies only a test-owned existing strict v2 record/secret/receipt. No actual enrollment/commit/source authority is produced, and no native State or usable credential is installed.

The HTTP adapter is explicitly TEST ONLY and unregistered. Its simple body/header interpretation is glue, NOT an accepted production parser, ingress bound, source authenticator or server-route implementation. Passing its matching fields cannot create a permit. Production Server still has no dedicated registrar routes and no caller of the private operations except tests. O(history) validation/ingress-cost blocker remains.

## Executed paths

Fresh child invokes production constructor with explicit document/origin/request bounds, no caller transport/RootCA/insecure-TLS injection. Initial CurrentMap traverses actual HTTPS/JSON/current credential verification/actual owner snapshot and ACL finish. RegistrarEndpoints traverses HTTPS and SAME-lock verification + actual endpoint publication. Next explicit CurrentMap sees exact published endpoint, not a synthetic map response.

Two cases:

- Denied: before a fourth request's map wrapper, adapter calls actual Store ExpireNode. Actual current map refuses, adapter returns401, real client permanently retires copied owner with distinct nonlegacy denial. Another endpoint call never sends. Exactly4 HTTP requests, no Join/rejoin or credential rotation. Owning Store Close/reopen retains revoked phase, exact original receipt and intended endpoint publication.
- Lost endpoint acknowledgement: adapter commits actual endpoint update then closes its TCP connection without response. Client reports unknown once, no automatic retry. A subsequent separately explicit map request reads actual published endpoints. Exactly3 HTTP sends. Owning Close/reopen confirms intended durable endpoint and unchanged registrar receipt/proof history. A lost acknowledgement was not interpreted as absent side effect.

Each case starts from real strict Store save/Close/reopen, uses fixed isolated embedded-localhost fixture certificate trust in child, checks response body never logs native secret, and ends with real owner reopen. Node count and legacy token map unchanged. Fixture-only updates to original source times/origin/proof index before initial save are test data setup, never represented as authenticated production commit. The existing record is fabricated by established test-only strict fixture, not a new Store constructor or public trust bypass.

No external account/service/network, installed native credential, actual consumer/handler deployment, source eligibility lease, relay or packet lifecycle, physical/native OS or release acceptance. This connects current-control mechanics while preserving all missing authority and activation boundaries.

Run targeted evidence with:

```
go test -race ./internal/control -run RegistrarActualClient -count=3
```

The ordinary child test skips without the explicit private fixture environment; no credential fixtures are loaded from a user's saved state.
