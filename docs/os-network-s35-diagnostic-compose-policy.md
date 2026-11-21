# S35 canonical diagnostic Compose, source and UNRUN tests

No compile/vet/test/mutant execution in this revision. S34 independent150 green
is prior evidence, not this batch. Compose ownership path and policy are connected
for diagnostics ONLY. No real supplier, device/run, reader/dump or sysctl effects.

## Narrow shared Compose change

Existing CapturedBaseline.Compose delegates to a private shared implementation
with nil owned-AF proof. It retains its existing strict attr26 refusal. The new
canonical owned-TUN diagnostic route can admit26 only through a privately built
proof bound to this session's captured baseline, canonical handle, CURRENT reader
pass, exact received owned link and same-pass sysctl map. Before the exception it
runs the full FULL profile/phase parser. Raw.Links must equal the reader-decoded
links exactly. A caller cannot substitute an owned link or a profile bool.
All existing baseline raw attrs/rules/sysctls remain exactly compared, unknown
owned attrs remain refused, and exact address/route composition remains unchanged.
The old generic contradiction isn't called solved; a new explicit diagnostic
route can compose the reviewed shape. Active readiness still cannot use it.

The inert reader now requests global IPv4, all/default directory and named leaves
in the same pass, so diagnostic Compose can run existing baseline validation.
Only the test-memory source implements that interface. No live read calls added.
Same-pass is still sequential, not atomic/stable kernel proof.

## IPv6 absence is in this slice

For ledger forwardStep3..6, BEFORE shared Compose, refuse ANY IPv6 address or route
in the supplied whole raw namespace, including family/address/source/gateway/
preferred-source evidence. No link-name-only exemption. Post-disable phase still
requires disable1/forwarding0/force0 and matching applicable directory. The tests
cover address+route at each phase3/4/5/6, not deferred until readiness conversion.

## Named per-word product policy, not inherited default trust

Product invariant: one local endpoint /32 and exact peer /32, no IP transit or
implicit IPv6 state. TUN packets enter/leave userspace, not an excuse to treat the
interface's kernel devconf switches as harmless. Sources:
https://docs.kernel.org/networking/tuntap.html
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/devinet.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/addrconf.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/ipv6.h
IPv4 indexed names from inspected v6.12 ip.h (count33), exact fixture count33;
IPv6 indexed names from v6.17 UAPI/count60 and source emitter. No version proof.

owned_tun_word_policy.go enumerates ALL33 IPv4 and ALL60 IPv6 names/indices/values.
It applies named necessary no-transit forwarding0 rules to IPv4 index0 and IPv6
indices0/59. Phase disable, addrgen range/matching and omitted zero slot are
marked as separately validated named rules. These checks are NECESSARY, never
sufficient for a policy-complete ready path.

Every remaining named word has explicit UNRESOLVED-effect-on-exact-local-endpoint-
no-transit-invariant, a release BLOCKER, not audit-only acceptance. Examples:
IPv4 multicast forwarding, proxy ARP/PVLAN, redirect acceptance/send, rp_filter,
source routing, localnet/accept_local, XFRM/policy bypass, ARP lifecycle, reverse
path and multicast timings need policy decisions; IPv6 RA/autoconf/redirects,
DAD/source routing/proxy/segment routing/RPL/IOAM, policy bypass, lifetimes and
routing metrics need separate effect review. Default or baseline equality is
NOT safety. Retained unexamined words don't become authorization.

The diagnostic output is a private distinct container with the exact raw bytes,
modeled inventory and named policy assessment. Its ONLY activeInventory method
unconditionally returns owned-tun-devconf-policy-incomplete. There is no success
branch gated by an ignorable unchecked flag. Disabled candidate still distinct;
changing uncheckedDevconf=false in-package cannot unlock conversion. Existing
active observe method still uses generic Compose and cannot bypass the gate.
No broad NetworkingReady claim is possible from the new diagnostic route.

## UNRUN independent oracles

- Drive canonical ledger phases through value fake ACK/readback, separately build
  raw received FULL pass with baseline links and same-pass named witnesses.
- Exact generic attr26 refusal remains; canonical diagnostic route should compose
  without raw mutation, enumerate93 named words and remain policy-blocked.
- In-package ignored unchecked flag cannot make diagnostic activeInventory pass.
- Caller raw/pass mismatch and a received baseline mutation both refuse.
- IPv6 addresses/routes at each phase3..6 refuse with typed exact reason.
- Unresolved IPv4 rp_filter and IPv6 autoconf mutations stay named unresolved,
  never declared safe merely because values were retained.

No test result claimed. Source review/effect containment precedes reviewer-owned
once-only inert execution. Remaining production gap is explicit safety policy
and reviewed real supplier/custody, not an optional status feature. Real supplier
risks (path/symlink/ENOENT provenance, sender/truncation, stable inventory and
namespace/FD identity) are not solved by the memory reader or this Compose route.
