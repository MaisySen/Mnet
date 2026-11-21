# Actual daemon peer chooser checkpoint

Run from a Linux checkout with Go, Python/Pillow, Xvfb, xdotool and ImageMagick:

```
python3 scripts/daemon-peer-chooser-validation.py
```

This builds race-enabled mnet, mnetd and mnet-control. It starts one private loopback control service with newly created disposable network/auth key, plus two actual mnetd processes on separate fresh owned State directories. Each uses explicit validation-only ChannelTUN. No external account, saved network, host route, installed service or real interface is touched. Control/admin secret, daemon credentials and local capability are freshly generated fixture-only data; do not reuse them. Temporary artifacts persist for inspection and contain fixture-only credentials, never upload whole State directories as user evidence.

Actual daemon local API Up obtains its key and node identity through real control Join/map. The native chooser reads actual localapi/status and session-peers. Harness compares its sole eligible peer's exact full key with the other actual daemon's saved public key. It selects the row and opens details, then actual localapi/down disposes the viewer daemon's device. Manual Refresh yields unavailable, clears disclosed key and selection, and View only/Pointer view clicks remain inactive. Actual fresh mnetd process reopens the same identity with a fresh ChannelTUN, performs saved-identity Up, and manual Refresh restores the peer without restoring selection/details. Exact durable State bytes stay unchanged. Escape exits normally; all subprocesses and disposable X server are joined/terminated.

ChannelTUN injection is one-shot by design. Re-Up in the SAME process after Down consumes no new injected TUN and falls through to real kernel TUN creation, which correctly fails in an unprivileged environment. The final harness deliberately uses a fresh process for recovery, not an implicit fake device reinstallation or a production change. Initial harness failures are retained: healthz returns plain text rather than JSON; and same-process re-Up exposed this one-shot validation lifecycle. Neither is evidence of a production daemon regression.

Pixels inspected: ready with actual host-state/100.64.0.2, selected exact-key details, down with empty rows/disabled modes, fresh-process recovery with no selection. Labels and controls readable, no observed clipping in these four states. This is same-host X11/local-control acceptance, not physical display/native OS acceptance.

## What it does not prove

No viewer session is requested. Native viewer OS net.Dial must reach meshIP:5909; ChannelTUN does not install a host OS TCP route. Full actual-daemon attended view/pointer interoperability remains untested here. Current attended X11 harness exercises actual built viewer, real host code, local prompts/capture/input/retirement but uses a synthetic local status/session-peers API/current callback. Current userspace session-proof uses actual daemons but bypasses native chooser/OS dialing. Combining their successes does not close the missing path.

Full remaining run needs a disposable privileged kernel-TUN environment: actual local control, two actual daemon identities in isolated network namespaces, OS mesh routes/TCP, actual chooser-derived eligible peer, then independent host view approval, first-screen pixels, separate pointer approval, local/host stop and map revocation cleanup. Never substitute loopback peer addresses or relax source-address/full-key checks to make this pass. Here TUN is root-only, no effective capabilities/sudo, private user/net namespace device creation refuses, and ip/tc/iptables are absent. No capability escalation or host setup is attempted.

No later RD UI/harness commit through43d63ee is included. Current integrated RD remains exact reviewed36d48a9 patch only; this change adds a validation script and this document, no product surface.
