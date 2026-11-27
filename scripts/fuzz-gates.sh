#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

# Bound the work by executions. Keep an independent test timeout for hangs.
go test -run '^$' -fuzz '^FuzzFrameDecoder$' -fuzztime=550000x -parallel=2 -timeout=5m ./internal/session
go test -run '^$' -fuzz '^FuzzParseResponse$' -fuzztime=1000000x -parallel=2 -timeout=5m ./internal/stun
