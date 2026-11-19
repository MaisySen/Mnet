#!/bin/sh
# Native macOS SwiftUI component pixels only. No iOS simulator, app install,
# preferences, provider installation, tunnel launch, signing or account access.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
OUT=$(mktemp -d "${TMPDIR:-/tmp}/mnet-swiftui-render.XXXXXX")
trap 'rm -rf "$OUT"' EXIT HUP INT TERM
: "${MNET_SWIFTUI_OUTPUT:?Set MNET_SWIFTUI_OUTPUT for synthetic PNG output}"
python3 "$ROOT/scripts/test-ios-layout-safety.py"
# Interpret these two sources, rather than linking a new host executable whose
# Apple-silicon default would add an ad-hoc signature. No app is built/installed.
cat "$ROOT/ios/MnetConnectionPanel.swift" "$ROOT/ios-tests/MnetLayoutRender.swift" > "$OUT/main.swift"
printf '%s\n' 'try await MnetLayoutRender.main()' >> "$OUT/main.swift"
xcrun swift -swift-version 5 "$OUT/main.swift"
printf '%s\n' 'PASS shared production SwiftUI component render; macOS renderer, not iOS visual/runtime acceptance'
