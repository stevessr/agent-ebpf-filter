"""Offline, standard-library regression tests for source-grounded annotation."""
import json
from pathlib import Path
from shutil import copytree
import tempfile
import unittest

import audit as annotation


class AnnotationAuditTests(unittest.TestCase):
    def test_baseline_has_no_fake_real_world_labels(self):
        result = annotation.audit()
        self.assertEqual(result["collected_source_references"], 11)
        self.assertEqual(result["analyst_constructed_cases"], 33)
        self.assertEqual(result["source_reported_procedures"], 8)
        self.assertEqual(set(result["verdict_counts"].values()), {11})
        self.assertEqual(result["real_system_call_traces"], 0)
        self.assertEqual(result["independent_human_ground_truth"], 0)
        self.assertFalse(result["production_training_authorized"])

    def mutate(self, filename, callback):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "audit"
            copytree(annotation.ROOT, root)
            path = root / filename
            if filename.endswith(".jsonl"):
                rows = annotation.load_jsonl(path)
                callback(rows)
                path.write_text("".join(json.dumps(x, ensure_ascii=False) + "\n" for x in rows))
            else:
                data = json.loads(path.read_text())
                callback(data)
                path.write_text(json.dumps(data, ensure_ascii=False))
            with self.assertRaises(ValueError):
                annotation.audit(root)

    def test_rejects_fabricated_human_verification(self):
        self.mutate("cases.jsonl", lambda rows: rows[0].update(human_verified=True))
        self.mutate("cases.jsonl", lambda rows: rows[0].update(ground_truth_label=1))
        self.mutate("cases.jsonl", lambda rows: rows[0].update(event_trace_observed=True))

    def test_rejects_source_and_scope_tampering(self):
        self.mutate("cases.jsonl", lambda rows: rows[0].update(source_id="unverified"))
        self.mutate("cases.jsonl", lambda rows: rows[0].update(authorization="approved"))
        self.mutate("sources.json", lambda value: value["sources"][0].update(url="http://example.invalid"))

    def test_rejects_missing_and_noncanonical_tags(self):
        self.mutate("cases.jsonl", lambda rows: rows[0].update(tags=["FILE_READ"]))
        self.mutate("cases.jsonl", lambda rows: rows[0].update(missing_evidence=[]))
        self.mutate("cases.jsonl", lambda rows: rows[0].update(signal_hypotheses=["one", "one"]))

    def test_rejects_documented_report_promoted_to_real_capture(self):
        self.mutate("documented_cases.jsonl", lambda rows: rows[0].update(raw_trace_included=True))
        self.mutate("documented_cases.jsonl", lambda rows: rows[0].update(training_ground_truth_usable=True))
        self.mutate("documented_cases.jsonl", lambda rows: rows[0].update(syscall_sequence_verified=True))
        self.mutate("documented_cases.jsonl", lambda rows: rows[0].update(procedure_id="unverified"))

    def test_rejects_duplicate_identity(self):
        self.mutate("cases.jsonl", lambda rows: rows[1].update(case_id=rows[0]["case_id"]))
        self.mutate("documented_cases.jsonl", lambda rows: rows[1].update(case_id=rows[0]["case_id"]))


if __name__ == "__main__":
    unittest.main()
