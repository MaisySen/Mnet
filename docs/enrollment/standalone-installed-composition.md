# Standalone registrar to installed save/connect composition

Separate test-only candidate over accepted4e300f2/649878. No service/router,
Store, native command or product file is changed. New opt-in cross-app test
starts the actual standalone loopback registrar binary, a separate journal-owned
counterpart process and actual installed setup then connect command processes.
The actual fresh owned browser prepares login/workspace approval and Return
BEFORE any counterpart receipt is committed. No real account/device is used.

After counterpart commit and endpoint publication, actual browser Return lets
installed setup perform own-PKCE/proof/confirmation/native State finalization
and save the optional local one-shot ledger. Setup exits BEFORE connect starts.
Counterpart uses its authenticated current map to configure the enrolled
installed peer key/allowed IP before connect's first handshake. No endpoint is
invented; WireGuard learns it from authenticated traffic. This avoids a test
race where native's first handshake arrived before counterpart configuration,
then the unchanged3s dial deadline elapsed before WireGuard's later retry.

Connect spends the entire original14-request slice, creates only existing
userspace proofnet TUN, completes fresh challenge/SHA256 TCP over encrypted
WireGuard to the one approved counterpart, displays Network active then Network
ended under original peer expiry, and joins its existing owners. Native State
and pending journal bytes are unchanged during connect. Both actual enrollment
source exchanges/reserves are counted. Standalone image has two registrar nodes,
zero legacy tokens/auth keys. New final phase only checks spent ledger marker;
no refund or renewal/resume is invented. The counterpart uses a finite32-request
client inside the existing64-request registrar ingress, all disposable.

Run paired `tests/mnet-standalone-installed-validation.sh` with Mnet/Multies
paths. It creates a fresh owned browser profile and desktop. No service fixture
switch, Store injection or router test hook was added. Native OS TUN, physical
hosts, useful duration, durable admission, refresh grants, hosted policy, input
and remote desktop remain unimplemented/unapproved. Tests are local composition
checks only, never user consent, deployment permission or readiness.
