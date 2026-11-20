# Unsigned iOS app and tunnel linkage check

The full Swift SDK typecheck establishes API conformance, not linkage to the current Go core or correct extension packaging. The next CI step builds a disposable arm64 iOS-simulator xcframework from the module-pinned gomobile/gobind tools, generates the actual Xcode app/extension project, and builds without launching anything.

The project spec had root-relative paths despite living in ios/, no app dependency embedding MnetTunnel, and an APPL package type in the extension plist. Paths are now relative to the spec, app sources are explicit, the tunnel is an embedded target dependency and its package type is XPC!. Only the extension links the static Go xcframework; a static Go binary is not copied as a dynamic embedded framework.

XcodeGen is pinned to2.46.0 and the downloaded archive is checked against SHA2564d9e34b62172d645eed6457cac13fc222569974098ef4ee9c3368bedf0196806. This hash was read from the release API and independently matched against the downloaded archive. Source: https://github.com/yonaskolb/XcodeGen/releases/tag/2.46.0 . Spec path/dependency semantics: https://yonaskolb.github.io/XcodeGen/Docs/ProjectSpec.html .

The build forces simulator SDK/arm64, CODE_SIGNING_ALLOWED=NO, CODE_SIGNING_REQUIRED=NO, empty signing identity/team and linker no-ad-hoc-codesign. No provisioning flag, account, signing credential, physical device or simulator launch is used. A product checker requires the embedded .appex, exact bundle/package/principal-class identity, both actual arm64 Mach-O executables, absence of code signatures and Go bridge/runtime symbols in the tunnel executable. Outputs remain in the temporary directory and are deleted; there is no release, distribution artifact or upload.

Local shell/Python/source checks are not a successful Apple build. Exact-revision CI must be inspected for the actual linker/package check before claiming linked acceptance. Any signing or account requirement is a stop boundary, not permission to register or provision. Even a pass would mean unsigned simulator compile/link/package only, not launch, entitlement enforcement, actual PacketTunnelFlow/OS VPN startup/recovery, signed extension delivery, Multies enrollment or production readiness. No UI layout is changed or accepted by this check.


The initial run completed xcodebuild without a signing/account request, then stopped in verification because codesign's diagnostic text was not a reliable unsigned predicate. The checker now inspects every actual executable's Mach-O load commands and rejects LC_CODE_SIGNATURE, including ad-hoc signatures, rather than trusting a diagnostic phrase. The build also disables debug-dylib splitting so bridge symbols belong to the checked extension executable. A module-name warning exposed the app module Mnet colliding with the Go module; the app module is now MnetApp. The initial result is partial build evidence, not accepted unsigned packaging or runtime. The corrected product checks must pass before completion.

## Exact simulator platform gate

The linked-product checker now requires one arm64 Mach-O LC_BUILD_VERSION for iOS Simulator (platform7), rather than accepting any arm64 Mach-O reported by file. Device iOS, macOS, Catalyst, other simulator platforms, missing/duplicate platform commands and legacy ambiguous minimum-version commands refuse acceptance. Signature rejection remains in place. Command headers, eight-byte alignment, declared command lengths/counts and build-tool entries are checked before reading. Malformed input raises a bounded diagnostic rather than reading beyond a declared command region.

Six synthetic parser tests cover every truncation of the minimal valid binary, wrong architecture/magic, device and other platform confusion, duplicate/missing platform declarations, signature/legacy commands and malformed counts/sizes/tool lists. They run normally and with Python optimization to ensure these parser checks do not disappear with assertions disabled. These are fixture tests only. The updated parser still needs a new exact-revision actual macOS linked-product run; no signed, launched or physical-device acceptance is implied. This does not close the containing-app entitlement/signing alignment gap or select a developer team.

Mach-O platform and command definitions: https://github.com/apple-oss-distributions/cctools/blob/7a5450708479bbff61527d5e0c32a3f7b7e4c1d0/include/mach-o/loader.h .

## Non-optional package identity checks

A following local checker pass requires MH_EXECUTE, refusing an arm64 object, dynamic library or bundle even when its platform metadata resembles the simulator. Bundle identity/package/executable, extension point/principal, signature-directory and linked Go bridge/runtime checks now use explicit refusal rather than Python assertions. They remain enforced when Python runs with optimization.

Twelve synthetic parser/package tests pass normally and optimized. Package fixtures exercise both app and extension mismatches, missing executables, unexpected signature directories, wrong provider roles and missing Go bridge/runtime symbols. Host inspection commands are mocked for these tests, so they do not establish actual linkage or signing. The full macOS linked-product check remains the separate required evidence. No app entitlement, signing team, provisioning, account or release setting changes in this slice.
