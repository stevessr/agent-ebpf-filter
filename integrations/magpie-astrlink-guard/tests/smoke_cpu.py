"""Actual AstrLink Guard v0.1.3 smoke test; needs ONNX model and dependencies.

Run only in the opt-in/model-enabled CI job; offline mock unit tests do not
require external model downloads.
"""
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from service import GuardScanner

root = Path(sys.argv[1])
scanner = GuardScanner(root, "cpu", strict=True, threshold=0.8)
assert scanner.backend == "cpu", scanner.backend
sentence = "Contact alice@example.com about this deployment."
spans = scanner.detect(sentence)
assert any(s["label"] == "email" and "alice@example.com" in sentence[s["start"]:s["end"]]
           for s in spans), "Expected email entity was not detected"
print("AstrLink Guard CPU inference smoke passed; selected provider:", scanner.ep)
