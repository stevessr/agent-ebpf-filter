#!/usr/bin/env python3
"""Group-held-out offline trainer for the opt-in IPv4 cgroup linear guard.

Labels must be independently adjudicated; this script never rewrites existing
runtime policies or claims model confidence/probability calibration.
"""
import argparse
import csv
import hashlib
import ipaddress
import json
import math
from pathlib import Path

FEATURE_COUNT = 6
MODEL_SCHEMA = "kernel-cgroup-linear-v1"
REPORT_SCHEMA = "kernel-guard-holdout-v2"


def external_ipv4(address):
    ip = ipaddress.ip_address(address)
    if ip.version == 6 and ip.ipv4_mapped is not None:
        ip = ip.ipv4_mapped
    if ip.version != 4:
        return False
    a, b, c, _ = ip.packed
    if a in (0, 10, 127) or a >= 224:
        return False
    if a == 100 and 64 <= b <= 127:
        return False
    if a == 169 and b == 254:
        return False
    if a == 172 and 16 <= b <= 31:
        return False
    if a == 192 and (b == 168 or (b == 0 and c == 2)):
        return False
    if a == 198 and (b in (18, 19) or (b == 51 and c == 100)):
        return False
    if a == 203 and b == 0 and c == 113:
        return False
    return True


def features(ip, port):
    if not external_ipv4(ip):
        return (0, 0, 0, 0, 0, 0)
    return (1, int(port == 22), int(port == 4444), int(port >= 49152),
            int(port == 443), int(port == 53))


def split_group(run_id):
    # Whole runs go to exactly one side. This is deterministic, not a
    # substitute for a temporally independent final evaluation.
    digest = hashlib.sha256(run_id.encode("utf-8")).digest()
    return "holdout" if digest[0] < 51 else "train"


def load_csv(path):
    rows = []
    with open(path, newline="", encoding="utf-8") as stream:
        reader = csv.DictReader(stream)
        expected = {"agent_run_id", "destination_ip", "destination_port", "label"}
        if set(reader.fieldnames or []) != expected:
            raise ValueError("CSV header must be exactly: " + ",".join(sorted(expected)))
        for line, record in enumerate(reader, 2):
            run = record["agent_run_id"].strip()
            if not run or len(run) > 128:
                raise ValueError(f"line {line}: invalid run id")
            port_text = record["destination_port"].strip()
            if not port_text.isascii() or not port_text.isdecimal():
                raise ValueError(f"line {line}: invalid destination_port")
            port = int(port_text)
            if not 1 <= port <= 65535:
                raise ValueError(f"line {line}: port out of range")
            label = record["label"].strip().lower()
            if label not in ("benign", "attack"):
                raise ValueError(f"line {line}: label must be benign or attack")
            rows.append((run, features(record["destination_ip"], port),
                         int(label == "attack")))
    if not rows:
        raise ValueError("empty training dataset")
    return rows


def train(train_rows, epochs=100):
    if not train_rows:
        raise ValueError("empty training side")
    if not any(y for _, _, y in train_rows) or not any(not y for _, _, y in train_rows):
        raise ValueError("train side must contain both labels")
    weights = [0.0] * FEATURE_COUNT
    bias = 0.0
    # Compact L2-regularized logistic SGD, fixed iteration order and bounded
    # logits. Model selection threshold is fit on TRAIN only.
    for epoch in range(epochs):
        rate = 0.12 / (1 + epoch / 24)
        for _, x, y in train_rows:
            z = max(-20, min(20, bias + sum(w * v for w, v in zip(weights, x))))
            p = 1 / (1 + math.exp(-z))
            residual = y - p
            for i in range(FEATURE_COUNT):
                weights[i] += rate * (residual * x[i] - 0.0005 * weights[i])
            bias += rate * residual
    quantized = [max(-2048, min(2048, round(w * 256))) for w in weights]
    bias = max(-4096, min(4096, round(bias * 256)))
    if not any(w > 0 for w in quantized[1:]):
        raise ValueError("trained model lacks positive port feature; not exportable")
    baseline = bias + quantized[0]
    scores = [bias + sum(w * v for w, v in zip(quantized, x))
              for _, x, _ in train_rows]
    negatives = sum(1 for _, _, y in train_rows if y == 0)
    # Rank thresholds by train-side recall subject to an operational 0.1% FPR
    # point target. The holdout set is never consulted for model selection.
    best = None
    candidate_thresholds = sorted({s for s in scores if 1 <= s <= 8192} | {8192})
    for threshold in candidate_thresholds:
        if baseline >= threshold:
            continue
        fp = sum(1 for i, (_, _, y) in enumerate(train_rows)
                 if not y and scores[i] >= threshold)
        if fp * 1000 > negatives:
            continue
        tp = sum(1 for i, (_, _, y) in enumerate(train_rows)
                 if y and scores[i] >= threshold)
        ranking = (tp, -fp, -threshold)
        if best is None or ranking > best[0]:
            best = (ranking, threshold)
    if best is None:
        raise ValueError("no threshold satisfies train-side safety constraints")
    return {
        "schemaVersion": MODEL_SCHEMA,
        "generation": 1,
        "bias": bias,
        "threshold": best[1],
        "weights": quantized,
    }


