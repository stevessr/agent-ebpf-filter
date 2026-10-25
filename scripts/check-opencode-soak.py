#!/usr/bin/env python3
"""Wait until OpenCode's marker-file operations appear in captured events."""

from __future__ import annotations

import argparse
import json
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
    # Standard redaction replaces the runner's concrete home prefix with "~".
    home = os.path.expanduser("~").rstrip(os.sep)
    if home and path.startswith(home + os.sep):
        return path, "~" + path[len(home) :]
    return (path,)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--api-key", required=True)
    parser.add_argument("--expect", action="append", required=True, help="absolute marker path; may be repeated")
    parser.add_argument("--timeout", type=float, default=45.0)
    parser.add_argument("--poll-interval", type=float, default=2.0)
    parser.add_argument("--limit", type=int, default=500)
    args = parser.parse_args()

    expected = [path for path in args.expect]
    if any(not path.startswith("/") for path in expected):
        parser.error("all --expect paths must be absolute")
    base_url = args.base_url.rstrip("/")
    query = urllib.parse.urlencode({"limit": args.limit})
    url = f"{base_url}/events/recent?{query}"
    deadline = time.monotonic() + args.timeout
    path_candidates = {path: captured_path_variants(path) for path in expected}
    last_missing = expected[:]
    last_error: str | None = None
    last_events: list[dict] = []

    while time.monotonic() < deadline:
        try:
            events = fetch_events(url, args.api_key, timeout=10.0)
            last_events = events
            event_strings = [text for event in events for text in strings_in(event)]
            last_missing = [
                path
                for path, candidates in path_candidates.items()
                if not any(candidate in text for candidate in candidates for text in event_strings)
            ]
            last_error = None
            if not last_missing:
                print(f"[soak] captured all {len(expected)} marker paths across {len(events)} recent events")
                return 0
        except (OSError, ValueError, json.JSONDecodeError, urllib.error.URLError) as exc:
            last_error = str(exc)

        time.sleep(min(args.poll_interval, max(0.0, deadline - time.monotonic())))

    if last_error:
        print(f"[soak] recent-events query failed: {last_error}", file=sys.stderr)
    if last_missing:
        print(f"[soak] marker paths absent from captured events: {', '.join(last_missing)}", file=sys.stderr)
    print(f"[soak] recent-events returned {len(last_events)} records; sample:", file=sys.stderr)
    for event in last_events[:5]:
        print(f"[soak] {json.dumps(event_summary(event), sort_keys=True)}", file=sys.stderr)

    return 1


if __name__ == "__main__":
    sys.exit(main())
