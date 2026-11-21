# S21 source gate: inert protocol and poisoned cleanup

This is uncompiled, untested review source. No real Kernel implementation,
callback, device operation, process launch, installation, service activation or
production default changes. No new preflight. S19 EACCES is still the only actual
existing-device feasibility result, specific to that runner. User deployment is
unknown. S20/S21 are not a complete Linux adapter.

## Custody and ambiguous setup

A canonical creation handle and copied Plan survive setup failure. A failed ACK
poisons further setup. Successful ACK still needs exact operation readback;
forward steps must follow the copied Plan, not arbitrary caller order. Caller
TagConfirmed may lag but may never promote canonical state.

Cleanup permanently ends forward setup. A fresh fake inventory must select the
exact first rollback operation. Only exact route/address deletion is admitted;
foreign policy, namespace mismatch or a replaced link refuses. An errored delete
keeps the pending operation until a later readback proves it absent. There is no
broad namespace flush or name-only deletion. Pending setup evidence is retained.

Close requires only the original canonical handle, with no pending deletion and
no explicit route/address deletion left. Its one-shot intent and recorded result
are distinct. A failed close is never blindly retried. Handle custody is retained
in the ledger even after successful close. Exact original raw inventory and
sysctl equality, not close success, proves absence. Volatile baseline equality
can remain unresolved, and that is intentionally not declared clean.

## Fake transaction protocol

inertAdapterProtocol implements the existing Kernel interface but takes only a
package-private marker interface. No implementation of that marker is added.
Future reviewed test fakes can supply it. Existing Transaction is not rewired.
The marker is a source discipline, not a security boundary against package code.
The protocol obtains fresh inventory before deletion/close, passes canonical
handles, and records ACK/readback separately. It does not perform raw composition
or certify real FDs. The future live adapter still needs reviewed raw parsing,
original FD provenance and namespace custody. Capability projection consumes
explicit facts only and initiates nothing. Its verified-path label is restricted
to the specified target/path, never arbitrary ordinary-app networking.

## Proposed test gate, not run

Review source and reachability before enabling any new test. Then use exclusively
inert fake protocol implementations: clean Transaction start/stop; each forward
error before/after fake mutation; missing ACK/readback; out-of-order operation;
caller-promoted tag; altered namespace/ifindex/name/alias; foreign policy;
replacement link; errored deletion that did/did not happen; failed close once;
raw absence mismatch and custody retention. Capability cases must distinguish
transport-only, runner denial, unknown user target, lost authority and pending
cleanup, without automatic action.

Existing suite/mutant ownership stays with the reviewer. No tests, compile or
mutants were run for this source packet. No claim of passing integration.
