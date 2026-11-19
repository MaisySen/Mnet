# Canonical enrollment key identity

The control store compared WireGuard public keys by encoded text while admission checked only whether base64 decoded to 32 bytes. That allowed distinct text to represent one cryptographic key without matching its existing saved identity. Three actual join-handler regressions reproduced the defect: LF, CRLF and nonzero padding bits each allocated another node with a new address and saved credential. These nodes were pending without enrollment authorization. The tests do not establish approval bypass, possession of the private key or live exploitation.

New join requests now require the standard padded canonical base64 form. Strict decoding rejects nonzero padding bits; an exact re-encoding check also rejects ignored line breaks. Rejection occurs before network lookup, enrollment-key use, address allocation or store mutation. Normal generated client keys already use this form. Existing saved-node credential requirements and fresh authorization after expiry are unchanged.

The HTTP regression compares saved bytes, map version, address cursor, node count and reopened state, and checks the response does not expose the existing credential. Canonical-key positive checks, the existing actual-handler owner/no-owner cases and the store expired-rejoin tests preserve legitimate behavior. Twenty focused race repetitions and the full repository race suite, vet and source checks pass locally. The normal native test selector includes this coverage without changing workflow triggers.

This is new-request admission, not a stored-state migration. It does not scan, merge or repair historical alternate-key records. A store containing old noncanonical records needs a separately reviewed audit and recovery plan. There is no production service change, device/native acceptance, Multies work, routing/default change or app release. Fresh hosted verification is pending.

```sh
go test -race -count=20 -timeout=2m ./internal/control -run '^TestJoinKeyEncoding|^TestRejoinRequires|^TestExpiredRejoin'
go test -race -count=1 -timeout=5m ./...
go vet ./...
```
