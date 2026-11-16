"""JSON-only binary classifier export. Numeric HGB splits only; fail closed on others."""
import numpy as np
from scipy.special import expit


def export_model(model, kind):
    if kind == "logistic":
        scaler, linear = model.steps[0][1], model.steps[1][1]
        return {"format": "numeric-binary-model.v1", "kind": kind, "mean": scaler.mean_.tolist(),
                "scale": scaler.scale_.tolist(), "coef": linear.coef_[0].tolist(), "intercept": float(linear.intercept_[0])}
    trees = []
    for iteration in model._predictors:
        assert len(iteration) == 1
        nodes = iteration[0].nodes
        assert not nodes["is_categorical"].any()
        trees.append([{k: n[k].item() for k in ["value", "feature_idx", "num_threshold", "missing_go_to_left", "left", "right", "is_leaf"]} for n in nodes])
    return {"format": "numeric-binary-model.v1", "kind": kind, "baseline": float(model._baseline_prediction.ravel()[0]), "trees": trees}


def predict(saved, x):
    if saved["kind"] == "logistic":
        return expit(((x-np.asarray(saved["mean"]))/np.asarray(saved["scale"]))@np.asarray(saved["coef"])+saved["intercept"])
    score = np.full(len(x), saved["baseline"])
    for tree in saved["trees"]:
        # Vectorized traversal with bounded depth (at most the number of nodes).
        positions = np.zeros(len(x), dtype=int)
        for _ in range(len(tree)):
            active = np.array([not tree[n]["is_leaf"] for n in positions])
            if not active.any():
                break
            ids = np.flatnonzero(active)
            for pos in np.unique(positions[ids]):
                subset = ids[positions[ids] == pos]; node = tree[pos]
                value = x[subset, node["feature_idx"]]
                left = np.where(np.isnan(value), bool(node["missing_go_to_left"]), value <= node["num_threshold"])
                positions[subset] = np.where(left, node["left"], node["right"])
        score += np.array([tree[n]["value"] for n in positions])
    return expit(score)
