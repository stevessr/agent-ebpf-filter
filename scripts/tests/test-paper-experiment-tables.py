#!/usr/bin/env python3
"""Regression tests for the evidence-only paper table generator."""
import csv
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "paper-experiment-tables.py"
spec = importlib.util.spec_from_file_location("paper_experiment_tables", SCRIPT)
tables = importlib.util.module_from_spec(spec)
spec.loader.exec_module(tables)


def csv_file(path, columns, entries):
    with path.open("w", encoding="utf-8", newline="") as stream:
        writer = csv.DictWriter(stream, fieldnames=columns)
        writer.writeheader()
        writer.writerows(entries)


class PaperEvidenceTests(unittest.TestCase):
    def test_missing_data_never_looks_like_success(self):
        markdown = tables.build(None, None, None)
        self.assertIn("| 32 | — | — | Not measured |", markdown)
        self.assertIn("| 4 | Three modalities | N/A | N/A | N/A | 0 |", markdown)
        self.assertIn("| mmap | N/A | 0/0 | — | — |", markdown)

    def test_legacy_two_attached_phase_benchmark_is_not_overhead(self):
        with tempfile.TemporaryDirectory() as tmp:
            folder = Path(tmp)
            (folder / "summary_c1.json").write_text(json.dumps({
                "concurrency": 1,
                "operations": [{"overhead_percent": -17.0}],
                "baseline_state": "attached",
                "ebpf_state": "attached",
            }), encoding="utf-8")
            output = "\n".join(tables.concurrency_table(folder))
            self.assertIn("Invalid baseline", output)
            self.assertNotIn("-17.00", output)

    def test_verified_control_pair_can_report_mean(self):
        with tempfile.TemporaryDirectory() as tmp:
            folder = Path(tmp)
            (folder / "summary_c8.json").write_text(json.dumps({
                "concurrency": 8, "comparison_valid": True,
                "baseline_state": "detached", "ebpf_state": "attached",
                "operations": [
                    {"overhead_percent": 4.0},
                    {"overhead_percent": 6.0},
                ],
            }), encoding="utf-8")
            self.assertIn("| 8 | 5.00 | — | Verified detached/attached comparison |",
                          "\n".join(tables.concurrency_table(folder)))

    def test_attribution_confusion_partition(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "attribution.csv"
            fields = ["event_id", "agent_count", "command", "truth_agent", *tables.MODES]
            csv_file(path, fields, [
                dict(event_id="a", agent_count=2, command="git", truth_agent="A",
                     pid_only="A", lifecycle_hook_only="⊥", trimodal="A"),
                dict(event_id="b", agent_count=2, command="node", truth_agent="B",
                     pid_only="A", lifecycle_hook_only="B", trimodal="⊥"),
                dict(event_id="c", agent_count=2, command="python", truth_agent="B",
                     pid_only="⊥", lifecycle_hook_only="B", trimodal="B"),
            ])
            text = "\n".join(tables.attribution_table(path))
            self.assertIn("| 2 | PID only | 33.33% | 33.33% | 33.33% | 3 |", text)
            self.assertIn("| 2 | Three modalities | 66.67% | 0.00% | 33.33% | 3 |", text)

    def test_enforcement_requires_errno_and_control(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "enforcement.csv"
            fields = ["case", "control_ok", "denied", "errno", "restored",
                      "apply_us", "revoke_us"]
            csv_file(path, fields, [
                dict(case="tcp_connect", control_ok="true", denied="true",
                     errno="EACCES", restored="true", apply_us=101, revoke_us=78),
                dict(case="tcp_connect", control_ok="true", denied="true",
                     errno="ETIMEDOUT", restored="true", apply_us=120, revoke_us=83),
                dict(case="udp_sendto", control_ok="false", denied="true",
                     errno="EPERM", restored="true", apply_us=201, revoke_us=91),
            ])
            text = "\n".join(tables.enforcement_table(path))
            self.assertIn("| tcp_connect | 50.00% | 1/2 | 110.5 / 119.0 |", text)
            self.assertIn("| udp_sendto | 0.00% | 0/1 | 201.0 / 201.0 |", text)
            self.assertIn("| mmap | N/A | 0/0 | — | — |", text)

    def test_wilson_uncertainty_for_perfect_observed_rates(self):
        self.assertEqual(tables.wilson_interval(0, 0), "N/A")
        self.assertEqual(tables.wilson_interval(100, 100), "96.30%–100.00%")
        self.assertEqual(tables.wilson_interval(400, 400), "99.05%–100.00%")
        with self.assertRaises(ValueError):
            tables.wilson_interval(101, 100)

    def test_numerical_percentile(self):
        self.assertAlmostEqual(tables.percentile([0, 10, 20, 30, 40], 95), 38.0)


if __name__ == "__main__":
    unittest.main()
