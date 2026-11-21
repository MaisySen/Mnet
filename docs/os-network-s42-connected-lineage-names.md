# S42 connected lineage hooks, source diagnostic reader and observed feature names

Source-only UNRUN. S41 independent177 green remains fake/model evidence. Author
ran no compile/test/vet/race, helper, actual FD/netns/ioctl/ethtool/sysfs/netlink
read/write or OS process. Existing kernelExecutionReleased=false remains unchanged.
No active/readiness conversion, real Kernel implementation or peer authentication.

## Connected lineage, not merely another constructor

The original retained-resource constructor now attaches its private ledger directly
to the original *fileCustody, refusing already attached or previously closed
custody. fileCustody tracks Close attempt even before attachment. Its once-only
Close sets closed and invalidates attached lineage BEFORE original file.Close;
close still runs on invalidation error, and both errors stay sticky. A later
resource constructor cannot bind evidence to an earlier closed custody object.

ownedCreation.beforeEffect validates exact original creation handle fields (tag
confirmation may advance) and requires this attached original ledger. It then
invalidates. Missing lineage, wrong handle or closed object prevents effects.
guardedEffect calls that guard before entering the effect callback; success,
error or ambiguous effect cannot restore evidence. Fake recorder tests check
invalidation/generation change before callback entry and after both return paths.

Real source paths are now connected:

- residentNetlink.applyExact requires original ownedCreation, wraps socket creation
  and ApplyExact inside guardedEffect. It cannot configure a name-only target.
- routeSocket.Exchange checks mutation message kinds and requires the owned
  creation's beforeEffect again immediately before Sendto. Read-only dump sockets
  have no creation and cannot send those mutation kinds, even if their caller
  bypasses the higher wrapper. Both hooks are intentional: an earlier failed
  open invalidates, and no stale pass survives the final send boundary either.
- disableOwnedIPv6 now requires ownedCreation and exact expected Link, invalidates
  before opening the sysctl path, still namespace-checks and verifies the link,
  and only writes the existing exact owned disable_ipv6 leaf. No global writes.
- fileCustody.Close covers every existing ownedCreation close through that object.
- No post-creation real owned TUN ioctl, dup or export helper exists in the audited
  source. A control/duplicate/export request through the supplied refusal seam
  invalidates before returning unreleased, never exposes a raw descriptor.
- The initial TUNSETIFF is still birth setup before an ownedCreation exists. No
  new offload ioctl, persistence, attach-existing or duplicate/export effect added.
  Birth setup already fails closed/cleans its original FD before a handle return.

Read-only existing residentNetlink positional constructors are mechanically changed
to keyed ns: fields after adding creation. Inert stubs mirror signatures and still
return unreleased. Tests inspect source ordering without invoking these helpers.

Important limitation: config intent is deliberately terminal uncertainty in this
conservative slice, even if a later ACK succeeds. Thus configured ready packet
lineage is NOT yet reachable; there is no hidden automatic re-attestation after
legitimate alias/MTU/address/up/route setup. A future reviewed exact-operation
ACK+raw-readback transition must mint a fresh generation for known allowed setup
while preserving original FD identity/offload history. This slice proves all
existing effect paths invalidate old evidence, not that the product can activate.
Untracked access or new helpers can still bypass source conventions; no hostile
root/containment claim. Single-owner serialization contract, no concurrent actions.

## Actual source reader connection

readPolicySysctlsSource now collects every33 ALL and DEFAULT IPv4 leaf from the
correct pinned table, records force_forwarding along with existing IPv6 operands,
parses canonical signed effective operands and applies the scoped nested TUN
framing decoder to dedicated mno- links. The existing actual bounded relative
O_NOFOLLOW reader performs these steps if a future release allows it; NO call was
executed. Missing optional/config-dependent leaves fail instead of being invented.

An existing-tag-separated collectSourceDiagnostic method connects ReadStableInventory
and readPolicySysctlsSource, under held namespace checks and unchanged unreleased
execution guard, then shared pure sourceDiagnosticFromRecords. It selects the
exact owned hardware/core flags/index/name/alias/MTU link, applies scoped TUN
TYPE/PI/VNET/PERSIST/MULTI_QUEUE decode and computes the actual observed ALL/local
expressions. Returns immutable raw/sys diagnostics and explicit lineage usability,
never Inventory or Ready. Inert counterpart always returns unreleased.

This connects the source path to the SAME parsers as fake wire, not a second
policy transcription. Sequential collection is not atomic. TUN FD identity/netns
query, actual ethtool transport/dynamic family resolution and authenticated peer
MTU are not in this source method. A true held-FD supplier is still not implemented.
The retained pointer token alone remains correlation until original kernel checks,
all caller discipline and allowed transition/readback lifecycle are established.

## STRSET observed names/index verification

Primary sources inspected, pinned Linux v6.17:

