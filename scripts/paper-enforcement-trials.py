#!/usr/bin/env python3
"""Privileged, loopback-only, evidence-grade BPF enforcement trials.

Never turns a generic socket error into a claimed deny. Saves one CSV row per
completed trial, including negative control, errno and post-revoke recovery.
Use in an isolated, authorized BPF LSM + cgroup v2 test VM only.
"""
from __future__ import annotations

import argparse
import csv
import errno
import json
import mmap
import os
from pathlib import Path
import platform
import socket
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request
from contextlib import contextmanager
from typing import Callable

CASES = (
    "tcp_connect", "udp_connect", "udp_sendto", "ipv4_mapped_ipv6",
    "existing_fd_read", "existing_fd_write", "mmap",
)
FIELDS = ("case", "control_ok", "denied", "errno", "restored", "apply_us", "revoke_us")


class Backend:
    def __init__(self, base_url: str, token: str):
        self.base_url = base_url.rstrip("/")
        self.headers = {"Content-Type": "application/json"}
        if token:
            self.headers["X-API-KEY"] = token

    def request(self, method: str, path: str, payload: dict | None = None) -> dict:
        request = urllib.request.Request(
            self.base_url + path,
            data=json.dumps(payload).encode() if payload is not None else None,
            headers=self.headers, method=method,
        )
        with urllib.request.urlopen(request, timeout=8) as response:
            if not 200 <= response.status < 300:
                raise RuntimeError(f"{method} {path}: HTTP {response.status}")
            body = response.read()
        return json.loads(body) if body else {}

    def update(self, path: str, payload: dict) -> float:
        start = time.monotonic_ns()
        self.request("POST", path, payload)
        return (time.monotonic_ns() - start) / 1000

    def require_attached(self) -> None:
        for component in ("cgroup", "lsm"):
            data = self.request("GET", f"/sandbox/{component}/status")
            if data.get("available") is not True or data.get("attached") is not True:
                raise RuntimeError(f"Live {component} enforcement not attached: {data}")


@contextmanager
def tcp_listener():
    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    server.bind(("127.0.0.1", 0))
    server.listen(32)
    server.settimeout(0.2)
    stop = threading.Event()

    def accept_loop():
        while not stop.is_set():
            try:
                conn, _ = server.accept()
                conn.close()
            except socket.timeout:
                continue
            except OSError:
                break

    listener = threading.Thread(target=accept_loop, daemon=True)
    listener.start()
    try:
        yield server.getsockname()[1]
    finally:
        stop.set()
        server.close()
        listener.join(timeout=2)


@contextmanager
def udp_listener():
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.bind(("127.0.0.1", 0))
    try:
        yield sock.getsockname()[1]
    finally:
        sock.close()


def network_op(case: str, port: int) -> None:
    if case in ("tcp_connect", "ipv4_mapped_ipv6"):
        family = socket.AF_INET6 if case == "ipv4_mapped_ipv6" else socket.AF_INET
        address = ("::ffff:127.0.0.1", port) if family == socket.AF_INET6 else ("127.0.0.1", port)
        with socket.socket(family, socket.SOCK_STREAM) as sock:
            sock.settimeout(2)
            sock.connect(address)
    else:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
            if case == "udp_connect":
                sock.connect(("127.0.0.1", port))
            else:
                sock.sendto(b"agent-ebpf-proof", ("127.0.0.1", port))


def file_op(case: str, fd: int) -> None:
    if case == "existing_fd_read":
        os.lseek(fd, 0, os.SEEK_SET)
        os.read(fd, 1)
    elif case == "existing_fd_write":
        os.lseek(fd, 0, os.SEEK_SET)
        os.write(fd, b"X")
    elif case == "mmap":
        with mmap.mmap(fd, 4096, access=mmap.ACCESS_READ) as mapping:
            _ = mapping[0]
    else:
        raise ValueError(f"unknown file probe {case}")


def attempt(operation: Callable[[], None]) -> tuple[bool, int | None]:
    try:
        operation()
        return True, None
    except OSError as exc:
        return False, exc.errno


