import json
import unittest
from adapt_capture import adapt
from train_http import template, canonical, select_features
import numpy as np


class PipelineTests(unittest.TestCase):
    def test_archive_missing_is_not_success_or_resolved_path(self):
        o, status = adapt({"event": {"type": "read", "path": "/etc/shadow", "bytes": 4096,
                                   "risk_score": 1, "decision": "allow"}}, "archive")
        self.assertEqual(status, "syscall")
        self.assertNotIn("return", o["syscall"])
        self.assertNotIn("fd_path", o["syscall"])
        self.assertNotIn("risk_score", json.dumps(o))
        self.assertEqual(o["syscall"]["requested_bytes"], 4096)
        self.assertNotIn("process_start_ns", o["context"])

    def test_open_flags_not_mode_and_zero_retained(self):
        o, _ = adapt({"type": "open", "retval": 0, "mode": "0755", "extra_info": "flags=0x241 mode=0644"}, "archive")
        self.assertEqual(o["syscall"]["return"], 0)
        self.assertEqual(o["syscall"]["open_flags"], 0x241)
        self.assertEqual(o["syscall"]["permission_mode"], 0o755)
        self.assertTrue(o["syscall"]["relative_base_unknown"])

    def test_tls_map_cannot_prove_duplicate_headers(self):
        o, _ = adapt({"method": "POST", "url": "/api", "direction": "write", "headers": {"Content-Length": "3"}}, "tls")
        self.assertFalse(o["http"]["headers_preserve_duplicates"])
        self.assertNotIn("direction", o["http"])
        self.assertNotIn("truncated", o["http"])

    def test_ip_sni_and_tool_call_not_http_or_syscall(self):
        for e in [{"type": "connect", "dst_ip": "127.0.0.1", "sni": "example.test"},
                  {"type": "tool_use", "tool": "read", "path": "/etc/shadow"}]:
            o, reason = adapt(e, "archive")
            self.assertIsNone(o)
            self.assertEqual(reason, "no_syscall_or_request_evidence")

    def test_grouping_and_bounded_decoding(self):
        self.assertEqual(template("SELECT alpha FROM beta WHERE id=123"), template("select gamma from delta where id=999"))
        self.assertEqual(canonical("%25252e"), "%2e")

    def test_selector_uses_only_supplied_fit_rows(self):
        rng = np.random.default_rng(1)
        x = rng.uniform(size=(200, 5)); x[:, 4] = 1
        y = (x[:, 0] > .5).astype(int)
        cols, report = select_features(x, y, list("abcde"))
        self.assertNotIn(4, cols)
        self.assertEqual(report["fit_rows"], 200)
        self.assertEqual(cols[0], 0)


if __name__ == "__main__":
    unittest.main()
