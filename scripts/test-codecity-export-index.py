#!/usr/bin/env python3
"""Metadata-only CodeCity manifest export safety tests."""
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("codecity-export-index.py")


class ExportTests(unittest.TestCase):
    def test_export_excludes_hidden_directories_and_symlinks(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            root = base / "project"
            root.mkdir()
            (root / "src").mkdir()
            (root / "src" / "main.rs").write_text("some private source", encoding="utf-8")
            (root / ".git").mkdir()
            (root / ".git" / "config").write_text("secret")
            (root / "node_modules").mkdir()
            (root / "node_modules" / "ignored.js").write_text("secret")
            (root / "secret_link").symlink_to(root / ".git" / "config")
            manifest_path = base / "output.json"
            result = subprocess.run(
                [sys.executable, str(SCRIPT), str(root), "--output", str(manifest_path)],
                capture_output=True, text=True, check=True,
            )
            self.assertIn("1 relative file paths", result.stdout)
            data = json.loads(manifest_path.read_text())
            self.assertEqual(data, {"version": 1, "files": ["src/main.rs"]})
            self.assertNotIn("some private source", manifest_path.read_text())
            self.assertEqual(manifest_path.stat().st_mode & 0o777, 0o600)


if __name__ == "__main__":
    unittest.main()
