#!/bin/bash
# Session validation: a local mnet-control, then mnetd -session-proof
# runs two nodes on userspace tunnels and proves a full remote-desktop
# session between them through the encrypted mesh: X11 capture from an
# Xvfb display, delta frames, chat, file transfer, input injection.
# Everything dies when the script exits. Needs: go, Xvfb, ImageMagick.
set -u
cd "$(dirname "$0")/.."

TMP=/tmp/sessval
rm -rf $TMP && mkdir -p $TMP

go build -o $TMP/mnet-control ./cmd/mnet-control || exit 1
go build -o $TMP/mnetd ./cmd/mnetd || exit 1

$TMP/mnet-control -listen 127.0.0.1:18081 -data $TMP/control.json -admin-password valpass123 -base-url http://127.0.0.1:18081 >$TMP/control.log 2>&1 &
CPID=$!
sleep 1
curl -sf http://127.0.0.1:18081/admin/api/login -X POST -H 'Content-Type: application/json' -d '{"password":"valpass123"}' -c $TMP/cookies.txt -o /dev/null || { echo "CONTROL LOGIN FAIL"; cat $TMP/control.log; kill $CPID; exit 1; }
NET=$(curl -sf --max-time 10 -b $TMP/cookies.txt http://127.0.0.1:18081/admin/api/networks -X POST -H 'Content-Type: application/json' -d '{"name":"sessval"}')
NETID=$(echo "$NET" | jq -r .id)
KEY=$(curl -sf --max-time 10 -b $TMP/cookies.txt http://127.0.0.1:18081/admin/api/authkeys -X POST -H 'Content-Type: application/json' -d "{\"network_id\":\"$NETID\",\"reusable\":true}" | jq -r .key)
echo "control up, network $NETID, auth key created"

# A display with known content; swapped mid-run to exercise deltas.
Xvfb :99 -screen 0 1280x800x24 >$TMP/xvfb.log 2>&1 &
XVFB=$!
sleep 2
export DISPLAY=:99
convert -size 1280x800 xc:'#203864' -fill white -pointsize 72 -gravity center -annotate +0-40 "MNET SESSION PROOF" $TMP/proof1.png
convert -size 1280x800 xc:'#5a1e1e' -fill yellow -pointsize 72 -gravity center -annotate +0-40 "SCREEN CHANGED" $TMP/proof2.png
display -window root $TMP/proof1.png >/dev/null 2>&1 &
DISP=$!
sleep 1
( sleep 20; display -window root $TMP/proof2.png ) &
SWAP=$!

$TMP/mnetd -session-proof -server http://127.0.0.1:18081 -network sessval -auth-key "$KEY" -proof-dir $TMP/proof >$TMP/proof.log 2>&1 &
PPID2=$!
wait $PPID2; RC=$?

kill $SWAP $DISP $XVFB $CPID 2>/dev/null
if [ $RC -eq 0 ]; then
    echo "SESSION VALIDATION PASS"
    echo "artifacts in $TMP/proof (frame-first.jpg, frame-latest.jpg, summary.json)"
    cat $TMP/proof/summary.json
else
    echo "SESSION VALIDATION FAIL"; tail -20 $TMP/proof.log
fi
exit $RC
