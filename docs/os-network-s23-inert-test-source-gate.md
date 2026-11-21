# S23 inert test source gate

S22 first authorized compile and vet both exited zero under only
mnet_enrollment_development, no launch tags. Go1.26.6, original temporary
pair-compile.mod, repository go.mod unchanged. Original first stdout/stderr and
exit-code files are packaged under validation/. No repair/rerun occurred.
This was go build plus go vet, NOT go test or test binary execution.

After that successful gate, new fake/pure test source was written, NOT compiled,
vetted or executed. Source implementation also adds one-shot consumption of the
preceding close snapshot context, and explicit final-state→cleanup mapping.
These changes have NOT been compiled/vetted and need independent review before
any execution, including test execution.

The capability input now requires a final ledger record. Missing final record,
rejected custody, attempted-but-unclosed creation, ambiguous close or pending
cleanup cannot be omitted by setting CleanupPending=false. Poison/possibly-
applied history resolves only with confirmed close and exact raw absence.
Observation/expiry times are explicitly stripped to UTC wall time, so a hidden
Go monotonic stamp does not change the projection. It does not enforce live
monotonic authority, which remains Transaction's job. Runner denial is labeled
historical with target unverified when no bound OS path exists, never readiness.

Test source is pure fake protocol plus existing value-only fakeKernel; it defines
the first inertProtocolOnly marker implementation exclusively in a _test.go file.
No live callback/FD/device/process/namespace/socket/signal is introduced. Test
cases derive from the independent S21/S22 tables: clean transaction, delete
budget and changed/readded operation, invalid created custody Stop refusal,
close-context consumption, clone shape guard/nil-empty/netip/nested mutation,
capability final-state mapping/expiry/target mismatch/stripped monotonic, poisoned
readback and close-error/raw-equal reconciliation. This is planned coverage, not
passing evidence. Further adversarial cases remain for reviewer assessment.

Never execute new tests before reviewer source gate. Existing accepted tests,
Transaction and all live/tag guard files unchanged. No repeated preflight.
