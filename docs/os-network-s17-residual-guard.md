# S17 residual guard and effect separation, no test execution

Reviewer conditional suite correctly did NOT run. This delta is source/static
and compile-only, no test/race/Python/mutant or actual device/namespace action.

Whole resident_linux.go now live process-launch only, so residentChild type,
pidfd SIGKILL AbortAndReap/reap/close methods, namespace ancestry/alive check,
resident pipe/nondumpability bootstrap are ABSENT from inert link. Pure command
constructor moved separately to resident_command_linux.go for existing exact
placement tests; creates Cmd only, never Start/Run. Source test retargeted.
No inert residentChild constructor or equivalent type supplied.

numericCurrentNamespace, rawCurrentNamespace and emitClientBootstrapEvidence
(with owner-signal0) moved unchanged to bootstrap_observation_live_linux.go,
process-launch tag. Inert read-only worker can only return release refusal via
parseSourceParentNamespace stub and inert client launch, actual entry default
inert. No use of former parent-signal function is available to tests. Standalone
rawReadBounded/clientFDInventorySource/status verifier remain uncalled source
reads, statically denied by extended test-reference guard.

Deny identifiers now includes requested parent-signal verifier, namespace
numeric/raw readers, pidfdAlive, FD/proc readers, status verifier, held namespace
and userns ancestry/process identity helpers. AST rejects references to concrete
residentChild/namespaceCustody/device ops/netlink types, not just CompositeLit,
covering new/cast/method expression routes. Denied import paths os/exec/syscall;
unix path only unaliased device-number helpers Mkdev/Major/Minor exception,
all other calls forbidden. Ban os.StartProcess and Start/Run method calls except
known tx transaction and t.Run subtests. These are source tripwires, not arbitrary
malicious-code proof; live-link separation is the main boundary. No fake-interface
resident AbortAndReap methods forbidden, they are required by supervisor tests.

Audit discovered TestResidentCommandPlacementSourceNoExecution actually invokes
pure residentCommand constructor (previous broad audit called it source-only).
Retained behavior and pure constructor, no Start/Run: it verifies exact flags,
mappings, mode and descriptor count on inert Cmd. Corrected description, not
an observed process launch. Earlier source-only wording was imprecise.

Compile-only both inert test binary and separately live executable clean. No
suite resumed here. Exact production-expression future mutant proposal in
separate attached file, NOT applied. Build tags/audit tests/forbid marker/inert
stubs/live bindings all excluded from future mutation proposal. Reviewer owns
first once inert suite only after offline acceptance of this exact tree.
