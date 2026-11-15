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
        events = [{"event": {"Path": path + "\x00\x00", "pid": 7}}]
        self.assertEqual(len(checker.match_marker_events(events, [path])[path]), 1)

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
