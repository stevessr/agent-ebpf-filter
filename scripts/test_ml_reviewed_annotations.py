"""Assertions for the individually reviewed, candidate-only Bash audit.
These do not elevate assistant assessments to independent human ground truth.
"""
import collections
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[1]
AUDIT = ROOT / "research" / "curated"
REVISION = "fcac513317e1487c75f5d3fd0f50f3ddb2ba325a"


def load_jsonl(filename):
    with (AUDIT / filename).open(encoding="utf-8") as stream:
        return [json.loads(line) for line in stream if line.strip()]


class ReviewedAnnotationContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.approved = load_jsonl("ai-reviewed-bash-actions.jsonl")
        cls.deferred = load_jsonl("ai-review-needs-context.jsonl")
        cls.manifest = json.loads((AUDIT / "ai-review-manifest.json").read_text(encoding="utf-8"))

    def test_approved_are_assistant_reviewed_with_rationale(self):
        self.assertEqual(collections.Counter(r["label"] for r in self.approved), {"ALLOW": 47, "BLOCK": 52})
        self.assertEqual(len(self.deferred), 25)
        all_rows = self.approved + self.deferred
        self.assertEqual(len(all_rows), 124)
        seen = set()
        for record in all_rows:
            with self.subTest(file=record["source_file"], line=record["source_row"]):
                self.assertEqual(record["label_source"], "assistant_reviewed")
                self.assertEqual(record["review_mode"].startswith("AI逐条静态语义审核"), True)
                self.assertTrue(record["review_reason_zh"].strip())
                self.assertTrue(record["command"].strip())
                self.assertEqual(record["source_revision"], REVISION)
                self.assertEqual(record["source_license"], "CC-BY-SA-4.0")
                self.assertTrue(record["run_id"].startswith("not-a-live-run-"))
                key = (record["source_file"], record["source_row"])
                self.assertNotIn(key, seen, "one original source row cannot be reviewed twice")
                seen.add(key)

    def test_uncertain_never_enters_training_corpus(self):
        for row in self.approved:
            self.assertEqual(row["review_status"], "approved_for_candidate_only")
            self.assertIn(row["label"], ("ALLOW", "BLOCK"))
        for row in self.deferred:
            self.assertEqual(row["review_status"], "needs_context")
            self.assertEqual(row["label"], "DEFER")

    def test_source_disagreements_are_intentional_and_traceable(self):
        reviewed = {(r["source_file"], r["source_row"]): r for r in self.approved}
        examples = [
            ("data/taglio4_3k/taglio4_safe.jsonl", 614, "safe", "BLOCK"),
            ("data/taglio4_3k/taglio4_safe.jsonl", 1595, "safe", "BLOCK"),
            ("data/taglio4_3k/taglio4_unsafe.jsonl", 238, "unsafe", "ALLOW"),
        ]
        for path, line, original, corrected in examples:
            with self.subTest(file=path, line=line):
                row = reviewed[(path, line)]
                self.assertEqual(row["source_original_label"], original)
                self.assertEqual(row["label"], corrected)
        self.assertEqual(len(self.manifest["sourceDisagreements"]), 3)
        self.assertEqual(self.manifest["readyRows"], len(self.approved))
        self.assertEqual(self.manifest["deferredRows"], len(self.deferred))

    def test_no_command_seen_with_conflicting_approved_actions(self):
        decisions = {}
        for row in self.approved:
            cmd = row["command"].strip()
            if cmd in decisions:
                self.assertEqual(decisions[cmd], row["label"], f"contradictory reviewed command {cmd!r}")
            decisions[cmd] = row["label"]
        self.assertEqual(len(decisions), len(self.approved))


if __name__ == "__main__":
    unittest.main()
