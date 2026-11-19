#!/bin/sh
# Native Android View rasterization on Robolectric, not an emulator/device test.
# Uses the actual packaged app resources; VPN/Go/accessibility endpoints are fakes.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
MVN=${MVN:-mvn}
REPO=${MNET_MAVEN_REPO:-$HOME/.m2/repository}
SDK=${ANDROID_HOME:?set ANDROID_HOME}
OUTPUT=${MNET_LAYOUT_OUTPUT:?set MNET_LAYOUT_OUTPUT to an absolute output directory}
case "$OUTPUT" in /*) ;; *) echo 'MNET_LAYOUT_OUTPUT must be absolute'; exit 1;; esac
mkdir -p "$OUTPUT"
OUT=$(mktemp -d "${TMPDIR:-/tmp}/mnet-layout.XXXXXX")
trap 'rm -rf "$OUT"' EXIT HUP INT TERM
cp "$ROOT/android/framework-tests/pom.xml" "$OUT/pom.xml"
mkdir -p "$OUT/src/main/java/au/fintex/mnet" "$OUT/src/test/java" "$OUT/src/test/resources/com/android/tools" "$OUT/libs"
cp -R "$ROOT/android/framework-tests/stubs/." "$OUT/src/main/java/"
rm "$OUT/src/main/java/au/fintex/mnet/R.java"
cp -R "$ROOT/android/framework-tests/layout-tests/." "$OUT/src/test/java/"
for source in AppUi ContentInsets MainActivity HostingActivity HostSessionService SessionControl SessionConsent ProjectionGuard VpnPlatform CaptureEncoder ViewerActivity ViewerGate ViewerTap; do
    cp "$ROOT/android/app/src/main/java/au/fintex/mnet/$source.java" "$OUT/src/main/java/au/fintex/mnet/"
done
BT=$SDK/build-tools/${MNET_ANDROID_BUILD_TOOLS:-36.0.0}
"$BT/aapt2" compile --dir "$ROOT/android/app/src/main/res" -o "$OUT/res.zip"
"$BT/aapt2" link -o "$OUT/resources.apk" -I "$SDK/platforms/android-${MNET_ANDROID_COMPILE_API:-36}/android.jar" --manifest "$ROOT/android/app/src/main/AndroidManifest.xml" --java "$OUT/src/main/java" "$OUT/res.zip"
printf 'android_merged_manifest=%s\nandroid_resource_apk=%s\nandroid_custom_package=au.fintex.mnet\n' "$ROOT/android/app/src/main/AndroidManifest.xml" "$OUT/resources.apk" > "$OUT/src/test/resources/com/android/tools/test_config.properties"
python3 - "$REPO" "$OUT/libs" <<'PY'
from pathlib import Path
import sys,zipfile
repo,out=map(Path,sys.argv[1:])
for path,name in [('androidx/test/monitor/1.8.0/monitor-1.8.0.aar','monitor.jar'),('androidx/test/espresso/espresso-idling-resource/3.7.0/espresso-idling-resource-3.7.0.aar','espresso.jar')]:
    with zipfile.ZipFile(repo/path) as z:(out/name).write_bytes(z.read('classes.jar'))
PY
cd "$OUT"
"$MVN" -B -Dmaven.repo.local="$REPO" -Dmnet.layout.output="$OUTPUT" test
printf '%s\n' 'Layout pixels only: actual app Views/resources, simulated API33/native rasterizer. No emulator/JNI/VPN/physical-device acceptance.'
