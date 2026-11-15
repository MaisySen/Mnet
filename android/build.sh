#!/bin/sh
# Build the Mnet Android APK without Gradle: gomobile for the Go core,
# then the SDK build-tools directly (aapt2, javac, d8, zipalign,
# apksigner). Produces mnet.apk signed with a debug key.
#
# Usage: android/build.sh [path/to/mnet.aar]
#
# Needs: go, gomobile + gobind in PATH, javac (JDK 17), and
# ANDROID_HOME + ANDROID_NDK_HOME set.

set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
APP=$ROOT/android/app
OUT=$ROOT/android/out
SDK=${ANDROID_HOME:?"set ANDROID_HOME"}
BT=$SDK/build-tools/34.0.0
AJAR=$SDK/platforms/android-34/android.jar
PKG=au.fintex.mnet

AAR=${1:-$ROOT/mobile/mnet.aar}
if [ ! -f "$AAR" ]; then
    echo "building mnet.aar with gomobile"
    (cd "$ROOT" && gomobile bind -target android -androidapi 24 -o mobile/mnet.aar fintex.au/mnet/mobile)
fi

rm -rf "$OUT"
mkdir -p "$OUT/aar" "$OUT/classes" "$OUT/dex"
unzip -q -o "$AAR" -d "$OUT/aar"

# Resources and manifest.
"$BT/aapt2" compile --dir "$APP/src/main/res" -o "$OUT/res.zip"
"$BT/aapt2" link \
    -o "$OUT/mnet-base.apk" \
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

# Dex everything.
"$BT/d8" --lib "$AJAR" --output "$OUT/dex" \
    $(find "$OUT/classes" -name '*.class') \
    "$OUT/aar/classes.jar"

# Assemble: base apk + classes.dex + native libs per ABI.
cp "$OUT/mnet-base.apk" "$OUT/mnet-unaligned.apk"
cd "$OUT"
zip -q mnet-unaligned.apk dex/classes.dex -j
mkdir -p lib
cp -r aar/jni/* lib/
zip -q -r mnet-unaligned.apk lib
cd "$ROOT"

"$BT/zipalign" -f 4 "$OUT/mnet-unaligned.apk" "$OUT/mnet-unsigned.apk"

# Signing: release upload key when MNET_RELEASE_KEYSTORE is set, otherwise
# the per-checkout debug key. The release keystore lives outside the repo
# (default ~/keystores/mnet-upload.keystore) and its password is read from
# the file in MNET_RELEASE_PASS_FILE, never hardcoded here.
if [ -n "$MNET_RELEASE_KEYSTORE" ]; then
    KS="$MNET_RELEASE_KEYSTORE"
    ALIAS="${MNET_RELEASE_ALIAS:-mnet-upload}"
    PASS=$(cat "${MNET_RELEASE_PASS_FILE:?set MNET_RELEASE_PASS_FILE}")
    PASS_ARG="pass:$PASS"
else
    KS="$ROOT/android/debug.keystore"
    ALIAS="mnet-debug"
    PASS_ARG="pass:android"
    if [ ! -f "$KS" ]; then
        keytool -genkeypair -v -keystore "$KS" -storepass android -keypass android \
            -alias mnet-debug -keyalg RSA -keysize 2048 -validity 10000 \
            -dname "CN=Mnet Debug, O=Fintex Australia" >/dev/null 2>&1
    fi
fi
"$BT/apksigner" sign --ks "$KS" --ks-pass "$PASS_ARG" --key-pass "$PASS_ARG" \
    --ks-key-alias "$ALIAS" \
    --out "$OUT/mnet.apk" "$OUT/mnet-unsigned.apk"
"$BT/apksigner" verify --print-certs "$OUT/mnet.apk" | head -2

echo "OK: $OUT/mnet.apk"
