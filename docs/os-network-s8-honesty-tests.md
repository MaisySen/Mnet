# S8 narrow evidence-honesty correction

No second bootstrap run. Source/tests only, no namespace/capdrop/filter/process
launch/probe. Original S6 failed outcome/custody uncertainty unchanged.

Client evidence guards are now small pure helpers invoked by the actual
evidence-tag path: reject client-supplied lifecycle, fail on start identityErr
or unobserved Wait, and mark pipe complete only without overflow AND WaitDelay.
Structural evidence-tag test requires those exact helpers at the actual sites.
Parent supervisor close/error semantics unchanged; new fake close-failure test
requires attempted=true, closed=false, retained=true. Another fake test sends
forged lifecycle data from child and requires parent snapshot to overwrite it.

Five uncached compile-checked mutants, under evidence tag where code lives:
1. Remove client lifecycle refusal: killed.
2. Force pipe completeness true: killed by overflow/WaitDelay truth table.
3. Ignore identityErr: killed.
4. Force custody closed=true on close error: killed.
5. Keep child lifecycle instead of parent overwrite: killed.
Logs retained, original source restored byte-identical, final race suites green.

These are pure/fake/structural tests, not actual syscall injection or child
execution. New binary compiled NOT RUN, original run evidence unchanged.
A next eventual execution still needs explicit release and must record exact
driver PID/start ticks, resident/client PID/start ticks, immediate owned tree
before/after and parent namespace identity without broad process-name cleanup.
No second invocation is authorized by this document.
