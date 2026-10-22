"""End-to-end local-only review: digest checks and no overwriting old audits."""
import json
import tempfile
from pathlib import Path
import unittest

from automl_review import review


class ReviewCLITests(unittest.TestCase):
    def test_snapshot_attestation_and_no_overwrite(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            artifact = "a" * 64
            freeze = root / "freeze.json"
            freeze.write_text(json.dumps(dict(
                mode="shadow_only", production_approved=False,
                artifact_sha256=artifact, threshold=0.6,
                empirical_calibration=dict(fpr_upper95=0.04))))
            predictions = root / "review.jsonl"
            examples = [
                dict(id="1", group="benign", domain="privacy_action",
                     split="independent_review", source="new_source",
                     tags=['FileRead', 'read-file'],
                     annotation_source="manually_reviewed", label=0,
                     score=0.2, artifact_sha256=artifact),
                dict(id="2", group="unsafe", domain="privacy_action",
                     split="independent_review", source="new_source",
                     annotation_source="manually_reviewed", label=1,
                     score=0.8, artifact_sha256=artifact)]
            predictions.write_text("".join(json.dumps(r) + "\n" for r in examples))
            output = root / "audit.json"
            result = review(freeze, predictions, output)
            self.assertEqual(result["metrics"]["tp"], 1)
            self.assertEqual(result['metrics']['slices']['by_tag'][0]['key'], 'file_read')
            self.assertEqual(result['metrics']['slices']['by_tag'][0]['rows'], 1)
            self.assertTrue(output.exists())
            self.assertFalse(result["gate"]["production_approved"])
            self.assertFalse(result["gate"]["shadow_review_candidate"])
            with self.assertRaisesRegex(ValueError, "overwrite"):
                review(freeze, predictions, output)
            examples[0]["artifact_sha256"] = "b" * 64
            predictions.write_text("".join(json.dumps(r) + "\n" for r in examples))
            with self.assertRaisesRegex(ValueError, "different model"):
                review(freeze, predictions, root / "other.json")


if __name__ == "__main__":
    unittest.main()

