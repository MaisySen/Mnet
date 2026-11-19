# Private exact native file-decision ownership

19 November 2026, Australia/Sydney. Go source preparation through real mux/receiver tests. No mobile API, JNI bridge, picker or consumer reception is enabled.

## Concrete private adapter

The unexported nativeFileDecision connects the existing exact FileApprovalGate request to the private native-shaped output adapter. Its constructor, Choose/decline/current/close methods and fields are all private to the session package. No exported mobile type, callback option, Go binding or registered Android component is added. Existing app/mobile receivers still have no destination and existing headless/path defaults are unchanged.

Construction validates the supplied original FileApprovalRequest against the same live gate entry. The object retains that original request and gate privately, rather than reconstruct an approval from a native filename, session string, JSON or URI. Choose creates one private output capability and calls the existing exact gate decision. Foreign gate/request, changed metadata, deadline/retirement or gate closure cannot authorize a new destination. Gate validation is still authoritative when Choose runs; a presentation Current hint is not consent.

Multiple private wrappers can refer to the same original pending request, but only one gate answer can win. A losing/repeated/closed choice releases its own supplied native output, never another request's selection. A caller-owned output passed to this method is consumed: accepted choice transfers it to the receiver; rejected choice releases it. Failure details remain fixed local errors, not private provider exception text.

## Prompt retirement is not selected-output cancellation

The original FileApprovalRequest.Done closes for successful answers too. The private decision holds its own mutex while the exact gate call settles, records the successful selected capability, then unlocks. Its Done watcher takes that same mutex and retires an unresolved prompt, but never treats a settled successful answer as export cancellation. No native callback is invoked while holding this decision or gate lock.

The separate local owner close path declines an unresolved prompt or releases its selected capability. The [one-time receiver selection release](file-export-selection-release.md) makes teardown safe with concurrent receiver cleanup. Close joins the prompt watcher. Selected output cancellation after handoff still belongs to stream/session/export context, native Activity destruction and explicit local owner teardown, not to the prompt Done event.

A native callback already entered must still cooperate or return; this private decision object does not make provider work forcibly interruptible. A later public/native route must retain this owner through session teardown and map Activity destruction to its close path. That route does not exist yet.

## Binding-shape constraints, not a runtime bridge

A separate compile-only experiment used the exact gomobile module pinned in go.mod, v0.0.0-20261011034528-197770444867. An exported Go request struct with private fields generated a public zero-argument Java constructor and equals/hashCode based only on exported fields. With no exported fields, different same-class proxies compare equal. Therefore a future bridge must never use Java wrapper construction/equality as approval authority.

A request-interface shape backed by a private Go object avoids a public request-class constructor. Native code could still implement that interface, so the eventual Go route must reject foreign implementations rather than call arbitrary native request methods as authority. The private object implemented in this checkpoint is not exported merely to obtain bindings.

The proposed Output Open/Write/Finish/Cancel interface generated Java methods with supported error/byte-slice signatures, and a tiny wrapper around DocumentExport compiled against SDK36. No JNI library was loaded; callback threads, Java exception translation, byte-array lifetime/mutation and actual permission/provider behavior remain untested. This experiment does not make the private adapter callable from Android.

Sources for binding semantics: [gobind documentation](https://pkg.go.dev/golang.org/x/mobile/cmd/gobind), [bind package](https://pkg.go.dev/golang.org/x/mobile/bind), and [generator logic](https://github.com/golang/mobile/blob/master/bind/genjava.go). The moving generator page is background reference; exact constructor/equality findings were reproduced with the repo-pinned local generator.

## Evidence

The real mux/receiver tests exercise:

- Successful choice closes prompt Done while the selected output remains usable through native Finish and final exact bytes.
- A blocked selected output is canceled by explicit owner close, never mistaken for a new pending prompt.
- Nil/zero/changed/foreign/retired request refusal and selected-output cleanup after a rejected/replayed choice.
- Pending owner close sends refusal and releases its watcher; End, expiry and gate closure cannot accept a later native choice.
- Two private wrappers for the same exact request choose concurrently; one gate answer wins and both selected/rejected native outputs end with one release each.
- Concurrent Choose and local owner close retain exactly one native owner and no receiver worker.

The combined decision/output/export/approval families pass20 race repetitions, including2000 Choose/close interleavings,1000 duplicate-wrapper choices and2000 existing decision/DATA/reset interleavings:

```sh
go test -race -count=20 -timeout=2m ./internal/session -run '^TestNativeFileDecision|^TestNativeFileExport|^TestFileExport|^TestFileApproval'
```

Full repository race/vet and current normal CI status are reported separately. These tests use explicit Go native-shaped callback fakes, not Java, installed Android or real storage providers. No visible UI/layout change or new pixels are claimed.

## Remaining route and product gates

The actual native request interface, mobile host wiring, callback executor/teardown, Android Activity owner binding and generated runtime bridge remain absent. Adding an opt-in mobile receiver API, launcher or manifest registration needs its own scope review; a passing private test is not permission to enable reception.

The prepared document Activity retains its temporary-grant lifetime, but installed task/grant enforcement, provider crash, actual process death, late placeholder documents, partial export recovery and the20second approval/30second sender UX gap remain open. These are not solved by a longer timeout or a private Current check. No Apple, signing/release, account, Multies or production service changes are included.
