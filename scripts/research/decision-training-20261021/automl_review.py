"""Offline review of an already frozen candidate. Never modifies weights or runtime."""
import argparse
import hashlib
import json
from pathlib import Path

from automl_protocol import research_gate, shadow_review_metrics


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def review(freeze_path, predictions_path, output_path):
    freeze = json.loads(freeze_path.read_text(encoding="utf-8"))
    if freeze.get("mode") != "shadow_only" or freeze.get("production_approved") is not False:
        raise ValueError("expected an unapproved shadow-only freeze")
    expected = freeze.get("artifact_sha256")
    if not isinstance(expected, str) or len(expected) != 64 or any(c not in "0123456789abcdef" for c in expected):
        raise ValueError("missing frozen checkpoint SHA-256")
    if output_path.exists():
        raise ValueError("refusing to overwrite an existing review")
    rows, scores = [], []
    for line in predictions_path.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        item = json.loads(line)
        if item.get("artifact_sha256") != expected:
            raise ValueError("review scored by a different model")
        rows.append(item)
        scores.append(item.get("score"))
    metrics = shadow_review_metrics(rows, scores, freeze["threshold"])
    gate = research_gate(freeze["empirical_calibration"], metrics)
    report = dict(version="privacy_automl_review.v1",
                  freeze_sha256=digest(freeze_path),
                  reviewed_predictions_sha256=digest(predictions_path),
                  model_sha256=expected, metrics=metrics, gate=gate,
                  note="Independent source and human labels must be audited separately; never an automatic policy promotion.")
    output_path.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--freeze", type=Path, required=True)
    parser.add_argument("--review-predictions", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    result = review(args.freeze, args.review_predictions, args.output)
    print(json.dumps(result["gate"], sort_keys=True))


if __name__ == "__main__":
    main()
