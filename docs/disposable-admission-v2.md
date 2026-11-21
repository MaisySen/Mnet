# Disposable V2 initial admission, review candidate

This is a separate, explicit development experiment. It does not activate normal
Server, install a service, operate a real account/device, extend a lease, deploy a
hosted backend, implement rates, grants or renewal, or migrate/adopt V1 roots.
Accepted baseline: Mnet 0ffcc28, Multies 0cc75f6. Existing commands remain V1.
The new tagged command is `cmd/mnet-registrar-admission-development`.

## Accounting, not product policy

A fresh private V2 root has one exact configured source authority namespace. Eight
source-work operations are its lifetime maximum. An operation durably spends one
before source HTTP. Only a validated authenticated source response selects the
principal tuple, whose lifetime maximum is four. A fifth observation for that
principal still spends source work, but cannot spend another principal debit or
enter the proof factory. Another authenticated principal may use remaining root
units. Unknown/invalid responses never debit a caller-named victim principal.
Aliases, key IDs and caller attempt IDs do not create accounting namespaces.

These are finite development totals, not a rate window, fairness policy, hosted
quota, global enforcement or DoS resistance. Invalid tickets can exhaust a root.
There is no refund, reset, eviction, epoch, automatic replacement root or adoption.
A deliberately created separate root is a separate experiment, not recovery.
Same-UID file copy, rollback and coordinated state edits are outside this boundary.

## Four publications

1. Reserve source work and immutable operation/request/descriptor digests before
   HTTP or proof work.
2. Publish authenticated observation fingerprint, original observation/deadline
   and observed-principal debit before the proof factory.
3. Publish public challenge fingerprint and original deadline before returning it.
4. Publish terminal admission reference in the same original guarded candidate as
   node, cursor, claim, consumed proof and immutable receipt.

Every published earlier debit remains spent after later failure. Postrename
uncertainty freezes the owner. Owning reopen inspects one complete image without
HTTP, rewrite, challenge reconstruction or confirmation-MAC recreation. Every
precommit operation without its original private RAM owner is spent and stranded.
It does not certify whether a remote effect occurred or permit a safe retry.
Same-ID exact or conflicting requests after reopen refuse. A fresh attempt is a
new operation under the original handoff/consent and source sticky-row semantics,
not an automatic replacement or refund. `InspectAdmission` is a local tagged
count-only inspection method, not an HTTP route or readiness/resume capability.

After stage 2, original source freshness is checked before entering the real
proof factory. After stage 3, original deadlines, source freshness, pinned config
and owner health are checked again before returning a challenge. No timestamp is
moved to compensate for slow publication. Original proof and expiry behavior,
including the installed 14-request connection slice, remains unchanged.

## Configure pin and incompatible storage

V2 creates a fresh private root, distinct root marker and non-JSON V2 image prefix.
Legacy JSON, V1 and V2 readers refuse other actual image formats. Markers do not
protect arbitrary absent filenames or prevent same-UID tampering.

Configure is initial-only. After writing/syncing sidecars it publishes the empty
ledger and exact-source/workflow-byte fingerprint in the same V2 Store candidate
before configured readiness. An unpinned or incomplete root cannot serve Handler
or begin. Existing sidecars after failed pin are not adopted or auto-completed.
A pinned root cannot Configure again, even after sidecar deletion. Reopen, Handler
construction and each existing workflow/source-owner config check compare the
pin before HTTP/proof work. Restoring exact sidecar bytes never refunds counters
or reconstructs RAM attempts. Changing alias/key/policy bytes refuses instead of
creating a namespace or repinning. Missing or inconsistent ledger fields refuse
without rewriting.

