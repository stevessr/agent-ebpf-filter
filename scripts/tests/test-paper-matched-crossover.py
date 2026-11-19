#!/usr/bin/env python3
"""These are synthetic test fixtures, never reported as research observations."""
from __future__ import annotations

import csv
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

SOURCE = Path(__file__).resolve().parents[1] / "paper-matched-crossover-summary.py"
spec = importlib.util.spec_from_file_location("paper_crossover", SOURCE)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class MatchedCrossoverTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.folder = Path(self.temp.name)
        self.manifest = {
            "kind": "matched_daemon_real_kernel_link_crossover",
            "restored_tracepoint_links": True, "seed": 1,
            "cycles_per_concurrency": 2, "samples_per_phase": 1000,
            "expected_link_count": 202, "kernel": "unit-test-kernel",
            "tested_sha": "example-sha", "phases": [],
        }
        for c in module.CONCURRENCIES:
            for pair in (1, 2):
                for mode in ("off", "on"):
                    filename = f"c{c}-pair{pair}-{mode}.csv"
                    with (self.folder / filename).open("w", newline="") as file:
                        writer = csv.writer(file)
                        writer.writerow(["latency_us"])
                        writer.writerows([[1.0 if mode == "off" else 2.0]] * 1000)
                    self.manifest["phases"].append({
                        "concurrency": c, "pair": pair, "mode": mode,
                        "file": filename, "backend_healthy": True,
                        "kernel_tracepoint_links": 202 if mode == "on" else 0,
                        "actual_kernel_link_ids_verified": True,
                    })
        self.write_manifest()

    def tearDown(self):
        self.temp.cleanup()

    def write_manifest(self):
        (self.folder / "manifest.json").write_text(json.dumps(self.manifest), encoding="utf-8")

    def test_positive_paired_crossover(self):
        data = module.analyze(self.folder)
        self.assertEqual(data["by_concurrency"]["32"]["off_samples"], 2000)
        self.assertEqual(data["by_concurrency"]["32"]["on_samples"], 2000)
        self.assertEqual(data["by_concurrency"]["32"]["paired_mean_change_percent"], 100.0)
        self.assertEqual(data["by_concurrency"]["32"]["on_p95_us"], 2.0)
        self.assertEqual(data["by_concurrency"]["32"]["paired_ci95_percent"], [100.0, 100.0])

    def test_rejects_unverified_off_state(self):
        self.manifest["phases"][0]["actual_kernel_link_ids_verified"] = False
        self.write_manifest()
        with self.assertRaisesRegex(ValueError, "kernel or backend state"):
            module.analyze(self.folder)

    def test_rejects_missing_samples(self):
        (self.folder / "c1-pair1-off.csv").unlink()
        with self.assertRaisesRegex(ValueError, "missing measured sample"):
            module.analyze(self.folder)

    def test_rejects_duplicate_pair(self):
        self.manifest["phases"][1]["mode"] = "off"
        self.write_manifest()
        with self.assertRaisesRegex(ValueError, "duplicate mode"):
            module.analyze(self.folder)

    def test_rejects_failed_kernel_restoration(self):
        self.manifest["restored_tracepoint_links"] = False
        self.write_manifest()
        with self.assertRaisesRegex(ValueError, "restored"):
            module.analyze(self.folder)

    def test_bootstrap_resamples_pairs_not_per_call(self):
        self.assertEqual(module.pair_bootstrap_ci([0, 0, 0], seed=3), (0.0, 0.0))
        with self.assertRaises(ValueError):
            module.pair_bootstrap_ci([1.0], seed=3)


if __name__ == "__main__":
    unittest.main()
