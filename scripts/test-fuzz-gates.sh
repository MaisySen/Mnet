#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

scratch=$(mktemp -d)
trap 'rm -f "$scratch/go" "$scratch/calls" "$scratch/expected" "$scratch/first"; rmdir "$scratch"' EXIT
cat >"$scratch/go" <<'SH'
#!/bin/sh
printf '%s\n' "$*" >>"$FUZZ_GATE_CALLS"
case "$*" in
    *"$FUZZ_GATE_FAIL_PACKAGE") exit 23 ;;
esac
SH
chmod +x "$scratch/go"
cat >"$scratch/expected" <<'EXPECTED'
test -run ^$ -fuzz ^FuzzFrameDecoder$ -fuzztime=550000x -parallel=2 -timeout=5m ./internal/session
test -run ^$ -fuzz ^FuzzParseResponse$ -fuzztime=1000000x -parallel=2 -timeout=5m ./internal/stun
EXPECTED
head -n 1 "$scratch/expected" >"$scratch/first"

export PATH="$scratch:$PATH" FUZZ_GATE_CALLS="$scratch/calls"
FUZZ_GATE_FAIL_PACKAGE=none sh scripts/fuzz-gates.sh
diff -u "$scratch/expected" "$FUZZ_GATE_CALLS"

for package in ./internal/session ./internal/stun; do
    : >"$FUZZ_GATE_CALLS"
    status=0
    FUZZ_GATE_FAIL_PACKAGE="$package" sh scripts/fuzz-gates.sh || status=$?
    if [ "$status" -ne 23 ]; then
        printf 'Fuzz gate swallowed a failed Go command for %s (status %s)\n' "$package" "$status" >&2
        exit 1
    fi
    case "$package" in
        ./internal/session) diff -u "$scratch/first" "$FUZZ_GATE_CALLS" ;;
        ./internal/stun) diff -u "$scratch/expected" "$FUZZ_GATE_CALLS" ;;
    esac
done
printf 'Fuzz gate commands and failure propagation passed.\n'
