# Explicit-origin native source exchange transport checkpoint

Inactive source-client engineering, not native enrollment or authority activation.
There is no runtime caller from the daemon, journal, UI, control Store or command.
The journal still exposes no secret getter. No browser, listener, real external
Multies account, service key or credential installation was used by these tests.

`internal/enrollmentsource.Native` constructs its own HTTPS transport for the exact
independently configured source origin and separately configured principal
namespace authority. No plain HTTP, paths, URL userinfo/query/fragment aliases,
caller transport, DefaultClient/DefaultTransport, environment proxy, cookie jar,
ambient browser authentication or TLS bypass is inherited. TLS uses system roots
and requires at least TLS1.2. Timeouts and connection count are explicit caller
engineering bounds, not access/lease/staleness product policy. No private value
is placed in a URL. There is no logging or server-body diagnostic forwarding.

Each Exchange checks the original saved sourceOrigin AND principalAuthority
against current constructor configuration BEFORE disclosing code/verifier.
It checks canonical code, exact saved PKCE challenge from verifier, saved launch
shape and original/current mapping shapes, and requires their intersection before
request. The exact one-shot endpoint and JSON proof shape match current Multies
exchange-v2. Request has no Cookie/Origin/Authorization/Referer, no-store and no
cache metadata. There is no retry loop, redirect-following or new consent path.

Every redirect status is refused without forwarding the POST body, even307/308.
Only status200, one application/json Content-Type, one exact no-store Cache-Control,
no content encoding, bounded headers and bounded64KiB body can reach the strict
source DATA parser. Error bodies, Location and url.Error details are not surfaced.
Any transport/status/protocol/source-binding error is remote-outcome-unknown,
not an assertion the one-shot code is still usable. Expiry check adds measured
monotonic request/read elapsed time to caller-supplied now; it does not freeze
response validation at request-start. Caller wall-clock accuracy remains unproven.

Returned SourceExchange remains exported forgeable DATA. HTTPS authenticated the
configured transport peer for this call but no private evidence/permit type exists.
The function cannot prove caller durably saved native secrets, owns the current
WireGuard key/listener, durably marked exchange-may-have-started, or gave actual
current config. Those remain the future native owner integration's responsibility.
Current config can change during the request; a future owner must read it again
before storing/using the result and refuse if original/current scope changed.
No bearer service-reserve client or registrar authority adapter was added here.

## Original journal source-origin correction

The preceding unshipped b31ec1c journal stored principalAuthority plus registrar
mappings but lacked a separately named original sourceOrigin. This checkpoint
adds required source_origin to the private journal record and Begin input, validates
exact HTTPS syntax and tests distinct source.test versus multies.test authority.
No equality is inferred. Original/current source identity must match before private
request disclosure. Existing b31ec1c-format test-only journal files now refuse
missing source_origin; no production journal had been integrated and there is no
silent backfill/migration or identity rotation. This is a correction to the pending
journal review, not acceptance of that earlier schema as complete.

## Executed evidence and limits

All HTTP tests run only against disposable local TLS test servers. Tests add that
specific server certificate to a test-only root pool through same-package access;
production NewNative exposes no TLS-root/transport bypass. Untrusted-certificate
case refuses before HTTP proof delivery. TLS hostname/certificate infrastructure
and actual external source deployment were not accepted by this test.

Wire checks cover fixed endpoint/body, exact private proof fields without printing
them, no ambient authentication, no cookie reuse after Set-Cookie, no default proxy
or transport inheritance, each301/302/303/307/308 without target delivery, response
loss after body receipt with one request only, error/body/flag/authority/header/
cache/size/encoding refusal, original-source/current-authority mismatch and removed
mapping/wrong-verifier preflight with zero private sends, and response delayed
beyond expiry. Transport responses deliberately adapt the separately verified
actual producer fixture to fresh local test PKCE/time; they are NOT a new actual
Multies producer end-to-end test. c3d1f8f producer and DATA parser parity have
separate actual-handler evidence and independent review.

Still open: reviewed owner+journal correction, actual native State/owned key and
all writers, owned callback and first-run UI, coherent owner+journal/client
integration and current-config recheck, source-authenticated authority adapters,
service client scope, response-loss recovery, clock/staleness/lease/admission,
reviewed possession/confirmation and actual atomic Store consumption. No tunnel,
live account, release, deployment or shared desktop file changed.
