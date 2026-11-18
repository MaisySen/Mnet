#!/bin/sh
# Compile-only app API check and host Swift policy tests. No simulator launch,
# preferences access, VPN operation, signing, provisioning or account changes.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
OUT=$(mktemp -d "${TMPDIR:-/tmp}/mnet-ios-source.XXXXXX")
trap 'rm -rf "$OUT"' EXIT HUP INT TERM
python3 "$ROOT/scripts/test-ios-manager-wiring.py"
xcrun swiftc -swift-version 5 "$ROOT/ios/TunnelManagerPolicy.swift" "$ROOT/ios-tests/TunnelManagerPolicyTest.swift" -o "$OUT/policy-test"
"$OUT/policy-test"
SDK=$(xcrun --sdk iphonesimulator --show-sdk-path)
xcrun swiftc -swift-version 5 -typecheck -target arm64-apple-ios16.0-simulator -sdk "$SDK" \
    "$ROOT/ios/TunnelManagerPolicy.swift" "$ROOT/ios/ContentView.swift" "$ROOT/ios/MnetApp.swift"
printf '%s\n' 'PASS actual iOS app SwiftUI/NetworkExtension source typecheck; no extension/Go framework/signature/runtime acceptance'

xcrun swiftc -swift-version 5 "$ROOT/ios/PacketFlowBuffer.swift" "$ROOT/ios-tests/PacketFlowBufferTest.swift" -o "$OUT/flow-test"
"$OUT/flow-test"
# Generate the actual current Objective-C API, without building/linking a Go
# framework or signing anything. Swift imports it for extension API typecheck.
(cd "$ROOT" && go run golang.org/x/mobile/cmd/gobind -lang=objc -outdir="$OUT/objc" fintex.au/mnet/mobile)
printf '%s\n' 'module Mnet { header "Mobile.objc.h" export * }' > "$OUT/objc/src/gobind/module.modulemap"
xcrun swiftc -swift-version 5 -typecheck -target arm64-apple-ios16.0-simulator -sdk "$SDK" \
    -I "$OUT/objc/src/gobind" "$ROOT/ios/PacketFlowBuffer.swift" "$ROOT/ios/Flow.swift"
printf '%s\n' 'PASS current generated Objective-C PacketFlow API and actual Swift flow SDK typecheck; no binary linkage/device proof'
