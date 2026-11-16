#!/usr/bin/env python3
"""Offline regressions for the real OpenCode -> kernel event capture checker."""

from __future__ import annotations

import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest


CHECKER = Path(__file__).with_name("check-opencode-soak.py")
SPEC = importlib.util.spec_from_file_location("opencode_soak_checker", CHECKER)
assert SPEC is not None and SPEC.loader is not None
checker = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(checker)


class TypedKernelCaptureTests(unittest.TestCase):
    def test_standard_home_redaction_keeps_work_component(self):
        home = os.path.expanduser("~")
        marker = os.path.join(home, "work", "_temp", "agent-soak", "marker.txt")
        variants = checker.captured_path_variants(marker)
        self.assertIn("~/work/_temp/agent-soak/marker.txt", variants)
        self.assertIn("~/_temp/agent-soak/marker.txt", variants)
        records = [{"Event": {"path": "~/work/_temp/agent-soak/marker.txt",
                              "type": "openat", "pid": 420}}]
        self.assertEqual(len(checker.match_marker_events(records, [marker])[marker]), 1)

    def test_kernel_evidence_survives_eviction_from_next_recent_window(self):
        first = "/tmp/soak/marker-one.txt"
        second = "/tmp/soak/marker-two.txt"
        ledger = {first: [], second: []}
        window_a = checker.match_marker_events(
            [{"Event": {"path": first, "type": "openat", "pid": 1}}],
            [first, second],
        )
        checker.merge_verified_capture_matches(ledger, window_a)
        self.assertTrue(ledger[first])
        self.assertFalse(ledger[second])
        window_b = checker.match_marker_events(
            [{"Event": {"path": second, "type": "openat", "pid": 2}}],
            [first, second],
        )
        checker.merge_verified_capture_matches(ledger, window_b)
        self.assertTrue(ledger[first])
        self.assertTrue(ledger[second])
        checker.merge_verified_capture_matches(ledger, window_b)
        self.assertEqual(len(ledger[second]), 1)

    def test_typed_bpf_path_matched_exactly(self):
        path = "/tmp/opencode-soak/workspace/nested/marker-one.txt"
        events = [
            {"Event": {"path": path, "pid": 1234, "type": "openat", "comm": "touch"}},
            {"Event": {"path": path + ".other", "pid": 22}},
        ]
        matches = checker.match_marker_events(events, [path])
        self.assertEqual(len(matches[path]), 1)
        self.assertEqual(matches[path][0]["pid"], 1234)

    def test_free_text_and_envelope_strings_are_not_kernel_file_evidence(self):
        path = "/tmp/opencode-soak/workspace/nested/marker-two.txt"
        fake = [
            {"Event": {"type": "write", "comm": "opencode", "path": "/tmp/other"},
             "ExtraInfo": f"model requested path {path}"},
            {"Event": {"type": "write"}, "Envelope": {"message": path}},
            {"event": {"type": "open", "cwd": path}},
            {"output": f"simulated stdout contains {path}"},
        ]
        self.assertEqual(checker.match_marker_events(fake, [path])[path], [])

    def test_event_field_alias_and_null_padding(self):
        path = "/tmp/opencode-soak/workspace/nested/marker.txt"
        events = [{"event": {"Path": path + "\x00\x00", "pid": 7, "type": "openat"}}]
        self.assertEqual(len(checker.match_marker_events(events, [path])[path]), 1)

    def test_exec_alert_and_failed_open_cannot_satisfy_capture(self):
        path = "/tmp/opencode-soak/workspace/nested/marker-one.txt"
        records = [
            {"Event": {"type": "process_exec", "path": path, "pid": 11}},
            {"Event": {"type": "semantic_alert", "path": path, "pid": 11}},
            {"Event": {"type": "syscall", "path": path, "pid": 11}},
            {"Event": {"type": "openat", "path": path, "pid": 11, "retval": -13}},
            {"Event": {"type": "openat", "path": path, "pid": 11, "retval": "-2"}},
            {"Event": {"type": "openat", "path": path, "pid": 11, "retval": "invalid"}},
        ]
        self.assertEqual(checker.match_marker_events(records, [path])[path], [])

    def test_successful_openat_proves_capture_even_with_string_return_code(self):
        path = "/tmp/opencode-soak/workspace/nested/marker-one.txt"
        records = [{"Event": {"type": "openat", "path": path, "pid": 11, "retval": "7"}}]
        self.assertEqual(len(checker.match_marker_events(records, [path])[path]), 1)

    def test_pid_presence_distinguishes_lifecycle_from_confirmed_open(self):
        workspace = "/tmp/soak/workspace"
        records = [
            {"Event": {"pid": 401, "type": "process_exec", "cwd": workspace,
                       "path": "/secret/agent-binary", "raw": "SECRET_PAYLOAD"}},
            {"Event": {"pid": 401, "type": "process_exit", "cwd": workspace}},
            {"Event": {"pid": 402, "type": "openat", "cwd": workspace,
                       "path": workspace + "/nested/matched", "retval": 8}},
            {"Event": {"pid": 403, "type": "openat", "cwd": workspace,
                       "path": workspace + "/nested/failed", "retval": -13}},
            {"Event": {"pid": 404, "type": "openat", "cwd": "/elsewhere",
                       "path": "/secret/outside", "retval": 4}},
            {"Event": {"pid": 405, "type": "openat", "cwd": workspace,
                       "path": workspace + "/nested/overflow", "retval": 6}},
        ]
        summary = checker.workspace_pid_event_presence(
            records, {workspace}, limit=3
        )
        self.assertEqual([row["pid"] for row in summary], [401, 402, 403])
        self.assertEqual(summary[0]["eventTypes"], ["process_exec", "process_exit"])
        self.assertEqual(summary[0]["fileOpens"], 0)
        self.assertEqual(summary[1]["confirmedFileOpens"], 1)
        self.assertEqual(summary[2]["fileOpens"], 1)
        self.assertEqual(summary[2]["confirmedFileOpens"], 0)
        output = json.dumps(summary)
        self.assertNotIn("SECRET_PAYLOAD", output)
        self.assertNotIn("/secret/", output)
        # This is diagnostic only: a process_exec path cannot confirm openat.
        marker = workspace + "/nested/matched"
        self.assertFalse(checker.match_marker_events(records[:2], [marker])[marker])

    def test_report_is_structured_and_does_not_copy_raw_event_payloads(self):
        path = "/tmp/opencode-soak/workspace/nested/marker.txt"
        record = {"Event": {"path": path, "pid": 777, "comm": "touch",
                            "type": "openat", "raw": "SECRET_SHOULD_NOT_EXPORT"}}
        matches = checker.match_marker_events([record], [path])
        with tempfile.TemporaryDirectory() as temporary:
            report = os.path.join(temporary, "capture-report.json")
            checker.write_report(report, [path], matches, 1, None)
            parsed = json.loads(Path(report).read_text(encoding="utf-8"))
            self.assertEqual(parsed["status"], "pass")
            self.assertEqual(parsed["source"], "kernel-event-typed-path")
            self.assertEqual(parsed["markers"][0]["matchingEvents"], 1)
            self.assertEqual(parsed["markers"][0]["pids"], ["777"])
            self.assertNotIn("SECRET_SHOULD_NOT_EXPORT",
                             Path(report).read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()
