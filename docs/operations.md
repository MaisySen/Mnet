# Running Mnet in production

## Control plane

Put mnet-control behind HTTPS with any reverse proxy (Caddy is simplest):

```
control.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

Then run it as a service:

```ini
# /etc/systemd/system/mnet-control.service
[Unit]
Description=Mnet control plane
After=network.target

[Service]
Environment=MNET_ADMIN_PASSWORD=choose-something-long
ExecStart=/usr/local/bin/mnet-control --listen 127.0.0.1:8080 \
  --base-url https://control.example.com --data /var/lib/mnet/control.json
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
```

The dashboard is at https://control.example.com/admin. The state file is
the only thing to back up.

## Nodes

Linux with systemd:

```ini
# /etc/systemd/system/mnetd.service
[Unit]
Description=Mnet node
After=network-online.target

[Service]
ExecStart=/usr/local/bin/mnetd --state /var/lib/mnet --up \
  --server https://control.example.com --network home
Restart=always

[Install]
WantedBy=multi-user.target
```

Join once with an auth key:

```bash
mnet --state /var/lib/mnet up --server https://control.example.com \
  --network home --auth-key mnet_...
```

or leave the key off and approve the device from the dashboard link the
CLI prints.

macOS: run mnetd with sudo or a LaunchDaemon. Windows: run mnetd.exe as
Administrator with wintun.dll next to it.

## Sizing

The relay carries only nodes that cannot find a direct path. Each relayed
node is one WebSocket; a small VM handles hundreds of relayed nodes.
Direct traffic never touches the control plane.

## Public deployment (validated 2026-11-15)

mnet-control is one static binary with one state file. On any public host:

    mnet-control -listen :443 \
        -base-url https://control.example.com \
        -data /var/lib/mnet/control.json

Put it behind any TLS-terminating reverse proxy (Caddy, nginx) or give it
a public port; the node API, admin dashboard, and WebSocket relay share
the one listener. Set the admin password with MNET_ADMIN_PASSWORD or
-admin-password, then create networks and auth keys in the dashboard at
https://control.example.com/admin.

Smoke tested end to end through a free pinggy tunnel
(ssh -p 443 -R0:127.0.0.1:18080 a.pinggy.io): two mnetd processes joined
over the public https URL and exchanged WireGuard traffic both ways in
about 7 seconds. scripts/public-validation.sh reproduces this.

Nodes join with:

    mnetd -up -server https://control.example.com -network mynet -auth-key mnet_...

## STUN and restrictive networks

Endpoint discovery sends STUN binding requests from the WireGuard socket
itself, so the discovered mapping is the one WireGuard can use. On
networks that filter outbound UDP (some sandboxes and corp networks),
STUN simply finds nothing; the daemon falls back to relay-only operation
and keeps retrying on a 45 s cycle. No configuration is needed either
way.

## Android

android/build.sh builds android/out/mnet.apk without Gradle: gomobile
for the Go core (all four ABIs), then aapt2 + javac + d8 + zipalign +
apksigner directly. It needs ANDROID_HOME and ANDROID_NDK_HOME set,
JDK 17, and gomobile + gobind in PATH. The debug signing key is
android/debug.keystore (regenerate for real distribution).

## iOS

See ios/README.md: sources and the gomobile build script are complete;
the xcframework build, signing and on-device testing need macOS, Xcode
and an Apple Developer account.
