# Google Play listing prep

Everything for the Play Console app entry, ready to paste. The console
steps themselves are Bidur's (account holder).

## On first upload: enroll in Play App Signing

Let Google generate and hold the real app-signing key. Our upload key
(mnet-upload) then only signs uploads and can be rotated through Play
Console support if it is ever lost. Do NOT opt out; without it a lost
upload key means a new app listing.

## App name

Mnet

## Short description (80 chars max)

Private mesh networking for your devices: one tap, one network, anywhere.

## Full description (4000 chars max)

Mnet connects your phones, laptops and servers into one private,
encrypted network, wherever they are. Devices get a stable mesh address
and reach each other directly - through NATs, firewalls and mobile
networks - with WireGuard encryption end to end.

- One-tap join: sign in, pick your network, done.
- WireGuard encrypted: modern, audited, minimal.
- Direct where possible, relayed where not: Mnet finds the fastest path.
- Exit nodes: route all traffic through a device you trust.
- Subnet routing: reach whole networks through one Mnet node.
- Remote sessions: view and control one Mnet device from another,
  right over the mesh. Screen, touch input, chat and file transfer,
  with your approval before anyone connects.
- SSO login: join with your existing identity provider.
- Your control plane: self-host the coordination server, keep your keys.

Mnet is provided by Fintex Australia Pty Ltd (ABN 60 688 406 108),
New South Wales, Australia. Contact: contact@fintex.au.

## Category

Tools (primary). 

## Content rating notes

No ads, no purchases, no UGC, no location sharing with other users.
VPN app: declare VpnService usage in the declaration section.

## Data safety (as of v0.5.1)

- Collects: email or SSO identity (for sign-in), device name (shown in
  the admin dashboard). Both encrypted in transit, not shared.
- Does not collect: traffic contents, browsing history, precise
  location, contacts, files.
- The control plane is operator-controlled; Fintex hosts the default at
  mnet-control.onrender.com.

## Store graphics (in store-assets/, produced 2026-11-15)

- icon-512.png: hi-res icon, export of drawable/ic_mnet at 512x512.
- feature-graphic-1024x500.png: feature graphic.
- shot-1-main.png, shot-2-vpn-consent.png: phone screenshots, real
  device captures from the Firebase Test Lab robo run (Pixel 9,
  1080x2160, 2:1 for Play).

## Release track

Internal testing first: upload android/out-aab/mnet.aab (release-signed,
targetSdk 34), add testers by email list, then promote.
