#!/bin/bash
# Public validation: mnet-control behind a public localhost.run tunnel,
# two real mnetd processes join over the public URL and exchange
# WireGuard traffic. Everything dies when the script exits.
set -u
cd "$(dirname "$0")/.."

TMP=/tmp/pubval
rm -rf $TMP && mkdir -p $TMP/a $TMP/b

go build -o $TMP/mnet-control ./cmd/mnet-control || exit 1
go build -o $TMP/mnetd ./cmd/mnetd || exit 1

$TMP/mnet-control -listen 127.0.0.1:18080 -data $TMP/control.json -admin-password valpass123 -base-url http://127.0.0.1:18080 >$TMP/control.log 2>&1 &
CPID=$!
sleep 1
curl -sf http://127.0.0.1:18080/admin/api/login -X POST -H 'Content-Type: application/json' -d '{"password":"valpass123"}' -c $TMP/cookies.txt -o /dev/null || { echo "CONTROL LOGIN FAIL"; cat $TMP/control.log; kill $CPID; exit 1; }
echo "control up locally"

# Public tunnel.
ssh -o StrictHostKeyChecking=no -o ExitOnForwardFailure=yes -p 443 -R0:127.0.0.1:18080 a.pinggy.io >$TMP/tunnel.log 2>&1 &
TPID=$!
PUB=""
for i in $(seq 1 25); do
    PUB=$(grep -o 'https://[a-z0-9.-]*\(free\.pinggy\.net\|run\.pinggy-free\.link\)' $TMP/tunnel.log | head -1)
    [ -n "$PUB" ] && break
    sleep 1
done
if [ -z "$PUB" ]; then echo "TUNNEL FAIL"; cat $TMP/tunnel.log; kill $CPID $TPID 2>/dev/null; exit 1; fi
echo "public URL: $PUB"
curl -sf --max-time 10 "$PUB/admin/api/login" -X POST -H 'Content-Type: application/json' -d '{"password":"valpass123"}' -c $TMP/cookies.txt -o /dev/null || { echo "PUBLIC LOGIN FAIL"; kill $CPID $TPID 2>/dev/null; exit 1; }
echo "control reachable over public URL"

# Network + reusable auth key through the public URL.
NET=$(curl -sf --max-time 10 -b $TMP/cookies.txt "$PUB/admin/api/networks" -X POST -H 'Content-Type: application/json' -d '{"name":"pubval"}')
NETID=$(echo "$NET" | jq -r .id)
KEY=$(curl -sf --max-time 10 -b $TMP/cookies.txt "$PUB/admin/api/authkeys" -X POST -H 'Content-Type: application/json' -d "{\"network_id\":\"$NETID\",\"reusable\":true}" | jq -r .key)
echo "network $NETID, auth key created"

# Two real daemons join over the public URL. First node gets 100.64.0.1,
# second 100.64.0.2.
$TMP/mnetd -state $TMP/a -fake-tun -self-test 100.64.0.2 -server "$PUB" -network pubval -name val-a -auth-key "$KEY" >$TMP/a.log 2>&1 &
APID=$!
sleep 2
$TMP/mnetd -state $TMP/b -fake-tun -self-test 100.64.0.1 -server "$PUB" -network pubval -name val-b -auth-key "$KEY" >$TMP/b.log 2>&1 &
BPID=$!

RC=0
for i in $(seq 1 24); do
    A=$(grep -c "self-test: PASS" $TMP/a.log 2>/dev/null); A=${A:-0}
    B=$(grep -c "self-test: PASS" $TMP/b.log 2>/dev/null); B=${B:-0}
    [ "$A" -ge 1 ] && [ "$B" -ge 1 ] && break
    sleep 5
done
echo "--- node A:"; tail -3 $TMP/a.log
echo "--- node B:"; tail -3 $TMP/b.log
grep -q "self-test: PASS" $TMP/a.log && grep -q "self-test: PASS" $TMP/b.log && echo "PUBLIC_VALIDATION_PASS" || { echo "PUBLIC_VALIDATION_FAIL"; RC=1; }
kill $APID $BPID $CPID $TPID 2>/dev/null
exit $RC