The domain/version-separated fingerprint includes exact config bytes and finite
accounting constants. Because the config includes a high-entropy service secret,
it is a secret verifier, not secrecy: it reveals equality and enables guessing
weak secrets. Never log or disclose it. Other digests also reveal equality and
permit dictionary attacks on low-entropy input; principal digests are not anonymous
or encrypted. The private image retains principal identity metadata. It does not
retain raw ticket/source secret/private DH material, proof response or confirmation
MAC. Command config uses the existing RawMessage transport: remarshal normalizes
whitespace/escaping. Pin binds the exact resulting installed sidecars, not the
original command input's wire bytes.

## Bounds and validation

Actual Go structs measured with encoding/json.MarshalIndent give a conservative
worst-grammar eight-entry ledger witness of 23891 bytes. This uses a valid principal
of 256 '<' bytes, each HTML-escaped by the serializer, fixed 43-character digest
keys and 19-digit timestamp fields. It is a serializer/shape witness, not a valid
authority ledger. Hard entry cap is 4096 bytes; whole ledger reserve is 36864;
whole actual encoded image, including prefix, must fit the configured cap at or
below 1048576. No truncation or identity normalization is used to squeeze fields.

Before remote work, use overflow-safe B + L + nR <= C: non-ledger encoded image B
including prefix, L=36864, R=262144 per outstanding commit, C<=1048576. A nonzero
baseline image already limited the old 1MiB profile to three full reserved slots;
V2 does not newly reduce four usable slots to three. The test verifies three
capacity reservations fit and the fourth refuses. RAM live/attempt/proof limits
remain in force independently.

V2 rejects duplicate, unknown, wrong-case, missing, null or wrong-type ledger
fields; oversized maps/depth/int64 numbers; malformed tuple/index/counter/phase
relationships; and terminal record/challenge/receipt mismatches in both directions.
The optional shared ledger is nil/omitempty for legacy/V1. Those codecs reject a
non-nil ledger. A deterministic baseline/candidate nil-codec byte comparison is
identical, SHA256 86b7b04f7989f1a1f550e0058a4b4f3f758b97de999de8b37987527d8f593de3.
A private per-Store proof-factory test observation seam is nil in ordinary use and
calls the original proof.NewServer. No external selector is added. Review this
common-runtime seam explicitly.

## Validation and limits of evidence

- Real TLS source, proof, commit, owning reopen, source8/principal4, duplicate and
  conflict, alias/key pin, invalid ticket, malformed images and reader matrices.
- 32 injected I/O failures across four stages: tempcreate/write/shortwrite/filesync/
  close/rename/postrename/dirsync. Separate prepublication stage-1 checks assert
  actual source calls zero and proof-factory allocations zero.
- 24 actual externally verified OS SIGKILL boundaries across four stages:
  tempcreate/write/filesync/before-rename/after-rename/after-directory-sync.
  Old/new complete image, source debit and atomic terminal composition are checked.
  Ten repeated 24-boundary runs also pass. This is not power-loss certification.
- Delayed original stage-2 and stage-3 intervals expire without returned readiness.
- Actual installed V2 save exits, separate connect uses its saved one-shot slice,
  actual own-key counterpart and encrypted TCP, then original expiry leaves zero
  peers and disabled Stop. Unchanged installed V1 standalone regression also passes.
  Desktop screenshots were inspected; there is no mobile audit.
- Default and tagged package race coverage was completed in groups, then the
  complete default and tagged race commands passed using the resulting cache.
  Default/tagged vet/build, nil-codec baseline comparison and ESLint pass.
  Initial monolithic test invocations hit execution windows, not assertion failures.
  Those were not represented as all-suite passes; successful complete-command and
  detailed stage/kill logs are provided.
- One full tagged run exposed a fixture IPC race: boundary metadata was initially
  signaled by direct file creation, so the killer could read a partial file. The
  fixture now atomically renames complete metadata before signaling. Store fault
  boundaries, SIGKILL verification and production behavior were not relaxed.

Independent implementation review is required before acceptance. None of these
checks establish production readiness, native installation, current eligibility
for renewal, account authority, screen access or keyboard/mouse access.
