#!/usr/bin/env python3
"""Rootless launcher lifecycle tests using fake build/Vite/MyGo commands."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("renew-desktop.sh")


class RenewLauncherTest(unittest.TestCase):
    def run_launcher(self, desktop_status=0, frontend_status=None):
        with tempfile.TemporaryDirectory(prefix="renew launcher ") as temporary:
            root = Path(temporary)
            (root / "scripts").mkdir()
            shutil.copy2(SCRIPT, root / "scripts/renew-desktop.sh")
            (root / "frontend/node_modules").mkdir(parents=True)
            (root / "backend").mkdir()
            (root / "backend/.port").write_text("8097")
            (root / "desktop/renew").mkdir(parents=True)
            tools = root / "tools"
            tools.mkdir()
            commands = {
                "make": '#!/bin/bash\nprintf "%s\\n" "$*" >> "$TEST_LOG"\n',
                "curl": '#!/bin/bash\nif [[ -f "$TEST_READY" ]]; then echo "/@vite/client"; else exit 7; fi\n',
                "bun": """#!/bin/bash
set -e
if [[ -n "$FRONTEND_STATUS" ]]; then exit "$FRONTEND_STATUS"; fi
printf 'frontend %s %s\n' "$VITE_AGENT_BACKEND_URL" "$*" >> "$TEST_LOG"
trap 'echo frontend-stopped >> "$TEST_LOG"; exit 0' TERM INT
touch "$TEST_READY"
while true; do sleep 0.1; done
""",
                "go": """#!/bin/bash
printf 'desktop %s %s %s\n' "$AGENT_RENEW_UI_URL" "$AGENT_RENEW_DEV" "$AGENT_RENEW_BACKEND_BIN" >> "$TEST_LOG"
exit "$DESKTOP_STATUS"
""",
            }
            for name, source in commands.items():
                executable = tools / name
                executable.write_text(source)
                executable.chmod(0o755)
            env = os.environ.copy()
            for key in ("AGENT_BACKEND_URL", "AGENT_BACKEND_PORT", "RENEW_DESKTOP_DIR", "DEV_ENV_FILE"):
                env.pop(key, None)
            log = root / "test.log"
            env.update(
                PATH=f"{tools}:{env['PATH']}", TEST_LOG=str(log),
                TEST_READY=str(root / "ready"), DESKTOP_STATUS=str(desktop_status),
                FRONTEND_STATUS="" if frontend_status is None else str(frontend_status),
                RENEW_FRONTEND_PORT="5178",
            )
            result = subprocess.run([str(root / "scripts/renew-desktop.sh"), "dev"], env=env,
                                    text=True, capture_output=True, timeout=10)
            return result, log.read_text() if log.exists() else ""

    def test_default_stack_and_cleanup(self):
        result, log = self.run_launcher()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("backend-bare", log)
        self.assertIn("frontend http://127.0.0.1:8097", log)
        self.assertIn("--strictPort", log)
        self.assertIn("desktop http://127.0.0.1:5178 true", log)
        self.assertIn("frontend-stopped", log)

    def test_desktop_failure_propagates_and_cleans_frontend(self):
        result, log = self.run_launcher(desktop_status=7)
        self.assertEqual(result.returncode, 7, result.stderr)
        self.assertIn("frontend-stopped", log)

    def test_frontend_failure_does_not_open_desktop(self):
        result, log = self.run_launcher(frontend_status=9)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("desktop http", log)


if __name__ == "__main__":
    unittest.main()
