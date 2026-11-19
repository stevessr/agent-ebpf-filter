#!/usr/bin/env python3
"""CI-only balanced cross-over: change tracepoint links, NOT the daemon load.

Run as root on a disposable GitHub-hosted runner. The only links this script
unpins are /sys/fs/bpf/agent-ebpf/links/*, the project's tracepoint links.
Never run against production hosts or when CI/RUNNER_TEMP guards are absent.
Raw samples are recorded separately for each true kernel off/on state.
"""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import random
import subprocess
import time
import urllib.request

PIN_DIR = Path("/sys/fs/bpf/agent-ebpf/links")
LEVELS = (1, 4, 8, 16, 32)


def command(*args: str, timeout: int = 60) -> str:
    try:
        return subprocess.check_output(args, text=True, timeout=timeout, stderr=subprocess.STDOUT)
    except subprocess.CalledProcessError as exc:
        raise RuntimeError(f"{args!r} exit {exc.returncode}: {exc.output[-3000:]}") from exc


def global_link_rows() -> list[dict]:
    # perf-event tracepoint links may be discoverable via the global
    # BPF_LINK_GET_NEXT_ID API even when a pinned-path BPF_OBJ_GET fails.
    # --bpffs exposes the kernel-observed pin filenames for each link ID.
    raw = json.loads(command("bpftool", "-j", "-f", "link", "show"))
    if not isinstance(raw, list) or not raw:
        raise RuntimeError("bpftool did not return kernel BPF link records")
    return raw


def pinned_links() -> dict[str, int]:
    if not PIN_DIR.is_dir() or PIN_DIR.is_symlink():
        raise RuntimeError(f"{PIN_DIR}: pinned tracepoint directory missing or symlink")
    entries = sorted(PIN_DIR.iterdir())
    if len(entries) < 100:
        raise RuntimeError(f"only {len(entries)} project tracepoint pins; expected >=100")
    if not all(p.is_file() and not p.is_symlink() for p in entries):
        raise RuntimeError("unexpected directory/symlink in project tracepoint pins")
    index: dict[str, dict] = {}
    for row in global_link_rows():
        pinned = row.get("pinned", [])
        if isinstance(pinned, str):
            pinned = [pinned]
        for path in pinned:
            if path in index:
                raise RuntimeError(f"duplicate pinned BPF link path: {path}")
            index[path] = row
    ids = {}
    for p in entries:
        info = index.get(str(p))
        if not info:
            raise RuntimeError(f"{p}: kernel link ID absent from bpftool -j -f global listing")
        if str(info.get("type", "")).lower() not in {"perf_event", "tracing"}:
            raise RuntimeError(f"{p}: unexpected link type {info.get('type')}")
        ids[p.name] = int(info["id"])
    if len(set(ids.values())) != len(ids):
        raise RuntimeError("two pin paths refer to the same link ID")
    return ids


def global_link_ids() -> set[int]:
    return {int(row["id"]) for row in global_link_rows()}


def ensure_backend_responsive() -> None:
    with urllib.request.urlopen("http://127.0.0.1:8080/system/bootstrap-health", timeout=10) as response:
        if response.status != 200:
            raise RuntimeError("backend became unavailable during controlled switch")


def switch_off(current_ids: dict[str, int]) -> None:
    for path in sorted(PIN_DIR.iterdir()):
        if path.name not in current_ids:
            raise RuntimeError(f"unknown project link added during benchmark: {path}")
        path.unlink()
    if list(PIN_DIR.iterdir()):
        raise RuntimeError("project tracing links still pinned")
    active = global_link_ids()
    lingering = active.intersection(current_ids.values())
    if lingering:
        raise RuntimeError(f"cannot claim detached baseline: tracepoints still loaded: {sorted(lingering)[:8]}")


def switch_on(backend: Path, initial_names: set[str]) -> dict[str, int]:
    # Reattach the owned project's syscall tracepoints while the SAME
    # collection backend stays up; cgroup/LSM enforcement pins are untouched.
    command(str(backend), "--ebpf-bootstrap", timeout=120)
    updated = pinned_links()
    if set(updated) != initial_names:
        raise RuntimeError("tracepoint set changed across bootstrap")
    active = global_link_ids()
    missing = set(updated.values()) - active
    if missing:
        raise RuntimeError(f"project link IDs not attached after bootstrap: {sorted(missing)[:8]}")
    return updated


