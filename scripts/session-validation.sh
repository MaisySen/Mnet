#!/bin/sh
# Isolated local mesh proof. Preserve artifacts on failure and success.
set -eu
cd "$(dirname "$0")/.."
TMP=$(mktemp -d /tmp/mnet-session.XXXXXX)
CPID= XVFB= SWAP= PROOF=
cleanup() {
    for pid in "$PROOF" "$SWAP" "$XVFB" "$CPID"; do
        if [ -n "$pid" ]; then kill "$pid" 2>/dev/null || true; fi
    done
    for pid in "$PROOF" "$SWAP" "$XVFB" "$CPID"; do
        if [ -n "$pid" ]; then wait "$pid" 2>/dev/null || true; fi
    done
}
trap cleanup EXIT
trap 'exit 130' INT TERM
printf 'Session artifacts: %s\n' "$TMP"
go build -o "$TMP/mnet-control" ./cmd/mnet-control
go build -o "$TMP/mnetd" ./cmd/mnetd
# The control server accepts a free loopback port selected for this run.
PORT=$(python3 - <<'PY'
import socket
with socket.socket() as s:
    s.bind(('127.0.0.1', 0))
    print(s.getsockname()[1])
PY
)
BASE=http://127.0.0.1:$PORT
"$TMP/mnet-control" -listen "127.0.0.1:$PORT" -data "$TMP/control.json" -admin-password valpass123 -base-url "$BASE" >"$TMP/control.log" 2>&1 &
CPID=$!
for attempt in $(seq 1 100); do
    if curl -sf --max-time 1 "$BASE/healthz" >"$TMP/health.json"; then break; fi
    kill -0 "$CPID" || { cat "$TMP/control.log"; exit 1; }
    sleep .1
done
curl -sf --max-time 10 "$BASE/admin/api/login" -X POST -H 'Content-Type: application/json' -d '{"password":"valpass123"}' -c "$TMP/cookies.txt" >"$TMP/login.json"
curl -sf --max-time 10 -b "$TMP/cookies.txt" "$BASE/admin/api/networks" -X POST -H 'Content-Type: application/json' -d '{"name":"sessval"}' >"$TMP/network.json"
NETID=$(jq -er .id "$TMP/network.json")
curl -sf --max-time 10 -b "$TMP/cookies.txt" "$BASE/admin/api/authkeys" -X POST -H 'Content-Type: application/json' -d "{\"network_id\":\"$NETID\",\"reusable\":true}" >"$TMP/authkey.json"
KEY=$(jq -er .key "$TMP/authkey.json")
# Xvfb allocates its own display, so concurrent/repeated proofs cannot
# overwrite each other's wallpaper or kill an unrelated display server.
Xvfb -noreset -displayfd 3 -screen 0 1280x800x24 3>"$TMP/display-number" >"$TMP/xvfb.log" 2>&1 &
XVFB=$!
for attempt in $(seq 1 100); do
    [ -s "$TMP/display-number" ] && break
    kill -0 "$XVFB" || { cat "$TMP/xvfb.log"; exit 1; }
    sleep .1
done
export DISPLAY=:$(cat "$TMP/display-number")
xsetroot -solid '#203864'
# Change only after a decoded initial frame is saved, rather than racing
# tunnel startup with a fixed sleep. xsetroot exits, no wallpaper process.
(
    for attempt in $(seq 1 600); do
        if [ -s "$TMP/proof/frame-first.jpg" ]; then
            xsetroot -solid '#5a1e1e'
            printf 'changed\n' >"$TMP/wallpaper-changed"
            exit 0
        fi
        sleep .1
    done
    exit 1
) >"$TMP/swap.log" 2>&1 &
SWAP=$!
MNET_SESSION_TRACE=${MNET_SESSION_TRACE:-1} "$TMP/mnetd" -session-proof -server "$BASE" -network sessval -auth-key "$KEY" -proof-dir "$TMP/proof" >"$TMP/proof.log" 2>&1 &
PROOF=$!
if wait "$PROOF"; then RC=0; else RC=$?; fi
PROOF=
if [ "$RC" -eq 0 ]; then
    echo 'SESSION VALIDATION PASS'
    cat "$TMP/proof/summary.json"
else
    echo 'SESSION VALIDATION FAIL'
    tail -40 "$TMP/proof.log"
fi
printf '\nArtifacts retained: %s\n' "$TMP"
exit "$RC"
