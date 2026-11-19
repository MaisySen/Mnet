# CLI current mesh address

The daemon retains its last map for private diagnostics after Down, during a new startup and when authorization expires. Previously `mnet ip` treated a retained IPv4 address as enough to report success, even when the status was no longer Up. Five actual CLI subprocess cases failed against the old source over the capability-protected Unix HTTP path: Down, preparing, joining, waiting-approval and key-expired all printed the old address with a successful exit.

The command now requires current status Up as well as a nonempty address. Other states return the existing not-up failure without printing the retained address. A genuine current Up address keeps the same plain output and successful exit. The private status command and diagnostic map history are unchanged. Current Up still does not prove successful peer, DNS, subnet or internet traffic.

Twenty race-enabled subprocess/API repetitions cover all five inactive states, unknown status, current Up and Up without an address. The full repository race suite, vet and source checks pass locally. The test fixture binds a private short Unix socket and cleans up only its own directory. Windows uses a fixed local TCP port, so these Unix subprocess fixtures are not Windows acceptance; production CLI logic is shared. Existing macOS/Go normal CLI checks select these tests without changing any workflow trigger.

Hosted verification for this correction remains separate and pending. No daemon state mutation, routing/default change, installed-app/device test, Multies integration, production deployment or app release is implied.

```sh
go test -race -count=20 -timeout=2m ./cmd/mnet -run '^TestCLIIP'
go test -race -count=1 -timeout=5m ./...
go vet ./...
```
