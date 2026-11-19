# ACL request admission

An authenticated operator's policy update must be unambiguous. Previously a missing or misspelled rules field silently cleared a restrictive policy, and duplicate/case-conflicting fields, trailing JSON or ignored fields within a rule could broaden access. Ten local HTTP regressions failed against that source: the peer map reopened access and a reopened store retained the unwanted change. This was found in isolated fixtures, not a production network or an observed attack. It is not an unauthenticated bypass.

## Correction

Only the ACL replacement endpoint changes. A request must contain one exact network_id and one explicit rules field, each once, with no other fields or trailing JSON. Each rule accepts src, dst and optional comment once each, with string values. Unknown, duplicate or case-altered rule fields and malformed/null rule elements are refused before policy persistence or a map-version update. Existing semantic rule validation still runs afterward.

Explicit empty arrays and explicit null rules retain the API's existing clear-to-allow-all behavior. This preserves older clients that intentionally serialize a nil rules slice. Missing rules are not a clear operation. Network selection by existing ID or name, the 1 MiB body bound, administrator session/origin/JSON guards, policy defaults and the Rules editor's save placement remain unchanged.

## Verification

The HTTP regressions compare saved bytes, map version, current peer visibility and visibility after reopening the store. Parser tests cover exact valid requests, comments, field order, both existing clear shapes, duplicate escaped names and malformed object/array/string/null/trailing shapes. Existing policy tests still cover deliberate clearing and route/peer filtering.

An expanded actual-Chrome operator journey also failed against the old handler: a rule carrying an unsupported field was accepted instead of rejected. It now checks that the error keeps the rejected draft editable, leaves the stored restrictive policy intact, and supports explicit reload followed by a valid empty-array clear. Desktop and320px/200% recovery pixels were inspected. A test-only confirmation step honors the existing dirty-draft discard dialog rather than suppressing it in the app.

Twenty focused race repetitions, the full repository race suite, vet, source checks and the expanded complete Chrome journey pass locally. Native normal CI now includes the ACL policy/request tests in its existing control-state check. Hosted verification for this correction is still separate and pending. These screens and API fixes are source work, not a control-service deployment, device release or Multies change.

```sh
go test -race -count=20 -timeout=2m ./internal/control \
  -run '^TestACLRequest|^TestACLPolicy|^TestAdminRequest'
go test -race -count=1 -timeout=5m ./...
go vet ./...
node scripts/dashboard-validation.mjs
```
