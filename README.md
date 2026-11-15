# Mnet

A development mesh VPN and remote-session project. Devices use WireGuard-encrypted direct UDP or an encrypted relay, with a self-hosted control plane for enrollment, approval, maps and access policies.

**Test builds only. Do not use Mnet for sensitive traffic or important file transfers yet.** It is not complete, independently audited, or a demonstrated replacement for Tailscale or a production remote-desktop service.

## Start here

- [Documentation index](docs/README.md): guides, design, evidence and history.
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

## Build and try locally

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

Addresses come from `100.64.0.0/10`. This range can conflict with an existing VPN or carrier network; check your environment. Magic DNS listens on the node's mesh IP and requires client resolver configuration.

## Remote sessions

Start one daemon with hosting enabled, rather than a second daemon sharing its state:

```sh
sudo ./mnetd -state /var/lib/mnet-test -session-host -session-auto-accept -session-allow laptop,phone
# After joining the host and viewer to the mesh:
./mnet --state /path/to/viewer-state session connect host-name
./mnet --state /path/to/viewer-state session send host-name ./test-file.zip
```

The headless host requires explicit auto-accept; the optional allowlist narrows verified peers. Viewer identity is checked against its source mesh address. The CLI is a terminal viewer, not a full native desktop app. See [session protocol and policies](docs/session-layer.md) and [reliability evidence](docs/session-reliability.md). Physical capture/input and mobile tests remain incomplete.

## Routing, authorization and expiry

Enrollment, approval, OIDC admission policy, network maps and pair-level ACLs are implemented. ACLs are not application-port rules. Known expiry timestamps are enforced locally in newer source; unseen offline admin revocations cannot be learned until reconnection. See [status](docs/STATUS.md) for what is actually shipped.

Exit-node and subnet-route fields and WireGuard allowed-IP configuration exist, but complete OS routing, safe route ownership and teardown are not verified. A desktop exit-selection guard is currently inverted and skips the intended setup. Do not rely on exit-node internet routing or subnet routing; the Linux kernel tests currently cover mesh addresses only. [Operations](docs/operations.md#exit-nodes-and-subnet-routes) tracks the limits.

## Validation

```sh
sh scripts/local-gates.sh           # uncached race tests, builds, vet, scans and mesh stress
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
