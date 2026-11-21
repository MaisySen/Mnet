# Prepared recovery review reconciliation

This follow-up does not claim review acceptance or production activation. It is
on top of3a270a3 and retains independent ten-case cross-app success separately
from the control race instability found during review.

## Exact baseline reproduction and timing cause

The reviewer reported close-file/same-reserve fresh-process reconciliation
failure without retained failing output. We did not dismiss it as an established
flake. Exact3a270a3 and exact126a4c parent archives were tested with full stdout,
stderr and exit statuses retained. Sequential ten runs each passed. Concurrent
30-run races of the same unchanged control test reproduced failures on BOTH:
head had six close-file failures and eleven same-reserve reconciliation errors;
parent had six close-file failures and thirteen same-reserve errors. Neither
source's control workflow test nor production workflow source differed.

Disposable diagnostic copies of both exact revisions recorded the precise clock
passed to DecodeServiceReserve and source checkedAt. Actual source observation
could be AFTER the computed decoder time while already BEFORE actual wall time.
Example from parent: local omitted pre-send time4.50ms, source checkedAt2.42ms
ahead of the computed decoder clock. This explained fail-closed availability
refusals, including the same-reserve path. Deliberately bad adversarial replies
also fail in these suites and are not all timing failures.

The pool accepted a supplied clock before locking/validation. It passed that
unadvanced clock to Service. Service did local validation/request preparation but
advanced the supplied clock only by elapsed time starting just before HTTP.
Both local intervals were omitted. Under contention, legitimate source timestamps
could look future. The fix starts elapsed measurement at each method's entry and
passes Pool's full elapsed advancement to Service, whose own full elapsed time
then reaches response decoding. No tolerance is widened, source timestamp is not
rewritten, source expiry is not extended, and genuinely future evidence is still
refused. Capacity, one-shot accounting and unknown-response semantics are unchanged.

Deterministic tests delay local admission before HTTP separately at Service and
Pool, and separately hold Pool's mutex. All three fail before and pass after.
A genuine future source reply remains refused. The SAME minimal service/pool fix
applied to disposable parent and corrected head makes both concurrent30-run race
commands pass. Diagnostic changes exist only in disposable copies and are NOT
in the source commit. Full diagnostic and original/fixed logs are supplied.

## Explicit expired prepared5 and prepared7 native controls

Each new native control first performs actual durable native key proof with
verified TLS and pins the original valid receipt. Prepared5 forces rollback-safe
State publication failure AFTER its prepared marker. Prepared7 first closes and
reopens, obtains an actual authenticated historical reply and then forces the
same publication failure. Both close, verify normal New/LoadState refusal, open
the narrow explicit exact-original-State owner, then WAIT past the unchanged
receipt's real original access expiry. No file date/projection or injected caller
clock is edited to fake expiry.

The explicit bounded historical read can return the original history but must
refuse readiness/finalization. Tests assert sole State byte-identical, no native
credential or historical preparation, original finalization JSON byte-identical,
strict full challenge/history receipt equality and unchanged access expiry.
Normal New/LoadState remain refused. A fresh explicit narrow owner cannot renew
or publish State; original5 spends at most its second original slot, while
original7's already-spent two-slot budget sends nothing further. Reopen cannot
refund a slot. These tests pass under race and are part of full graph validation.

## Scope and validation

This follow-up includes only the elapsed-origin fix, its deterministic tests,
two explicit naturally expired prepared5/7 controls and this evidence note.
Final full graph race, vet, tagged vet, build and the unchanged ten-case cross-app
suite all exit0, with stdout/stderr and explicit exit files retained. Broader
production, physical, current revocation, access renewal, erased-anchor and
availability limitations in earlier documents remain. The review still decides
acceptance. No repository push, service deployment or native activation occurred.
