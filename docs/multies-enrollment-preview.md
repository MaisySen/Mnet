# Native entry to separate joining consent

`mnet multies-enroll-preview --server TRUSTED_MULTIES_ORIGIN` is an explicit
engineering entry to the actual Multies joining-consent page. It is separate
from the List command, is not normal home setup, and claims no enrollment result.

It prepares the daemon's existing durable public identity using the private
local API, validates the exact response shape/canonical full public key, prints
that full key and opens the configured origin's `/mnet/enroll` in the system
browser with a new random attempt and platform. Unlike the List handoff this is
the actual full public key, not its SHA256 thumbprint. Missing/ambiguous identity,
cancellation or invalid origin refuses before browser opening. No shell is used
for the browser URL. The explicit local development origin convention matches
the existing List bridge; production remote origins require HTTPS.

There is no callback listener, enrollment ticket, key proof, node API, saved
Multies credential, tunnel, view or input operation. Browser opening is reported
as opening only, never approval or network success. The attempt is ephemeral,
not a crash-recoverable registration record. Restart requires another explicit
preview; this must not be extended into registrar recovery by inference.

The separately gated Multies consent branch displays the same full key and the
operator-selected workspace/network. It records approval/decline only and says
the registrar is not connected. Its explicit displayed-tuple comparison prevents
approval of an unseen changed mapping. A browser key match remains a human
comparison, not cryptographic proof. The inactive proof experiment remains
unimported by this command.

Thirty focused native race repetitions cover strict duplicate/unknown/case/null/
noncanonical identity refusal, explicit origins, fresh attempts, exact full-key
browser encoding, one local identity request per invocation and canceled-owner
absence of effects. These are local tests, not completed consumer setup or
physical cross-machine acceptance.
