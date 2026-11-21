# Explicit local-namespace config v2 contract, not installed policy

Inactive config groundwork only. It does NOT install/load/watch a file, modify
cmd/mnet setup UI, connect journal/native key/source clients to the daemon, select
live endpoints, or create authority. Current offline setup-config-check remains
on v1 and deliberately rejects v2 rather than silently treating it as installed.
Multies producer config and source are unchanged.

The existing v1 schema names issuer+subject, meaning OIDC identity. It must not
silently become local Multies users.id or infer identity because URL strings match.
New ParseLocalV2/ValidateLocalV2 have a separate version2 shape:

- source_origin: exact independently configured Multies HTTPS origin.
- principal_authority: independent exact HTTPS authority for local user identity.
- principal_namespace: literal multies-local-user-v1.
- issuer_label: exact mapping label, not evidence of an OIDC subject.
- registrar_origin and audience: exact scope, no caller URL or label normalization.
- service_id: independent expected registrar service identity, not selected from
  an untrusted challenge/reserve response.
- revision and explicit workspace/network bindings with local principal_ids.

Version1 issuer/subject cannot parse as v2, and v2 cannot parse through v1. There
is no migration, alias, fallback or default. Source origin/authority/issuer label
may intentionally have equal URL bytes while their named semantics remain distinct.
No configured principal IDs are inferred from email/display name or source launch.
Native may not know the current local principal until its own PKCE response, but
its original/current operator policy must constrain that returned local identity.

LocalPolicyV2 stores defensive private copies of arrays and local-principal sets.
Snapshot and SourceMappings return defensive DATA copies. AllowsObservedDATA matches
exact source authority/namespace/local principal/service and full current mapping
against one immutable syntax-checked snapshot. It authenticates NONE of those
inputs, does not prove this policy is installed/current and creates no permit or
inaccessible verified-source token. Nil/zero policy refuses. No trust is conferred
by constructing a LocalConfigV2 or parsing attacker-supplied bytes.

The future actual native/registrar owner must independently load/pin this explicit
policy through its coordinated installed-file/lifecycle path, save the original
source/authority/namespace/service/local-principal/map expectation, and re-read
current policy before/after source work and every new-authority action. Current
pending journal's generic source/map snapshot does NOT yet pin the complete v2
principal/service policy. It must be versioned/extended before actual native
challenge acceptance, not repaired by copying service/principal from a server
response. Neither this new parser nor earlier DATA comparison is that integration.

Strict JSON requires all nested fields, exact names, no aliases/duplicates/null/
unknown/trailing data and bounded input/depth/arrays. Shared exact numeric grammar
now uses json.Number.Int64; actual validators still require their precise version1
or2. This does not make a different version valid through either parser. Config
binding/principal defensive bounds match the current isolated producer's20/100
limits, not a new product admission/retention policy. Mapping syntax reuses the
actual source contract, so this schema does not invent a contradictory ID grammar.

Executed tests distinguish v1/v2 and source-authority/issuer namespaces, pin the
expected service/local principal, preserve input/output immutability, refuse changed
mapping revision/principal/service scope, exercise same URL bytes without semantic
aliasing, remove/null every required nested field, reject duplicates/case/trailing/
noninteger versions and validate all existing v1 tests/offline command tests.
Affected config/contract/cmd-mnet race and config vet pass. No UI pixels changed,
so this is not first-run/native-screen acceptance. No service/native constructor
or runtime caller consumes this policy yet. Actual owner review, nativeOS execution,
installed policy lifecycle, bootstrap/proof/admission/atomic Store consumption and
live activation gates remain unchanged.