def probe_once(backend: Backend, case: str) -> dict:
    is_network = case in CASES[:4]
    if is_network:
        listener = tcp_listener() if case in ("tcp_connect", "ipv4_mapped_ipv6") else udp_listener()
        with listener as port:
            return measure_policy(
                backend, case, lambda: network_op(case, port),
                "/sandbox/cgroup/block-port", "/sandbox/cgroup/unblock-port",
                {"port": port},
            )

    with tempfile.TemporaryDirectory(prefix="agent-ebpf-enforce-") as tmp:
        path = Path(tmp) / ("probe-" + os.urandom(8).hex())
        path.write_bytes(b"A" * 4096)
        fd = os.open(path, os.O_RDWR)
        try:
            return measure_policy(
                backend, case, lambda: file_op(case, fd),
                "/sandbox/lsm/block-file-name", "/sandbox/lsm/unblock-file-name",
                {"name": path.name},
            )
        finally:
            os.close(fd)


def measure_policy(backend: Backend, case: str, operation: Callable[[], None],
                   add_path: str, remove_path: str, payload: dict) -> dict:
    control_ok, _ = attempt(operation)
    apply_us = revoke_us = None
    denied = False
    observed_errno = None
    try:
        apply_us = backend.update(add_path, payload)
        denied, observed_errno = attempt(operation)
        denied = not denied
    finally:
        if apply_us is not None:
            revoke_us = backend.update(remove_path, payload)
    restored, _ = attempt(operation)
    return {
        "case": case, "control_ok": str(control_ok).lower(),
        "denied": str(denied).lower(),
        "errno": observed_errno if observed_errno is not None else "",
        "restored": str(restored).lower(), "apply_us": f"{apply_us:.3f}",
        "revoke_us": f"{revoke_us:.3f}",
    }


def mapped_ipv6_works() -> bool:
    try:
        with tcp_listener() as port:
            network_op("ipv4_mapped_ipv6", port)
        return True
    except OSError:
        return False


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--backend-url", default="http://127.0.0.1:8080")
    parser.add_argument("--token", default=os.getenv("AGENT_ACCESS_TOKEN", ""))
    parser.add_argument("--trials", type=int, default=100)
    parser.add_argument("--cases", nargs="+", choices=CASES, default=list(CASES))
    parser.add_argument("--output", type=Path,
                        default=Path("reports/paper-experiments/enforcement.csv"))
    args = parser.parse_args()
    if args.trials < 1:
        parser.error("--trials must be >= 1")
    backend = Backend(args.backend_url, args.token)
    backend.require_attached()
    requested_cases = list(dict.fromkeys(args.cases))
    if "ipv4_mapped_ipv6" in requested_cases and not mapped_ipv6_works():
        print("SKIP ipv4_mapped_ipv6: no working mapped IPv6 pre-control")
        requested_cases.remove("ipv4_mapped_ipv6")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    # Do not append to old evidence. Each invocation is a distinct manifest.
    manifest = {
        "purpose": "real live enforcement; API round-trip timing not propagation",
        "kernel": platform.release(),
        "cases_requested": args.cases,
        "cases_tested": requested_cases,
        "trials_per_case": args.trials,
        "timestamp_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    try:
        manifest["git_sha"] = subprocess.check_output(
            ["git", "rev-parse", "HEAD"], stderr=subprocess.DEVNULL, text=True
        ).strip()
    except (FileNotFoundError, subprocess.CalledProcessError):
        manifest["git_sha"] = None
    args.output.with_suffix(".manifest.json").write_text(
        json.dumps(manifest, indent=2) + "\n", encoding="utf-8"
    )
    with args.output.open("w", encoding="utf-8", newline="") as file:
        writer = csv.DictWriter(file, fieldnames=FIELDS)
        writer.writeheader()
        for case in requested_cases:
            for index in range(args.trials):
                result = probe_once(backend, case)
                writer.writerow(result)
                file.flush()
                success = (
                    result["control_ok"] == "true" and result["denied"] == "true"
                    and str(result["errno"]) in (str(errno.EPERM), str(errno.EACCES))
                    and result["restored"] == "true"
                )
                print(f"{case} {index + 1}/{args.trials}: {'PASS' if success else 'FAIL'} "
                      f"errno={result['errno']} apply_us={result['apply_us']} "
                      f"revoke_us={result['revoke_us']}", flush=True)
    print(f"Measured trial CSV and manifest: {args.output}")


if __name__ == "__main__":
    main()
