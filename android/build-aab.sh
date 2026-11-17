#!/bin/sh
# Build the Mnet Android App Bundle (.aab) without Gradle: gomobile for the
# Go core, aapt2 --proto-format for the base module, bundletool for the
# bundle, jarsigner for release signing.
#
# Usage: android/build-aab.sh [path/to/mnet.aar]
#
# Needs: go, gomobile + gobind in PATH, JDK 17, ANDROID_HOME set, and
# bundletool.jar at $ANDROID_HOME/bundletool.jar.
# Release signing: set MNET_RELEASE_KEYSTORE and MNET_RELEASE_PASS_FILE
# (defaults: ~/keystores/mnet-upload.keystore). Without them the bundle is
# signed with the per-checkout debug key so the pipeline can be tested.

set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
APP=$ROOT/android/app
OUT=$ROOT/android/out-aab
SDK=${ANDROID_HOME:?"set ANDROID_HOME"}
BT=$SDK/build-tools/${MNET_ANDROID_BUILD_TOOLS:-36.0.0}
AJAR=$SDK/platforms/android-${MNET_ANDROID_COMPILE_API:-36}/android.jar
BUNDLETOOL=${BUNDLETOOL_JAR:-$SDK/bundletool.jar}

AAR=${1:-$ROOT/mobile/mnet.aar}
if [ ! -f "$AAR" ]; then
    echo "building mnet.aar with gomobile"
    (cd "$ROOT" && CGO_LDFLAGS="${CGO_LDFLAGS:-} -Wl,-z,max-page-size=16384 -Wl,-z,common-page-size=16384" gomobile bind -target android -androidapi 24 -o mobile/mnet.aar fintex.au/mnet/mobile)
fi

python3 "$ROOT/scripts/check-android-native-pages.py" "$AAR"

rm -rf "$OUT"
mkdir -p "$OUT/aar" "$OUT/classes" "$OUT/dex"
unzip -q -o "$AAR" -d "$OUT/aar"

# Resources and manifest in proto format (the App Bundle module format).
"$BT/aapt2" compile --dir "$APP/src/main/res" -o "$OUT/res.zip"
"$BT/aapt2" link \
    --proto-format \
    -o "$OUT/base.zip" \
    -I "$AJAR" \
    --manifest "$APP/src/main/AndroidManifest.xml" \
    --java "$OUT/gen" \
    "$OUT/res.zip"

# App Java plus the gomobile bindings.
find "$APP/src/main/java" "$OUT/gen" -name '*.java' > "$OUT/sources.txt"
javac -source 11 -target 11 \
    -cp "$AJAR:$OUT/aar/classes.jar" \
    -d "$OUT/classes" \
    @"$OUT/sources.txt"

"$BT/d8" --min-api 24 --lib "$AJAR" --output "$OUT/dex" \
    $(find "$OUT/classes" -name '*.class') \
    "$OUT/aar/classes.jar"

# Add dex and native libs into the base module zip, and move the proto
# manifest under manifest/ where bundletool expects it.
cd "$OUT"
mkdir -p lib manifest
cp -r aar/jni/* lib/
unzip -q -o base.zip AndroidManifest.xml -d manifest-extract
mv manifest-extract/AndroidManifest.xml manifest/AndroidManifest.xml
zip -q -d base.zip AndroidManifest.xml
zip -q base.zip manifest/AndroidManifest.xml dex/classes.dex
zip -q -r base.zip lib
cd "$ROOT"

# Build the bundle. modules are module zips; base is the base module.
java -jar "$BUNDLETOOL" build-bundle \
    --modules="$OUT/base.zip" \
    --output="$OUT/mnet.aab"

# Sign. Upload key by default; debug key if no release keystore is set.
if [ -n "${MNET_RELEASE_KEYSTORE:-}" ]; then
    KS="$MNET_RELEASE_KEYSTORE"
    ALIAS="${MNET_RELEASE_ALIAS:-mnet-upload}"
    PASS=$(cat "${MNET_RELEASE_PASS_FILE:?set MNET_RELEASE_PASS_FILE}")
else
    KS="$ROOT/android/debug.keystore"
    ALIAS="mnet-debug"
    PASS="android"
    if [ ! -f "$KS" ]; then
        keytool -genkeypair -v -keystore "$KS" -storepass android -keypass android \
            -alias mnet-debug -keyalg RSA -keysize 2048 -validity 10000 \
            -dname "CN=Mnet Debug, O=Fintex Australia" >/dev/null 2>&1
    fi
fi
jarsigner -keystore "$KS" -storepass "$PASS" -keypass "$PASS" \
    "$OUT/mnet.aab" "$ALIAS"
jarsigner -verify "$OUT/mnet.aab" | head -3

echo "OK: $OUT/mnet.aab"
