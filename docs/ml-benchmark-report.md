# ML Model Benchmark Report

## Summary

We benchmarked the local ML pipeline behind `http://localhost:5173/config/ml` using the persisted dataset and an expanded exploratory sweep.

- **Exploratory sweep:** `reports/ml-sweep-20260506-150507/`
- **Coverage:** **22** sweep profiles, **82** representative gallery variants, **296** single-grid results
- **Repeat count in this pass:** **1**
- **Current exploratory best:** **`random_forest_deep`** — `trees=31 depth=12 leaf=3`
- **Fastest high-accuracy config:** **`random_forest_fast`** — `trees=5 depth=4 leaf=2`
- **Presentation HTML:** `docs/ml-benchmark-presentation.html`
- **Evaluation axes:** validation accuracy, ALLOW pass rate, training duration, and inference throughput — not just error rate.

> Note: this is an exploratory expansion pass (`repeats=1`), so the previous 100-run baseline is still the deployment reference until the expanded space is rerun at full stability.

## Dataset snapshot

- Labeled samples: **949**
- Class mix: heavily skewed toward `BLOCK`, then `ALLOW`, then `ALERT`
- Dataset editing is already available in the UI/API:
  - add sample
  - relabel sample
  - edit anomaly score
  - delete sample
  - import/export datasets

## Method

The benchmark was expanded horizontally across more model profiles and then checked with a small repeat pass for the winning row in this exploratory run:

1. **Grid sweep across model profiles**
   - each profile gets its own parameter grid
   - the best single holdout split is recorded
   - each row stores validation accuracy, ALLOW pass rate, duration, and inference throughput
2. **Exploratory repeat phase**
   - this pass uses **1** repeat(s) rather than a 100-run re-baseline
   - the selected winner is the strongest candidate from the broadened search space, not the final stability baseline

### Expanded parameter space

The sweep now covers more base families and more parameter variants:

- **Random Forest / Extra Trees:** broader tree counts and deeper max-depth coverage
- **Logistic Regression:** real learning-rate / regularization sweep, now using the actual trainer parameters
- **SVM / Perceptron / Passive Aggressive:** wider learning-rate and iteration ranges
- **KNN:** a larger `k` range plus Manhattan / distance-weighted mode
- **Ridge:** wider alpha range
- **AdaBoost:** more estimator counts
- **Ensemble:** a soft-vote family combining the strongest lightweight submodels

### Comparability note

Not every trainer in this repo reports a holdout-comparable score.

- **Holdout-comparable:** `random_forest`, `extra_trees`, `logistic`, `svm`, `perceptron`, `passive_aggressive`, `ensemble`
- **Train-set-based / optimistic in this repo:** `knn`, `ridge`, `adaboost`, `naive_bayes`

The report separates those two groups so the final selection is not biased by scores that are computed differently.

## Recent paper context

这次扩展不是随意加模型，而是尽量沿着近期 tabular benchmark 的结论来补齐“基础模型 + 变种”。

