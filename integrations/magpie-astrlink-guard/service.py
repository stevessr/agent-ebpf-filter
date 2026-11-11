#!/usr/bin/env python3
"""Local-only AstrLink Guard 0.1.3 ONNX scanner for the Magpie audit hook.

No model downloads, remote inference, raw-content logging, or shell calls happen
in the request path. Supply model files separately from the upstream publisher.
"""
from __future__ import annotations

import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import ipaddress
import json
import os
from pathlib import Path
import threading
import time
from collections import Counter


MODEL_FILES = {
    "cpu": ("int8", "model_int8.onnx", "CPUExecutionProvider"),
    "cuda": ("fp16", "model_fp16_static512.onnx", "CUDAExecutionProvider"),
    "tensorrt": ("fp16", "model_fp16_static512.onnx", "TensorrtExecutionProvider"),
    "rocm": ("fp16", "model_fp16_static512.onnx", "ROCMExecutionProvider"),
    "directml": ("fp16", "model_fp16_static512.onnx", "DmlExecutionProvider"),
    "coreml": ("fp16", "model_fp16_static512.onnx", "CoreMLExecutionProvider"),
    "openvino": ("fp16", "model_fp16_static512.onnx", "OpenVINOExecutionProvider"),
    "webgpu": ("fp16", "model_fp16_static512.onnx", "WebGPUExecutionProvider"),
    # Official ONNX Runtime wheels do NOT ship VulkanExecutionProvider.
    # This entry only works with a separately built/installed EP that advertises it.
    "vulkan": ("fp16", "model_fp16_static512.onnx", "VulkanExecutionProvider"),
}
AUTO_ORDER = ("cuda", "tensorrt", "rocm", "directml", "coreml", "openvino", "webgpu", "vulkan", "cpu")


def choose_backend(backend: str, available: list[str], model_dir: Path, strict: bool = False):
    """Select only an actually available EP with its complete local ONNX model."""
    if backend != "auto" and backend not in MODEL_FILES:
        raise ValueError(f"Unknown backend {backend}")
    order = AUTO_ORDER if backend == "auto" else (backend, "cpu")
    if backend != "auto" and strict:
        order = (backend,)
    for name in order:
        folder, filename, ep = MODEL_FILES[name]
        p = model_dir / folder
        if ep in available and (p / filename).is_file() and (p / "tokenizer.json").is_file() and (p / "labels.json").is_file():
            return name, ep, p / filename
    raise RuntimeError("No available ONNX execution provider with complete local model files")


def utf16_offsets(text: str) -> list[int]:
    positions = [0]
    for char in text:
        positions.append(positions[-1] + (2 if ord(char) > 0xFFFF else 1))
    return positions


def parse_bio(tag: str):
    if tag == "O" or not tag:
        return None, None
    if tag[:2] in ("B-", "I-", "B_", "I_"):
        return tag[0], tag[2:]
    return None, None


def collect_entities(labels, offsets, special_mask, scores, min_score: float):
    """Decode one 512-token window into character spans; ignore special tokens."""
    spans = []
    active = None
    for index, tag in enumerate(labels):
        start, end = offsets[index]
        kind, name = parse_bio(tag)
        if special_mask[index] or start == end or kind is None:
            if active is not None:
                spans.append(active)
                active = None
            continue
        score = float(scores[index])
        if kind == "B" or active is None or active["label"] != name:
            if active is not None:
                spans.append(active)
            active = {"start": start, "end": end, "label": name, "scores": [score]}
        else:
            active["end"] = max(active["end"], end)
            active["scores"].append(score)
    if active is not None:
        spans.append(active)
    return [
        {"start": s["start"], "end": s["end"], "label": s["label"], "score": sum(s["scores"]) / len(s["scores"])}
        for s in spans if sum(s["scores"]) / len(s["scores"]) >= min_score
    ]


def merge_windows(spans: list[dict]) -> list[dict]:
    """Deduplicate overlapping stride windows; prefer higher-confidence spans."""
    result = []
    for span in sorted(spans, key=lambda s: (-s["score"], s["start"], -s["end"])):
        if any(span["start"] < seen["end"] and seen["start"] < span["end"] for seen in result):
            continue
        result.append(span)
    return sorted(result, key=lambda s: (s["start"], s["end"]))


