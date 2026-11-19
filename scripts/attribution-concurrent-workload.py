#!/usr/bin/env python3
"""Run actual same-name tools from concurrently registered, distinct Agent roots.

Produces ground-truth process lifetimes, NOT attribution predictions. To claim
an accuracy number, join live backend event IDs to these lifetimes and evaluate
each independently configured ablation mode.
"""
from __future__ import annotations

import argparse
import csv
import json
import multiprocessing as mp
import os
from pathlib import Path
import queue
import shutil
import subprocess
import tempfile
import time
import urllib.request
import urllib.parse

COMMANDS = ("git", "python", "node")


def register(backend: str, token: str, pid: int, agent_id: str, action: str) -> None:
    payload = {"pid": pid} if action == "unregister" else {
        "pid": pid, "root_agent_pid": pid, "agent_run_id": agent_id,
        "task_id": "attribution-ablation", "tag": "attribution-ablation",
    }
    request = urllib.request.Request(
        f"{backend.rstrip('/')}/{action}",
        data=json.dumps(payload).encode(),
        headers={
            "Content-Type": "application/json",
            **({"X-API-KEY": token} if token else {}),
        },
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=5) as response:
        if response.status != 200:
            raise RuntimeError(f"{action} returned HTTP {response.status}")


def tool_command(name: str, repo: Path, destination: Path) -> list[str]:
    if name == "git":
        return ["git", "-C", str(repo), "status", "--porcelain"]
    if name == "python":
        return ["python3", "-c",
                "import pathlib,sys,time; pathlib.Path(sys.argv[1]).write_text('agent'); time.sleep(0.10)",
                str(destination)]
    if name == "node":
        return ["node", "-e",
                "require('fs').writeFileSync(process.argv[1],'agent'); setTimeout(()=>{},100)",
                str(destination)]
    raise ValueError(name)


def worker(agent_count: int, agent_index: int, cycle: int, gate: mp.Event,
           output: mp.Queue, backend: str, token: str, repo: str, folder: str,
           dry_run: bool) -> None:
    agent_id = f"agents-{agent_count}-cycle-{cycle}-agent-{agent_index}"
    root_pid = os.getpid()
    try:
        if not dry_run:
            register(backend, token, root_pid, agent_id, "register")
        if not gate.wait(timeout=15):
            raise TimeoutError("start barrier not released")
        for command in COMMANDS:
            destination = Path(folder) / f"{agent_id}-{command}"
            started = time.time_ns()
            child = subprocess.Popen(
                tool_command(command, Path(repo), destination),
                stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
            )
            try:
                _, stderr = child.communicate(timeout=10)
            except subprocess.TimeoutExpired:
                child.kill()
                child.communicate()
                raise
            finished = time.time_ns()
            if child.returncode:
                raise RuntimeError(
                    f"{command} exit {child.returncode}: {stderr.decode(errors='replace')[:200]}"
                )
            output.put({
                "agent_count": agent_count, "cycle": cycle, "command": command,
                "agent_id": agent_id, "root_pid": root_pid, "child_pid": child.pid,
                "started_ns": started, "finished_ns": finished,
                "returncode": child.returncode,
            })
    except Exception as exc:
        output.put({"error": f"{agent_id}: {exc}"})
    finally:
        if not dry_run:
            try:
                register(backend, token, root_pid, agent_id, "unregister")
            except Exception as exc:
                output.put({"error": f"{agent_id}: unregister: {exc}"})


