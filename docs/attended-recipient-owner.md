# Attended recipient-local injector owner

This is a bounded Linux/X11 engineering change to explicit attended Remote Desktop, not general crash-proof input or Shared Desk. The existing explicit legacy injector stays unchanged.

## Failure model

The coordinator and local injector now run in separate processes for each approved control grant. The injector, not the coordinator, owns its pressed-input ledger and an exclusive cooperating-injector X selection. When the coordinator dies or stops renewing a two-second local lease, the surviving injector releases only its recorded input and exits. Closing the command pipe retires it immediately. The parent renews every500ms; this is local process liveness, not a mesh heartbeat or network-partition claim.

If the injector itself dies, the X server can retain XTEST button state. A hung/killed X server, machine failure and physical-input provenance are not solved. Another application's active keyboard grab can still prevent the coordinator's stop shortcut. No always-available emergency-stop or consumer-safe recovery claim follows from this design.

X11 has shared button state, not per-application proof of physical ownership. A button or ordinary text key already down before a proposed press is refused. Local modifiers that alter text are also refused, never cleared for convenience. An Up with no recorded matching press is refused, never used as a guessed cleanup sweep. A physical or different synthetic press overlapping after our own press is not distinguishable here. The ledger records this injector's requests, not OS-wide provenance. No unrelated held state is scanned and released.

## Private local boundary

The coordinator starts its own installed executable with a fixed private child entry. Commands and replies use inherited anonymous pipes, without a shell, TCP listener, remote path or environment-selected executable. The child receives no peer approval UI or ability to grant control. Exact current session/request, peer/local-owner authorization and transport lifetime remain in the coordinator. A new grant gets a new process and pipe generation.

Local messages and replies are bounded. Input command IDs must increase, and replies must match the current request. The parent requires child readiness before returning the backend, verifies reported capture-frame geometry before granting control, bounds pipe writes and waits for confirmed child exit during Close. Retired/failed children do not fall back to legacy injection. Failed cleanup terminates the affected session. A forced kill after failed joined shutdown is reported as unconfirmed release, never clean recovery.

The surviving child holds its own exclusive injector selection until its input cleanup finishes. The coordinator's attended selection alone would disappear on its death, admitting a new injector before old input was released. A second cooperating child now refuses while the old child's selection exists. Losing that child-owned selection retires its input; cleanup destroys only its own window, never another client's replacement selection. This is cooperating-process arbitration, not exclusion of arbitrary X applications or the unchanged legacy mode.

Text remains the bounded, non-remapping, no-clipboard logical-text path from the prior checkpoint. During a multi-stroke event, current owner/transport cancellation closes the pipe from outside the busy input lock, and child liveness is checked between strokes. Paired releases are attempted for this stroke's recorded keys/modifiers. Pointer coordinates still match capture scaling; mismatched geometry and invalid coordinates refuse. Scroll uses a checked owned press/release pair rather than inheriting unchecked legacy wheel injection.

## Actual tests and preserved negatives

An owned disposable-Xvfb fixture builds and launches the actual mnetd child entry. It observes X-server button state directly:

- Coordinator SIGKILL with injected left-button hold: surviving injector released it in approximately1-2ms in local short samples.
- Coordinator SIGSTOP: independent two-second lease expired and released the hold in approximately2.0s. The child-owned selection remained while the coordinator was stopped, then disappeared after input cleanup.
- Injector SIGKILL with held button: the button remained logically pressed. This negative is required, not hidden or relabeled as recovery.
- Externally preheld left button: child refused ownership and coordinator-loss cleanup did not release that external hold.

Additional actual-X11 process tests cover ordinary joined Close, refusal after retirement, second-child exclusion, reuse only after cleanup, selection theft releasing owned input without erasing a replacement owner, and malformed local commands retiring owned input. The existing actual native consent/control/pause/revoke/held-pointer-stop journey runs through the actual child executable at1024x768 and2048x1536. Backend unit tests retain no-remap text, interruption, Shift release and invalid-coordinate coverage.

The initial standalone experiment was not acceptance of production code. The later fixtures exercise the actual local process seam and built child entry. Results remain same-host synthetic-loopback/Xvfb evidence, not a physical keyboard/mouse, multi-machine, mixed-OS, active-grab, overlap-provenance or secure-desktop test.

Current-source build, Windows/Darwin cross-build, vet, vulnerability scan, uncached whole-repository race, ordinary/burst mesh proofs and source gates pass locally. Twenty focused race repetitions and both native display sizes pass. The final actual-child failure fixture observed coordinator SIGKILL release in2ms and local lease expiry in1.998s; these short samples are not a latency guarantee. Fresh hosted verification remains required. The preceding22294c7 exact-input checkpoint had all five [normal CI jobs](https://github.com/MaisySen/Mnet/actions/runs/44597680222) and [Pages](https://github.com/MaisySen/Mnet/actions/runs/44597679940) successful, with deployed docs byte-verified. That does not certify these later process changes. Multies, release/signing/TestFlight, real accounts and devices remain untouched; attended files/recording remain disabled.


## Idle child-death grant retirement

A follow-up actual-X11 protocol regression found that killing an approved child while no remote input was being sent left the parent session/control grant active. The parent discovered failure only when another input event arrived. The original regression failed before correction; it used real capture matching the child geometry, then killed the actual child and sent no further input.

Each admitted backend now exposes its terminal lifetime. The attended grant owner watches that exact lifetime and ends the session when the current backend stops, including idle exit or pipe retirement. Normal pause/revoke retires the old watcher before closing its backend. A late old-child exit therefore cannot end a newly resumed grant. Teardown joins these watchers, rather than leaving a detached failure notification behind. This retires session/grant state; it does not release XTEST input from a child that is already dead.

A source factory error is corrected as part of this boundary: a failed typed-nil process pointer must not be returned as a nonnil injector interface. Startup failure now returns an actual nil backend, tested with an absent executable. Readiness, geometry and cleanup checks remain unchanged.

The actual idle-child-death regression passes after the fix. A real-protocol lifetime test verifies old-exit/new-grant isolation, input progress on the new backend and terminal current-backend death without a later remote event. Twenty focused race repetitions, fresh full build/cross-build/vet/vulnerability/race, ordinary/burst mesh/source gates and both actual native display sizes pass locally. Fresh hosted verification is required for this follow-up. The preceding b608 source completed all five [normal CI jobs](https://github.com/MaisySen/Mnet/actions/runs/44602232022) and [Pages](https://github.com/MaisySen/Mnet/actions/runs/44602231481), with actual hosted failure-case logs and deployed docs verified. Its successful result does not certify this later correction.
