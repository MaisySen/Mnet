# S37 practical product decision: endpoint admission, no host-global tuning

DESIGN ONLY. No policy code/knob changes, compile/tests, real supplier/run or OS
effects. S36 independent155 green acknowledged; its blanket hardening choices
are NOT selected as the deployment policy and must not be built on further.

## Decision

Use the user's existing host policy unchanged. Do NOT require all inherited
per-link devconf words equal a hardened vector. Enforce the mesh's exact endpoint
invariant at the original TUN FD's userspace packet-admission boundary, backed by
canonical hardware/flags/creation witnesses and exact routes. Preserve inherited
knobs unless a source-proven necessity requires a narrowly scoped owned-link
write. Never change host globals to make this work. Respect existing host firewall,
XFRM and routing policy; report compatibility failures rather than bypassing them.

This is a change in safety mechanism, NOT a declaration that unchecked words are
harmless. Conditional data-path decisions have explicit preconditions and must be
implemented/tested before conversion. S35/S36 active conversion remains blocked.
No newly proposed write is authorized, and current experimental writes have not
been released for execution.

## Source read: actual effective rules, no remembered semantics

https://raw.githubusercontent.com/torvalds/linux/v6.17/include/linux/inetdevice.h
IN_DEV_FORWARD is per-interface only. MC/BC forwarding and ACCEPT_SOURCE_ROUTE /
BOOTP_RELAY use logical ALL AND interface. RP_FILTER is numeric MAX(ALL,interface).
SRC_VMARK, ACCEPT_LOCAL, LOG_MARTIANS, PROXY_ARP/PVLAN, SHARED_MEDIA, SEND_REDIRECTS,
SECURE_REDIRECTS, PROMOTE_SECONDARIES, ROUTE_LOCALNET, IGNORE_ROUTES_WITH_LINKDOWN,
ARPFILTER are ALL OR interface. ARP accept/announce/ignore/notify use numeric MAX.
ARP_EVICT_NOCARRIER uses ALL AND interface. ACCEPT_REDIRECTS uses AND when this
interface forwards, but OR when it does not. These are net-namespace scoped ALL
values, not necessarily the user's host namespace. Default values initialize new
links; they are not generally the effective ALL operand. No global write needed.

Consequences: setting owned accept_redirects=0 cannot defeat ALL=1 on an endpoint;
setting owned noxfrm/nopolicy=0 cannot defeat ALL OR override; forcing rp_filter=1
cannot override ALL=2. Thus S36's assert vector is neither necessary nor sufficient.
It would reject ordinary configurations and tempt global edits, which we reject.

## Why the minimal policy can tolerate ordinary defaults

Sources below refer to pinned v6.17, not proven running-kernel provenance.
https://raw.githubusercontent.com/torvalds/linux/v6.17/drivers/net/tun.c
TUN initialization: ARPHRD_NONE, zero addr/header length, POINTOPOINT|NOARP|
MULTICAST. TAP is separate. Bind these actual creation facts to original exclusive
nonpersistent FD/namespace and the whole33/60-word vectors BEFORE tagging/setup;
compare later except individually approved owned-link transitions. A kind string
or prior fixture alone isn't that proof. Do not require flags equal zero. Permit
only documented owned UP/RUNNING/carrier transitions while keeping core type flags.

https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/arp.c
arp_rcv and arp_xmit have early NOARP exits; arp_constructor creates NUD_NOARP on
NOARP/point-to-point. Preserve ARP tunables unchanged, not hardening writes. The
carrier notifier still consults ARP_EVICT_NOCARRIER: leave it as host policy, audit
that neighbor state never widens the gateway-free exact peer route. No require1
merely for hardening. A new non-NOARP/replacement link is unsupported, not adopted.

https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/route.c
For exact local destination, ip_route_input_slow takes RTN_LOCAL to local_input
before forwarding branch. Nonlocal destinations need IN_DEV_FORWARD, but admission
will reject them before TUN injection. __ip_do_redirect additionally requires an
existing AF_INET gateway route matching old gateway. The mesh peer route is /32
on-link, NO gateway, so it cannot satisfy that redirect-update precondition. Still
reject ICMP redirects before injection as independent protocol policy; do not rely
on ALL.accept_redirects=0. SEND_REDIRECTS is on forwarding paths not admitted local
packets. SHARED_MEDIA/SECURE_REDIRECTS do not matter after no redirects/gateway.

https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/fib_frontend.c
Source validation: the exact peer /32 reverse route matches the TUN device and
returns before strict mismatch rejection. Therefore valid selected peer packets
are compatible with rp_filter0/1/2 in the isolated exact-rule model. Do NOT force1.
ALL mark-based reverse lookup, host custom tables/VRF/XFRM/firewall can still
change compatibility. Respect them, verify the actual required route lookup and
actual app-path tests in a later authorized environment. Don't promise arbitrary
host networks work or change their routing to hide a conflict. Source=local,
loopback, broadcast/multicast and all non-peer addresses are admission refusals;
ACCEPT_LOCAL/ROUTE_LOCALNET/SRC_VMARK need no global hardening workaround.

