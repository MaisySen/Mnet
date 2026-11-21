# Disconnected provisioning consistency contract

Model only. No deployed route, membership change, service key, generated secret,
installed config, network owner or new authority. Normal setup unchanged.

## Legacy relationship must be explicit

CURRENT: normal Server.Handler exposes legacy /api/join/account/OIDC/approve.
Store.Join refuses a key already claimed by registrar but does not reserve an
entire network to registrar. mapSnapshotLocked includes approved unexpired nodes
on the same network regardless of legacy/registrar origin. Admin effects and
relay also exist. Therefore current server does NOT enforce registrar-only
Multies networks. Same-network legacy authority cannot be hand-waved away by
calling registrar credentials strict. Legacy remains a working separate product
path, not a Multies-only interim implementation.

PROPOSED TARGET ONLY: registrar replaces legacy enrollment for Multies-managed
networks. Legacy may remain for separate non-Multies/self-hosted networks, never
as another route into the same protected network. No existing live path is
retired by this model. Activation requires reviewed per-network classification,
pre-pending-node Join/account/OIDC/admin guards, inventory/migration/reapproval
of existing nodes, map/relay peer enforcement, version floor, and actual positive/
negative direct+relay tests. Existing nodes are not implicitly converted or
expelled here. A plan must declare this proposed boundary, not current fact.

## Runnable artifact, not an installation file

```
go run -tags mnet_enrollment_development ./cmd/mnet-provisioning-development < internal/enrollmentprovisioning/testdata/plan.json
```

Bounded exact JSON stdin, deterministic JSON stdout; no filenames/options accepted,
no implicit loader, no write, network, OS or secret handling. The shared example
uses reserved example.test names and synthetic workspace/user IDs. It supplies
three independent snapshots, not independently authenticated sources:
- Actual Go LocalConfigV2 syntax/validator for native policy, principal authority,
  namespace, issuer, source/registrar, audience/service and explicit bindings.
- Multies operator-shaped snapshot, using its actual camelCase field names.
  Cross-language vector passes through real enrollmentOperatorSchema first.
- Secret-free registrar source snapshot and Multies service-scope snapshot.
  Deliberately not installable registrarSourceEntry, which requires a secret.
  Cross-language vector validates service fields through real serviceFixtureSchema
  with synthetic fixture metadata/digest, then checks the common scope. This does
  not instantiate a service, verify an actual key digest or bypass fixture gates.

The verifier requires exact source/authority/namespace/issuer/registrar/audience/
service/key/mapping revision/workspace/network/principal agreement. Workspace
network IDs must be unique, not silently shared across workspaces. Extra/duplicate/
aliased/missing/null/float/oversized JSON refuses. No secret/signature field is
accepted. Explicit owner-reference labels and a declared independent native-trust
root fingerprint are required. They are unverified DATA, NOT signatures, installed
trust, current authenticated membership or approvals. If an attacker supplies
all matching snapshots, the result STILL verifies no authority and cannot deploy.

Only initial-provision-only-no-rotation-or-migration plans are accepted as
configuration-consistent. Rotation/revision migration is NOT implemented by
changing a label. Later owner must verify no existing/pending conflicting scope,
and a separate reviewed rotation plan must specify scoped overlap, cutover,
pending refusal, old receipt/grant residual and rollback/version floor. This
model does not assert such checks happened or silently waive them.

Report ALWAYS says:
- kind=disconnected-configuration-consistency-only
- configuration_consistent=true only for matching DATA
- authority_verified=false, deployable=false
- each required source/ownership/trust/protected-service check is unverified,
  legacy isolation/renewal/durable-admission policy unresolved
- canonical plan SHA256 is semantic DATA identity, not a signature or permission.

Semantic hash includes every declared owner reference and trust fingerprint.
Object/array-set ordering and whitespace are canonicalized, no clock/randomness.
Changing audience/service/root/owner/mapping changes the hash or refuses. Future
executor must recover and authenticate actual owner approvals against exact hash,
re-read current source membership/policy and installed trust, then fail if stale.
A plan may never authorize its own executor or authenticate its own root.

The model is intentionally narrower than all existing valid inputs: native policy
requires v2 provenance while Multies currently permits it omitted, and plan
revision is restricted to the existing Multies identifier grammar. A model is
not a replacement for either real owning validator. The companion test imports
real Multies validators and reuses the SAME Go testdata JSON, not a second copied
schema. Valid-but-inconsistent audience/key/principal/revision/origin mutations
pass individual syntax but refuse cross-language consistency.

## Owner/executor boundaries for later integration

- Operator-owner source ref: authenticated deployment operator verifies source/
  registrar identity and authorizes scope; future provisioning executor installs
  only reviewed hash-bound policy. No such executor in this patch.
- Workspace-owner source ref per workspace: authenticated Multies membership and
  explicit workspace-owner policy transaction, not caller principal IDs. Native
  allowlist is eligibility configuration, never proof of actual membership.
- Independent native-trust owner ref: signed installer/trusted update pipeline
  verifies policy root through an independent trust chain before config loading.
  No bootstrap signature verifier or self-signed URL trust is implemented here.
- Protected service owner ref: server credential provisioning/rotation in protected
  storage, never native credential or browser-readable service bearer. No keygen,
  credential access or vault call exists in this patch.
- Engineering policy/deployment owner: new grants, server admission, account-event
  revoke, peer/relay teardown, OS permission, real duration, hosting cost and live
  rollout remain unresolved and require their own exact scoped authority.

## Tests and limits

Go race tests: same-input determinism, reordered-set equivalence, hash-sensitive
owner-source ref, mismatch/refusal for origin/namespace/authority/audience/key/
service/revision/workspace/network/principal, shared-network refusal, empty and
zero-root trust declarations, illegal rotation/legacy-open proposal, duplicates/
unknown/secret/signature/null/trailing/numeric/oversized fields.

Cross-language actual subprocess: build tagged executable, real Multies operator
and service Zod validators, common shared vector, output never authority/deployable,
individually valid scope mutations refused. Normal no-env test skips, not a pass.
Initial vector used wrong snake_case SourceMapping names, so both Go and actual
subprocess refused; corrected the shared data to actual camelCase type. Failure
was not a production defect and did not weaken exact parsing.

This closes duplicated configuration agreement as an executable contract only.
It does not prove authenticated provenance, current membership, bootstrap
signature, installed root, rotated key state, production route, useful lifetime,
server admission, D1 deployment or physical device. The about10s initial fixture
and prior unexplained initial-map failure remain unchanged and unresolved.
