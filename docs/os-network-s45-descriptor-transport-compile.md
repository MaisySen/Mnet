# S45 descriptor-safe access, real ethtool source and compile-only result

S44 independent193 green remains inert test evidence. S45 author executed NO
unit tests, go run, binary/preflight/launch, device/ioctl/netlink/sysfs read/write,
helper or live transition. Compile-only permission was used as reported below.
All actual integration source remains behind existing live tag and unchanged
kernelExecutionReleased=false. No setup finish/mint, readiness or packet path run.

## Descriptor access without File.Fd side effects

S44 verifier called os.File.Fd. Installed Go source explicitly shows file_unix.go
File.fd calls SetBlocking when f.nonblock is true. That could silently alter the
TUN's nonblocking mode. The verifier now uses File.SyscallConn then RawConn.Control.
All original descriptor syscalls are inside that callback; no descriptor number
is stored in a field, returned, duplicated, closed, passed to a future goroutine or
used after callback return. The inner inspection method is called synchronously
only within Control. Original fileCustody mutex still spans verifier and dependent
source use, guarding owned Close. Control itself takes a runtime descriptor
reference and keeps File alive; no SetBlocking or F_SETFL is invoked.

F_GETFL is sampled before and after callback inspection and must match exactly.
F_GETFD observes required CLOEXEC. Errors or status-flag drift fail; there is no
attempt to restore flags by writing them or hide another writer's interference.
The returned device-netns FD is distinct and is closed within callback lifetime;
the original TUN FD is never closed by observation. Context and locks still cannot
promise a hard cancellation deadline for every ioctl/kernel stall.

Grounding inspected in installed Go sources:

- os/file.go SyscallConn698..703, Fd documentation705..723 recommends SyscallConn.
- os/file_unix.go70..85 File.fd performs SetBlocking for nonblock File.
- os/rawconn.go18..25 Control calls pfd.RawControl and runtime.KeepAlive.
- internal/poll/fd_posix.go56..62 RawControl incref/decref around callback.

Current1.26.11 source comes from the preinstalled toolchain module. These source
contracts explain the chosen access, not an actual runtime TUN observation. The
source verifier remains proposed and never invoked.

## Real owned ethtool transport source, no execution

New ownedEthtoolSocket source opens a private NONBLOCK|CLOEXEC NETLINK_GENERIC
socket in the held namespace, binds an actual kernel-assigned local port and uses
THAT SAME socket for controller family resolution, STRSET feature names and compact
FEATURES reply. One collection per socket, strictly increasing sequence, no
multicast subscription or mutation command. Namespace custody/current-thread is
checked before send and during receive. No fake-only marker is implemented by
this real source; tests cannot call it as an inert wire.

The transport uses bounded4096-byte single datagrams with recvmsg truncation and
kernel sockaddr sender0 checks. Request header must match current local port and
sequence/length/REQUEST flag. Only controller GETFAMILY3/version2 on family16, or
resolved ethtool STRSET_GET1/FEATURES_GET11/version1 are admitted. Reply decoders
already bind exact length/family/sequence/recipient/device/command/schema; errors,
notifications, multipart, truncation and trailing frames fail. Poll10ms and
context250ms are approximate response bounds, NOT a hard interruptible close or
kernel syscall guarantee. Any collection error closes/poisons the owned socket;
no retry reassociates a late old response. The socket closes normally on success.

Controller schema is grounded in pinned Linux v6.17 genetlink.c ctrl_fill_info:
NEWFAMILY1 response from controller family16/version2, unflagged NLA attrs and
unflagged OPS/MCAST nests. Requires observed family name exactly ethtool, dynamic
u16 family ID>=16, ethtool version1 and header size0 plus maxattr u32. Optional
operation/mcast lists are bounded/framed, not treated as authentication or guessed
feature schema. Operations are id/flags u32; groups name/id. Unknown attributes
refuse this profile. Count/version labels still never replace actual STRSET
feature-index/name matching on the next request.

The explicit family request asks by exact observed name; no cached constant or
fallback to a remembered family number. Verified STRSET names and ACTIVE bits then
use existing actual same-socket sender/sequence and ifindex/name comparisons.
Hardware/wanted do not determine raw output compatibility. Ordinary fresh TUN
positive mask from S44 stays supported; no blanket all-active-bits-zero policy.

Sources inspected through readable fetch after direct source endpoint returned429:

https://raw.githubusercontent.com/torvalds/linux/v6.17/net/netlink/genetlink.c
ctrl_fill_info emits name/ID/version/hdrsize/maxattr plus unflagged OPS/MCAST,
ctrl_getfamily resolves and replies NEWFAMILY, genl_ctrl version2.
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/genetlink.h
GENL_ID_CTRL=16, CTRL_CMD_GETFAMILY3, NEWFAMILY1 and attribute IDs1..7.

Earlier feature sources remain:
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ethtool/strset.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ethtool/features.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/ethtool_netlink_generated.h
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ethtool/common.c

The readable source content was inspected at ctrl_fill_info and ctrl_getfamily,
not inferred from429 or a search highlight. Local fake family-schema vectors are
UNRUN: dynamic observed family/name success, incorrect sender/seq/port/command/
version/family/name/ID/header/duplicate/flags/operation-layout failure.

## Proposal-to-post evidence design connected in source

collectSetupPostSource is an UNUSED unreleased source method, first Plan step only.
It holds original file mutex, verifies original TUN/device-netns through
SyscallConn.Control, collects actual stable raw inventory and canonical ALL/default
operands, opens the owned generic socket, resolves family/STRSET/FEATURES, then
verifies original device again. The before/after original identity, kernel-derived
namespace/name must match. Returns originalSetupPostProposal preserving the
before/after kernel-source evidence and raw readback record with actual verified
names/features. It deliberately does NOT call parsePost, mint setupPostEvidence,
call finish or set Ready. The future bridge must run the existing pure parsePost
checks for exact token/generation, expected raw/sys, explicit counter/timestamp
policy, framing and verified names/ACTIVE compatibility before pairing a matching
ACK. This boundary was tightened after the successful compile, so the final
proposal-record-only revision has NOT itself been compiled by the author.

No actual source caller exists. A future reviewed orchestration must pair the actual
same-token ACK, current captured generation, live authority and this record before
committing. Source existence/compilation is not proof kernel identity checks ran,
permission worked, expected raw profile matched, or runtime reassembly/host app
networking works. Failing close/config invalidation still wins by serialized
latest-generation comparison. Later Plan disable/address/up/route steps remain
unsupported, as do any claim of authenticated peer MTU or ordinary-host route path.

## Compile-only attempts, both preserved

Before build, audited package source for init functions, go:generate, go:linkname,
C imports: none. go.mod has tool declarations for gobind and govulncheck, not
invoked by an explicit package go build. No build scripts/Makefile/generation or
commands were run. CGO disabled and explicit package target prevents producing or
executing task binaries. Offline flags prevent dependency/toolchain downloads.

Attempt1 used default installed local Go1.26.6, with GOTOOLCHAIN=local. It FAILED
before compiling source because go.mod requires>=1.26.11. Log retained verbatim,
no go.mod lowering or automatic download. This was reported promptly.

After separate parent permission for read-only local compiler discovery and one
retry if a preinstalled1.26.11 exists, found:
/home/sandbox/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.11.linux-amd64/bin/go
Verified that binary reports go version go1.26.11 linux/amd64. Attempt2 succeeded
exit0 with no compiler diagnostics:

CGO_ENABLED=0 GOFLAGS= GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
<verified-existing-go1.26.11> build -mod=readonly
-tags 'mnet_enrollment_development mnet_os_process_launch' ./experiments/osownership

NEVER preflight_launch, never go test/run or compiled binary execution. Separate
attempt logs delivered. Empty success log means no compiler output, not missing
failure. This verifies current live-tag PACKAGE source compiles and x/sys helpers/
constants resolve on linux/amd64 with this toolchain. Source inspection also found
TUNGETDEVNETNS and IoctlRetInt/IoctlIfreq in installed x/sys0.50.0. It does NOT
prove kernel ioctl availability, runtime permissions, device access, or correctness.
Inert tests in this slice still await independent once-only reviewer run.

## Remaining gates

Source review of real descriptor lifetime, ethtool bounds/schema and same-pass
identity/readback before any execution; no helper/read/ioctl/mutation allowed by
the compile permission. No complete real adapter orchestrates creation -> first
ACK -> post evidence -> canonical ledger. No subsequent setup transitions, strict
active Compose/full IPv6, authenticated peer-MTU exchange, host routing/firewall/
XFRM/CGNAT overlap or real installed ordinary-app proof. Three product tools retain
separate authority. No shared Mux/mobile/session/view/sessionproof edits.
