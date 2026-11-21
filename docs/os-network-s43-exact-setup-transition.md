# S43 exact read send gate, attached diagnostics and serialized setup transition

Source-only UNRUN. S42 independently183 green proved inert tests, NOT live source
compile or host behavior. Author ran no compile/test/vet/race, live helper, device,
FD/netns, ioctl, netlink, sysfs read/write or process. kernelExecutionReleased=false
unchanged. One gofmt parse failure in new source (unparenthesized Go composite
literal in a condition) was fixed, then formatting succeeded. No tests rerun or
failure evidence erased. Formatting is not compile proof.

## Exchange allowlist replaces the incomplete mutation denylist

Actual routeSocket.Exchange now calls the same pure guardReadOnlyRouteSend used
by fake sends, immediately around Sendto. Allowed: exact AF_UNSPEC dump headers for
GETLINK18, GETADDR22, GETROUTE26 and GETRULE34, flags REQUEST|DUMP=0x301, nonzero
sequence and exact current socket local port ID, exact full frame/length/body.
ALL other kinds fail, including DELLINK17, SETLINK19, NEWLINK16, address/route
mutations, unknown requests, get-with-wrong-flags and extra payload. No generic
mutation escape based on a partial denylist or simply holding a creation pointer.

This deliberately refuses mutation attempts from residentNetlink.applyExact too.
That source wrapper still invalidates lineage before attempting its effect, but
actual route socket is read-only until a separately reviewed typed mutation sender
is implemented. The current source cannot configure a ready link through it, and
we do not claim otherwise. The new setup token runner below is fake-only and has
NO real transport implementation. No previously unreviewed mutation path released.

Pure fake send tests use the actual guard, checking that denied commands never
enter a fake sender and each exact permitted read does. Separate wrong sequence,
port, flags, size, trailing bytes and header vectors fail before fake send.
Source-substring checks remain only tripwires, not the evidence for these gates.

## Real fileCustody attachment and diagnostic data flow

Attachment now goes through the actual original fileCustody.attachLineage method
under its mutex. It rejects previously closed, missing-file and already-attached
objects. No detached ledger can masquerade as attachment merely by creating a
resource wrapper. Tests call retainOriginalPacketResources with an inert nonnil
os.File placeholder, inspect that exact object's attached ledger, then enter the
actual ownedCreation guard with a Go-only effect recorder to verify that THIS
attached ledger invalidated. No os.File method is called by these new tests.

fileCustody.Close serializes its closed/lineage capture with attachment. The
ownedCreation guardedEffect holds the file mutex through effect callback execution,
so the original close cannot slip between its lifetime check and callback. Ledger
locks are NOT held across effects. The original close still invalidates before
Close and remains once-only/sticky. Generic lineage invalidation refuses config
on an already closed lineage. Control/dup/export refusal paths remain fail-closed.

heldPacketLedger now serializes snapshot/match/invalidation/setup transitions with
a mutex. A captured generation is compared at every mint boundary. No serialized
state machine calls an external effect while holding the ledger mutex.

Removed parse-and-discard/name-prefix TUN validation from generic sysctl collector.
It still collects/parses canonical33 ALL/default operands. TUN parsing happens in
the held-handle-bound diagnostic collector, where its OUTPUT is retained together
with computed effective fields/raw/sys and a typed heldSourceRecord. The actual
unreleased source method captures original owner/session/ledger identity before
raw/sys reads, then diagnosticForHeldRecords rejects a changed generation and
selects exact canonical creation/current link, not every mno- named link. It
preserves parsed framing/effective values for downstream use, not just validates
and throws them away. Empty creation alias is not a wildcard matching all links.

Tests exercise actual data flow: parsed TUN type and send_redirects effective
value appear in the held diagnostic, copied operand maps don't change afterward,
and close invalidation prevents use of the captured held record. Wrong/replaced
handle still refuses. These are fake-record tests, NOT kernel identity proof.
The typed held record binds correlation to original resources; actual original
TUN FD/netns identity query remains UNIMPLEMENTED. No method calls it verified.

## Exact setup transition, first allowed Plan step only

