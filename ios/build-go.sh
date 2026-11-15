#!/bin/sh
# Build Mnet.xcframework for iOS from the Go node engine.
# Requires macOS with Xcode installed and gomobile + gobind in PATH.
set -eu
cd "$(dirname "$0")/.."
gomobile bind -target ios -o ios/Mnet.xcframework fintex.au/mnet/mobile
echo "OK: ios/Mnet.xcframework"
