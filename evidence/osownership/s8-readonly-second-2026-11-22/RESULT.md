# Second released read-only bootstrap attempt: FAIL

One invocation of source eb53b4e8bf07b764b06b675d8b1154cb42a7059a,
evidence-tag binary SHA256 ae7b255a13321edb25aec8b93ef37b25d458c9170ef8bf461f54556d5fa031e8.
Go1.26.6 temporary modfile. Outer uid1000. No retry or follow-on invocation.
Original S6 failure unchanged, this is a separate second released attempt.

Observed driver1297/start9565 exited1, wrapper reaped,13.448971ms. Resident
1312/start9565 and client1329/start9566 recorded by wrapper and bounded verdict.
Client stdout empty; Wait exit2 observed/reaped=true. Stderr captured1400 bytes,
complete=true, SHA256 cd6a7f7cf17a751d141b4c7914b4a766a48af3d79e5f11b302ed8d032a63c3c0.
Strict sanitized parser yielded phase unknown/no diagnostic. Raw child stderr
was not persisted by design. Exit2 is not a diagnosis: underlying cause unknown.

Parent-owned resident lifecycle: reap attempted/reaped=true, abort attempted,
exit1, custody-close attempted/closed=true, retained=false. Outer wrapper driver
exit/reap and immediate exact PID/start checks found all3 recorded lifetimes
absent. No timeout and no wrapper signals or process-name cleanup. Sampling every
1ms can miss short-lived unrecorded descendants; task-leader children view is not
a general all-thread descendant audit. This is positive evidence for these3
recorded lifetimes, not arbitrary-process absence or broad containment proof.

Outer namespace dev7/inode4026531833 unchanged. Resident dev7/inode4026532293
distinct. Baseline hash059eb9de8ca8571fbadfad1726660bf71a45419fbbdba58b49b77e87615dbba6
and link count10 saved. Actual directory collector preserved gre0/ip_vti0/tunl0
ENOENT, all/default and other7 links present. This is now inspectable initial
read-only applicability evidence, not final networking cleanup/teardown proof.

Failure at client-bootstrap/class runner. final_hash empty,
cleanup_confirmed=false, client_joined/baseline_unchanged false default values
are no final success, NOT evidence of surviving process. Client failure records
explicit reaped state separately. No client caps/NNP/FD/filter/signal0 pass claim.
No TUN, networking mutation, sysctl write, traffic, repeated pair or activation.
All outer stdout/stderr/wrapper/tree samples preserved unchanged. No raw private
child output copied or guessed. Future diagnostics/execute need separate review.
