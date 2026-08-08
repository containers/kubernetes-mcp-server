"""Shared kubectl helper functions for e2e test fixtures."""

from __future__ import annotations

import re
import subprocess
import tempfile
import time


def kill_proc(proc: subprocess.Popen) -> None:
    """Terminate a subprocess, escalating to SIGKILL if needed."""
    try:
        proc.terminate()
        proc.wait(timeout=10)
    except OSError:
        pass
    except subprocess.TimeoutExpired:
        proc.kill()
        try:
            proc.wait(timeout=10)
        except (subprocess.TimeoutExpired, OSError):
            pass


def start_port_forward(
    namespace: str, name: str, kubectl_bin: str, remote_port: int = 8080,
) -> tuple[str, subprocess.Popen]:
    """Start a kubectl port-forward and return (local_url, process).

    Uses ``:remote_port`` so kubectl picks a free local port atomically.
    Both streams go to temp files to avoid PIPE deadlocks on long-lived
    connections.
    """
    stdout_file = tempfile.TemporaryFile()
    stderr_file = tempfile.TemporaryFile()
    try:
        proc = subprocess.Popen(
            [
                kubectl_bin, "port-forward",
                "-n", namespace,
                f"svc/{name}",
                f":{remote_port}",
            ],
            stdout=stdout_file,
            stderr=stderr_file,
        )
    except BaseException:
        stdout_file.close()
        stderr_file.close()
        raise
    proc._stdout_file = stdout_file
    proc._stderr_file = stderr_file

    deadline = time.monotonic() + 30.0
    while time.monotonic() < deadline:
        stdout_file.seek(0)
        output = stdout_file.read().decode(errors="replace")
        m = re.search(r"Forwarding from 127\.0\.0\.1:(\d+)", output)
        if m:
            return f"http://127.0.0.1:{int(m.group(1))}", proc
        if proc.poll() is not None:
            break
        time.sleep(0.1)

    stdout_file.seek(0)
    out = stdout_file.read().decode(errors="replace")
    m = re.search(r"Forwarding from 127\.0\.0\.1:(\d+)", out)
    if m:
        return f"http://127.0.0.1:{int(m.group(1))}", proc
    kill_proc(proc)
    stderr_file.seek(0)
    err = stderr_file.read().decode(errors="replace")
    stdout_file.close()
    stderr_file.close()
    raise RuntimeError(
        "kubectl port-forward did not report a local port within 30s "
        f"(exit code {proc.returncode}).\n"
        f"--- stdout ---\n{out}\n--- stderr ---\n{err}"
    )