def predict(model, x):
    if not x[0]:
        return False
    score = model["bias"] + sum(w * v for w, v in zip(model["weights"], x))
    return score >= model["threshold"]


def wilson_bounds95(successes, total):
    """One-sided 95% Wilson bounds. Correlated event rows weaken confidence."""
    if total <= 0:
        return 0.0, 1.0
    z = 1.6448536269514722
    n = float(total)
    p = successes / n
    z2 = z * z
    denominator = 1 + z2 / n
    center = p + z2 / (2 * n)
    margin = z * math.sqrt(p * (1 - p) / n + z2 / (4 * n * n))
    return (center - margin) / denominator, (center + margin) / denominator


def evaluate(model, holdout_rows, model_digest):
    benign = attacks = false_pos = true_pos = 0
    benign_groups = set()
    attack_groups = set()
    for run, x, label in holdout_rows:
        positive = predict(model, x)
        if label:
            attack_groups.add(run)
            attacks += 1
            true_pos += int(positive)
        else:
            benign_groups.add(run)
            benign += 1
            false_pos += int(positive)
    _, fpr_upper = wilson_bounds95(false_pos, benign)
    recall_lower, _ = wilson_bounds95(true_pos, attacks)
    return {
        "schemaVersion": REPORT_SCHEMA,
        "modelSHA256": model_digest,
        "benign": benign, "attacks": attacks,
        "falsePositives": false_pos, "truePositives": true_pos,
        "independentGroups": len({run for run, _, _ in holdout_rows}),
        "benignGroups": len(benign_groups),
        "attackGroups": len(attack_groups),
        "falsePositiveUpper95": fpr_upper,
        "attackRecallLower95": recall_lower,
        "falsePositiveRate": false_pos / benign if benign else None,
        "recall": true_pos / attacks if attacks else None,
        "note": "Point estimates on group-held-out data; not attested and not calibrated.",
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--holdout", required=True)
    args = parser.parse_args()
    rows = load_csv(args.input)
    training = [r for r in rows if split_group(r[0]) == "train"]
    holdout = [r for r in rows if split_group(r[0]) == "holdout"]
    if not holdout:
        parser.error("empty holdout; more independent runs required")
    model = train(training)
    Path(args.model).parent.mkdir(parents=True, exist_ok=True)
    Path(args.holdout).parent.mkdir(parents=True, exist_ok=True)
    payload = (json.dumps(model, sort_keys=True, indent=2) + "\n").encode()
    Path(args.model).write_bytes(payload)
    digest = hashlib.sha256(payload).hexdigest()
    report = evaluate(model, holdout, digest)
    # Go report loader is strict about unknown fields; retain extended metrics
    # on stdout while storing only the versioned enforcement evidence contract.
    print(json.dumps(report, indent=2, sort_keys=True))
    for key in ("falsePositiveRate", "recall", "falsePositiveUpper95",
                "attackRecallLower95", "note"):
        del report[key]
    Path(args.holdout).write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    print("modelSHA256:", digest)


if __name__ == "__main__":
    main()
