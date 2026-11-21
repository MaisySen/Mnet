# S13 existing-device preflight source, NO RUN

Tags: linux + mnet_enrollment_development + mnet_os_readonly_evidence +
mnet_os_device_preflight. No kernelExecutionReleased bypass. TUN-create helper
still refuses immediately. Existing four bootstrap dispatch modes unchanged;
new work function deliberately has NO executable dispatch/caller. Build/tests
are not device access, namespace launch or permission. No device metadata was
collected from this runner or user machine in this stage.

Owner-side source accepts held isolated namespace custody, validates pinned
thread/current namespace before work/open/after open/after close. Captures raw
read-only inventory+sysctl baseline before/after; compares same-run hash only.
Volatile fields can safely false-fail. No cross-run equivalence/normalization.
Snapshots use already-reviewed bounded read-only transport, no netlink writes.

Device source lstat exact /dev/net/tun, refuses symlink/nonchar/wrong10,200 before
open; O_RDWR|NONBLOCK|CLOEXEC|NOFOLLOW, never create/truncate. Opened-FD fstat must
match full original dev/inode/rdev/mode/uid/gid, requires char10,200, F_GETFD
CLOEXEC and F_GETFL NONBLOCK. Single deferred close originalFD even readback/
namespace/flag failure, never retries a failed close. Captures numeric FD,
close attempted/success, before/after FD inventories (exact equality) and
namespace recheck. Missing/denied metadata or open is valid feasibility outcome,
not permission to bypass. Unclassified error/identity race/close failure/changed
baseline or FD inventory remains error. Device mismatch is refused-object, not
an attempted fallback. No ioctl, create/change, mknod/chmod/modprobe/sudo/setcap,
privileged helper, installed path, traffic or user-machine access.

Boundary: successful open only proves node access here at that instant, NOT
TUNSETIFF permission, complete Kernel adapter, user deployment or real network.
Pre-open metadata cannot prove future access; lstat/fstat race mismatch refused.
The actual live blocking-bound/host device-driver behavior remains a separate
review issue, not solved by fake tests or NONBLOCK alone.

No source for a runnable preflight binary/outer dispatch is added yet. Before
execution recommendation a separately reviewed entry must route ONLY the new
owner work, serialize its evidence, verify outer namespace before/after and
resident exact lifetime/reap/custody, and bind this to the reviewed process
wrapper. Never repurpose --bootstrap-readonly to perform an implicit open.
The owner raw-baseline fields here do not pretend to capture outer network
inventory; existing outer namespace identity oracle is a separate responsibility.

Wrapper source library now adds Tgid/cmdline/per-read monotonic start/end and
collection bounds, bracketed stat lifetime, explicit error records. Does not
launch/signal/open-device on import, not a runnable wrapper. Task-leader children
only and non-atomic exec identity still explicit. Only fixed no-secret command
modes may be observed; no arbitrary-process cmdline harvesting. Three offline
fixture tests pass. Driver/resident first-StartProcess implicit stdlib effects
listed: self pidfd open/waitid ECHILD/self-signal0, transient0x5100 (already
includes PIDFD) clone child exit0/reap/close. Fresh process Once each. Fixed
client stays nonspawning/filter unchanged.937 remains source-fit only, no new
attribution. Lifecycle comment clarifies Wait reaping vs helper custody N/A and
stdlib-managed pidfd possibility/ignored close return; no schema rewrite.

Nine uncached compile-valid mutants killed: ioctl insertion, CLOEXEC bypass,
opened identity bypass, namespace skip, close omission, wrongmajor/minor,
NOFOLLOW removal, NONBLOCK bypass. Fake-only package race and selected tagged
race, vet/build. No live preflight, namespace/filter/probe, TUN or mutation.
Next source gate is exact runnable outer-owner routing+bounded wrapper review,
not automatic execution. Kernel adapter work remains inert/independent.
