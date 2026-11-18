# Controlled minimal VPN baseline

This isolated engineering app is not Mnet and is never built into an Mnet APK.
It is a single differential investigation, not a new readiness gate or release.

The baseline uses the same target36, API34/36 google_apis x86_64 image family,
MTU1280, 100.64/10 route, 198.18.0.53 resolver route and VpnService consent flow.
It preserves the original owner socket's explicit VPN/source binding and the
separate app UID's unbound UDP behavior, with a five-second receive timeout.
The owner and independent UID each run an original UDP query and OS name lookup
at initial connection and reconnect. One owner fresh-connected contrast follows
an original failure, never replacing that failure.

Instead of the daemon/WireGuard/gVisor/control plane, one Java reader directly
answers IPv4 UDP DNS packets from the VpnService descriptor. It keeps the default
nonblocking descriptor and polls it, and computes IP and UDP checksums. A pure
JVM packet regression checks tuple, lengths, DNS answer, checksums and truncation
before the APKs are made. The baseline has no peer, credentials, external server,
root, trace hooks, appops changes, permission grants or security-policy changes.
The owner chooses the same bounded active-VPN/address publication predicate as
the existing original fixture. Service teardown waits for the worker to exit.

Differences deliberately remain: fixed synthetic mesh/peer addresses, different
package/UID numbers, no enrollment/startup workload, direct serialized UDP-only
responder, no encrypted peer transfer or TCP DNS server. OS lookup can depend on
its cache and transport choice. Owner and independent probes run at both phases,
whereas the original Mnet independent app ran after reconnect. These differences
bound the inference.

Interpretation:

- A similar permission denial or first-query timeout with this direct responder
  shows that this symptom can occur without Mnet's engine. It does not by itself
  identify the Android policy or lifecycle decision causing it.
- Baseline success cannot establish an Mnet cause or upgrade Mnet readiness.
- An orchestration/build/consent failure is inconclusive, not a networking result.
- No new original Mnet run is part of the baseline workflow. Original failures
  remain recorded separately, including independent UID, DNS and bulk gates.

Only reconstructed fixed-category JSON is retained. No raw logs, APK, state,
packet bytes or addresses are uploaded. If the result cannot discriminate, stop
and report that limit rather than extend the experiment into a patch sequence.
