#!/usr/bin/env python3
"""Wait until OpenCode's marker-file operations appear in captured events."""

from __future__ import annotations

import argparse
import json
from collections import Counter
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request


def fetch_events(url: str, api_key: str, timeout: float) -> list[dict]:
    request = urllib.request.Request(url, headers={"Accept": "application/json"})
    request.add_header("X-API-KEY", api_key)
    with urllib.request.urlopen(request, timeout=timeout) as response:
        payload = json.loads(response.read().decode("utf-8", "replace"))
    events = payload.get("events")
    if not isinstance(events, list):
        raise ValueError("recent-events response did not contain an events array")
    return [event for event in events if isinstance(event, dict)]


def strings_in(value: object):
    if isinstance(value, str):
        yield value
    elif isinstance(value, dict):
        for child in value.values():
            yield from strings_in(child)
    elif isinstance(value, list):
        for child in value:
            yield from strings_in(child)


def event_summary(record: dict) -> dict[str, object]:
    event = record.get("Event", record.get("event"))
    if not isinstance(event, dict):
        return {"record_keys": sorted(record)}
    fields = ("type", "eventType", "pid", "comm", "path", "cwd")
    return {
        key: event[key]
        for key in fields
        if key in event
    }


def captured_path_variants(path: str) -> tuple[str, ...]:
    """Match canonical paths after documented standard home-path redaction.

    The backend maps /home/<user>/work/... to ~/work/...; some older
    collectors omit the first home-relative component. Both retain the
    full marker filename, unlike basename-only matching.
    """
    home = os.path.expanduser("~").rstrip(os.sep)
    if home and path.startswith(home + os.sep):
        relative = path[len(home) + 1 :]
        variants = [path, f"~{os.sep}{relative}"]
        parts = relative.split(os.sep, 1)
        if len(parts) == 2:
            variants.append(f"~{os.sep}{parts[1]}")
        return tuple(dict.fromkeys(variants))
    return (path,)


def capture_event(record: dict) -> dict:
    """Only use the typed kernel event; ignore arbitrary message/extra-info strings."""
    event = record.get("Event", record.get("event"))
    return event if isinstance(event, dict) else {}


def kernel_paths(record: dict) -> set[str]:
    event = capture_event(record)
    fields = ("path", "Path", "filePath", "file_path")
    return {
        value.rstrip("\x00")
        for name in fields
        if isinstance((value := event.get(name)), str) and value
    }


def match_marker_events(events: list[dict], expected: list[str]) -> dict[str, list[dict]]:
    """Demand path evidence in an Event field, not a free-form log mention."""
    candidates = {path: set(captured_path_variants(path)) for path in expected}
    matched = {path: [] for path in expected}
    for record in events:
        paths = kernel_paths(record)
        if not paths:
            continue
        for path, variants in candidates.items():
            if paths.intersection(variants):
                matched[path].append(event_summary(record))
    return matched


