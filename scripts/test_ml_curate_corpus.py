"""Offline regression tests for reviewed pretraining corpus preparation."""
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("ml-curate-corpus.py")
SPEC = importlib.util.spec_from_file_location("ml_curate_corpus", SCRIPT)
corpus = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(corpus)


class CuratedCorpusTests(unittest.TestCase):
    def write_rows(self, directory, rows, filename="source.jsonl"):
        target = Path(directory) / filename
        target.write_text("".join(json.dumps(row) + "\n" for row in rows), encoding="utf-8")
        return str(target)

    def rows(self):
        result = []
        for scenario in range(10):
            for i in range(8):
                result.append({
                    "command": f"tool-{scenario} --case={i} --mode={i % 2}",
                    "label": "ALLOW" if i % 2 == 0 else "BLOCK",
                    "label_source": "human_reviewed",
                    "scenario_id": f"scenario-{scenario}",
                    "run_id": f"run-{scenario // 2}",
                })
        return result

    def test_deterministic_scenario_disjoint_split_and_hashes(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = self.write_rows(temporary, self.rows())
            one = corpus.build_corpus({"source": path}, {"source": "MIT"}, Path(temporary)/"one", min_per_class=2)
            two = corpus.build_corpus({"source": path}, {"source": "MIT"}, Path(temporary)/"two", min_per_class=2)
            self.assertEqual(one, two)
            group_sets = {}
            command_sets = {}
            for split in ("train", "validation", "test"):
                raw = (Path(temporary)/"one"/f"{split}.jsonl").read_bytes()
                entries = [json.loads(line) for line in raw.splitlines()]
                group_sets[split] = {(row["dataset"], row["scenario_id"]) for row in entries}
                command_sets[split] = {row["command"] for row in entries}
                self.assertEqual(one["split"][split]["sha256"], hashlib.sha256(raw).hexdigest())
                self.assertGreater(len(entries), 0)
            for left, right in (("train", "validation"), ("train", "test"), ("validation", "test")):
                self.assertFalse(group_sets[left] & group_sets[right])
                self.assertFalse(command_sets[left] & command_sets[right])

    def test_duplicate_command_purged_across_splits(self):
        with tempfile.TemporaryDirectory() as temporary:
            rows = self.rows()
            for scenario in range(10):
                rows.append({
                    "command": "uname -a",
                    "label": "ALLOW",
                    "label_source": "human_reviewed",
                    "scenario_id": f"scenario-{scenario}",
                    "run_id": f"run-{scenario // 2}",
                })
            path = self.write_rows(temporary, rows)
            manifest = corpus.build_corpus({"a": path}, {"a": "MIT"}, Path(temporary)/"out", min_per_class=2)
            self.assertGreater(sum(s["deduplicatedOrPurgedRows"] for s in manifest["split"].values()), 0)
            all_commands = []
            for split in ("train", "validation", "test"):
                all_commands.extend(json.loads(line)["command"] for line in
                                    (Path(temporary)/"out"/f"{split}.jsonl").read_text().splitlines())
            self.assertEqual(len(all_commands), len(set(all_commands)))

    def test_unreviewed_attack_window_cannot_become_block(self):
        with tempfile.TemporaryDirectory() as temporary:
            rows = self.rows()
            rows[0]["label_source"] = "attack_window"
            path = self.write_rows(temporary, rows)
            with self.assertRaisesRegex(ValueError, "reviewed action provenance"):
                corpus.build_corpus({"source": path}, {"source": "MIT"}, Path(temporary)/"out", min_per_class=1)

    def test_conflicting_labels_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            rows = self.rows()
            rows[1]["command"] = rows[0]["command"]
            path = self.write_rows(temporary, rows)
            with self.assertRaisesRegex(ValueError, "conflicting action labels"):
                corpus.build_corpus({"source": path}, {"source": "MIT"}, Path(temporary)/"out", min_per_class=1)

    def test_source_license_required_and_nonbinary_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            rows = self.rows()
            rows[0]["label"] = "ATTACK"
            path = self.write_rows(temporary, rows)
            with self.assertRaisesRegex(ValueError, "explicit binary"):
                corpus.build_corpus({"source": path}, {"source": "MIT"}, Path(temporary)/"out", min_per_class=1)
            with self.assertRaisesRegex(ValueError, "matching"):
                corpus.build_corpus({"source": path}, {}, Path(temporary)/"out", min_per_class=1)


if __name__ == "__main__":
    unittest.main()
