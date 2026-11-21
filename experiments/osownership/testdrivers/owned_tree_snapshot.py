"""Inert observation library for a FUTURE separately reviewed wrapper.
No launch, namespace, signal, device access, retry, or executable entry point.
"""
import os
import time
from pathlib import Path

IMPLICIT_EFFECTS = {
    "driver_first_StartProcess": ["pidfd_open(self)", "waitid(self) expects ECHILD", "pidfd_send_signal(self,0)", "clone0x5100 transient child", "child exit0", "waitid-WCLONE/reap", "pidfd close"],
    "resident_first_StartProcess": ["same process-local pidfd feature check before real client"],
    "fixed_client": ["nonspawning", "explicit owner-signal0 EPERM", "exact filter unchanged"],
}


def identity(pid, proc_root=Path('/proc')):
    start = time.monotonic_ns()
    path = proc_root / str(pid)
    try:
        raw = (path / 'stat').read_text()
        fields = raw[raw.rindex(')')+1:].split()
        status = (path / 'status').read_text()
        tgid = [int(x.split(':', 1)[1]) for x in status.splitlines() if x.startswith('Tgid:')]
        if len(tgid) != 1:
            return {'pid': pid, 'observation_error': 'tgid-missing', 'read_start_ns': start, 'read_end_ns': time.monotonic_ns()}
        # Bounded raw cmdline for fixed no-secret owned command modes ONLY.
        with (path / 'cmdline').open('rb') as f:
            command = f.read(4097)
        if len(command) > 4096:
            return {'pid': pid, 'observation_error': 'cmdline-overflow', 'read_start_ns': start, 'read_end_ns': time.monotonic_ns()}
        exe = os.readlink(path / 'exe')
        # Bracket identity across the non-atomic reads; exec can still change
        # command/exe inside one lifetime. Do not pretend this is atomic.
        again = (path / 'stat').read_text()
        check = again[again.rindex(')')+1:].split()
        if fields[19] != check[19]:
            return {'pid': pid, 'observation_error': 'lifetime-changed', 'read_start_ns': start, 'read_end_ns': time.monotonic_ns()}
        return {'pid': pid, 'tgid': tgid[0], 'ppid': int(fields[1]), 'start_ticks': int(fields[19]), 'state': fields[0], 'exe': exe, 'cmdline': command.rstrip(b'\x00').decode('utf-8', errors='replace').split('\x00') if command else [], 'read_start_ns': start, 'read_end_ns': time.monotonic_ns()}
    except (OSError, ValueError, IndexError):
        return {'pid': pid, 'observation_error': 'unreadable-or-gone', 'read_start_ns': start, 'read_end_ns': time.monotonic_ns()}


def snapshot(driver, proc_root=Path('/proc')):
    result = {'collection_start_ns': time.monotonic_ns(), 'records': [], 'children_reads': []}
    queue = [driver]
    seen = set()
    while queue:
        pid = queue.pop(0)
        if pid in seen:
            continue
        seen.add(pid)
        record = identity(pid, proc_root)
        result['records'].append(record)
        if 'observation_error' in record:
            continue
        began = time.monotonic_ns()
        try:
            ids = [int(x) for x in (proc_root / str(pid) / 'task' / str(pid) / 'children').read_text().split()]
            read = {'pid': pid, 'children': ids, 'read_start_ns': began, 'read_end_ns': time.monotonic_ns()}
            queue.extend(ids)
        except (OSError, ValueError):
            read = {'pid': pid, 'observation_error': 'children-unreadable', 'read_start_ns': began, 'read_end_ns': time.monotonic_ns()}
        result['children_reads'].append(read)
    result['collection_end_ns'] = time.monotonic_ns()
    result['scope'] = 'task-leader children only, non-atomic, not syscall attribution'
    return result
