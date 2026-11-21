# Separate test correction after V2 acceptance

Accepted production/source candidate 5f8eebd and Multies 8b9296f stay immutable.
This branch changes tests and adds a local test runner only. No protocol, receipt,
config, lifetime, proof engine, grant or durable accounting behavior changes.

Independent review found a cold failure and failures in six concurrent compiled
fault matrices on one core. Sequential repeats passed. This is nonhermetic,
load-sensitive fixture evidence. A finite source/freshness interval is a plausible
cause, not an established mechanism. It does not establish universal stability.

The matrix now logs only bounded stage diagnostics: requested and reached
publication, elapsed stage-entry times, real source and proof-factory counts,
public phase, observation age, original challenge time remaining and failed bit.
No identities, config verifier, secret, proof or receipt is printed. Diagnostics
can distinguish pre-source, post-source/pre-proof and later publication failures;
they do not by themselves prove why a source call or time guard failed.

Use:

    python3 scripts/validate-admission-faults-isolated.py

The runner takes a private per-UID advisory flock, waits at most 180 seconds, then
builds a race test binary and runs an explicit matrix-only child in the package's
required working directory. Build and test subprocess timeouts remain bounded.
No automatic retry hides a failure. Concurrent callers of this runner serialize;
uncooperative tests and unrelated host load are not prevented. Keep these finite
freshness matrices separate from competing CPU-heavy validation jobs. This is
orchestration for valid evidence, not an all-load stability fix or benchmark.
The original complete TestAdmissionV2FreshTLSChild still includes the same matrix.

Two simultaneously launched runner calls, using the same compiled changed test,
passed with lock waits 0.000s and 3.305s. All real source/proof/persistence stages
and all 32 injected cases were retained. Initial runner smoke calls failed because
the compiled binary was started from repository root, not internal/control;
working-directory selection was corrected before these passes. This was runner
setup evidence, not a Store or source failure. No expiry was widened and no real
stage was bypassed. Further failures must retain their stage diagnostics.
