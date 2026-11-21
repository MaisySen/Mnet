# S34 coherent inert source/test review gate

Source and UNRUN test batch only. No compile/vet/test execution since S31 accepted
146 green. No fresh capture, reader/device/netlink/sysctl effect. The new reader
exists only behind an interface whose only implementation is test-owned value
memory. Structural containment is source discipline, not an authentication claim.
Compose and every live reader/wire remain unchanged. Strict raw active readiness
is STILL blocked by owned-link attr26 contradiction.

## Reader-owned provenance and same-pass witness

inertAFReader constructs the current exact 32byte netlink request via existing
strict client: type18, flags0x301, increasing sequence, local recipient, sixteen
zero payload bytes, no ext-mask. A recording wire copies exact request and raw
reply datagrams, retaining every received header including sender/truncation.
Existing client validates sequence/recipient/kernel-sender/multipart/completion/
padding/interrupt before any pass is produced. readPass is privately constructed;
new read invalidates previous pass even on failure. No caller request-claim can
replace sent bytes. Parsed links are from the received payloads, not separately
supplied caller LinkRecords. Copies prevent source aliases changing stored bytes.

After the same dump call, the reader explicitly requests per-link directory and
IPv4 forwarding, plus applicable IPv6 forwarding/force_forwarding/disable/addrgen.
Missing requested leaf refuses pass. These maps are copied into that pass. This
is a sequential SAME-PASS observation, NOT an atomic snapshot/stable kernel proof.
The reader doesn't inspect entire namespace rules/routes or all/default policy.
No real supplier exists and no extra reads were added to the live sysctl reader.

## Phase-carrying diagnostic types

Creation applicability binds once to the canonical original handle while still
created/untagged/no pending/poison/cleanup. It is derived from that received link's
AF_SPEC and matching directory evidence, parsed by the FULL helper. Actual decoded
kind must be tun, not a caller label. Later diagnostic calls derive phase from
ledger state, compare original handle, retain original creation applicability and
refuse changes. No supplied caller post-disable boolean can promote the public
wrapper. Underlying unused low-level parser still takes data claims; it is not an
execution or readiness API.

OwnedAFDiagnostic carries shape/readback information only. Separate private
OwnedAFDisabledCandidate requires ledger phase confirmed-disable and the disabled
profile. It explicitly marks uncheckedDevconf=true and cannot convert to Inventory
or active observation. Current generic Compose is deliberately not changed.

## Explicit unchecked policy scope

IPv4 CONF has33 words. This parser validates forwarding word0 only and retains
all other words, NOT safe host-networking policy. IPv6 CONF has60 words; named
forwarding/force/disable/addrgen and omitted slot are checked, other words retained,
NOT assumed harmless. Neither diagnostic nor disabled candidate means policy-
complete, arbitrary-app network readiness, or permission to mutate. Unknown word
counts/attrs refuse; recognized-but-unchecked words are distinct from unknown
shape. Preserving bytes is audit fidelity, not a security policy decision.
Before any future Compose/readiness conversion a separate policy audit must choose
and justify every relevant word, both IPv4 and IPv6, plus namespace witnesses.
This prevents broad readiness claims being laundered through retained raw arrays.

## Decisive proposed tests, UNRUN

- Exact received/sent provenance, cloned aliases, increasing sequence, stale-pass
  refusal, altered reply sequence, requested same-pass force witness and missing
  force leaf. All exchanges and witness reads mutate Go values only.
- Recorded gretap0 FULL shape with synthetic extra force witness explicitly
  labeled, raw bytes unchanged. This is not recorded owned-TUN/sysctl evidence.
- Count59/61 refuse, index59 nonzero, stats/icmp first-count word mismatch,
  unknown nested/family/flag, malformed padding, zero RA-MTU, missing force,
  disable/proc disagreement, post-disable flags, IPv4 count/forward and profile.
- Explicit pre/post-disable and directory table: pre0 diagnostic only, post0
  refuses, absent+ENOENT pre diagnostic only, absent post refuses, contradictions
  refuse. Creation cannot rebind or caller-promote a disabled candidate.

No test result is claimed. This packet needs source/effect review before any
once-only inert execution. The tests do not solve raw active contradiction or
prove kernel behavior. No optional status abstraction, journal or activation added.
