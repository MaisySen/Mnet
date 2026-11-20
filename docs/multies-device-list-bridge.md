# Experimental Multies device-list bridge

Local branch work only. No deployed endpoint or default origin is configured.
The normal installed home remains the local engineering home, not a finished
Multies setup journey.

The experimental command is:

```
mnet multies-signin --server TRUSTED_MULTIES_ORIGIN
mnet multies-open --server TRUSTED_MULTIES_ORIGIN
```

Use only a bridge origin you control and explicitly trust. The separate Multies
branch has its feature flag off by default. This command prepares a local daemon
key using the private local API, shows its thumbprint, opens the system browser,
and binds browser approval to a new native PKCE proof and loopback listener. It
can read only the signed-in person's own current approved-device records in the
chosen existing workspace. The command does not create a workspace, join a
network, start a tunnel, install a service, or start Remote Desktop/Shared Desk.

The credential stays in that command's memory and expires within 10 minutes. It
is not a browser session or MCP token. Membership, account, SSO and session state
are checked live. A browser revoke removes access. Device-list approval is not
proof of connectivity or possession of a WireGuard private key. Consequently
this bridge grants **list only**, never enrollment, view, control, file transfer
or recording. Actual network registration needs a separate key-proof contract.

The callback binds literal IPv4 loopback, a fresh ephemeral port, a fixed path,
state and one-use code. Wrong/duplicate callbacks do not consume the intended
attempt. Cancellation closes the listener. The native client follows no HTTP
redirects and rejects browser cookies and oversized or scope-changing responses.

The opt-in Linux X11 window offers a local Start, exact origin/key review,
explicit Open browser and a short-lived own-device snapshot. Closing any phase
joins its sign-in worker and closes its callback listener. Rows expire 30 seconds
from read start. The branded first-run surface uses embedded proportional Go fonts and a short
setup sequence, not a diagnostic transcript. It keeps the full origin and exact
key review visible and leaves the normal home unchanged. It is still a local
preview, not finished network setup.

## Current acceptance evidence

- Race tests cover transport, callback ownership/cancellation, fresh policy
  checks, credential redaction, and response rejection.
- Local daemon tests cover durable private identity creation, concurrent calls
  retaining one identity, no server/network/enrollment mutation, unauthorized
  and browser request refusal, invalid state and a closed daemon.
- A real local Go client plus the real separately gated Multies app and Chrome
  exercises exact-key approval, callback, exchange, list and browser revocation.
- Actual window checks cover closing before Start, before browser launch and
  while waiting for the callback. Initial close creates no identity; waiting
  close joins the worker and closes the listener.
- The signed-out browser path must return from real Multies login to this exact
  approval request, not drop into My Work. The separate Multies branch fixes
  that narrow return destination and tests its boundary.
- Third-party font/image notices accompany the installed binaries in README.txt.
- This is not production deployment, mobile callback
  acceptance, VPN enrollment, cross-machine connectivity, or input authority.

## Later system-browser boundary check

Exact installed local520fbdf binaries also complete the journey through the real
Linux xdg-open, gio and desktop association into full Chrome. A new disposable
home/browser profile and scoped session bus carry only the local fixture login;
no saved user browser profile or system default was changed. Native/browser keys
match, the signed-out login returns to the exact request, native rows expire and
browser revoke succeeds. Installation and unchanged-prefix removal pass.

The first actual-browser fixture timed out while Chrome covered the native
window. The automation's click missed the native target and its occluded native
images were black. It was not counted as completed acceptance. Raising/focusing
the native window before capture/actions and owning the session bus corrected
the fixture; the later full check passes without a source or deadline change.
This is local Linux association evidence, not physical desktop, other OS or
production bridge readiness.

## Credential destination and exact response contract

Local2efd762 binds each memory-only credential to the exact exchanging bridge
client. A recreated client or different origin cannot use or transmit it.
Failing-first real two-server tests originally observed two wrong-server
requests; the corrected path makes none. The actual normal command still uses
its original client throughout. No external disclosure was observed.

Locala06ed3f added recursive exact JSON schema validation, rejecting duplicate
fields, escaped duplicate names, unknown fields and case aliases before Go's
permissive decoder can choose a value. That checkpoint's installed native
journey **failed**: the original Evidence struct had relied on case-insensitive
camelCase decoding. The browser fixture incorrectly exited zero because it
checked approval and saved screenshots without confirming native success. The
actual pixels showed the error, so that run was not accepted.

Local73cd91d declares the actual Multies camelCase evidence fields and adds a
raw wire-schema regression. A corrected fixture also asserts native success
and expiry screen text. Exact installed73cd91d completes real system-browser
signed-out login, matching-key approval, native List, timed clearing and browser
revoke; the inspected native images show the correct states. Install and
unchanged-prefix uninstall pass. No UI redesign or capability change occurred.
Full uncached race, vet and Windows/Darwin CLI source builds pass. The separate
10-second JSON-schema fuzz check passes73,665executions. Other original failed
checkpoints are not repaired or replaced by these results.

The [registration design](multies-registration-boundary.md) documents the
independent operator mapping, person authority, key-possession, atomic
registration and mesh-revocation work still missing. Its real legacy-handler
checks confirm a List credential cannot become operator, account or saved-node
authority. The public legacy pending-join route still exists; a pending record
is not Multies enrollment or an approved network peer.
