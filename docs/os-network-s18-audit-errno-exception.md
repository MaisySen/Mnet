# S18 narrow errno exception, source only

Reviewer alone ran S17 inert suite: reported128 pass/1 guard failure, no new
builder run. Two established tests import syscall only for errno data/types:
codec_negative_test.go EPERM/EEXIST/EBUSY/ENODEV/Errno;
failure_evidence_test.go EPERM. They do not call kernel operations.

Audit allows exactly these four names plus Errno. No prefix-E wildcard. Every
syscall SelectorExpr inspected (not just calls), so assigning syscall.Kill or
other callable to local variable is refused too. syscall import must have no
explicit alias, including same-name/dot/blank alias. Direct syscall calls permit
ONLY unary nonvariadic Errno conversion. All other imports/types/launch/syscall
restrictions unchanged. Errno methods are data/error methods, not OS calls.

No tests/mutants/Python or live action run here; no permission to resume inferred.
Source delta for reviewer before its one inert rerun. Mutants remain held.
