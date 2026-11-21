#!/usr/bin/env python3
"""Serialize real finite-freshness fault tests. Not a performance/stability test."""
import argparse
import fcntl
import os
from pathlib import Path
import stat
import subprocess
import tempfile
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--repo', type=Path, default=Path(__file__).resolve().parent.parent)
parser.add_argument('--binary', type=Path, help='Existing compiled candidate test binary')
parser.add_argument('--wait-seconds', type=float, default=180)
args = parser.parse_args()
root = Path(tempfile.gettempdir()) / ('mnet-admission-validation-' + str(os.getuid()))
try:
    root.mkdir(mode=0o700)
except FileExistsError:
    pass
info = root.lstat()
if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o700:
    raise SystemExit('Refuse non-private validation lock directory')
fd = os.open(root / 'fault-matrix.lock', os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
info = os.fstat(fd)
if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_nlink != 1:
    raise SystemExit('Refuse unexpected validation lock file')
started = time.monotonic()
print('Waiting for isolated finite-freshness fault-matrix slot', flush=True)
while True:
    try:
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        break
    except BlockingIOError:
        if time.monotonic() - started >= args.wait_seconds:
            raise SystemExit('Isolation wait expired, matrix did not run')
        time.sleep(0.1)
print('Isolation acquired after %.3fs. No freshness intervals changed.' % (time.monotonic() - started), flush=True)
with tempfile.TemporaryDirectory(prefix='candidate-', dir=root) as tmp:
    binary = args.binary.resolve() if args.binary else Path(tmp) / 'control.test'
    if not args.binary:
        subprocess.run(['go', 'test', '-race', '-tags', 'mnet_enrollment_development', '-c', '-o', str(binary), './internal/control'], cwd=args.repo, check=True, timeout=120)
    result = subprocess.run([str(binary), '-test.run=^TestAdmissionV2FaultMatrixOnly$', '-test.v', '-test.timeout=60s'], cwd=args.repo / 'internal/control', timeout=75)
print('Isolated matrix exit=%d. This is not an all-load stability claim.' % result.returncode, flush=True)
raise SystemExit(result.returncode)
