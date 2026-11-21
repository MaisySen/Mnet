# S29 unused owned AF_SPEC schema proposal

NO patch applied to Compose, NO compile/test/fixtures executed, NO device capture,
netlink request/write or other runtime effects. New helper is unused. S28 strict
contradiction oracle remains unchanged. Source proposal can be reviewed separately
from Transaction/status diff. A proposed integration patch is supplied, NOT applied.

## Exact grounding

Linux tag v6.12, not a claim about the runner kernel:
- https://raw.githubusercontent.com/torvalds/linux/v6.12/include/uapi/linux/if_link.h
  AF_SPEC family nested framing; IFLA_INET_CONF1; IPv6 flags1/conf2/cache5.
- https://raw.githubusercontent.com/torvalds/linux/v6.12/include/uapi/linux/ip.h
  33 IPv4 devconf words; forwarding enum1 maps emitted array index0.
- https://raw.githubusercontent.com/torvalds/linux/v6.12/include/uapi/linux/ipv6.h
  59 IPv6 s32 words; forwarding0, disable_ipv6 index26, addr_gen_mode47.
- https://raw.githubusercontent.com/torvalds/linux/v6.12/net/ipv4/devinet.c
  inet_fill_link_af emits IPV4_DEVCONF_MAX*4 bytes under CONF.
- https://raw.githubusercontent.com/torvalds/linux/v6.12/net/ipv6/addrconf.c
  ipv6_store_devconf zero-fills then writes indexed s32 values, omits stable_secret
  slot36. inet6_fill_ifla6_attrs emits flags4bytes/cache16bytes/conf59*4 then
  RETURNS EARLY under RTEXT_FILTER_SKIP_STATS. This old version therefore has no
  token/addrgen attributes in that selected profile. Full dumps differ.

Later kernels changed skip-stats behavior. Observed source change:
https://github.com/torvalds/linux/commit/7ac6ea4a3e0898db76aecccd68fb2c403eb7d24e
This makes a broad version-independent profile unsafe. This proposal deliberately
pins v6.12 skip-stats shape, refusing other shapes. Existing reader does NOT
request this profile. No active path becomes usable by adding this helper.

Existing decoder decodeAttributes refuses duplicate attrs, nested/net-byteorder
flag bits, nonzero padding/truncation/misalignment. The helper uses it at both
family and nested levels, accepts EXACTLY families2+10, IPv4 attr1 only, IPv6
attrs1/2/5 only. Known payload lengths are fixed, never divisible-by4 alone.
No bytes normalized or removed; original raw attributes remain in inventory.

## Validation table

| Item | Proposed rule |
| --- | --- |
| Family missing/unknown/repeated/malformed | refuse |
| IPv4 nested attrs | exactly CONF1, exactly132bytes |
| IPv6 nested attrs | exactly FLAGS1(4bytes)/CONF2(236bytes)/CACHE5(16bytes) |
| IPv6 FLAGS | zero only, deliberate narrow policy, not asserted universal |
| forwarding words + proc snapshots | zero, match both IPv4 and IPv6 |
| IPv6 disable word + proc |1, directory present |
| IPv6 addr-gen word + proc |0..3, identical decimal value |
| stable-secret omitted slot | zero as source fill requires |
| Other named devconf/cache words | retained verbatim, NOT claimed harmless or fully semantically validated |
| Different kernel/skip-stats shape | refuse pending separate review |

The code is structural validation plus named policy witnesses, not a proof of
every devconf semantic or exact running-kernel provenance. Flags-zero and the
version profile may reject legitimate dumps. That is an honest narrow failure,
not permission to expand until some live dump passes. No baseline attrs, codecs,
policy or recorded fixtures changed. Proposed Compose diff admits26 ONLY with
this helper, never blanket acceptance. No patch changes forwarding's generic
baseline validation. IPv6-disabled precondition means this profile is for final
active readback, not pre-disable creation; a phase-aware policy is still required
before using it for general Compose snapshots. This is a remaining design gate.

## Proposed decisive fixtures, NOT created or run

Synthetic v6.12-profile bytes plus explicit matching sysctl snapshot. Valid
profile could test helper framing, not raw active readiness. Mutate each nested
family/type/count/length/padding/flag/forwarding/disable/addrgen/zero slot; missing
IPv6 directory and sysctl mismatch must refuse. Preserve unrelated devconf/cache
bytes exactly. Full-dump token/stats must refuse this profile. A later reviewed
phase-aware proposal must separately handle pre-disable creation. Current S28
contradiction test remains red-gate evidence in source, not a test-run result.
