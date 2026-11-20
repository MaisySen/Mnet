#!/bin/sh
# Disposable unsigned simulator link/package check. No simulator boot, profile
# access, entitlement registration, credentials, signing or upload.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
python3 "$ROOT/scripts/test-ios-linked-build-safety.py"
python3 "$ROOT/scripts/test-ios-linked-macho.py"
python3 -O "$ROOT/scripts/test-ios-linked-macho.py"
OUT=$(mktemp -d "${TMPDIR:-/tmp}/mnet-ios-link.XXXXXX")
trap 'rm -rf "$OUT"' EXIT HUP INT TERM
cp -R "$ROOT/ios" "$OUT/ios"
python3 - "$OUT" <<'PY'
import hashlib, io, pathlib, sys, urllib.request, zipfile
out = pathlib.Path(sys.argv[1])
url = 'https://github.com/yonaskolb/XcodeGen/releases/download/2.46.0/xcodegen.zip'
with urllib.request.urlopen(url) as response:
    data = response.read()
assert hashlib.sha256(data).hexdigest() == '4d9e34b62172d645eed6457cac13fc222569974098ef4ee9c3368bedf0196806', 'XcodeGen archive hash mismatch'
with zipfile.ZipFile(io.BytesIO(data)) as archive:
    archive.extractall(out / 'tools')
(out / 'tools/xcodegen/bin/xcodegen').chmod(0o755)
PY
(cd "$ROOT" && go build -o "$OUT/gomobile" golang.org/x/mobile/cmd/gomobile && go build -o "$OUT/gobind" golang.org/x/mobile/cmd/gobind)
export PATH="$OUT:$PATH"
(cd "$ROOT" && "$OUT/gomobile" bind -target=iossimulator/arm64 -iosversion=16.0 -o "$OUT/ios/Mnet.xcframework" fintex.au/mnet/mobile)
"$OUT/tools/xcodegen/bin/xcodegen" generate --spec "$OUT/ios/project.yml" --project "$OUT/ios"
xcodebuild -project "$OUT/ios/Mnet.xcodeproj" -scheme Mnet -configuration Debug \
    -sdk iphonesimulator -destination 'generic/platform=iOS Simulator' \
    -derivedDataPath "$OUT/derived" ARCHS=arm64 ONLY_ACTIVE_ARCH=YES \
    MNET_BUILD_NUMBER=1 ENABLE_DEBUG_DYLIB=NO CODE_SIGNING_ALLOWED=NO CODE_SIGNING_REQUIRED=NO \
    CODE_SIGN_IDENTITY= DEVELOPMENT_TEAM= 'OTHER_LDFLAGS=$(inherited) -Wl,-no_adhoc_codesign' build
python3 "$ROOT/scripts/check-ios-linked-product.py" "$OUT/derived/Build/Products/Debug-iphonesimulator/Mnet.app"