def write_report(
    destination: str,
    expected: list[str],
    matches: dict[str, list[dict]],
    event_count: int,
    error: str | None,
    diagnostics: dict | None = None,
) -> None:
    """Publish metadata only: never dump raw prompt, headers or body strings."""
    if not destination:
        return
    markers = []
    for path in expected:
        records = matches.get(path, [])
        markers.append({
            "basename": os.path.basename(path),
            "captured": bool(records),
            "matchingEvents": len(records),
            "eventTypes": sorted({str(item.get("type", item.get("eventType", ""))) for item in records}),
            "pids": sorted({str(item["pid"]) for item in records if "pid" in item}),
            "commands": sorted({str(item["comm"]) for item in records if "comm" in item}),
        })
    summary = {
        "schemaVersion": 1,
        "source": "kernel-event-typed-path",
        "status": "pass" if all(item["captured"] for item in markers) else "fail",
        "recentEvents": event_count,
        "markers": markers,
        "lastFetchError": error,
        "diagnostics": diagnostics or {},
    }
    with open(destination, "w", encoding="utf-8") as output:
        json.dump(summary, output, indent=2, sort_keys=True)
        output.write("\n")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--api-key", required=True)
    parser.add_argument("--expect", action="append", required=True, help="absolute marker path; may be repeated")
    parser.add_argument("--timeout", type=float, default=45.0)
    parser.add_argument("--poll-interval", type=float, default=2.0)
    parser.add_argument("--limit", type=int, default=500)
    parser.add_argument("--report", default="", help="write sanitized structured capture summary")
    parser.add_argument("--done-file", default="", help="file created when the agent has exited")
    parser.add_argument("--post-run-timeout", type=float, default=45.0)
    args = parser.parse_args()

    expected = [path for path in args.expect]
    if any(not path.startswith("/") for path in expected):
        parser.error("all --expect paths must be absolute")
    base_url = args.base_url.rstrip("/")
    query = urllib.parse.urlencode({"limit": args.limit})
    url = f"{base_url}/events/recent?{query}"
    deadline = time.monotonic() + args.timeout
    matched = {path: [] for path in expected}

    workspace_candidates = {
        candidate
        for path in expected
        for candidate in captured_path_variants(os.path.dirname(os.path.dirname(path)))
    }
    last_missing = expected[:]
    last_error: str | None = None
    last_events: list[dict] = []
    last_workspace_events: list[dict[str, object]] = []
    observed_types: Counter[str] = Counter()
    typed_path_fields = 0
    poll_count = 0
    done_deadline_set = False

    def diagnostics() -> dict:
        return {
            "polls": poll_count,
            "typedPathFieldsSeen": typed_path_fields,
            "sampledEventTypes": dict(observed_types.most_common(20)),
        }

    while time.monotonic() < deadline:
        if args.done_file and not done_deadline_set and os.path.exists(args.done_file):
            deadline = min(deadline, time.monotonic() + args.post_run_timeout)
            done_deadline_set = True
        try:
            events = fetch_events(url, args.api_key, timeout=10.0)
            last_events = events
            poll_count += 1
            observed_types.update(str(capture_event(record).get("type", "")) for record in events)
            typed_path_fields += sum(bool(kernel_paths(record)) for record in events)
            last_workspace_events = []
            for record in events:
                summary = event_summary(record)
                cwd = summary.get("cwd")
                if isinstance(cwd, str) and any(candidate in cwd for candidate in workspace_candidates):
                    last_workspace_events.append(summary)
            matched = match_marker_events(events, expected)
            last_missing = [path for path in expected if not matched[path]]
            last_error = None
            if not last_missing:
                write_report(args.report, expected, matched, len(events), None, diagnostics())
                print(f"[soak] captured all {len(expected)} marker paths in typed kernel events across {len(events)} recent records")
                return 0
        except (OSError, ValueError, json.JSONDecodeError, urllib.error.URLError) as exc:
            last_error = str(exc)

        time.sleep(min(args.poll_interval, max(0.0, deadline - time.monotonic())))

    write_report(args.report, expected, matched, len(last_events), last_error, diagnostics())
    print(
        f"[soak] sampled {poll_count} snapshots; typed path fields={typed_path_fields}; "
        f"event types={dict(observed_types.most_common(12))}",
        file=sys.stderr,
    )
    if last_error:
        print(f"[soak] recent-events query failed: {last_error}", file=sys.stderr)
    if last_missing:
        print(f"[soak] marker paths absent from captured events: {', '.join(last_missing)}", file=sys.stderr)
    print(
        f"[soak] recent-events returned {len(last_events)} records; "
        f"{len(last_workspace_events)} reference the soak workspace (last 20):",
        file=sys.stderr,
    )
    for event in last_workspace_events[-20:]:
        print(f"[soak] {json.dumps(event, sort_keys=True)}", file=sys.stderr)

    return 1


if __name__ == "__main__":
    sys.exit(main())
