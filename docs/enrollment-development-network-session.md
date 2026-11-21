# Disposable connected-session increment, work in progress

This is separate from the accepted historical joining result. The accepted
standalone setup command still finishes without starting networking. No normal
CLI, startup, local API, OS TUN or service is changed. No screen/input consent
is inherited. SharedDesk is outside this increment.

The tagged RunDisposableEnrollmentNetwork API retains the same daemon owner
after its actual browser/source/proof/finalization/current-map path. It requires
an explicit injected disposable device, a finite registrar request budget and
a callback that owns the session. Its timeout must equal the supplied workflow
limit. It uses only the existing receipt interval, never renewal/recovery. The
normal RunDisposableEnrollment API is unchanged.

The new controller starts the same retained-key WireGuard device, publishes
only a loopback UDP endpoint and applies the actual native current map. Nonlocal
peer endpoints, routes and exit nodes refuse. It runs existing known-map-expiry
removal. Original receipt expiry, context cancellation or changed local
State/policy stops the network. Explicit map refresh failure or exhausted shared
request budget also ends it. There is no automatic remote-map poll/reconnect.
Remote revocation is only known after an explicit successful map check; do not
claim instant offline account revocation.

The callback gets a fatal context. Own registers listener/connection/Mux
resources for close on stop, before device teardown. Close joins device/expiry
ownership. The callback remains responsible for joining its own workers after
close; arbitrary caller callbacks and arbitrary blocking Close implementations
cannot be forcibly joined by this API. CurrentSessionPeers performs an explicit
source check, then returns approved unexpired exact name/IP/key identities.
It is not a persistent membership grant. Peers is only a local snapshot and
must never be used as a substitute for that current source check at admission.

## Current evidence and gaps

Actual opt-in Multies app/SQLite/source/Store/native transport runs use the new
controller instead of the old manual start. They preserve the existing15s
lease, original State/key and actual encrypted payload checks both ways. An
explicit session refresh removes a deleted peer before new packets are denied.
Stop, local policy change and original deadline separately close the device,
unblock registered real TCP accept/read operations, join the owner and return
no peers. The listener/read checks are resource ownership evidence, NOT TCP
over the encrypted mesh. A first expiry test incorrectly expected current-map
access after expiry; its log remains failed evidence. The corrected check
requires denial after the original deadline, without extending it.

This is NOT yet the connected installed browser journey. The standalone command
has no network option wired, the device in the current transport proof is a
channel TUN, and no actual userspace TCP pair or RD viewer bridge has run. The
existing proofnet package can supply an in-memory TCP/IP TUN; its source
compatibility is not execution proof. The earlier browser fixture counterpart
nodes expire while login is underway. Their enrollment must move after consent
or otherwise fit within the original interval; policy must not be widened.

The proposed next bridge is a development-only nonreserved TCP port with exact
peer key/IP/name, source checks before/after dial and fatal context ownership of
listener/connection/Mux. Host attended viewing must still default decline and
require its own approval each run; pointer permission stays separate. A supplied
RD contract harness uses loopback wrappers, not actual proofnet, so it does not
close these gaps. No real display/keyboard/mouse, normal5909 listener, production
lease, service, physical pairing or live account is authorized by this work.
