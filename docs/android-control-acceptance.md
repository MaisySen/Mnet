# Bounded native attended-control acceptance

This is one selected manual API34/API36 matrix, distinct from the closed combined Android DNS/bulk/tracing investigation. It accepts only the new attended-control journey on disposable emulators. It is not Android-to-Android, physical/OEM, VPN reliability, Multies or release acceptance.

## Route and fixed verdict

`android-control-acceptance.yml` is manual-only, using standard public Linux runners. Each job creates one fresh English Google APIs emulator at 320x800/mdpi, uses a disposable debug build/current native binding, and joins an isolated local mesh service. The test counterpart is a real userspace encrypted-mesh peer, not an Android transport mock. No personal account/device, production service, paid runner, release signer or Multies code is involved.

Only setup/mesh/session prerequisites needed by this journey are exercised. No DNS probes/contrast, bulk benchmark, root/BPF tracing, alternate socket binding, route experiment or timeout widening is included. Unavailable prerequisites produce a blocked verdict, not a successful control test or evidence of a specific networking cause. A reached control assertion failure produces failed. A passed result must then receive actual screenshot inspection before UI acceptance is reported.

One matrix attempt is selected. Failure or unavailable prerequisites return to the coordinator with the fixed stage and completed checklist; no automatic rerun, new tracing or expanded investigation follows. Existing DNS/reliability failures are neither retried nor cleared. Standard build failures are blocked before acceptance.

## Two orientations

1. **Android host, Go viewer.** Real app viewing approval, MediaProjection JPEGs decoded by the counterpart, viewer request/cancel, new request with decline, another request with explicit local Allow, actual AccessibilityService gesture firing a synthetic on-screen target. Host controls pause input before Keep/Revoke. Revoke refuses viewer input and a raw stale-bound input attempt, preserves new decoded frames, and permits no more observed target clicks. A canceled local answer is refused. Optional accessibility permission removal disarms control; Stop hosting closes the transport.
2. **Android ViewerActivity, Go host.** Real visible Request/Cancel buttons and status, native binding/session callbacks, decoded synthetic JPEG screen, real injected test touch events through the Activity, no counterpart input before approval, exact bound input after a fixture-local host decision, pause/Keep/revoke, stale request answer refusal, a new connection with no inherited session/grant, and an unsupported peer remaining view-only. The Go screen and input counter are synthetic, not an actual desktop OS target. Large-font viewer pixels are captured at 1.3 scale after the normal narrow-layout cases.

The fixture-local Go host has no automatic control approval. Its loopback-only private command surface drives explicit test host decisions. This surface exists only in a test binary and is not shipped. Android host decisions are exercised through their actual dialogs, not a permission toggle or direct local grant call. Direct local API use is limited to verifying that a stale canceled answer is refused. OS accessibility enablement goes through real disclosure/Settings UI; removal affects only the disposable emulator.

## Evidence and privacy

Artifact paths are an exact allowlist: sanitized summary/checkpoint results plus dedicated synthetic host/viewer/consent screenshots. Enrollment MainActivity is never captured after credentials are entered. No APK, credentials, keys, application state, raw logcat, instrumentation arguments/output or packet data is uploaded. Private fixture files and child processes are cleaned up. A screenshot is a sampled current screen, not a continuous recording or full lifecycle proof.

Before dispatch, the fresh full Go race suite, vet, twenty synthetic-counter/capture fixture repetitions, Java instrumentation compilation, runner syntax, workflow/manual-only/artifact safety guards and the existing combined-fixture safety suite pass locally. These checks prove preparation, not native execution. Record the actual matrix URL/results separately; do not prewrite success.

## First attempt and bounded fixture correction

The first matrix at commit3a3c171 reached both real control orientations on API34/API36 but blocked waiting for the viewer's fresh connection after closing the first Go host: https://github.com/MaisySen/Mnet/actions/runs/44259439995 . Reconnect/no-inherited-grant, unsupported-peer and large-font native checks were not reached. Actual normal-font host/viewer pixels were inspected; this remains partial evidence, not a pass.

A bounded review reproduced a fixture lifecycle defect: a new per-session gVisor listener rebind at the same port failed with port-in-use after the accepted connection closed. The old asynchronous command had swallowed that failure while reporting success. This is a reproduced strong explanation, not forensic proof of the original private native error, which was deliberately absent from uploaded artifacts.

The correction retains one fixture-owned listener for sequential sessions, closes only the active Host/mux between cases, and makes initial listen succeed synchronously before reporting command readiness. A listen failure returns an unsuccessful command and no listener-ready state. The listener closes at fixture-process cleanup. No app reconnect, TCP reuse/routing/binding behavior, checklist or timeout changes. The regression fails before correction, then passes30fresh race repetitions across three real gVisor/session-control handshakes, unique sessions, no inherited input grant and prior-session answer refusal. A collision test verifies readiness cannot be claimed after failed listen. One confirmatory matrix is selected separately; its result must be recorded without erasing the first blocked attempt.

## Confirmatory result and pending fixture-readiness revision

The confirmation at2bdc1a1 passed the API36 functional checklist, including fresh reconnect and unsupported view-only: https://github.com/MaisySen/Mnet/actions/runs/44261308859 . API34 blocked earlier waiting for Allow viewing; its original cause remains unknown. General changed-code CI passed all five jobs: https://github.com/MaisySen/Mnet/actions/runs/44261307578 . This is not a two-API pass.

Pixel inspection rejected the API36 large-font active-viewing claim: the image showed a recreated viewer's initial unavailable state and no frame. Source review and20fresh deterministic proofnet overlay repetitions demonstrate that a single-accept fixture cannot answer a recreated Activity's second hello without arming another accept. This explains a fixture gap, not an app reconnect defect.

The pending fixture-only revision returns fixed private admission outcomes (queued/dialing/hello-pending/admitted, absent authorized peer, failed TCP dial or failed hello), waits for the existing real host-listener-start flag and resumed Activity, and checks current peer authorization before the one existing dial. It observes the existing pending view-consent state before trying the real Allow viewing button. No keys, addresses or raw errors are added to public summaries. Failure stages remain prerequisites, not control assertion failures. There is no new dial retry or timeout increase.

For font configuration, one additional unsupported session accept is armed without closing the still-active connection, preventing the old Activity from consuming it with an unrelated pre-change retry. The OS font change then recreates the Activity. Fixture lifecycle callbacks identify the new resumed viewer, and a predicate requires a different instance, actual1.3font resources, a new decoded frame, negotiated unavailable control, zero received input and complete visible status/button bounds before capture. No control grant is inherited or added. Active large-font viewing with unsupported control is the selected case; granted-control large-font UI is not claimed.

Local deterministic regressions reject absent-peer/dial/hello failures as admitted, prove arming the next accept preserves the live session, and reject old Activity/blank frame/wrong scale/granted state/received input/clipped bounds as font recovery. Fresh whole Go race/vet,20focused race repetitions, seven fixture/JVM guards and Java/API24dex compilation pass. The final revision is awaiting diff review; no further emulator matrix has been launched or acceptance added by these local checks.
