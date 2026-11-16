#!/bin/sh
# Real selected app Java on Robolectric's simulated Android framework, with
# explicit Go/VPN/accessibility fakes. No emulator, JNI, screen or device proof.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
MVN=${MVN:-mvn}
REPO=${MNET_MAVEN_REPO:-$HOME/.m2/repository}
OUT=$(mktemp -d "${TMPDIR:-/tmp}/mnet-framework.XXXXXX")
trap 'rm -rf "$OUT"' EXIT HUP INT TERM
cp "$ROOT/android/framework-tests/pom.xml" "$OUT/pom.xml"
mkdir -p "$OUT/src/main/java/au/fintex/mnet" "$OUT/src/test/java" "$OUT/libs"
cp -R "$ROOT/android/framework-tests/stubs/." "$OUT/src/main/java/"
cp -R "$ROOT/android/framework-tests/tests/." "$OUT/src/test/java/"
for source in HostSessionService SessionConsent ProjectionGuard VpnPlatform CaptureEncoder; do
    cp "$ROOT/android/app/src/main/java/au/fintex/mnet/$source.java" "$OUT/src/main/java/au/fintex/mnet/"
done
cd "$OUT"
"$MVN" -B -Dmaven.repo.local="$REPO" test-compile
# Maven resolves AARs but does not put their nested classes.jar on its JVM
# classpath. Extract the pinned AndroidX transitive runtime artifacts only.
python3 - "$REPO" "$OUT/libs" <<'PY'
from pathlib import Path
import sys, zipfile
repo, output = map(Path, sys.argv[1:])
for source, name in [
    ('androidx/test/monitor/1.8.0/monitor-1.8.0.aar', 'monitor.jar'),
    ('androidx/test/espresso/espresso-idling-resource/3.7.0/espresso-idling-resource-3.7.0.aar', 'espresso.jar')]:
    with zipfile.ZipFile(repo / source) as archive:
        (output / name).write_bytes(archive.read('classes.jar'))
PY
"$MVN" -B -Dmaven.repo.local="$REPO" test
printf '%s\n' 'PASS selected Android service/helper framework simulation; Go/VPN/accessibility are fakes, no emulator/device/capture/manifest permission enforcement proof'