def run(args: argparse.Namespace) -> None:
    if os.geteuid() != 0 or os.environ.get("CI") != "true" or not os.environ.get("RUNNER_TEMP"):
        raise RuntimeError("must be root on an explicitly CI=true ephemeral GitHub runner")
    if not str(Path.cwd()).startswith(os.environ["GITHUB_WORKSPACE"] + "/") and Path.cwd() != Path(os.environ["GITHUB_WORKSPACE"]):
        raise RuntimeError("refusing to manipulate project links outside checked-out CI workspace")
    if not args.backend.is_file() or not args.sampler.is_file():
        raise RuntimeError("missing experiment binaries")
    if args.cycles < 2 or args.cycles % 2 or args.cycles > 20 or args.samples < 1000:
        raise RuntimeError("cycles must be even in [2,20]; at least 1000 read samples per phase")
    if not os.path.ismount("/sys/fs/bpf"):
        raise RuntimeError("bpffs not mounted")
    args.out.mkdir(parents=True, exist_ok=True)
    ensure_backend_responsive()
    original = pinned_links()
    if set(original.values()) - global_link_ids():
        raise RuntimeError("initial project kernel links do not match pinned objects")
    initial_names = set(original)
    manifest = {
        "kind": "matched_daemon_real_kernel_link_crossover",
        "seed": args.seed,
        "cycles_per_concurrency": args.cycles,
        "samples_per_phase": args.samples,
        "expected_link_count": len(original),
        "hostname": os.uname().nodename,
        "kernel": os.uname().release,
        "cpu_count": os.cpu_count(),
        "tested_sha": command("git", "rev-parse", "HEAD").strip(),
        "phases": [],
    }
    path = args.out / "manifest.json"
    states = "on"
    ids = original.copy()

    def save():
        path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")

    save()
    rng = random.Random(args.seed)
    levels = list(LEVELS)
    rng.shuffle(levels)
    try:
        for concurrency in levels:
            orders = [["off", "on"]] * (args.cycles // 2) + [["on", "off"]] * (args.cycles // 2)
            rng.shuffle(orders)
            for cycle, order in enumerate(orders, start=1):
                for phase in order:
                    if states != phase:
                        if phase == "off":
                            # Mark non-attached BEFORE mutating pins. A
                            # partial unlink failure must still trigger
                            # tracepoint restoration in finally.
                            states = "off"
                            switch_off(ids)
                        else:
                            ids = switch_on(args.backend, initial_names)
                            states = "on"
                    ensure_backend_responsive()
                    # Avoid counting pin/reload syscall burst directly in
                    # application latency samples.
                    time.sleep(0.25)
                    filename = f"c{concurrency}-pair{cycle}-{phase}.csv"
                    output = args.out / filename
                    command(str(args.sampler), "--concurrency", str(concurrency),
                            "--samples", str(args.samples), "--warmup", "200",
                            "--output", str(output), timeout=90)
                    manifest["phases"].append({
                        "concurrency": concurrency, "pair": cycle, "mode": phase,
                        "file": filename, "backend_healthy": True,
                        "kernel_tracepoint_links": len(ids) if phase == "on" else 0,
                        "actual_kernel_link_ids_verified": True,
                        "finished_ns": time.time_ns(),
                    })
                    save()
                    print(f"[crossover] C={concurrency} pair={cycle}/{args.cycles} "
                          f"phase={phase}, verified link IDs, samples={args.samples}", flush=True)
    finally:
        # Fail safe for developer reruns: restore project instrumentation
        # even on an exception. Do not leave a daemon running unobserved.
        if states == "off":
            try:
                switch_on(args.backend, initial_names)
                manifest["restored_tracepoint_links"] = True
            except Exception as exc:
                manifest["restored_tracepoint_links"] = False
                manifest["restore_error"] = str(exc)
        else:
            manifest["restored_tracepoint_links"] = True
        save()
    if not manifest["restored_tracepoint_links"]:
        raise RuntimeError("FAILED to restore project tracepoints after controlled experiment")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--backend", required=True, type=Path)
    parser.add_argument("--sampler", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--cycles", type=int, default=10)
    parser.add_argument("--samples", type=int, default=5000)
    parser.add_argument("--seed", type=int, default=20261119)
    run(parser.parse_args())


if __name__ == "__main__":
    main()
