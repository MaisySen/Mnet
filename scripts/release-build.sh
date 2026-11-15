#!/bin/sh
# Builds the release binaries into dist/. Usage: scripts/release-build.sh
set -eu
cd "$(dirname "$0")/.."
V=$(grep -o '"[0-9.]*"' internal/version/version.go | tr -d '"')
mkdir -p dist
rm -f dist/mnet* dist/SHA256SUMS
for target in linux-amd64 linux-arm64 darwin-arm64 windows-amd64; do
    GOOS=${target%-*}
    GOARCH=${target#*-}
    ext=""
    [ "$GOOS" = windows ] && ext=".exe"
    for bin in mnet mnetd mnet-control; do
        echo "building $bin $target"
        GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" \
            -o "dist/${bin}-${target}${ext}" "./cmd/$bin"
    done
done
(cd dist && sha256sum mnet* > SHA256SUMS)
echo "release $V binaries in dist/"
