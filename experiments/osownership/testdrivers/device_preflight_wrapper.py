"""SOURCE ONLY. Explicit separate reviewed execution permission required.
No actions on import. Use only exact fixed no-secret owned preflight binary.
"""
import hashlib
import json
import os
import signal
import stat
import subprocess
import time
from pathlib import Path
from owned_tree_snapshot import identity, snapshot, IMPLICIT_EFFECTS


def namespace():
    s = os.stat('/proc/self/ns/net')
    return {'device': s.st_dev, 'inode': s.st_ino}


def same_lifetime(old, now):
    return all(k in old and k in now and old[k] == now[k] for k in ('pid', 'start_ticks', 'exe', 'tgid'))


def checked_kill(record, sig):
    # No name/group cleanup and no signaling a replacement process.
    now = identity(record['pid'])
    if not same_lifetime(record, now):
        return False
    os.kill(record['pid'], sig)
    return True


def run_once(binary, expected_sha256, output_dir):
    binary = Path(binary)
    if os.getuid() == 0 or not binary.is_absolute() or binary.is_symlink():
        raise ValueError('unprivileged absolute nonsymlink binary required')
    bs = binary.stat()
    if not stat.S_ISREG(bs.st_mode) or hashlib.sha256(binary.read_bytes()).hexdigest() != expected_sha256:
        raise ValueError('binary identity mismatch')
    # O_EXCL-equivalent new directory, never overwrite an earlier run.
    output = Path(output_dir)
    output.mkdir(mode=0o700, parents=False, exist_ok=False)
    meta = {'version': 1, 'binary_sha256': expected_sha256, 'invocations': 0,
            'outer_uid': os.getuid(), 'outer_initial': namespace(),
            'implicit_effects_inventory': {'driver': IMPLICIT_EFFECTS['driver_first_StartProcess'], 'resident': ['NO StartProcess/client in this mode', 'metadata/open-close and read-only netlink/sysctl only'], 'fixed_client': ['NOT invoked in device preflight; existing bootstrap nonspawning/filter unchanged']},
            'limits': '6s Go supervisor plus cleanup are cooperative bounds; kernel device call may remain D state. Wrapper10s deadline does not prove termination. Single-user writable binary path has prehash/exec race.'}
    records = {}
    samples = []
    start = time.monotonic_ns()
    with (output/'stdout.json').open('xb') as out, (output/'stderr.txt').open('xb') as err:
        # Minimal environment, no user-supplied mode/path/env handed to child.
        p = subprocess.Popen([str(binary), '--device-open-preflight'], stdout=out, stderr=err, env={'PATH': '/usr/bin:/bin'})
        meta['invocations'] = 1
        meta['driver_pid'] = p.pid
        driver = identity(p.pid)
        meta['driver_initial'] = driver
        if 'start_ticks' in driver:
            records[(driver['pid'], driver['start_ticks'])] = driver
        meta['driver_exe_sha_verified'] = False
        if 'exe' in driver:
            try:
                meta['driver_exe_sha_verified'] = hashlib.sha256(Path('/proc', str(p.pid), 'exe').read_bytes()).hexdigest() == expected_sha256
            except OSError:
                pass
        deadline = time.monotonic() + 10
        while True:
            sample = snapshot(p.pid)
            samples.append(sample)
            for r in sample['records']:
                if 'start_ticks' in r:
                    records[(r['pid'], r['start_ticks'])] = r
            own = next((r for r in sample['records'] if r['pid'] == p.pid), {})
            # A disappeared exe alone is NOT exit: zombies often lose exe.
            # waitid WNOWAIT identifies exit without releasing driver PID.
            w = os.waitid(os.P_PID, p.pid, os.WEXITED | os.WNOHANG | os.WNOWAIT)
            if w is not None:
                break
            if not meta['driver_exe_sha_verified'] or time.monotonic() >= deadline:
                meta['timeout_or_identity_failure'] = True
                meta['driver_kill_sent'] = checked_kill(driver, signal.SIGKILL)
                break
            time.sleep(.001)
        try:
            meta['exit_code'] = p.wait(timeout=2)
            meta['driver_reaped'] = True
        except subprocess.TimeoutExpired:
            meta['driver_reaped'] = False
            meta['pending_custody'] = True
    meta['outer_final'] = namespace()
    meta['outer_unchanged'] = meta['outer_initial'] == meta['outer_final']
    meta['elapsed_ms'] = (time.monotonic_ns()-start)/1e6
    meta['recorded_lifetimes'] = list(records.values())
    meta['post_exact_lifetime_checks'] = [{'recorded': r, 'current': identity(r['pid'])} for r in records.values()]
    # Unknown reads are UNKNOWN, never silently absent.
    meta['postcheck_note'] = 'unreadable-or-gone is not definitive absence; Wait only proves driver reap, resident lifecycle must be validated from bounded verdict. Unrecorded descendants remain unverified.'
    meta['classification'] = 'unreviewed-output'
    (output/'outer-verdict.json').write_text(json.dumps(meta, indent=2)+'\n')
    (output/'owned-tree-samples.json').write_text(json.dumps(samples, indent=2)+'\n')
    return meta

if __name__ == '__main__':
    import argparse
    parser = argparse.ArgumentParser(description='Explicit one-shot owned device preflight. Separate release required.')
    parser.add_argument('--binary', required=True)
    parser.add_argument('--sha256', required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    # No guessed defaults, retries, alternate modes or fallback.
    result = run_once(args.binary, args.sha256, args.output)
    print(json.dumps(result, indent=2))