This slice implements a complete inert transition for Plan step1,
set-alias-mtu-exact. It does not pretend to have derived all kernel side effects
for disable/address/up/route. Those later steps explicitly refuse this transition
and remain source work. This narrower step is chosen because its full raw expected
change can be derived without guessing IPv6 removal, address origins or generated
routes. No generic caller-specified expected-state callback or diff exception list.

Begin requires original typed held diagnostic on the same usable generation,
exact Plan/canonical session phase1, original owner and live owner deadline, exact
operation==Plan.Operations[1], original creation handle, expected name/alias/MTU,
and re-parsed framing/effective values equal the retained diagnostic output. It
freezes complete raw inventory and all sys operands. Expected poststate changes
ONLY that exact original link's decoded/raw alias and MTU to Plan values. Every
other raw attribute, header, link, address, route, rule and sysctl remains exact.
Nothing can be reacquired by an interface name or recycled index after custody loss.

Begin invalidates BEFORE an effect can run and creates a private transition token.
Authorized repeated guards bound to THAT token increment and capture the actual
latest generation. It does not assume generation changed by exactly1: tests use
begin plus extra guard plus runner guard. Generic invalidation from close, config,
dup/export, namespace/custody loss cancels the token instead of updating its lease.
No general unpoison or mint-by-boolean function exists.

The fake-only wire returns a raw netlink ACK Datagram plus structured readback.
ACK evidence requires kernel sender0/no truncation, exact36-byte NLMSG_ERROR with
zero error, capped flag, exact sequence/local recipient and echoed original request
header. Error/partial/truncated/unknown sender/TLV/multipart variants are not ACK
proof. This shape is grounded in upstream v6.17 netlink_ack: success zero error
sets NLM_F_CAPPED regardless of NETLINK_CAP_ACK and copies the original request
header into struct nlmsgerr. This is a supported narrow success profile, not an
invented boolean ACK.

Source URL and anchor:
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/netlink/af_netlink.c
netlink_ack2465..2506, success/cap condition2480..2484 and echoed header2495..2500.

Poststate evidence is minted only after exact full raw/sys expected-state equality,
before+after modeled same original object/generation/device+target namespace,
framing reparse and verified observed feature-name compatibility. Replaced object
with same serial, namespace drift, unrelated attribute drift, changed ALL, partial
MTU, VNET framing, checksum/GSO active or changed feature names all fail. The
readback record is supplied by a marker interface implemented ONLY in tests.
These modeled original-FD identity records are NOT an actual FD/netns verifier.
A real supplier must implement those kernel facts before using this design for
anything live. This token runner cannot implement or bypass the actual read-only
routeSocket gate.

Finish requires the exact typed ACK and post evidence from the same token, captured
latest generation still equal current generation, original lineage not closed,
and identical expected state. It mints once, increments generation and marks only
this alias/MTU lineage state usable/tag-confirmed. It does NOT advance canonical
adapterSession ACK/readback state, set Transaction.Ready, relax full devconf policy,
verify authenticated peer MTU or open a packet path. No live caller exists.

Effect error, ACK error, read failure, timeout, partial/drifting poststate or other
invalidation stays uncertain/dead. Concurrent close/config invalidation between
postread and commit wins by the serialized pointer/generation check, and cannot
be undone by delayed evidence. Reusing completed/stale tokens cannot run another
effect or poison a newer unrelated successful lineage. Close after a successful
mint invalidates it in the ordinary way. Tests include a separate goroutine that
invalidates before finish, joined with a channel, with no OS effects.

## Scope and next gates

UNRUN vectors cover actual attachment/data flow; exact allowed read fake sends;
positive alias/MTU ACK+whole-poststate mint; double guards; once-only; cross-token
ACK; wrong/error/partial ACK, effect/read failure, timeout, replaced original,
netns/raw/ALL/framing/feature drift and concurrent close/unrelated config. Existing
source oracles are retained only as guards against disconnecting real call sites.

Actual live compile, original FD/device-netns proof, real ethtool family/name wire,
typed real mutation sender, subsequent allowed Plan transitions and raw generated
side effects, full strict Compose, effective IPv6, authenticated peer MTU and real
installed host routing/firewall/XFRM/CGNAT/app path remain incomplete. Packet and
readiness output remain blocked. No actual read/helper/effect run authorized or
taken; no host-global changes, shared Mux/mobile/view edits or app byte rewriting.
