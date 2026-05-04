# ML Model Benchmark Report

## Summary

We benchmarked the local ML pipeline behind `http://localhost:5173/config/ml` using the persisted dataset and a wider parameter sweep than before.

- **Chosen model:** `random_forest`
- **Stable config:** `trees=5 depth=8 leaf=3`
- **100-run mean validation:** **99.68% ± 0.42%**
- **Success rate:** **100/100**
- **Latest full sweep output:** `reports/ml-sweep-20260504-193542/`
- **Best single grid point:** `trees=3 depth=4 leaf=3` at **100.00%** validation on one split
- **Presentation HTML:** `docs/ml-benchmark-presentation.html`
- **Presentation coverage:** 28 representative model variants, so the deck now goes beyond the requested 20+

The earlier forest vote bug is already fixed in `backend/decision_forest.go`, so the forest-family results below are meaningful.

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

The benchmark was expanded horizontally across all model families and then stabilized with repeated runs:

1. **Grid sweep across model types**
   - each family gets its own parameter grid
   - the best single holdout split is recorded
2. **Stability phase**
   - only the config points that affect final accuracy are repeated
   - default is the top grid point per family
   - each selected config is repeated **100 times**

### Expanded parameter space

The new sweep is broader than the earlier quick/full runs:

- **Random Forest / Extra Trees:** more tree counts and deeper max-depth coverage
- **Logistic Regression:** real learning-rate / regularization sweep, now using the actual trainer parameters
- **SVM / Perceptron / Passive Aggressive:** wider learning-rate and iteration ranges
- **KNN:** larger `k` range
- **Ridge:** wider alpha range
- **AdaBoost:** more estimator counts

This is intentional: we are measuring the effect of the model parameters on the final score, not repeating internal training steps one-by-one.

### Comparability note

Not every trainer in this repo reports a holdout-comparable score.

- **Holdout-comparable:** `random_forest`, `extra_trees`, `logistic`, `svm`, `perceptron`, `passive_aggressive`
- **Train-set-based / optimistic in this repo:** `knn`, `ridge`, `adaboost`, `naive_bayes`

The report separates those two groups so the final selection is not biased by scores that are computed differently.

## All-model accuracy chart and data

The chart below shows the 100-run mean validation accuracy for every model family.

- Chart: `reports/ml-sweep-20260504-193542/stability_best.svg`
- Raw data: `reports/ml-sweep-20260504-193542/stability-summary.csv`

| Model | Stable config | Mean val | Std | Duration mean | Note |
|---|---:|---:|---:|---:|---|
| `random_forest` | `trees=5 depth=8 leaf=3` | **99.68%** | **0.42%** | 0.461s | holdout-comparable |
| `logistic` | `lr=0.200 reg=l1 iter=1000` | **98.74%** | **0.87%** | 8.320s | holdout-comparable |
| `extra_trees` | `trees=11 depth=6 leaf=3` | 88.07% | 2.15% | 0.080s | holdout-comparable |
| `perceptron` | `lr=0.001 iter=1000` | 15.46% | 19.41% | 1.470s | holdout-comparable |
| `passive_aggressive` | `lr=0.050 iter=2000` | 5.00% | 2.44% | 3.177s | holdout-comparable |
| `svm` | `lr=0.005 iter=250` | 3.27% | 0.00% | 0.388s | holdout-comparable |
| `knn` | `k=1` | **100.00%** | 0.00% | 0.756s | train-set / optimistic |
| `ridge` | `alpha=0.01` | **99.37%** | 0.00% | 1.093s | train-set / optimistic |
| `naive_bayes` | `default` | **98.52%** | 0.00% | 0.024s | train-set / optimistic |
| `adaboost` | `estimators=300` | 91.62% | 2.68% | 1.257s | train-set / optimistic |

Notes:

- `overall_best.svg` is the one-off best grid point by family.
- `stability_best.svg` is the fairer cross-model chart because it uses the 100-run mean validation.
- For deployment choice, the comparable group is the one that matters.

## Representative variant gallery

To make the presentation cover more than 20 model variants, the HTML deck now includes a gallery of **28 representative configs**:

- top 3 configs per family where available
- charted as two PPTX-style gallery slides in `docs/ml-benchmark-presentation.html`
- useful when you want to point at more than just the family-level best row

