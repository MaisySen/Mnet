# Attended logical-text ownership, local engineering checkpoint

The new attended Linux/X11 mode must not inherit the legacy injector's global keyboard remapping or clipboard-paste behavior. It must also stop an in-flight text event when its exact control request, local owner, transport or session ends. This work is restricted to attended mode. The explicit legacy engineering backend is unchanged.

## Observed defect

An actual disposable-X11 regression called the existing attended injector with an unmapped compass character and compared the complete keyboard map before and after. It failed because that input changed a spare global X keycode. The preceding live-stop implementation correctly treats a mapping change as terminal, but a remote text event should not mutate the desktop mapping in the first place.

The legacy text routine also loops through every rune in one input event while holding the input dispatch lock. Closing the transport cannot safely be treated as proof that this already-running loop has stopped. The input owner now passes an exact event-bound liveness check into the attended backend. That check includes the current control session/request, peer/local-owner authorization, transport closure and session cancellation, rather than depending on teardown to first acquire the busy input lock.

## Pointer coordinates and bounds

A separate failing actual-X11 probe on a 2048x1536 display found that capture downscaled by2 while the separate injector stayed at scale1. Frame point20,20 therefore clicked desktop20,20 instead of40,40. The attended injector now initializes the matching default-root geometry and capture scale. Control admission refuses a backend whose frame geometry differs from its capture. Out-of-frame/negative/overflow-sized coordinates, unknown buttons/actions and empty scroll are refused instead of wrapping coordinates or treating an unknown button as left-click. The explicit legacy backend is unchanged.

The native fixture now runs at1024x768 and2048x1536 in normal CI. Its actual desktop pointer assertion scales from the physical display, instead of hardcoding the same20px expected value at both sizes. This does not establish multi-monitor, rotation, dynamic-resolution or arbitrary desktop-layout support.

## Bounded attended behavior

- Text is logical text only, not physical key-down/up, repeat, modifier forwarding, keyboard-layout synchronization, IME support or Shared Desk authority.
- A text event is limited to 4,096 UTF-8 bytes and 1,024 runes. Invalid or larger input is refused, never truncated.
- The full event is planned against the existing base-group level-0/Shift mapping before any key is emitted. Unavailable characters refuse the entire event without typing a supported prefix. Newline and tab use the existing Return/Tab keys. There is no global remapping or clipboard fallback.
- Modified local keyboard state, including locks/group modifiers that change interpretation, is refused rather than cleared or guessed. This limited engineering path does not promise full Unicode, every keyboard layout or consumer-ready text entry.
- Each stroke rechecks the exact event's liveness. Key and owned Shift releases are checked and attempted around that stroke even when cancellation intervenes. Only one stroke can be in flight between checks. Already-processed OS actions cannot be recalled.
- A text/backend failure ends the attended session and closes its owned input backend. It does not silently retry, continue with a different backend or broaden permissions.

X11 still has shared desktop state. A local application or physical event can race a state snapshot, and synthetic events cannot be distinguished as trusted physical input here. The change is not secure-desktop isolation or protection against arbitrary programs with access to the same X server.

## Verification and limits

The original keymap-changing regression failed before the new wrapper and passes afterward. Actual-X11 tests now check a byte-for-byte unchanged map, interrupted multi-stroke output, paired Shift release, and rejection of a later unavailable rune before emitting a prefix. Protocol/owner tests verify that a wrong request never reaches the checked backend, retirement during an event fails the event and closes its backend, and local End interrupts an in-flight checked input without waiting for the same busy dispatch lock. Twenty focused race repetitions and the actual native pointer-consent/live-stop journey pass locally.

The preceding exact-SHA checkpoint has finished. Current-source full build, Windows/Darwin cross-build, vet, vulnerability scan, fresh whole-repository race, ordinary/burst mesh checks and source gates pass locally. Both native display sizes and twenty focused race repetitions pass. Fresh hosted acceptance of this text/coordinate follow-up is still required. The preceding [7ef7d3a live-stop source](https://github.com/MaisySen/Mnet/commit/7ef7d3a4c06820202f42d293caaaa6ec9ae42748) passed all five [normal CI jobs](https://github.com/MaisySen/Mnet/actions/runs/44594539145), [Pages](https://github.com/MaisySen/Mnet/actions/runs/44594537966) and its [hosted native pointer journey](https://github.com/MaisySen/Mnet/actions/runs/44594539145/job/135809178170). That run does not verify these later text changes.

Independent negative tests on that preceding Mnet source established two remaining limits: another X client's active keyboard grab prevented the shortcut until the grab was released; killing the Mnet test process with SIGKILL left the XTEST left button logically pressed on the surviving server. The displays were disposable, the peer was synthetic loopback, and no personal desktop was driven. Session cleanup is not process-death recovery, silent-partition recovery or an always-available physical emergency stop. A separate cleanup process would need its own ownership and failure model, not a stronger description of an in-process handler.

Multies, TestFlight, signing, release distribution, real accounts and devices remain untouched. Files and recording remain unavailable in attended mode.
