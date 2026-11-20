# Experimental Multies device-list bridge

Local branch work only. No deployed endpoint or default origin is configured.
The normal installed home remains the local engineering home, not a finished
Multies setup journey.

The experimental command is:

```
mnet multies-signin --server TRUSTED_MULTIES_ORIGIN
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

## Current acceptance evidence

- Race tests cover transport, callback ownership/cancellation, fresh policy
  checks, credential redaction, and response rejection.
- Local daemon tests cover durable private identity creation, concurrent calls
  retaining one identity, no server/network/enrollment mutation, unauthorized
  and browser request refusal, invalid state and a closed daemon.
- A real local Go client plus the real separately gated Multies app and Chrome
  exercises exact-key approval, callback, exchange, list and browser revocation.
- This is not installed GUI acceptance, production deployment, mobile callback
  acceptance, VPN enrollment, cross-machine connectivity, or input authority.
