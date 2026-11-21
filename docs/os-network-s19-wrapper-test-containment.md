# S19 wrapper mock containment, source only before review

No modified Python tests run. No process launch/signal, device metadata/open,
namespace or Go test/mutant invocation here. Existing S18 reviewer result remains
its separate accepted record; this delta has NO executed test claim.

All run_once mock-launch tests inherit SignalIsolated base that patches os.kill
in setUp and keeps patch through full test/cleanup. Even a newly taken timeout
path cannot signal fixture PID17. Existing normal case asserts no kill. Direct
changed-identity test retains inner explicit kill patch plus no-call assertion.
New timeout cases mock Popen, waitid, identity, namespace, snapshots, monotonic
clock and executable bytes. No real subprocess/identity/kill path in fixtures.

Added source cases: immediate deterministic10s expiry, exact owned mock SIGKILL
and2s wait; identity changed before signal -> no kill; Wait timeout -> pending
custody and reaped=false; post-exec hash mismatch -> bounded mocked kill only.
Each asserts exactly one mocked Popen and sample. New recovery cases duplicate
JSON keys and131073-byte overflow refuse without recovering a lifetime. Existing
survivor/reuse/unreadable/absent tests retained. All tests awaiting static review
before execution. No wrapper production behavior changed by this patch.

Moved live-body comparison: extract full function declarations from be0a102
S14 pre-isolation source versus current locations. Twelve launch/bootstrap/
client-exec/custody functions BYTE IDENTICAL with matching SHA256; full hashes
in live-body-comparison.txt. This checks body text not whole-file tags/imports;
new tags/inert bindings were intentional separately reviewed differences. Actual
standalone body doesn't claim stronger confinement or authorize execution.

Live preflight boundary remains no ioctl/create/network write/install/fallback;
cooperative deadline doesn't promise kernel D-state termination, fixed target
node open only runner feasibility not user-machine access or TUN permission.
Await reviewer static acceptance then separately authorized offline test rerun.
