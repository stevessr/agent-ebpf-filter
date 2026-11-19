#!/usr/bin/env python3
"""Offline checks for the live enforcement trial recorder."""
import errno
import importlib.util
from pathlib import Path
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / "paper-enforcement-trials.py"
spec = importlib.util.spec_from_file_location("paper_enforcement_trials", SCRIPT)
evidence = importlib.util.module_from_spec(spec)
spec.loader.exec_module(evidence)


class FakeBackend:
    def __init__(self):
        self.blocked = False
        self.calls = []

    def update(self, path, payload):
        self.calls.append((path, payload))
        self.blocked = "/unblock-" not in path
        return 12.5


class EnforcementRecorderTests(unittest.TestCase):
    def test_control_deny_restore_lifecycle(self):
        backend = FakeBackend()

        def operation():
            if backend.blocked:
                raise PermissionError(errno.EACCES, "denied by fixture")

        row = evidence.measure_policy(
            backend, "existing_fd_read", operation,
            "/sandbox/lsm/block-file-name", "/sandbox/lsm/unblock-file-name",
            {"name": "fixture"},
        )
        self.assertEqual(row["control_ok"], "true")
        self.assertEqual(row["denied"], "true")
        self.assertEqual(row["errno"], 13)
        self.assertEqual(row["restored"], "true")
        self.assertEqual(row["apply_us"], "12.500")
        self.assertEqual(len(backend.calls), 2)
        self.assertFalse(backend.blocked)

    def test_observe_actual_error_never_coerces_errno(self):
        ok, returned = evidence.attempt(
            lambda: (_ for _ in ()).throw(OSError(errno.ECONNREFUSED, "refused"))
        )
        self.assertFalse(ok)
        self.assertEqual(returned, errno.ECONNREFUSED)


if __name__ == "__main__":
    unittest.main()
