# Local Up request admission

The capability-protected daemon API starts asynchronous connection work and may save identity and routing configuration. This correction rejects ambiguous input before that work is scheduled. It does not change the local capability requirement, enroll a real device, enable a native entry point or alter routing defaults.

## Failure reproduced

A real local-API regression showed HTTP 200 startup acknowledgements for JSON null, repeated or case-conflicting target fields, trailing JSON values, unknown fields and explicit null values. These requests also reached startup code and created identity/routing state despite looking malformed. The endpoint was authenticated throughout; this is not a claim that an unauthenticated caller could connect a device.

## Correction

The Up endpoint now accepts one exact JSON object with each supported field at most once and its existing type. Null values, unknown or case-altered field names, duplicates, wrong types and trailing values return HTTP 400 before an asynchronous worker is created. Route arrays must contain strings. The existing 1 MiB request-body bound and fixed error response remain in place.

The ordinary CLI's empty-object saved-state Up remains valid. Explicit server/network/name/auth-key strings, routing booleans and arrays retain their values. The existing check against combining routing reset with routing roles still runs after parsing. Valid input does not bypass any saved-target, capability, routing or startup authorization check.

## Verification

The local HTTP regression first failed against the old source. It now checks refusal and that rejected requests leave the daemon Down without creating an identity. Platform-independent parser tests cover exact valid values, empty objects, whitespace, routing reset and malformed/ambiguous fields. Twenty race-enabled local API/parser repetitions, the full repository race suite, vet and tracked-source checks pass locally.

Normal native-platform CI now explicitly includes these parser/lifecycle cases, the recent account sign-out/provider-retirement cases, and loopback forwarded-DNS fallback cases. Hosted results are collected separately. Local socket/parser evidence is not an installed-app, OS routing, physical-device or live control-service test.

## Native fixture follow-up

The first expanded hosted run at92fe457 passed Go, kernel, session and Windows, but macOS failed before the local API assertions: its long default temporary root plus Go's test-name directories exceeded the Unix socket pathname limit. Later macOS steps were skipped. That run is not all-five acceptance.

The fixture now allocates a private, uniquely owned short directory under the Unix temporary root, checks a conservative socket-path bound, and removes its own directory at cleanup. A separate regression verifies a long TMPDIR cannot lengthen the socket path and confirms an actual Unix listener binds. Real socket/HTTP assertions remain enabled; production socket selection and parser behavior are unchanged. Twenty focused local race repetitions, full local race tests and vet pass. Fresh hosted macOS verification remains required.

```sh
go test -race -count=20 -timeout=2m ./internal/daemon \
  -run '^TestLocalAPI|^TestLocalUp'
go test -race -count=1 -timeout=5m ./...
go vet ./...
sh scripts/check-secrets.sh
```
