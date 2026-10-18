"""Public HTTP *parameter* benchmark, never an execution or kernel label dataset.

Uses the Go extractor used by replay. No payload is executed or sent to a URL.
Fit, feature selection, threshold calibration and test partitions are disjoint
by normalized lexical template. Metadata and aggregate results are publishable;
raw data, per-row hashes and model binaries stay out of the source repository.
"""
import argparse
import csv
import hashlib
import importlib.metadata
import json
import math
import re
import subprocess
import tempfile
import time
from collections import Counter, defaultdict
from pathlib import Path
from urllib.parse import unquote_plus

import numpy as np
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.feature_selection import mutual_info_classif
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import (accuracy_score, average_precision_score, balanced_accuracy_score,
                             confusion_matrix, f1_score, roc_auc_score)
from sklearn.model_selection import GroupShuffleSplit
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler

REVISION = "926670a710283f87c05b554680facf3f9530548c"
DATA_SHA256 = "a4e62ba13435ad3dd583c5790db2175629020fc41c56e1b26baa02a9ae45f03d"
KEYWORDS = set("select union all from where and or insert into delete update drop table sleep benchmark script svg img iframe onerror onload javascript alert eval document cookie window location etc passwd shadow bin bash sh cmd powershell http https file gopher dict exec system cat wget curl true false null order by group having limit exists case when then else end as like not in is asc desc waitfor delay information_schema concat cast char load_file outfile".split())


def canonical(s):
    for _ in range(2):
        n = unquote_plus(s, errors="replace")
        if n == s:
            break
        s = n
    return re.sub(r"\s+", " ", s).strip()


def template(s):
    s = canonical(s).lower()
    s = re.sub(r"[\w.+-]+@[\w.-]+\.[a-z]{2,}", "<email>", s)
    s = re.sub(r"\b(?:0x[0-9a-f]+|\d+(?:\.\d+)*)\b", "<n>", s)
    s = re.sub(r"[a-z_][a-z_0-9]*", lambda m: m[0] if m[0] in KEYWORDS else "w", s)
    return s


def split_groups(y, groups):
    # First adequate split is selected using counts only, before any model fit.
    # Constraints prevent single-class metrics; group imbalance is reported.
    ids = np.arange(len(y))
    for seed in range(1729, 1829):
        train, rest = next(GroupShuffleSplit(n_splits=1, test_size=.4, random_state=seed).split(ids, y, groups))
        va, tail = next(GroupShuffleSplit(n_splits=1, test_size=.625, random_state=seed+1).split(rest, y[rest], groups[rest]))
        cal, te = next(GroupShuffleSplit(n_splits=1, test_size=.6, random_state=seed+2).split(tail, y[rest[tail]], groups[rest[tail]]))
        parts = {"train": train, "validation": rest[va], "calibration": rest[tail[cal]], "test": rest[tail[te]]}
        if all(min(np.bincount(y[v], minlength=2)) >= 50 for v in parts.values()):
            sets = [set(groups[v]) for v in parts.values()]
            assert all(not sets[a] & sets[b] for a in range(4) for b in range(a))
            return parts, seed
    raise ValueError("no adequate group split")


def engineer(x, names):
    columns = [x]
    result_names = list(names)
    for j, name in enumerate(names):
        if name.endswith(("_count", ".bytes")):
            columns.append(np.log1p(x[:, j:j+1])); result_names.append("log1p:"+name)
    index = {n: i for i, n in enumerate(names)}
    size = np.maximum(1, x[:, index["payload.bytes"]])
    for name in names:
        if name.endswith("_count"):
            columns.append((x[:, index[name]]/size)[:, None]); result_names.append("per_byte:"+name)
    hints = [n for n in names if n.endswith("_hint")]
    for j, a in enumerate(hints):
        for b in hints[:j]:
            columns.append((x[:, index[a]]*x[:, index[b]])[:, None]); result_names.append(a+"*"+b)
    return np.column_stack(columns), result_names


def select_features(x, y, names):
    valid = np.flatnonzero(np.var(x, axis=0) > 1e-10)
    # Binary/count variables have a discrete estimator; continuous ratios do not.
    discrete = np.array([len(np.unique(x[:, j])) <= 32 and np.allclose(x[:, j], np.rint(x[:, j])) for j in valid])
    mi = mutual_info_classif(x[:, valid], y, discrete_features=discrete, random_state=2026)
    order = sorted(range(len(valid)), key=lambda j: (-mi[j], names[valid[j]]))
    selected = []
    for j in order:
        c = valid[j]
        if all(abs(np.corrcoef(x[:, c], x[:, k])[0, 1]) < .98 for k in selected):
            selected.append(int(c))
        if len(selected) == 24:
            break
    return selected, {"selected": [names[j] for j in selected], "indices": selected,
                      "fit_rows": len(y), "fit_partition": "train", "max_features": 24,
                      "correlation_cutoff": .98, "mutual_information": {names[j]: float(v) for j, v in zip(valid, mi)}}


