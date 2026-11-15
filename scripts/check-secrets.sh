#!/bin/sh
# Pre-push secret and style check, per the Fintex runbook rules.md.
# Run before every push: sh scripts/check-secrets.sh
set -eu
cd "$(dirname "$0")/.."
fail=0
# .github is excluded: workflows reference secret NAMES, never values.
if git grep -nIE 'BEGIN [A-Z ]*PRIVATE KEY|password *= *"[^"]+|token *= *"[a-zA-Z0-9]|\.p8|AIza[0-9A-Za-z_-]{20,}' -- . ':!vendor' ':!*_test.go' ':!docs' ':!.github' | grep -v check-secrets; then
    echo "POSSIBLE SECRET ABOVE"
    fail=1
fi
if git grep -n $'\xe2\x80\x93\|\xe2\x80\x94' -- . | grep -v check-secrets; then
    echo "EM OR EN DASH ABOVE (runbook forbids them)"
    fail=1
fi
[ "$fail" = 0 ] && echo "clean: no secrets, no em/en dashes"
exit $fail