https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/ip_options.c
Source-routing options are an explicit optional-header path. Mesh admission will
accept only IPv4 header length20 initially. No IP options, source routes, loose
routes, record-route or timestamp options enter the kernel from peers. This is a
stated v1 protocol restriction, not universal IPv4 support. Do not assume checksum
or equality checks alone prevent option-driven routing.

## Packet-admission contract needed before safety resolution

Both directions are checked BEFORE each authenticated plaintext crosses TUN FD:
IPv4 version4, exact total/header length and checksum, IHL5 initially, bounded by
owned MTU, no fragments initially (nonzero offset or MF refuses), exact own↔selected
peer /32 depending direction, unicast, no loopback/multicast/broadcast, explicitly
supported protocols. Initial TCP/UDP + ICMP echo and carefully validated ICMP errors
is a proposal, not implemented support. ICMP redirects/router advertisements/IGMP
must refuse. Error packets need validated type/code, bounded quoted header and
peer/address binding; PMTU/error semantics require separate tests, not blindly
dropping all ICMP then claiming ordinary TCP compatibility. Datagram MTU and IPv4
fragmentation restrictions must be surfaced in support scope, not hidden failures.

Reject means no kernel injection/no peer send, not broaden route authority.
Kernel-generated outbound multicast/IGMP is drained and discarded locally. Ingress
IGMP/multicast never injected. Membership controls do NOT establish authorization.
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/igmp.c
ip_mc_up automatically joins ALL_HOSTS. Therefore 'no groups exist' was the wrong
precondition; ordinary link-up can create membership without an app. Preserve IGMP
version/timers and multicast flag unchanged; boundary filtering, bounded drain and
no wildcard route ensure no multicast traverses mesh. Multicast discovery is not
supported by this endpoint-only mode. Local malicious privileged code can change
namespace state; required ownership model excludes untrusted CAP_NET_ADMIN and
continuously checks identity/policy, not claims immunity to host root.

## IPv6 and narrowly necessary owned-link configuration

https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/ip6_input.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/ip6_output.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/route.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/addrconf.c
Disabled IPv6 blocks ingress/output and nexthop/address setup paths inspected.
Continue explicit owned-link disable=1 before UP, exact family/directory witness,
no IPv6 addresses/routes at every later phase and IPv6 packet refusal. Other IPv6
knobs are unchanged from creation under those gates, not hard-coded default0.
FORCE_FORWARDING word59 is retained and evaluated in policy but cannot make IPv6
packets cross this disabled link/boundary. Require disable evidence, not global
IPv6 off. Current source's force_forwarding0 rule may be overrestrictive and must
be reviewed against this model rather than silently relaxed.

If fresh TUN inherits disable=0, propose ONE owned-link disable_ipv6=1 transition
only, after exact original FD/type/name/index/namespace/path proof and before UP.
No ALL/default/host global changes, no rp_filter/redirect/XFRM/ARP hardening writes.
This is proposal scope for separate review, not authority to execute. No restore
of a name after closing a nonpersistent link; deleting original link removes its
private sysctl directory. Replaced/symlink/ENOENT paths refuse rather than retarget.
If necessary transition cannot be safely made, diagnose unsupported and stop.

## Architecture fit and actual plug-and-play gap

Current captured-empty-namespace experiment can serve processes explicitly placed
in that namespace. It DOES NOT give arbitrary existing host applications a route
to peer. Keep it as isolated conformance model, not finished product. For the
ordinary-app product, recommend separately installed least-privilege owner in the
appropriate host network context, with exact nonpersistent TUN/local address/peer
routes and original FD custody. It must check address overlap, route lookup,
policy routing/VRF/firewall/XFRM and both directions without changing user globals.
Installer/service/host route writes and packet boundary need separately reviewed
source and explicit user release. They aren't authorized by this design or prior
sandbox probes. No proxy/HTTP substitute and no fake-adapter-as-kernel proof.

## Recommendation and smallest next review slice

Select endpoint-admission + unchanged effective host policy, NOT blanket asserts.
First C: immutable creation ARPHRD/flags/vector evidence, core-flag transition rules
and same original custody. Then A: exact effective namespace ALL rules/route lookup
compatibility as observations, no global adjustments. Then B: pure packet-admission
parser and bounded queues/drain, including IGMP/ICMP/options/fragments/MTU. Only when
these are tested and reviewed should named policy states resolve; shared Compose
remains diagnostic until then. All baseline/foreign-state refusal remains intact.
No implementation started on this revised policy. Existing S36 hardening decisions
are suspended, not mislabeled a viable deployed vector. Full journal/restart and
real supplier are still separate work, no additional status abstractions proposed.
