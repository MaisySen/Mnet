#!/bin/sh
# Portable tracked-source credential/style gate. Does not replace a full
# history/release-artifact disclosure audit or a maintained secret scanner.
set -eu
cd "$(dirname "$0")/.."
python3 scripts/check-source.py