def model(kind, param, seed):
    if kind == "logistic":
        return make_pipeline(StandardScaler(), LogisticRegression(C=param, max_iter=2000, class_weight="balanced", random_state=seed))
    return HistGradientBoostingClassifier(max_leaf_nodes=param, max_iter=180, learning_rate=.07,
        l2_regularization=2, min_samples_leaf=25, early_stopping=False, random_state=seed)


def threshold(y, p):
    # Strict > with the empirical negative 95th percentile; ties conservative.
    return float(np.quantile(p[y == 0], .95, method="higher"))


def metrics(y, p, t):
    pred = p > t
    tn, fp, fn, tp = confusion_matrix(y, pred, labels=[0, 1]).ravel()
    return {"rows": len(y), "accuracy": float(accuracy_score(y, pred)),
        "balanced_accuracy": float(balanced_accuracy_score(y, pred)), "f1": float(f1_score(y, pred, zero_division=0)),
        "average_precision": float(average_precision_score(y, p)), "roc_auc": float(roc_auc_score(y, p)),
        "recall": float(tp/max(1, tp+fn)), "fpr": float(fp/max(1, fp+tn)),
        "tn": int(tn), "fp": int(fp), "fn": int(fn), "tp": int(tp)}


def main():
    p = argparse.ArgumentParser(); p.add_argument("--dataset", type=Path, required=True)
    p.add_argument("--extractor", type=Path, required=True); p.add_argument("--out", type=Path, required=True)
    args = p.parse_args(); args.out.mkdir(parents=True, exist_ok=True); start = time.monotonic()
    if hashlib.sha256(args.dataset.read_bytes()).hexdigest() != DATA_SHA256:
        raise ValueError("dataset does not match pinned public revision")
    raw = list(csv.DictReader(args.dataset.open()))
    grouped = defaultdict(list)
    for row in raw:
        if row["label"] not in ("norm", "anom"):
            raise ValueError("unexpected public label")
        grouped[canonical(row["payload"])].append(row)
    conflicts = sum(len(v) for v in grouped.values() if len({r["label"] for r in v}) > 1)
    rows = [v[0] for _, v in sorted(grouped.items()) if len({r["label"] for r in v}) == 1]
    assert all(len(r["payload"].encode()) <= 65536 for r in rows)
    y = np.array([r["label"] == "anom" for r in rows], dtype=np.int64)
    groups = np.array([hashlib.sha256(template(r["payload"]).encode()).hexdigest() for r in rows])
    parts, split_seed = split_groups(y, groups)
    names = None; values = []
    with tempfile.TemporaryFile(mode="w+") as f:
        for r in rows:
            f.write(json.dumps({"source": "public_http_parameter_dataset", "parameter": r["payload"]})+"\n")
        f.seek(0)
        proc = subprocess.Popen([str(args.extractor)], stdin=f, stdout=subprocess.PIPE, text=True)
        for line in proc.stdout:
            o = json.loads(line); assert o["kind"] == "http_parameter" and o["schema"] == "kernel-http-behavior.v1"
            features = o["features"]
            if names is None:
                names = sorted(n for n in features if n.startswith("payload."))
            assert all(features[n]["present"] for n in names)
            values.append([features[n]["value"] for n in names])
        if proc.wait() != 0:
            raise RuntimeError("extractor failed")
    x = np.asarray(values, dtype=np.float64); assert len(x) == len(rows)
    expanded, enames = engineer(x, names)
    selected, selection = select_features(expanded[parts["train"]], y[parts["train"]], enames)
    views = {"structural": x, "auto_selected": expanded[:, selected]}
    trials = []; winners = {}; result = {}; all_predictions = {}
    for view, xx in views.items():
        candidates = []
        for kind, params in [("logistic", [.1, 1.]), ("hist_gradient_boosting", [7, 15])]:
            for param in params:
                m = model(kind, param, 2026); m.fit(xx[parts["train"]], y[parts["train"]])
                score = float(average_precision_score(y[parts["validation"]], m.predict_proba(xx[parts["validation"]])[:, 1]))
                trial = {"view": view, "kind": kind, "param": param, "validation_AP": score}
                trials.append(trial); candidates.append((score, kind, param))
        _, kind, param = max(candidates)
        winners[view] = {"kind": kind, "param": param}; seed_results = []; preds = []
        for seed in [2026, 2027, 2028]:
            m = model(kind, param, seed); m.fit(xx[parts["train"]], y[parts["train"]])
            cp = m.predict_proba(xx[parts["calibration"]])[:, 1]; t = threshold(y[parts["calibration"]], cp)
            tp = m.predict_proba(xx[parts["test"]])[:, 1]
            by_type = {}
            for typ in sorted({r["attack_type"] for r in rows}):
                mask = np.array([rows[i]["attack_type"] == typ for i in parts["test"]])
                by_type[typ] = {"rows": int(mask.sum()), "flagged": int((tp[mask] > t).sum())}
            seed_results.append({"seed": seed, "threshold": t, "calibration": metrics(y[parts["calibration"]], cp, t),
                                 "test": metrics(y[parts["test"]], tp, t), "fixed_0_5_test": metrics(y[parts["test"]], tp, .5),
                                 "fixed_0_5_validation": metrics(y[parts["validation"]], m.predict_proba(xx[parts["validation"]])[:, 1], .5), "by_attack_type": by_type})
            preds.append(tp > t)
            all_predictions[view+"_"+str(seed)] = tp.tolist()
            from portable_model import export_model, predict
            saved = export_model(m, kind)
            error = float(np.max(np.abs(predict(saved, xx[parts["test"]])-tp)))
            assert error < 1e-10
            seed_results[-1]["portable_prediction_max_abs_error"] = error
            saved.update(schema="kernel-http-behavior.v1", view=view, threshold=t, shadow_only=True, selected_indices=selected if view=="auto_selected" else None,
                         input_names=enames if view=="auto_selected" else names)
            (args.out/(view+"_"+str(seed)+".json")).write_text(json.dumps(saved))
        result[view] = seed_results
        all_predictions[view+"_votes"] = np.mean(preds, axis=0).tolist()
    # Paired group bootstrap respects the held-out template unit, not IID rows.
    test = parts["test"]; unique = np.unique(groups[test]); rng = np.random.default_rng(2026)
    truth = y[test]; effects = []
    votes = {v: np.asarray(all_predictions[v+"_votes"]) > .5 for v in views}
    ix = {g: np.flatnonzero(groups[test] == g) for g in unique}
    for _ in range(1000):
        ids = np.concatenate([ix[g] for g in rng.choice(unique, len(unique), replace=True)])
        if len(np.unique(truth[ids])) < 2:
            continue
        effects.append(balanced_accuracy_score(truth[ids], votes["auto_selected"][ids])-balanced_accuracy_score(truth[ids], votes["structural"][ids]))
    report = {"schema": "kernel-http-behavior.v1", "task": "public_parameter_anomaly_proxy_only", "dataset": {
        "url": "https://github.com/Morzeux/HttpParamsDataset", "revision": REVISION,
        "file_sha256": hashlib.sha256(args.dataset.read_bytes()).hexdigest(), "original_rows": len(raw),
        "conflicting_rows_removed": conflicts, "retained_unique_rows": len(rows), "duplicates_removed": len(raw)-conflicts-len(rows),
        "groups": len(set(groups)), "class_counts": dict(Counter(r["label"] for r in rows))},
        "split_seed": split_seed, "split_algorithm": "canonical dedupe then lexical-template GroupShuffleSplit; counts-only first adequate seed",
        "splits": {k: {"rows": len(v), "groups": len(set(groups[v])), "positive": int(y[v].sum()), "negative": int((y[v]==0).sum())} for k,v in parts.items()},
        "features": {"structural": names, "engineered_candidate_count": len(enames), "selection": selection},
        "trials": trials, "winners": winners, "results": result,
        "paired_group_bootstrap_ba_delta_95ci": np.quantile(effects, [.025,.975]).tolist(),
        "group_overlap": 0, "model_fits": len(trials)+6, "seconds": time.monotonic()-start,
        "versions": {n: importlib.metadata.version(n) for n in ["numpy", "scikit-learn", "scipy"]},
        "limitations": ["Parameter samples only: no syscall, full request, exploit outcome, or privacy leakage ground truth.",
          "Synthetic/generated public corpus, dominated by SQLi; source-independent generalization untested.",
          "5% calibration empirical FPR is not a test/production guarantee; probabilities are not calibrated.",
          "Template grouping is a heuristic, not proof that all semantic near duplicates are removed.",
          "Three deterministic seeds can produce identical fits; no claim of three independent datasets.",
          "Initial test was inspected before fixing the discrete MI mask and adding fixed-0.5 diagnostics; these are reused-test follow-up results, not a fresh blind test."]}
    report["promotion"] = {view: {rule: all(v[key]["fpr"] <= .05 and v[key]["recall"] >= .7 for v in records)
        for rule, key in [("calibrated_threshold", "test"), ("fixed_0_5_diagnostic", "fixed_0_5_test")]} for view, records in result.items()}
    (args.out/"results.json").write_text(json.dumps(report, indent=2)+"\n")
    # Local-only row provenance enables audits without publishing payload hashes.
    private = {"test_indices": test.tolist(), "test_labels": truth.tolist(), "predictions": all_predictions,
        "split_indices": {k:v.tolist() for k,v in parts.items()}, "groups": groups.tolist()}
    (args.out/"private_replay.json").write_text(json.dumps(private))
    np.savez_compressed(args.out/"clean_features.npz", structural=x, engineered=expanded, labels=y)
    print(json.dumps({"splits": report["splits"], "results": result, "seconds": report["seconds"]}, indent=2))


if __name__ == "__main__":
    main()
