# S33 unused FULL profile proposal, no integration or execution

Source grounding and an UNUSED parser proposal only. No Compose/reader patch,
compile, vet, test/mutant, device/capture/netlink/system effect. S29 skip-stats
helper and its helper-only tests are retired from current source, retained in git
history. Its old NOT-APPLIED patch is historical and must not be applied.

## Count60 resolved as source shape, not kernel identity

Pinned Linux v6.17:
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/ipv6.h
DEVCONF_FORCE_FORWARDING follows ACCEPT_RA_MIN_LFT at index59; DEVCONF_MAX=60.
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/addrconf.c
ipv6_store_devconf explicitly writes cnf.force_forwarding to index59. This is
an actual forwarding policy switch, not padding that can be dropped. The same
file registers force_forwarding sysctl; disabling all.forwarding clears interface
force_forwarding. A count60 profile must validate that last word, not truncate.
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/net/if_inet6.h
Defines IF_RS_SENT0x10, IF_RA_RCVD0x20, IF_RA_MANAGED0x40, IF_RA_OTHERCONF0x80,
IF_READY0x80000000. Unknown bits refuse, zero is not universal.

Local downloaded v6.16 UAPI has59 words, v6.17 and v6.18 have60. This narrows the
public-source addition, but does NOT establish that the fixture ran v6.17. Backports
and later versions can share shape. Shape ID deliberately omits a kernel claim.
One public download returned429, retained as a fetch limitation; no retry loop.

## FULL request/reply contract grounded independently

https://raw.githubusercontent.com/torvalds/linux/v6.17/net/core/rtnetlink.c
rtnl_dump_ifinfo initializes ext_filter_mask=0 and sets it only from IFLA_EXT_MASK.
Current DumpRequestHeader(18) is16 zero bytes, Request dump flags=0x301
(REQUEST|ROOT|MATCH), no mask attr. No source request changes proposed.

https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv6/addrconf.c
inet6_fill_ifla6_attrs emits flags4/cache16/conf DEVCONF_MAX*4, FULL stats,
ICMP stats, token16, addrgen1; RA-MTU4 only if nonzero. Stats helpers emit their
count in the first u64. Skip-stats behavior is NOT selected here.
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/snmp.h
IPSTATS_MIB_MAX=38, ICMP6_MIB_MAX=7, matching304/56byte payloads. Count words
must agree as well as lengths; no multiples-of8 wildcard.
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ipv4/devinet.c
inet_fill_link_af emits cnf.data indexed vector under IFLA_INET_CONF. Existing
recorded FULL fixture directly confirms132bytes/count33. Its exact enum order
is also in previously inspected v6.12 ip.h. A requested v6.17 ip.h fetch returned
429 and is not claimed inspected. This residual UAPI cross-version v4 review
must close before integration; no hidden assumption of exact running version.

## Parser boundary

Shape ID full-v4c33-v6c60-ip38-icmp7 is an explicit caller claim paired with exact
request bytes/flags/no-mask, not self-authenticating request provenance. A future
reader would supply that record from its own request; no such wiring exists.
Parser accepts exact2+optional10 families; required2; nested strict decoder
preserves duplicates/padding/flag/unknown refusal. V4 CONF132; V6 flags/conf240/
stats304/cache16/icmp56/token16/addrgen1 plus optional nonzero RA-MTU4, no other
nested attrs. Raw bytes deep-copied for output, never normalized or truncated.

Named policy checks: v4+v6 forwarding0, IPv6 FORCE_FORWARDING index59=0,
disable0/1, addrgen0..3, matching proc leaves and top-level addrgen attr, omitted
stable-secret slot36 zero. Other devconf and cache/stat/token bytes are retained,
NOT asserted semantically harmless or fully audited. The proposal is not a
complete security validation of every config word or an OS ownership proof.

## Phase and directory table

Phase claim is explicitly caller data here, NOT a ledger authorization. Before
integration it must be derived from canonical owned transitions, never caller
promotion. Applicable-at-creation is retained; changes refuse pending review.

| Phase | Family10/directory | Policy |
| --- | --- | --- |
| created/tagged | presence exactly matches captured applicability and present/ENOENT directory evidence | applicable leaves required; disable0 or1 observed, never completed automatically |
| disable ACK/readback pending | same original applicability, no adoption | no promotion from ACK or absent family |
| disabled/active candidate | family10 required under current Plan | disable1, forwarding/force0, matching addrgen; proposed flags0 strict refusal if nonzero |
| cleanup | same retained applicability | uncertainty does not authorize wider deletion |
| closed | not parsed as owned link | exact original baseline historical comparison only |

Nonapplicable creation is parseable diagnostic evidence but cannot pass disabled
phase. No invented not-applicable disable transition. Flags0 post-disable remains
conservative proposed policy, not established universal behavior; exact driver/
config transitions remain review gate. Unknown flags never silently masked away.

## Concrete remaining source blockers before any integration

- Existing sysctl reader does NOT collect force_forwarding. This parser therefore
  refuses applicable count60 snapshots from that reader, correctly. An explicit
  separately reviewed read-only witness addition would be needed; no reads added.
- Generic Compose still rejects26; patch not applied. Positive strict raw active
  and ready display remain blocked, not manufactured with synthetic green.
- Untested/uncompiled helper has no caller. No positive tests added at this gate.
- Exact request claim, phase claim and extra sysctl witness must become owner-held
  evidence before use. No auth or kernel-version guarantee from structure.
- Full policy audit for other devconf/flags/cross-snapshot stability remains open.

## Decisive future test proposal, NOT run

Helper-only vectors anchored to FULL recorded shape: v6 count59/61 refuse, last
force word nonzero refuses, stats count words mismatch refuse, malformed and
unknown families/nested attr/length/padding refuse, optional RA-MTU zero refuses.
Creation disable0 accepted diagnostically; disabled phase0 refuses; family10
absent+ENOENT pre-phase accepts diagnostic only, never active; any directory
contradiction refuses. Source-derived valid synthetic profile is not OS proof.
Only after phase/witness review and separate Compose delta should a positive raw
ledger fixture and active invalidation table be introduced. No more status layers.