class GuardScanner:
    def __init__(self, model_dir: Path, backend: str, strict: bool, threshold: float):
        import numpy as np
        import onnxruntime as ort
        from tokenizers import Tokenizer

        available = ort.get_available_providers()
        candidates = AUTO_ORDER if backend == "auto" else ((backend,) if strict else (backend, "cpu"))
        errors = []
        for selected in candidates:
            try:
                name, ep, model = choose_backend(selected, available, model_dir, strict=True)
                folder = model.parent
                tok = Tokenizer.from_file(str(folder / "tokenizer.json"))
                tok.enable_truncation(max_length=512, stride=128)
                labels = json.loads((folder / "labels.json").read_text(encoding="utf8"))["id2label"]
                session = ort.InferenceSession(str(model), providers=[ep, "CPUExecutionProvider"] if name != "cpu" else [ep])
                if ep not in session.get_providers():
                    raise RuntimeError(f"Provider {ep} not active in loaded session")
                self.np, self.tokenizer, self.id2label = np, tok, labels
                self.session, self.backend, self.ep = session, name, ep
                self.fixed = name != "cpu"
                self.threshold = threshold
                self.input_names = {p.name for p in session.get_inputs()}
                self.lock = threading.Lock()
                return
            except Exception as error:
                errors.append(f"{selected}: {type(error).__name__}")
        raise RuntimeError("AstrLink Guard model unavailable (" + ", ".join(errors) + ")")

    def detect(self, text: str) -> list[dict]:
        np = self.np
        with self.lock:
            first = self.tokenizer.encode(text)
            windows = [first] + list(first.overflowing)
            results = []
            for window in windows:
                ids = np.asarray(window.ids, dtype=np.int64)
                n = len(ids)
                if self.fixed:
                    input_ids = np.full((1, 512), 1, dtype=np.int64)
                    mask = np.zeros((1, 512), dtype=np.int64)
                    input_ids[0, :n] = ids
                    mask[0, :n] = 1
                else:
                    input_ids = ids.reshape(1, -1)
                    mask = np.ones_like(input_ids)
                feed = {"input_ids": input_ids, "attention_mask": mask,
                        "token_type_ids": np.zeros_like(input_ids)}
                feed = {key: val for key, val in feed.items() if key in self.input_names}
                logits = self.session.run(None, feed)[0][0][:n]
                best = np.argmax(logits, axis=-1)
                shifted = logits - np.max(logits, axis=-1, keepdims=True)
                probs = np.exp(shifted)
                probs /= np.sum(probs, axis=-1, keepdims=True)
                tags = [self.id2label[str(int(i))] if isinstance(self.id2label, dict)
                        else self.id2label[int(i)] for i in best]
                confidences = [float(probs[j, i]) for j, i in enumerate(best)]
                results.extend(collect_entities(tags, window.offsets, window.special_tokens_mask,
                                                confidences, self.threshold))
            positions = utf16_offsets(text)
            return [dict(s, start=positions[s["start"]], end=positions[s["end"]])
                    for s in merge_windows(results) if 0 <= s["start"] < s["end"] <= len(text)]


def make_handler(scanner, token: str, max_bytes: int = 1048576):
    # Memory-only counts. Never store text or span values.
    metrics = {"requests": 0, "texts": 0, "spans": Counter(), "latency_ms_sum": 0.0}
    metrics_lock = threading.Lock()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, fmt, *args):
            pass  # Do not log paths, prompts, credentials, or matched spans.

        def respond(self, code, payload):
            raw = json.dumps(payload, separators=(",", ":")).encode("utf8")
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            self.wfile.write(raw)

        def do_GET(self):
            if self.path == "/stats":
                if token and self.headers.get("Authorization", "") != f"Bearer {token}":
                    return self.respond(401, {"error": "unauthorized"})
                with metrics_lock:
                    self.respond(200, {
                        "requests": metrics["requests"],
                        "texts": metrics["texts"],
                        "spansByLabel": dict(metrics["spans"]),
                        "meanLatencyMs": round(metrics["latency_ms_sum"] / max(1, metrics["requests"]), 3),
                        "backend": scanner.backend,
                    })
            elif self.path == "/health":
                self.respond(200, {"status": "ok", "backend": scanner.backend,
                                   "executionProvider": scanner.ep})
            else:
                self.respond(404, {"error": "not-found"})

        def do_POST(self):
            if self.path != "/scan":
                return self.respond(404, {"error": "not-found"})
            if token and self.headers.get("Authorization", "") != f"Bearer {token}":
                return self.respond(401, {"error": "unauthorized"})
            try:
                length = int(self.headers.get("Content-Length", "-1"))
                if not (0 <= length <= max_bytes):
                    return self.respond(413, {"error": "request-too-large"})
                obj = json.loads(self.rfile.read(length))
                texts = obj["texts"]
                if not isinstance(texts, list) or len(texts) > 64 or any(
                    not isinstance(t, str) or len(t) > 16384 for t in texts
                ):
                    return self.respond(422, {"error": "invalid-input"})
                began = time.perf_counter()
                spans = [scanner.detect(t) for t in texts]
                with metrics_lock:
                    metrics["requests"] += 1
                    metrics["texts"] += len(texts)
                    metrics["spans"].update(s["label"] for group in spans for s in group)
                    metrics["latency_ms_sum"] += (time.perf_counter() - began) * 1000
                self.respond(200, {"spans": spans, "backend": scanner.backend})
            except (ValueError, TypeError, KeyError, UnicodeDecodeError):
                self.respond(400, {"error": "bad-request"})
            except Exception:
                self.respond(503, {"error": "inference-unavailable"})

    return Handler


def main():
    parser = argparse.ArgumentParser(description="Local AstrLink Guard Magpie scan service")
    parser.add_argument("--model-dir", type=Path, required=True)
    parser.add_argument("--backend", choices=("auto", *MODEL_FILES), default="auto")
    parser.add_argument("--strict-provider", action="store_true",
                        help="Do not fall back to CPU when the selected EP cannot run")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8765)
    parser.add_argument("--threshold", type=float, default=0.8)
    args = parser.parse_args()
    if not ipaddress.ip_address(args.host).is_loopback:
        parser.error("--host must be an IP loopback address")
    if not 0 < args.threshold <= 1:
        parser.error("--threshold must be in (0,1]")
    scanner = GuardScanner(args.model_dir, args.backend, args.strict_provider, args.threshold)
    token = os.environ.get("ASTRLINK_GUARD_TOKEN", "")
    server = ThreadingHTTPServer((args.host, args.port), make_handler(scanner, token))
    print(f"Local Guard ready on {args.host}:{args.port} (backend={scanner.backend}, ep={scanner.ep})", flush=True)
    try:
        server.serve_forever(poll_interval=0.2)
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
