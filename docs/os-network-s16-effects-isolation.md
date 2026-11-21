# S16 stronger effects separation, source/compile only

Tests remain PAUSED. No ordinary/Python/mutant test executed. Compile-only
inert go test -c and separately live go build allowed; artifacts NOT RUN.

Real residentDevicePreflight and runDevicePreflightSource moved verbatim out of
pure ops/seam into device_preflight_effects_live_linux.go requiring BOTH launch
tags. Inert build has only runDevicePreflightSource refusal, no concrete device
ops type. No real lstat/open/fstat/fcntl/close device implementation in unit link.

Related effects audit and isolation:
- rtnetlink_linux.go entirely requires process-launch: real checkCurrent,
  routeSocket open/bind/exchange/close and residentNetlink Request/apply absent.
  Inert residentNetlink/checkCurrent methods refuse without syscall.
- kernel_linux.go requires launch: real proveNamespaceFD and TUN open/ioctl absent.
  Held types/false kernel guard/error moved to shared file. Inert entry stubs refuse.
- sysctl_read_linux.go entirely requires launch; inert collector refuses.
- disableOwnedIPv6 moved to live-tagged custody file; inert stub refuses.
  Pure fileCustody Close remains for tests with test-owned resources, not device
  creation. createTUNSource can only call inert TUN helper under unit builds.
- Seccomp installer moved live-tagged; pure filter generator/BPF interpreter
  remains testable, inert installer refuses without PRCTL.
- Prior live resident/client launch/bootstrap/exec boundaries retained, tests
  cannot link process-launch tag due deliberate duplicate marker compile failure.

Residual linked reads/control source: clientFDInventorySource/rawReadBounded,
process identity/status readers, numeric namespace reader, resident bootstrap
pipe/nondumpable code remain present but no tests call them, default dispatcher
inert and real launcher absent. Static test-reference audit denies key concrete
entry points, not proof against arbitrary source injection. No real device,
netlink socket, TUN/sysctl writer/filter installer via retained test dependency.
Future stronger read isolation can be separate, not silently called done.

Source-inspection tests retarget actual live-effect file paths. No assertions
removed, no tests run to claim updated assertions pass. Compile-only proves
syntax/type link, not runtime tests. Source bodies unchanged except build/link
separation; no new authority or fallback. Existing false mutation guard retained.

Mutation proposal for AFTER suite-only review and release, never now:
Allowed production expression edits ONLY device_preflight_entry_linux.go:
numeric equal-outer predicate, !Lifecycle.Reaped, Lifecycle.CustodyRetained,
!CleanupConfirmed, Bootstrap!=nil, 6*time.Second, len(args)!=1. These parser
callbacks are inert by default even if arity bypassed. Device pure file edits
only captureDevicePreflight namespace/identity/flag/close predicates and
runDevicePreflightBoundedSource isolation/hash predicates against fake ops.
Do NOT mutate build tags, *_inert*, live effects files, static audit/guard tests,
forbid_live_launch_test marker or callback binding/launch files. Supply exact
changed file/expression list for review before each future mutant batch. No
mutation batch released. First eventual requested execution is inert suite only.
