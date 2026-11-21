# S30 source/test corrections, UNRUN

Historical reconcile ordering now has its own lastHistoricalTime, completely
separate from owner-sampled active lastObservationTime/lastOwnerStamp. A caller
future-dating history can at worst reject its later historical timestamps, not
poison active ownership. Shared sequence still invalidates old observations on
any newer observation; this is intentional. No historical time is authority.

Contradiction oracles now assert EXACT errors: missing26 yields the existing
link-name-prefixed missing AF_SPEC applicability error; present26 with correctly
framed applicability/sysctl evidence yields unreviewed owned link attr26. Generic
nonnil error cannot pass this oracle. Until separate integration review this
second refusal remains intentional, and positive raw active is not asserted.

Owner tests now distinguish stop/expiry/different Transaction pointer refusals
(no poison or clock distrust) from wall/monotonic rollback (both sticky distrust
and poison). Future historical clock isolation has a direct separate test.

An UNRUN helper-only schema mutation test was added for the UNUSED S29 proposal.
It validates a synthetic exact v6.12 skip-stats shape and rejects nested family,
length, flags, padding, duplicate, named policy/sysctl and directory mutations.
It does not test Compose success or active readiness and is not real dump proof.
The helper stays unused and its Compose integration patch remains NOT APPLIED.

No compile, vet, suite, race, mutants or live effects. Proposed helper profile is
still phase/version limited, current reader does not request it. A broader current
kernel/phase policy remains a review decision. No blanket26 or decoder relaxation.
