#!/usr/bin/env python3
"""Fail closed on high-signal tracked-source leaks without printing values."""
import pathlib
import re
import subprocess
import sys

RULES = {
    "private key": rb"-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----",
    "GitHub credential": rb"(?:ghp_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,})",
    "Google API credential": rb"AIza[A-Za-z0-9_-]{35}",
    "AWS access key": rb"AKIA[A-Z0-9]{16}",
    "Mnet node credential": rb"mnet_[A-Za-z0-9_-]{32}(?![A-Za-z0-9_])",
}


def scan(data):
    findings = []
    for label, pattern in RULES.items():
        for match in re.finditer(pattern, data):
            findings.append((label, data[:match.start()].count(b"\n") + 1))
    if b"\0" not in data[:8000]:
        text = data.decode("utf-8", "replace")
        for number, line in enumerate(text.splitlines(), 1):
            if "\u2013" in line or "\u2014" in line:
                findings.append(("forbidden dash", number))
    return findings


def main():
    result = subprocess.run(["git", "ls-files", "-z"], check=True, capture_output=True)
    failed = False
    for raw in result.stdout.split(b"\0"):
        if not raw:
            continue
        path = pathlib.Path(raw.decode())
        if path.is_symlink() or not path.is_file():
            continue
        for label, line in scan(path.read_bytes()):
            # Only location/type are emitted; never print the credential.
            print(f"FAIL {path}:{line}: {label}", file=sys.stderr)
            failed = True
    if failed:
        return 1
    print("clean: tracked-source credential patterns and dash style")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
