#!/bin/sh
# Verify signed-archive evidence as well as exit status. jarsigner can return
# zero for an unsigned ZIP, so successful execution alone is insufficient.
set -eu
BUNDLE=${1:?supply App Bundle path}
RESULT=${2:?supply private verification report path}
LC_ALL=C jarsigner -verify "$BUNDLE" > "$RESULT" 2>&1
if ! grep -Fxq 'jar verified.' "$RESULT"; then
    printf '%s\n' 'App Bundle signature verification did not confirm a signed archive' >&2
    exit 1
fi
head -3 "$RESULT"
