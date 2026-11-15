#!/bin/sh
# Fail closed: no success-filtering pipelines, no cached test results.
# LOG_DIR retains the complete output and each stage's exit status.
set -eu
cd "$(dirname "$0")/.."
LOG_DIR=${LOG_DIR:-$(mktemp -d /tmp/mnet-gates.XXXXXX)}
mkdir -p "$LOG_DIR"
run() {
    name=$1; shift
    printf 'RUN %s\n' "$name"
    if "$@" >"$LOG_DIR/$name.log" 2>&1; then
        printf 'PASS %s\n' "$name"
    else
        rc=$?
        cat "$LOG_DIR/$name.log"
        printf 'FAIL %s (exit %s). Logs: %s\n' "$name" "$rc" "$LOG_DIR" >&2
        exit "$rc"
    fi
}
printf 'Logs: %s\n' "$LOG_DIR"
run build go build ./...
run windows env GOOS=windows go build ./...
run darwin env GOOS=darwin go build ./...
run vet go vet ./...
run race go test -race -count="${TEST_COUNT:-1}" ./...
run secrets sh scripts/check-secrets.sh
printf 'All local gates passed. This does not certify live platforms or scale.\n'
