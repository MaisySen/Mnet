# Separate service source DATA parser and HTTPS client checkpoint

Isolated, unhooked service-source engineering. No control Store caller, authority
permit/transaction, native daemon integration, real service key/account or remote
production request exists. Multies producer source remains clean at c3d1f8f.
NativeState3c2b remains a separately reviewable prerequisite, not activation.

## Do not invent native expected identity at the registrar

DecodeSourceReserve is deliberately a NATIVE comparison against native's own
separately saved PKCE result. Registrar does not possess that result when it asks
Multies for reserve evidence. It must not synthesize a SourceExchange from response
principal/session-binding merely to satisfy a helper's expected-value signature.

New SourceReserveRequest is the exact sent request DATA: ticket, handoffId,
reserveId, launch and mapping. It has no principal/provenance. The existing descriptor
encoder was factored to private explicit handoff/launch/mapping/service/reserve
parts, with unchanged bytes. Both native own-result and service actual-request
entry points use those same parts without fabricating a native identity result.
Service request's canonical ticket is checked but intentionally not hashed into
the existing descriptor-v1, exactly as the current Multies producer specifies.

DecodeServiceReserve strictly parses the actual v2 response against the exact sent
request, configured principal authority and service ID, recomputed descriptor,
reserved status, false enrolled/authority flags, explicit local-user namespace,
source observation timestamp and unexpired original deadline. Source principal
and sessionBinding are OBSERVED source-row DATA. Parser does not claim it previously
knew native's expected principal. A deliberate test changes source principal: the
service DATA parser observes it, while native parser independently refuses it
against its original own-PKCE result. This difference is intentional and prevents
false identity provenance. No parser result grants membership or network access.

## Explicit service source client

NewService takes explicit service scope, secret, allowed exact mappings and limits.
Source HTTPS origin, principal authority and registrar origin are independent.
It pins service/key IDs, issuer label and audience. Every allowed mapping must
match that issuer/registrar/audience, be valid, unique and is copied defensively.
It validates the actual request and exact configured mapping before private I/O.
No caller URL, remote source path, callback source identity or response-selected
mapping can redirect the request.

Reserve sends ONE POST to the fixed reserve-evidence-v2 endpoint. Authorization
uses the existing producer's exact `Bearer keyId.canonicalSecret` format, no new
bootstrap/auth scheme. Ticket is in the private request body, not URL. No cookie,
Origin, Referer, browser session or native credential is inherited. Raw service
secret IS BEARER-ON-WIRE and all source TLS terminators/header/APM/crash logs are
inside the threat boundary; application silence alone does not prove deployment
redaction. Trusted source provisioning/logging/topology remains a live acceptance
gate. No service credential appears in artifacts or logs except fixed disposable
test bytes explicitly generated in code.

Native and service share the already bounded fresh verified-HTTPS HTTP1.1/no-
keepalive/no-HTTP2/no-rewind/no-redirect transport and strict no-store JSON response
reader. This is a private mechanics extraction, not changed native authority.
Native regression tests remain in the affected suite. No DefaultTransport/client,
environment proxy, cookie jar, caller RoundTripper or public TLS bypass is added.
Only exact status200, one valid JSON media header, one exact no-store header,
no content encoding, bounded headers/body and strict DATA shape can return.
Transport elapsed time consumes deadline. Caller now remains DATA, not trusted
clock/lease/staleness policy. Invalid/error body and URL error details never leave
the client. Any network/status/protocol/source mismatch is outcome-unknown; the
source may already have created the immutable reserve. There is no retry/status/
ack/renew/reconsent action or local receipt/node/credential side effect.

Returned SourceReserve is still exported forgeable DATA, not an inaccessible
verified evidence type. A caller can also construct a new Service for an arbitrary
configured URL if it has a secret; that does NOT make it the current configured
registrar source owner. Future authority must own its actual current service-client
instance/policy and admit only its private authenticated result after fresh local
config/time/capacity checks. This checkpoint deliberately does NOT add an exported
DATA-to-permit constructor or pretend a client pointer is an operator grant.
Neither parser nor client imports control/daemon or calls Store.

## Evidence and limits

Service parser tests use the SAME actual c3d1f8f own-exchange/reserve response fixture
previously produced by the actual Multies handlers. No service-parser fixture
synthesizes a native result from returned provenance. Tests pin existing descriptor
bytes/digest and recursively remove/null every nested field, add unknown fields,
change original request/scope and authority/grant flags. SourceReserveRequest and
SourceReserve formatting redact private transport/row data; JSON/fields remain
sensitive and must not be logged. Data redaction is not proof of downstream logging.

Service HTTP tests run only against disposable local TLS servers with a test-only
specific-certificate root pool set through same-package access, not a public TLS
bypass. Wire checks assert exact fixed endpoint/Authorization/sent body/descriptor,
HTTP1, no ambient authentication/cookie reuse, zero target deliveries for each
301/302/303/307/308, zero sends for malformed/unconfigured original request,
source-authority/service/mapping/descriptor/grant/historical/error refusal without
echoing private body, and response loss AFTER body receipt with one request only.
These wire fixtures adapt original source observation/deadline to local test time;
they are NOT actual producer+service-client end-to-end execution. Actual producer
and parser parity remains separate evidence. No deployment/live account test ran.

Still required: nativeState independent acceptance and real OS execution; actual
registrar-owned current service client/config and private evidence adapter; global/
per-source/per-attempt bounded admission BEFORE remote work; service key provisioning,
rotation and redaction acceptance; source time/observation-age/lease policy; native
own-PKCE expected binding; reviewed challenge/bootstrap/DH/MAC/confirmation; and
actual atomic durable Store consumption. Max connection count is a transport bound,
not a complete rate/memory/capacity budget. Existing direct-mesh/revocation and
cross-system atomicity limits are not fixed by a successful HTTPS reserve call.
