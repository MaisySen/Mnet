# S32 DESIGN ONLY: owned-link AF_SPEC and phase contract

No code, reader request, Compose patch, test update/run or OS effect. The S31
one-line oracle delta is separate. S29 helper remains unused reference-only,
NOT a candidate for integrating the current reader. Its skip-stats/disabled-only
profile cannot cover creation or the current full dump. Prefer retire it from
future integration proposals rather than accrete silent compatibility exceptions.

## Source anchors and what is actually established

Current DumpRequestHeader(18) in netlink_codec.go returns sixteen zero bytes.
ReadStableInventory/readOne passes that header to the dump entry point; no
IFLA_EXT_MASK/RTEXT_FILTER_SKIP_STATS is supplied. Existing fixture uses a FULL
dump shape. A profile must record these exact request bytes/flags and whether a
mask was present, not infer request semantics from whichever reply passes.

Read-only offline decoding of recorded link-*.bin shows IPv4 CONF132 bytes and
IPv6 CONF240 bytes on the applicable recorded links. No recorded IPv4 CONF136
was found in this fixture, contrary to the relayed shorthand. This discrepancy
must be resolved by naming another exact fixture if136 is intended. The current
fixture's IPv6 siblings: flags4/cache16/stats304/icmpstats56/token16/addrgen1;
RA-MTU absent there. This is recorded baseline evidence, NOT owned-TUN evidence.
Different counts are distinct shapes, never rounded/truncated to a known prefix.

Pinned UAPI grounding used in S29:
https://raw.githubusercontent.com/torvalds/linux/v6.12/include/uapi/linux/if_link.h
https://raw.githubusercontent.com/torvalds/linux/v6.12/include/uapi/linux/ip.h
https://raw.githubusercontent.com/torvalds/linux/v6.12/include/uapi/linux/ipv6.h
https://raw.githubusercontent.com/torvalds/linux/v6.12/net/ipv4/devinet.c
https://raw.githubusercontent.com/torvalds/linux/v6.12/net/ipv6/addrconf.c
The v6.12 enum/source supports IPv4 count33, IPv6 count59, not the recorded IPv6
count60. Therefore it cannot identify the last recorded word. A proposed profile
for count60 must independently pin the exact matching UAPI/emitters and semantics
before implementation. Shape alone is not kernel-version proof.

## Proposed data ownership, no implementation

Owner snapshot would pair raw inventory/sysctls with an immutable request-profile
record: link request bytes, dump flags/ext-mask, family inclusion policy, selected
reviewed schema identifier, and captured provenance. Existing baseline bytes stay
immutable. The schema identifier describes an audited shape, never a guessed
kernel release. Unknown conf counts/attrs/family flags fail with a typed reason.
No schema fallback based on a failed parse. No normalization of volatile stats,
cache, devconf words or unknown attrs. Original bytes remain available for audit.

A phase witness must come from the canonical ledger's acknowledged/readback
transitions, not a caller-selected post-disable boolean. Allowed phases:

| Phase | Applicability and directory contract | IPv6 setting witness |
| --- | --- | --- |
| created, untagged | family2 required; family10 presence explicitly matched to directory present/ENOENT; known nested profile required | applicable directory leaves required; disable may be0 or1, captured as observed, not 'safe active' |
| tagged, pre-disable | same exact original handle/namespace and reviewed shape; no adoption | cannot treat0 as completed disable |
| disable call ACK, readback pending | same identity, raw/sysctl comparison still mandatory | no promotion from ACK alone; absence of family10 is NOT proof disable succeeded |
| disable confirmed, addressed/up/route | require reviewed family10/directory behavior supported by chosen profile | disable word/proc1, forwarding word/proc0, addrgen same valid0..3; no IPv6 addresses/routes |
| cleanup/ambiguous | preserve previously established applicability and observed changes; unknown/replacement refuses | no forward promotion or broad deletion; close only original proven custody |
| original close confirmed | original full raw baseline/rules/sysctls equality | historical absence only, never active readiness |

For a TUN whose creation has no family10/directory, do NOT silently skip disable.
Current Plan requires a disable-owned-link-ipv6 step. Either prove that step's
supported not-applicable semantics from exact driver/kernel source and review a
separate explicit phase transition, or keep that configuration unsupported. No
such proof or transition exists yet. Kernel build/config variability is not an
excuse to turn absence into completion.

## Nested schemas and flags

Both family and family-specific attrs use the strict existing decoder, preserving
duplicate/padding/length/unknown-family refusal. Family2 CONF requires an audited
exact word count, native signed32 layout and named policy indices. Family10 FULL
profile requires individually reviewed flags/conf/cache/stats/icmpstats/token/
addrgen/optional-RA-MTU shapes for that profile, not 'any multiple of4/8'.
Known-but-unmodeled values are retained, not automatically considered harmless.
A separate semantic decision is needed for every value that can affect ownership
or implicit networking. Counters/cache remain observations, not policy switches.

IPv6 if_flags is NOT a tun's generic IFF flags and zero is not universal. The
v6.12 emitter returns idev->if_flags. addrconf_ifdown clears IF_RS_SENT/IF_RA_RCVD/
IF_READY on its non-unregister path; other paths set IF_READY or IF_RS_SENT.
This supports phase sensitivity, not a universal zero rule. Before a mask policy,
pin exact definitions and transition paths for the selected source/profile.
Unknown bits refuse; known bits are admitted only with documented phase meaning.
No flags-zero policy should be inherited from S29 as established kernel truth.

## Smallest next source gate

1. Review this request/phase model and resolve the exact source for recorded
   count60, plus any claimed IPv4 count34 fixture. No new capture needed.
2. Define unused typed profile parser and structured phase-validation result,
   with exact full-profile synthetic vectors anchored to source and preserved
   recorded baseline fixtures. Keep Compose integration separate.
3. Review valid/invalid vectors: pre-disable0 vs post-disable1; family10 absent
   with ENOENT vs directory contradiction; exact conf counts; flags per phase;
   unknown family/nested attr/count; malformed/duplicate/padding; mismatched raw
   policy words/sysctl; replacement handle. No active readiness test yet.
4. Only after profile/phase contract and parser review, apply a small Compose
   integration delta, then add positive strict raw-ledger test and invalidation
   table. Fake positive proves parser/model integration, NOT OS/TUN ownership.

Connected product blocker remains real owned device/namespace/FD custody and
usable supported deployment. This design resolves source contradiction without
pretending an optional status display is that product. No further status layers
or runtime self-checks are proposed here. No full archives needed for this note.
