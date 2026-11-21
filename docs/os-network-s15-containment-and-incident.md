# S15 corrections and mutation incident, tests paused

## Incident, not a permitted execution

Before the pause instruction arrived, requested mode-extra mutant changed
len(args)!=1 to len(args)<1. Existing parser test called actual recognized
--device-open-preflight with an extra argument, relying on its arity guard.
Mutant bypassed it, started ONE resident test binary in fresh user+net namespace.
Child1214/start3582 exited2 (Go test executable rejects resident-mode argv),
supervisor abort/reap observed, custody closed=true, outer namespace unchanged.
No device metadata/open worker was dispatched. Evidence is original mutant
output in os-s15-mutants.log. No exact child syscall/tree or postscan exists;
no stronger containment or absence claim. No raw private stderr retained.
This process/namespace action exceeded source-only authority. Reported promptly.

Other six entry mutants killed; arity was then changed to static tripwire first,
rerun once killed without launch output. Base/selected suites and Python10 ran
before stop instruction delivery, included logs explicitly pre-containment.
Those results do NOT validate the subsequent containment correction. No tests
or mutants executed after pause receipt. Only source edits/static reads/gofmt
and compile-only go test -c/go build afterward, no init/test execution.

## Structural containment source, pending review

Test-order-only patch was insufficient. Pure devicePreflightDispatch takes
injected uid/namespace/supervise/resident/output functions. DevicePreflightMain
binds inert refusal callbacks ONLY. Thin DevicePreflightExecutableMain live
binding is in separate source requiring BOTH mnet_os_preflight_launch and
mnet_os_process_launch beyond prior tags. Unit parser tests use explicit fake
callbacks or default inert entry even for accepted modes.

All actual resident spawn/client Start and fixed-client syscall.Exec bodies
moved unchanged into mnet_os_process_launch-only files. Ordinary build has
same-signature inert stubs returning ErrExecutorUnreleased. BootstrapEvidenceMain
live switch similarly separated, default inert stub. Compiled unit/mutation
binary without tag cannot reach real spawn through bootstrap/supervisor defaults.
Production-only launch marker conflicts with a same-name declaration in tagged
_test.go, so adding process-launch tag to tests FAILS COMPILE, not runtime guard.
No test binary filename convention is used. This also means future live compile
must explicitly include launch tag, not prior four-tag default. Source behavior
of reviewed live bootstrap bodies kept, locations changed and source tests retargeted.

Compile-only checks: inert tagged test binary compiled, full separately tagged
preflight executable compiled, deliberate live-tag test binary refused with
expected duplicate declaration. Neither compiled binary run. Newly added static
AST test audits direct real-effect refs across every *_test.go and rejects
unix/syscall/exec call sites except pure Mkdev/Major/Minor. It is SOURCE ONLY,
not executed yet. Static guards are tripwires, not arbitrary-mutation proof.

## Audit of entire package test call graph

| Test family | Entry/effects | Boundary |
|---|---|---|
| Preflight parser/modes | default DevicePreflightMain/injected dispatch | inert callbacks, live binding absent |
| Preflight ops/baseline | fakeDevice and fake snapshot | no concrete device ops |
| Bootstrap schema/diagnostic | validate/parse/sourceFunction | reads source/fixtures, live Main absent |
| Supervisor/lifecycle | fakeSpawn/closeFailSpawn/stuckSpawn | explicit inert ResidentProcess fakes |
| Resident/client safety | sourceFunction only | actual launch/exec bodies absent from test link |
| Netlink inventory | fake dumps/manyRecords | ReadStableInventory gets explicit fake transport |
| Transaction/owner/clock | fakeKernel/fake clocks | no real Kernel adapter |
| FD inventory | parseOwnedFDInventory/sourceFunction | no actual /proc FD inventory invocation |
| Guards | constants/sourceFunction | removed direct openExclusive helper call to avoid guard-mutation escape |
| Python wrapper | tempfile fixtures/mocks | Popen/identity/kill mocked, no launch executed |

Read-only source grep audited accepted-mode calls. No test calls concrete
residentBootstrapSource/runDevicePreflightSource/openDevice/filter install.
New static tests are proposed for reviewer inspection before any resume.
Production helpers still exist in source; this does not prove arbitrary hostile
edits cannot introduce a new syscall. Review each future mutant's compile set.

## Requested evidence corrections

Outer validation now requires explicit isolated resident identity, valid outer
numeric identity and refuses equal DEVICE+INODE regardless of Isolated flag.
Equal-outer falseflag test decisively kills prior vacuous check. Reaped,
CustodyRetained,CleanupConfirmed,Bootstrap-nil,6s timeout and mode arity tests
added. Earlier six+arity mutants killed before containment edits; need re-review
and separately approved safe rerun now. Historical S14 defect explicitly withdrawn.

Wrapper recovers bounded owner verdict lifecycle PID/start into postchecks even
if samples missed resident. Duplicate JSON/overflow refused. Numeric PID/start
match for recovered records, explicit same_lifetime true survivor/false reused
or absent-at-read/null unreadable-unknown. Missing exe with existing stat remains
unknown, not absence. Per-read Tgid/argv/timestamps retained. Postchecks are not
process cleanup authority or all-descendant proof; expected child claim must
still be validated through Go owner lifecycle and evidence review. Earlier S14
omission corrected, no new process observation performed.

Existing no-ioctl/device identity/flags/single close and owner baseline safeguards
remain. No TUN/install/host/user-machine access authority, new run not released.
