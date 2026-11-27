# Mnet

A development companion for Multies, intended to let eligible Multies users connect their devices and start approved remote sessions. The intended setup is install, sign in through Multies, choose an eligible workspace and approve the relevant OS permissions. No separate Mnet signup or guest service, and no server, network or enrollment-key chores in the consumer journey.

**That integrated journey is not implemented yet.** Current engineering builds still use a mesh VPN and remote-session stack with manual enrollment. Devices use WireGuard-encrypted direct UDP or an encrypted relay. Existing test enrollment/account routes are not production Multies-only enforcement. The control-plane/server instructions below are developer tooling, not the intended consumer setup.

**Test builds only. Do not use Mnet for sensitive traffic or important file transfers yet.** It is not complete, independently audited, or a demonstrated replacement for Tailscale or a production remote-desktop service.

## Three distinct tools in development

The current product direction separates Mnet private networking, Remote Desktop screen viewing/control, and Synergy-like Shared Desk keyboard/mouse sharing. Current source includes distinct engineering paths for all three, not one consent switch. See the [three-tool map](docs/three-tools.md) for what is implemented, what remains and why network membership or view approval does not grant raw input. Multies integration and the installed consumer experience remain unfinished.

## Start here

- [Documentation index](docs/README.md): guides, design, evidence and history.
- [Consumer scope and release gates](docs/product-scope.md): intended journey versus current capability.
- [Project status](docs/STATUS.md): source versus published builds, verified checks and open work.
- [Latest published test build](https://github.com/MaisySen/Mnet/releases/tag/v0.8.2-test.1): update the CLI and daemon together.
- [Operations](docs/operations.md): configuration, credentials, enrollment and security limits.

## Components

| Component | Purpose |
| --- | --- |
| `mnet-control` | Node registry, approvals, enrollment keys, maps, OIDC, admin dashboard and encrypted-packet relay. |
| `mnetd` | WireGuard device, endpoint discovery, direct/relay selection, Magic DNS and optional session host. |
| `mnet` | Local authenticated daemon controls and session CLI. |
| `mobile`, `android`, `ios` | Shared Go engine, Android app and iOS source integration. Platform readiness differs. |

The control-plane operator sees device identities, addresses, endpoints and topology. A hosting operator can also observe that metadata. The relay sees encrypted WireGuard packets and traffic metadata, not their plaintext. Sessions may use the relay too. A compromised endpoint or control plane is outside a promise of absolute privacy.

## Build and try locally (engineering only)

Use the Go version pinned in [go.mod](go.mod), currently Go 1.26.11. Linux needs `iproute2` and access to `/dev/net/tun`; macOS needs a supported utun interface; Windows needs `wintun.dll` beside the daemon. Interface setup requires root or the appropriate administrative capabilities. Native build/CI does not imply every platform's VPN setup has passed.

```sh
go build -o mnet-control ./cmd/mnet-control
go build -o mnetd ./cmd/mnetd
go build -o mnet ./cmd/mnet

# Loopback-only test control plane. A password is generated if unset.
./mnet-control -listen 127.0.0.1:8080 -base-url http://127.0.0.1:8080
```

Open the dashboard at `http://127.0.0.1:8080/admin`, create a test network, then start a daemon on the same machine:

```sh
sudo ./mnetd -state /var/lib/mnet-test
# In another elevated terminal:
sudo ./mnet --state /var/lib/mnet-test up --server http://127.0.0.1:8080 --network home
sudo ./mnet --state /var/lib/mnet-test status
```

Approve the pending device in the dashboard. For multiple machines, use a reachable control-plane URL behind HTTPS, not loopback; see [operations](docs/operations.md). Enrollment keys are secrets. Do not copy them into documentation or service commands. Use a dedicated private state directory consistently for both CLI and daemon.

Addresses come from `100.64.0.0/10`. This range can conflict with an existing VPN or carrier network; check your environment. Desktop Magic DNS uses a mesh-IP listener and requires resolver configuration. Mobile source uses a routed userspace DNS endpoint instead; Android default-network reliability is still a release blocker. See [status](docs/STATUS.md).

## Remote sessions

Start one daemon with hosting enabled, rather than a second daemon sharing its state:

```sh
sudo ./mnetd -state /var/lib/mnet-test -session-host -session-auto-accept -session-allow laptop,phone
# After joining the host and viewer to the mesh:
./mnet --state /path/to/viewer-state session connect host-name
./mnet --state /path/to/viewer-state session send host-name ./test-file.zip
```

The headless host requires explicit auto-accept; the optional allowlist narrows verified peers. Viewer identity is checked against its source mesh address. Those legacy commands remain engineering paths. Separate attended Linux X11 `session view` and `session point` entries now open actual native windows, with host-local view approval and a separate pointer-only request in the latter. They are not a full installed cross-platform desktop app. See the [native viewer](docs/desktop-native-viewer.md) and [pointer entry](docs/desktop-pointer-viewer.md). See [session protocol and policies](docs/session-layer.md) and [reliability evidence](docs/session-reliability.md). Physical capture/input and mobile tests remain incomplete.

## Routing, authorization and expiry

Enrollment, approval, OIDC admission policy, network maps and pair-level ACLs are implemented. ACLs are not application-port rules. Known expiry timestamps are enforced locally in newer source; unseen offline admin revocations cannot be learned until reconnection. See [status](docs/STATUS.md) for what is actually shipped.

The daemon rejects exit-node selection and advertisement before enrollment or interface creation. Managed Linux supports a bounded RFC1918 IPv4 subnet-routing path. Kernel fixtures exercise separate-LAN TCP, route withdrawal and preservation of unrelated routes. Other platforms and full-tunnel internet routing remain unsupported. [Operations](docs/operations.md#exit-nodes-and-subnet-routes) tracks the limits.

## Validation

```sh
sh scripts/local-gates.sh           # uncached race tests, builds, vet, scans, fuzzing and mesh stress
sh scripts/session-validation.sh    # isolated X11 userspace proof, not a physical desktop
```

Native Windows/macOS CI covers private state, local API, expiry and loopback sessions. Windows also has a disposable SCM lifecycle check. Linux CI uses actual kernel TUN interfaces and OS TCP with controlled loss, including a direct-path check with the control/relay process stopped. [Native validation](docs/native-platform-validation.md) defines exactly what these checks prove and what remains untested. [Scale measurements](docs/scale-testing.md) are synthetic engineering measurements, not supported fleet capacity.

## Repository layout

- `cmd/`: control server, daemon and CLI.
- `internal/`: engine, WireGuard transport, control, relay, DNS, local API, private storage and sessions.
- `internal/proofnet/`: attributed proof-only userspace network adapter, not the desktop/mobile OS TCP stack.
- `mobile/`, `android/`, `ios/`: mobile bindings and platform sources.
- `scripts/`: build and disposable validation fixtures.
- `docs/`: maintained guides, status, measurement evidence and historical release notes.

Mnet is provided by Fintex Australia Pty Ltd (ABN 60 688 406 108), New South Wales, Australia. Contact: contact@fintex.au.