https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ethtool/strset.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/ethtool.h
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/uapi/linux/ethtool_netlink_generated.h
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ethtool/common.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/linux/netdev_features.h
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ethtool/features.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/ethtool/netlink.h

ETH_SS_FEATURES=4 from ethtool.h689..699. strset.c info_template maps it to the
non-per-device netdev_features_strings/count. Explicit requested ID4 can be queried
with an indexed device header, whose reply correlates the same device. The table
itself is global feature names, not hardware/WANTED or a per-device capability list.
strset_fill_set emits ID1/COUNT2/STRINGS3 nested; each repeated STRING1 contains
INDEX1/VALUE2. strset_fill_string and ethnl_put_strz guarantee null-terminated
strings, including empty reserved slots. No assumption that repeated STRING attrs
are duplicate singletons. This parser has a separate bounded repeated-list decoder.

New read-only STRSET_GET1/version1 encoder explicitly requests FEATURES ID4, not
all stringsets or counts-only. Reply binds kernel sender/no truncation, <=4096,
exact netlink frame/family/sequence/recipient, command1/version1 and ifindex/name.
Requires one stringset ID4/count64, exactly64 entries with unique in-range indexes,
correct nested schema, bounded<=33-byte terminated strings, no hidden interior NUL,
no missing/unknown/duplicate fields. Repeated indexes refuse even if count stays64.

The reference64 index/name array was GENERATED by parsing the actual pinned
netdev_features.h enum and common.c designated string initializers, preserving
empty reserved2/12/13. It was not typed from remembered bit numbers. Runtime
STRSET must match every observed name AT ITS OBSERVED INDEX. Same-size swapped
checksum/software-GSO names fail. A profile label/version1/count64 is no longer
sufficient to classify a reply. Unknown/new/renamed indexes fail closed as an
unsupported profile, not default-zero or 'no offload'. No actual STRSET read was
performed; only fake reply parsing implements this source protocol now.

heldPacketReader queries/verifies STRSET first, then FEATURES on the next sequence
in the same original-identity-bracketed collection. Both replies bind the same
dynamic family/recipient/device, with separate expected commands/sequences. The
result includes verified names and name-based compatibility, not only numeric
rawOutputCompatible. The old numeric helper remains an independently tested
pinned diagnostic; connected reader classification uses observed verified names.

## Fresh active features do not mean all bits zero

Sources:
https://raw.githubusercontent.com/torvalds/linux/v6.17/drivers/net/tun.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/net/core/dev.c
https://raw.githubusercontent.com/torvalds/linux/v6.17/include/linux/netdev_features.h

Fresh tun_net_init advertises SG, FRAGLIST, VLAN_CTAG_TX, VLAN_STAG_TX plus
TUN_USER_FEATURES. Fresh private set_features zero masks the user subset at the
registration fix-features step. Core dev.c registration adds software GSO/GRO via
NETIF_F_SOFT_FEATURES, exactly GSO11|GRO14 in netdev_features.h239. They are not
TSO or permission for raw userspace to receive an unmarked partial checksum.

Name classifier accepts fresh active tx-scatter-gather, tx-scatter-gather-fraglist,
tx-vlan-hw-insert, tx-vlan-stag-hw-insert, tx-generic-segmentation, rx-gro (and
benign highdma). Anything else active is explicitly incompatible for this v1 raw
contract. The fresh exact source mask has a positive fake vector, so fail-closed
does not silently implement blanket rejection of every ordinary fresh TUN.
Checksum and protocol segmentation names, renamed/new active features or unverified
names fail. Hardware and wanted may still advertise checksum/GSO without claiming
they are active. No TUNSETOFFLOAD(0) is added or demonstrated necessary here.
S39 conservative mandatory-zero-call fake admission is NOT silently relaxed or
made a real successful state. A future reviewed replacement can use this actual
observed-name classifier and continuous original history, with no new ioctl.

## Unrun test coverage and open outcomes

Fake before-effect recorder success/failure; wrong handle/missing lineage/control
escape refusal; source ordering checks for ApplyExact/disable/Close and shared
reader wiring; STRSET full names, swapped same64 semantics, reserved now named,
unique indexes/truncation/wrong device/sender/family/sequence/command/version/count/
unterminated/schema limits; actual fresh active-name positive mask and unsupported
active names; source diagnostic exact owned record/ALL computation/raw immutable
copy. Existing held-reader tests now emit STRSET then FEATURES, still fake only.

Independent source gate before once-only contained tests. Actual held FD/netns
read/ioctl execution, ethtool transport/resolver, generation ACK+readback recovery,
complete raw strict Compose, full IPv6 consumer review, authenticated peer MTU and
host routing/firewall/XFRM/CGNAT/installed app path remain genuine separate gates.
No device retry/access workaround/helper execution released. No shared Mux edits,
no application payload rewrites, no host-global policy changes.