def run(agents: tuple[int, ...], cycles: int, backend: str, token: str,
        repo: Path, dry_run: bool, capture_callback=None) -> list[dict]:
    for tool in ("git", "python3", "node"):
        if shutil.which(tool) is None:
            raise RuntimeError(f"{tool} not installed; real workflow cannot run")
    records: list[dict] = []
    with tempfile.TemporaryDirectory(prefix="agent-attribution-") as folder:
        for count in agents:
            for cycle in range(1, cycles + 1):
                gate = mp.Event()
                output: mp.Queue = mp.Queue()
                processes = [
                    mp.Process(target=worker, args=(
                        count, index, cycle, gate, output,
                        backend, token, str(repo), folder, dry_run
                    ))
                    for index in range(count)
                ]
                for process in processes:
                    process.start()
                # Registration time is not part of the concurrent tool window.
                time.sleep(0.25)
                gate.set()
                for process in processes:
                    process.join(timeout=30)
                    if process.is_alive():
                        process.terminate()
                        process.join()
                        raise TimeoutError("Agent worker stalled")
                while True:
                    try:
                        record = output.get_nowait()
                    except queue.Empty:
                        break
                    if "error" in record:
                        raise RuntimeError(record["error"])
                    records.append(record)
                output.close()
                expected = count * len(COMMANDS)
                got = sum(
                    r["agent_count"] == count and r["cycle"] == cycle
                    for r in records
                )
                if got != expected:
                    raise RuntimeError(
                        f"missing ground truth at C={count}, cycle={cycle}: {got}/{expected}"
                    )
                if capture_callback is not None:
                    capture_callback(count, cycle)
    return records


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--agents", nargs="+", type=int, default=[2, 3, 4])
    parser.add_argument("--cycles", type=int, default=10)
    parser.add_argument("--backend-url", default="http://127.0.0.1:8080")
    parser.add_argument("--token", default=os.getenv("AGENT_ACCESS_TOKEN", ""))
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument("--capture-events-dir", type=Path,
                        help="Capture backend /events/summaries after EVERY cohort; actual predictions only")
    parser.add_argument("--dry-run", action="store_true",
                        help="No backend registration; produces NON-EVIDENCE development output")
    parser.add_argument("--output", type=Path,
                        default=Path("reports/paper-experiments/ground-truth.csv"))
    args = parser.parse_args()
    if not args.agents or any(n not in (2, 3, 4) for n in args.agents):
        parser.error("--agents must contain values from 2, 3, 4")
    if args.cycles < 1:
        parser.error("--cycles must be positive")
    if args.dry_run and args.capture_events_dir:
        parser.error("--capture-events-dir requires real backend registration, not dry-run")
    callback = None
    if args.capture_events_dir:
        args.capture_events_dir.mkdir(parents=True, exist_ok=True)
        def capture_backend_events(count: int, cycle: int):
            # Allow the asynchronous kernel collector time to persist process
            # events. The time captured here is NOT a kernel event timestamp.
            time.sleep(0.4)
            url = args.backend_url.rstrip("/") + "/events/summaries?limit=1000"
            req = urllib.request.Request(url, headers=(
                {"X-API-KEY": args.token} if args.token else {}))
            with urllib.request.urlopen(req, timeout=10) as response:
                payload = json.load(response)
            if not isinstance(payload, dict) or not isinstance(payload.get("events"), list):
                raise RuntimeError("unexpected backend /events/summaries response")
            snapshot = {
                "agent_count": count, "cycle": cycle,
                "observed_at_ns": time.time_ns(),
                "source": payload.get("source"), "events": payload["events"],
            }
            target = args.capture_events_dir / f"events_agents{count}_cycle{cycle}.json"
            target.write_text(json.dumps(snapshot, ensure_ascii=False), encoding="utf-8")
            print(f"captured {len(snapshot['events'])} real backend events in {target}", flush=True)
        callback = capture_backend_events
    records = run(tuple(args.agents), args.cycles, args.backend_url,
                  args.token, args.repo.resolve(), args.dry_run, callback)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    fields = ("agent_count", "cycle", "command", "agent_id", "root_pid",
              "child_pid", "started_ns", "finished_ns", "returncode")
    with args.output.open("w", encoding="utf-8", newline="") as stream:
        writer = csv.DictWriter(stream, fieldnames=fields)
        writer.writeheader()
        writer.writerows(records)
    mode = "DRY RUN: NOT ATTRIBUTION EVIDENCE" if args.dry_run else "REAL WORKLOAD"
    print(f"{mode}: {len(records)} truth lifetimes -> {args.output}")


if __name__ == "__main__":
    main()
