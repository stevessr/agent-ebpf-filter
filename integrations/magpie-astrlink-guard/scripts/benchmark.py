#!/usr/bin/env python3
"""Warm inference latency benchmark with synthetic, never personal, strings."""
from pathlib import Path
import argparse
import json
import statistics
import sys
import time

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from service import GuardScanner, MODEL_FILES

parser = argparse.ArgumentParser(description="Local AstrLink Guard inference benchmark")
parser.add_argument("--model-dir", type=Path, required=True)
parser.add_argument("--backend", choices=("auto", *MODEL_FILES), default="auto")
parser.add_argument("--runs", type=int, default=30)
parser.add_argument("--strict-provider", action="store_true")
args = parser.parse_args()
if args.runs < 5:
    parser.error("--runs must be >= 5")

scanner = GuardScanner(args.model_dir, args.backend, args.strict_provider, .8)
inputs = [
    "Please redact alice@example.com and do not send the API key sk-ABCDEF0123456789012345678.",
    "请检查内部项目日志：张三，联系邮箱 zhangsan@example.com，并移除令牌。",
    "# Deployment example\nexport API_KEY=0123456789example-secret\nconsole.log('configured')",
]
for text in inputs:
    scanner.detect(text)  # warm-up
latencies = []
for i in range(args.runs):
    began = time.perf_counter_ns()
    scanner.detect(inputs[i % len(inputs)])
    latencies.append((time.perf_counter_ns() - began) / 1_000_000)
latencies.sort()
print(json.dumps({
    "backend": scanner.backend,
    "execution_provider": scanner.ep,
    "runs": len(latencies),
    "p50_ms": round(statistics.median(latencies), 3),
    "p95_ms": round(latencies[min(len(latencies) - 1, int(.95 * len(latencies)))], 3),
    "mean_ms": round(statistics.mean(latencies), 3),
}, indent=2))
