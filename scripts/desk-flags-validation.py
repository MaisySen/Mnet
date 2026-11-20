#!/usr/bin/env python3
"""Built daemon argument refusals only. No network, display or daemon startup."""
import pathlib
import subprocess
import tempfile

repo = pathlib.Path(__file__).resolve().parent.parent
with tempfile.TemporaryDirectory(prefix="mnet-desk-flags-") as root:
    binary = str(pathlib.Path(root) / "mnetd")
    subprocess.run(["go", "build", "-o", binary, "./cmd/mnetd"], cwd=repo, check=True)

    def invoke(args):
        return subprocess.run([binary, *args], capture_output=True, text=True, timeout=4)

    for name in ("desk-run", "desk-inspect"):
        for args in ([f"-{name}=", "-version"], [f"-{name}", "", "-version"],
                     [f"--{name}=", "-version"]):
            result = invoke(args)
            assert result.returncode != 0 and "cannot be empty" in result.stderr, "empty selector entered another mode"
            assert not result.stdout and "mnetd dev" not in result.stderr, "unexpected output from empty selector"
        for args in ([f"-{name}", "first-does-not-exist", f"-{name}", "second-does-not-exist"],
                     [f"--{name}=first-does-not-exist", f"-{name}=second-does-not-exist"]):
            result = invoke(args)
            assert result.returncode != 0 and "cannot be repeated" in result.stderr and not result.stdout, "repeated selector reached file/daemon mode"
        result = invoke([f"-{name}", "missing-file", "-version"])
        assert result.returncode != 0 and "cannot combine" in result.stderr and not result.stdout, "explicit mode mixed with another mode"
    result = invoke(["-version"])
    assert result.returncode == 0 and "mnetd " in result.stderr, "legacy version mode changed"
    print("PASS actual built daemon Desk selector: empty/equal/double-dash/repeated/mixed flags refuse before another mode; standalone version unchanged; no daemon/network/display operation")