## Best model parameter accuracy and duration

For the selected family, we also charted the full parameter grid with both accuracy and training duration.

- Accuracy chart: `reports/ml-sweep-20260504-193542/random-forest.svg`
- Duration chart: `reports/ml-sweep-20260504-193542/random-forest-duration.svg`
- Raw data: `reports/ml-sweep-20260504-193542/random-forest-grid.csv`

The parameter sweep used `leaf=3` fixed and scanned a wider `numTrees × maxDepth` grid.

| Config | Train | Validation | Duration |
|---|---:|---:|---:|
| `trees=5 depth=8 leaf=3` | **100.00%** | **100.00%** | 0.072s |
| `trees=3 depth=10 leaf=3` | 100.00% | **100.00%** | 0.089s |
| `trees=5 depth=4 leaf=3` | 100.00% | **100.00%** | 0.090s |
| `trees=5 depth=6 leaf=3` | 100.00% | **100.00%** | 0.090s |
| `trees=7 depth=8 leaf=3` | 100.00% | **100.00%** | 0.134s |
| `trees=11 depth=8 leaf=3` | 100.00% | **100.00%** | 0.304s |
| `trees=15 depth=10 leaf=3` | 100.00% | **100.00%** | 0.319s |
| `trees=11 depth=10 leaf=3` | 100.00% | **100.00%** | 0.321s |
| `trees=15 depth=6 leaf=3` | 100.00% | **100.00%** | 0.324s |
| `trees=15 depth=4 leaf=3` | 100.00% | **100.00%** | 0.347s |

Interpretation:

- the best single-split point was `trees=3 depth=4 leaf=3`
- the most stable 100-run point was `trees=5 depth=8 leaf=3`
- the stable point is the one used for the final selection

## Analysis

1. **Random forest is the only family that stays above 99% under 100 repeated runs.**  
   The single best grid split hit 100%, but the more important stability phase shows the real story: `trees=5 depth=8 leaf=3` keeps a **99.68% mean validation** with a small spread.

2. **The stable forest config is more trustworthy than the one-off 100% split.**  
   `trees=3 depth=4 leaf=3` looked perfect on one split, but the 100-run stability check prefers `trees=5 depth=8 leaf=3`.

3. **Logistic regression improved after the parameter sweep was widened, but it still trails the forest family.**  
   Logistic is now a real competitor in the linear family, but its 100-run mean is still below the random forest result and its training cost is much higher.

4. **The dataset is likely non-linear and class-skewed.**  
   The forest family handles the structure much better than the linear baselines, which fits the observed label skew.

5. **The “optimistic” families are not the deployment decision basis.**  
   `knn`, `ridge`, `naive_bayes`, and `adaboost` can still be compared inside their own trainer behavior, but their score path is not the same as the holdout-comparable group, so the final selection should not rely on them.

## Decision

**Final selection:** `random_forest`

**Final config:** `trees=5 depth=8 leaf=3`

Why this one:

- it clears the requested **99%+** threshold on the repeated benchmark
- it is stable across 100 repeats
- it is the best holdout-comparable result in the expanded horizontal sweep

## Reproduce / auto-tune

The repo includes an offline sweep runner that emits CSV, SVG, HTML, and JSON artifacts:

```bash
./scripts/ml-sweep.sh --mode full --repeats 100 --stability-top 1
# or
make ml-sweep
```

Render the PPTX-style HTML presentation:

```bash
python scripts/render_ml_presentation.py
# or
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
- `stability_best.svg` — 100-run stability comparison
- `random-forest.svg` — best model validation heatmap
- `random-forest-duration.svg` — best model duration heatmap
- `random-forest-grid.csv` — best model parameter grid data
- `results.csv` — raw grid sweep data
- `stability-runs.csv` — all repeat runs
- `stability-summary.csv` — aggregated 100-run stats
- `best.json` — best configuration snapshot

## Latest automated sweep

The latest full run was:

```text
reports/ml-sweep-20260504-193542/
```

Key output from that run:

```text
[ml-sweep] comparable best: random_forest | trees=5 depth=8 leaf=3 | mean=99.68% ± 0.42% (100x)
```

That is the benchmark result used for the final selection above.