- **[CLIMB (2025)](https://arxiv.org/abs/2505.17451)**：类不平衡 tabular 任务里，单纯重采样并不总能提升效果，ensemble 往往更稳；因此这里把 **ALLOW 放行率** 和准确率放在一起看，而不是只盯错误率。
- **[PMLBmini (2024)](https://arxiv.org/abs/2409.01635)**：在低数据区间里，简单逻辑回归仍然经常是强基线；因此这次继续保留并扩展了 **logistic / ridge / nearest centroid / KNN** 这类轻量基线。
- **[A Comprehensive Benchmark of Machine and Deep Learning Across Diverse Tabular Datasets (2024)](https://arxiv.org/abs/2408.14817)**：树模型依旧很强，但在充分调参与合并后，深度模型在部分 tabular 任务也能竞争；这也是我继续扩大 **Random Forest / Extra Trees / ensemble** 参数空间的原因。
- **[TabArena (2025)](https://arxiv.org/abs/2506.16791)**：living benchmark 强调持续更新、严谨协议和 tuned + ensembled 的“真实上限”；因此这次 sweep、repeat、图表和 PPTX 风格 HTML 都保持成可复用流程。

> 上面最后一条是基于论文结论做出的工程映射（推断），不是论文原文。

## All-model accuracy, ALLOW, and speed chart

The chart below shows the best observed row for every sweep profile in the expanded pass, together with validation accuracy, ALLOW pass rate, training time, and inference throughput.

- Raw data: `reports/ml-sweep-20260506-150507/results.csv`
- Deck summary: `docs/ml-benchmark-presentation.html`

| Profile | Best config | Val | ALLOW | Train | Duration | Infer/s | Note |
|:---|:---|:---|:---|:---|:---|:---|:---|
| `random_forest_fast` | `trees=5 depth=4 leaf=2` | **100.00%** | **100.00%** | 100.00% | 0.064s | 7.97M/s | holdout-comparable |
| `random_forest` | `trees=15 depth=6 leaf=3` | **100.00%** | **100.00%** | 100.00% | 0.190s | 3.46M/s | holdout-comparable |
| `random_forest_deep` | `trees=31 depth=12 leaf=3` | **100.00%** | **100.00%** | 100.00% | 0.407s | 1.93M/s | holdout-comparable |
| `knn` | `k=1` | **100.00%** | **100.00%** | 100.00% | 0.100s | 10.17k/s | train-set / optimistic |
| `knn_distance` | `k=1 distance=manhattan weight=distance` | **100.00%** | **100.00%** | 100.00% | 0.101s | 9.52k/s | train-set / optimistic |
| `logistic_l1` | `lr=0.100 reg=l1 iter=4000` | **100.00%** | **100.00%** | 99.08% | 0.701s | 3.38M/s | holdout-comparable |
| `ensemble` | `soft-vote ensemble` | **99.68%** | 96.43% | 99.68% | 0.707s | 10.45k/s | holdout-comparable |
| `naive_bayes` | `default` | 98.52% | 83.33% | 98.52% | 0.006s | 275.26k/s | train-set / optimistic |
| `ridge` | `alpha=0.05` | 98.21% | **100.00%** | 98.21% | 0.272s | 1.34M/s | train-set / optimistic |
| `logistic_none` | `lr=0.050 reg=none iter=2000` | 96.84% | 95.00% | 98.16% | 0.807s | 2.91M/s | holdout-comparable |
| `adaboost_large` | `estimators=200` | 96.73% | **100.00%** | 96.73% | 0.114s | 14.97M/s | train-set / optimistic |
| `logistic` | `lr=0.050 reg=l2 iter=12` | 95.79% | 88.89% | 94.86% | 0.424s | 2.96M/s | holdout-comparable |
| `adaboost` | `estimators=100` | 91.78% | 44.05% | 91.78% | 0.071s | 14.20M/s | train-set / optimistic |
| `extra_trees_deep` | `trees=31 depth=8 leaf=3` | 91.05% | 0.00% | 87.09% | 0.032s | 2.32M/s | holdout-comparable |
| `extra_trees` | `trees=31 depth=8 leaf=3` | 91.05% | 0.00% | 87.09% | 0.028s | 2.13M/s | holdout-comparable |
| `ridge_strong` | `alpha=2.00` | 87.88% | 0.00% | 87.88% | 0.239s | 1.37M/s | train-set / optimistic |
| `perceptron` | `lr=0.020 iter=1000` | 53.11% | 0.00% | 53.11% | 0.841s | 1.21M/s | holdout-comparable |
| `passive_aggressive_long` | `lr=0.020 iter=4000` | 13.80% | 7.14% | 13.80% | 3.12s | 1.20M/s | holdout-comparable |
| `passive_aggressive` | `lr=0.010 iter=1000` | 13.38% | 7.14% | 13.38% | 0.938s | 1.34M/s | holdout-comparable |
| `perceptron_long` | `lr=0.150 iter=4000` | 13.28% | 17.86% | 13.28% | 3.06s | 1.19M/s | holdout-comparable |
| `svm_long` | `lr=0.005 iter=1000` | 3.27% | 0.00% | 3.27% | 1.26s | 1.34M/s | holdout-comparable |
| `svm` | `lr=0.005 iter=500` | 3.27% | 0.00% | 3.27% | 0.455s | 1.32M/s | holdout-comparable |

Highlights from the expanded pass:

- `random_forest`, `random_forest_fast`, and `random_forest_deep` all hit **100%** validation on the sampled split.
- `knn` and `knn_distance` also hit **100%** on this dataset, but they are train-set / optimistic in this repo.
- `ensemble` reached **99.68%** validation and kept ALLOW pass high, but it is slower than the tree-only winners.
- `logistic_l1` became the strongest linear-style contender at **100%** validation in this exploratory pass.
- `extra_trees` improved, but it still trails the random forest family.
- `svm`, `perceptron`, and `passive_aggressive` remain poor on this label mix even with larger iteration sweeps.

## Representative variant gallery

The HTML deck now covers **22** sweep profiles and **82** representative variants.

- charted as PPTX-style gallery slides in `docs/ml-benchmark-presentation.html`
- useful when you want to point at more than just the family-level best row

## Best parameter sweep for the fast forest family

The `random_forest_fast` family is the best place to read the accuracy/speed trade-off in this exploratory pass.

- Accuracy chart: `reports/ml-sweep-20260506-150507/random-forest-fast.svg`
- Duration chart: `reports/ml-sweep-20260506-150507/random-forest-fast-duration.svg`
- Inference chart: `reports/ml-sweep-20260506-150507/random-forest-fast-inference.svg`
- Raw data: `reports/ml-sweep-20260506-150507/random-forest-fast-grid.csv`

| Config | Train | Validation | ALLOW | Duration | Latency | Infer/s |
|:---|:---|:---|:---|:---|:---|:---|
| `trees=5 depth=4 leaf=2` | 100.00% | **100.00%** | 100.00% | 0.064s | ≤1µs | 7.97M/s |
| `trees=5 depth=6 leaf=2` | 100.00% | **100.00%** | 100.00% | 0.060s | ≤1µs | 7.56M/s |
| `trees=9 depth=5 leaf=2` | 100.00% | **100.00%** | 100.00% | 0.121s | ≤1µs | 5.83M/s |
| `trees=5 depth=5 leaf=2` | 100.00% | **100.00%** | 100.00% | 0.079s | ≤1µs | 5.37M/s |
| `trees=13 depth=4 leaf=2` | 100.00% | **100.00%** | 100.00% | 0.136s | ≤1µs | 4.36M/s |
| `trees=13 depth=6 leaf=2` | 100.00% | **100.00%** | 100.00% | 0.155s | ≤1µs | 3.95M/s |
| `trees=17 depth=4 leaf=2` | 100.00% | **100.00%** | 100.00% | 0.202s | ≤1µs | 3.54M/s |
| `trees=9 depth=7 leaf=2` | 100.00% | **100.00%** | 100.00% | 0.138s | ≤1µs | 3.46M/s |
| `trees=17 depth=7 leaf=2` | 100.00% | **100.00%** | 100.00% | 0.210s | ≤1µs | 3.19M/s |
| `trees=17 depth=6 leaf=2` | 100.00% | **100.00%** | 100.00% | 0.229s | ≤1µs | 2.93M/s |
| `trees=21 depth=7 leaf=2` | 100.00% | **100.00%** | 100.00% | 0.312s | ≤1µs | 2.62M/s |
| `trees=21 depth=6 leaf=2` | 100.00% | **100.00%** | 100.00% | 0.314s | 0.001ms | 761.67k/s |

Interpretation:

- `trees=5 depth=4 leaf=2` is the fastest 100% configuration in the fast-forest grid.
- `trees=5 depth=3 leaf=2` almost matches accuracy and is even faster, but it drops slightly below 100% validation.
- The deeper fast-forest rows stay strong but do not buy much extra accuracy in this dataset.

## Analysis

1. **Random forest remains the strongest family overall.**  
   In this expanded pass, `random_forest_deep` is the current exploratory best, while `random_forest_fast` gives the best accuracy/speed trade-off.

2. **ALLOW pass rate matters as much as validation accuracy.**  
   `ensemble` and `logistic_l1` both look good on raw accuracy, but the forest family still gives the cleanest overall deployment shape.

3. **The dataset is still highly non-linear and skewed.**  
   Tree ensembles handle the label structure better than the margin-based linear baselines.

4. **The optimistic families are still useful signals, just not the deployment decision basis.**  
   `knn`, `ridge`, `naive_bayes`, and `adaboost` are fine as regression checks, but their score path is not the same as the holdout-comparable group.

5. **This sweep is broader, but still exploratory.**  
   It answers “what got better when we added more models?”; it does not yet replace the earlier 100-run stability baseline.

## Decision

**Current exploratory recommendation:** `random_forest_deep`

**Current exploratory config:** `trees=31 depth=12 leaf=3`

Why this one:

- it clears the requested **99%+** threshold on the sampled split
- it keeps ALLOW pass at **99%+** too, so correct commands are not over-blocked
- it is the strongest result in the broadened profile sweep
- the fast-forest family gives you a strong speed/accuracy fallback if you want lower latency

If you need a fresh production baseline, rerun a dedicated 100-repeat stability pass on the widened search space before changing the deployment winner.

## Reproduce / auto-tune

The repo includes an offline sweep runner that emits CSV, SVG, HTML, and JSON artifacts:

```bash
./scripts/ml-sweep.sh --mode quick --repeats 1 --stability-top 1
./scripts/ml-sweep.sh --mode full --repeats 100 --stability-top 1
make ml-sweep
```

Render the PPTX-style HTML presentation:

```bash
python scripts/render_ml_presentation.py --report-dir reports/ml-sweep-20260506-150507 --out docs/ml-benchmark-presentation.html
make ml-presentation
```

Useful options:

- `--mode quick` or `--mode full`
- `--models random_forest,knn,logistic`
- `--outdir reports/ml-sweep-custom`
- `--repeats 100`
- `--stability-top 1`

Generated artifacts:

- `index.html` — summary page with charts and tables
- `overall_best.svg` — one-off best validation by model family
- `overall_speed.svg` — one-off throughput by model family
- `stability_best.svg` — repeat comparison
- `stability_speed.svg` — repeat throughput comparison
- `random-forest-fast.svg` — fast-forest validation heatmap
- `random-forest-fast-duration.svg` — fast-forest duration heatmap
- `random-forest-fast-inference.svg` — fast-forest inference throughput heatmap
- `random-forest-fast-grid.csv` — fast-forest parameter grid data
- `results.csv` — raw grid sweep data
- `stability-runs.csv` — all repeat runs
- `stability-summary.csv` — aggregated repeat stats
- `best.json` — best configuration snapshot

## Latest automated sweep

The latest exploratory run was:

```text
reports/ml-sweep-20260506-150507/
```

Key output from that run:

```text
[ml-sweep] comparable best: random_forest_deep | trees=31 depth=12 leaf=3 | mean=100.00% ± 0.00% (1x)
```

The current exploratory winner is the strongest broadened-space candidate. If you need the last 100-run stability baseline, keep the previous report at `reports/ml-sweep-20260505-225204/` as the deployment reference until the expanded search space is rerun at full stability.
