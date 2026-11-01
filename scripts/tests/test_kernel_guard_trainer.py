"""Contract tests for trainer's deterministic split and fixed features."""
import importlib.util
import pathlib
import unittest

MODULE = pathlib.Path(__file__).resolve().parents[1] / "train-kernel-guard.py"
spec = importlib.util.spec_from_file_location("kernel_guard_trainer", MODULE)
trainer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(trainer)


class KernelGuardTrainerTests(unittest.TestCase):
    def test_ipv4_feature_contract(self):
        self.assertEqual(trainer.features("8.8.8.8", 4444), (1, 0, 1, 0, 0, 0))
        self.assertEqual(trainer.features("8.8.8.8", 22), (1, 1, 0, 0, 0, 0))
        self.assertEqual(trainer.features("8.8.8.8", 50000), (1, 0, 0, 1, 0, 0))
        for ip in ("127.0.0.1", "10.0.0.1", "172.19.1.2",
                   "192.168.1.9", "198.51.100.1", "203.0.113.2"):
            self.assertEqual(trainer.features(ip, 4444), (0,) * 6)
        self.assertEqual(trainer.features("::ffff:8.8.8.8", 4444), (1, 0, 1, 0, 0, 0))
        self.assertEqual(trainer.features("::ffff:10.0.0.1", 4444), (0,) * 6)
        self.assertEqual(trainer.features("::1", 4444), (0,) * 6)

    def test_wilson_bounds_are_conservative(self):
        _, fp_upper = trainer.wilson_bounds95(0, 1000)
        self.assertGreater(fp_upper, 0.001)
        lower, _ = trainer.wilson_bounds95(95, 100)
        self.assertLess(lower, 0.95)

    def test_v3_report_binds_runs_and_dataset(self):
        import hashlib
        model = {"bias": -100, "threshold": 100,
                 "weights": [0, 0, 300, 0, 0, 0]}
        rows = [("run-1", trainer.features("8.8.8.8", 443), 0),
                ("run-2", trainer.features("8.8.8.8", 4444), 1)]
        report = trainer.evaluate(model, rows, "b" * 64, "a" * 64)
        self.assertEqual(report["schemaVersion"], "kernel-guard-holdout-v3")
        self.assertEqual(report["validationSHA256"], "a" * 64)
        self.assertEqual(report["attackRunsWithTP"], 1)
        self.assertEqual(report["benignRunsWithFP"], 0)
        self.assertEqual(report["attackRunRecallLower95"] < 1, True)

    def test_run_split_does_not_split_one_run(self):
        one = trainer.split_group("agent-run-one")
        self.assertEqual(one, trainer.split_group("agent-run-one"))
        self.assertIn(one, ("train", "holdout"))

    def test_training_model_rejects_generic_traffic(self):
        data = [
            (f"run-{i}", trainer.features("8.8.8.8", port),
             int(port == 4444))
            for i in range(40) for port in [80, 443, 22, 4444]
            for _ in range(2)
        ]
        train = [r for r in data if trainer.split_group(r[0]) == "train"]
        model = trainer.train(train, epochs=40)
        self.assertLess(model["bias"] + model["weights"][0], model["threshold"])
        self.assertFalse(trainer.predict(model, trainer.features("8.8.8.8", 80)))


if __name__ == "__main__":
    unittest.main()
