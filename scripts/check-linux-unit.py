#!/usr/bin/env python3
"""Actual CLI print-only boundary checks against a freshly built Linux daemon."""
import pathlib
import subprocess
import sys
import tempfile

binary = str(pathlib.Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix="mnet-unit-check-") as root:
    state = pathlib.Path(root) / "uninitialized-state"
    base = [binary, "-state", str(state), "-service", "print-unit"]
    result = subprocess.run(base, capture_output=True, text=True, check=True)
    assert "[Service]" in result.stdout and " -up\n" in result.stdout
    assert not state.exists(), "print-unit initialized device state"
    assert not result.stderr, "successful generation printed diagnostics"
    for extra in [["-auth-key", "fixture-private-value"], ["-network", "fixture"],
                  ["-service", "install"], ["-service", "remove"], ["-service", "run"]]:
        rejected = subprocess.run(base + extra, capture_output=True, text=True)
        assert rejected.returncode != 0 and not rejected.stdout, "unsupported action emitted unit"
        assert "fixture-private-value" not in rejected.stderr, "secret echoed"
        assert not state.exists(), "rejected action initialized device state"
print("actual CLI: read-only generation and rejected mutation/secret options passed")
