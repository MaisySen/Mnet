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
