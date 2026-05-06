#!/usr/bin/env python3
"""Render a PPTX-style HTML presentation from an ML sweep report."""

from __future__ import annotations

import argparse
import csv
import html
import json
import re
import sys
from collections import defaultdict
from pathlib import Path
from typing import Any


PROFILE_TITLES = {
    "random_forest": "Random Forest",
    "extra_trees": "Extra Trees",
    "logistic": "Logistic Regression",
    "logistic_balanced": "Logistic Balanced",
    "svm": "Linear SVM",
    "svm_balanced": "Linear SVM Balanced",
    "perceptron": "Perceptron",
    "perceptron_balanced": "Perceptron Balanced",
    "passive_aggressive": "Passive Aggressive",
    "passive_aggressive_balanced": "Passive Aggressive Balanced",
    "knn": "KNN",
    "knn_cosine": "KNN Cosine",
    "ridge": "Ridge",
    "adaboost": "AdaBoost",
    "naive_bayes": "Naive Bayes",
    "naive_bayes_balanced": "Naive Bayes Balanced",
    "nearest_centroid": "Nearest Centroid",
    "nearest_centroid_balanced": "Nearest Centroid Balanced",
    "nearest_centroid_cosine": "Nearest Centroid Cosine",
}

PROFILE_ACCENTS = {
    "random_forest": "linear-gradient(135deg, #60a5fa 0%, #2563eb 100%)",
    "extra_trees": "linear-gradient(135deg, #a78bfa 0%, #7c3aed 100%)",
    "logistic": "linear-gradient(135deg, #34d399 0%, #059669 100%)",
    "logistic_balanced": "linear-gradient(135deg, #10b981 0%, #047857 100%)",
    "svm": "linear-gradient(135deg, #fb7185 0%, #e11d48 100%)",
    "svm_balanced": "linear-gradient(135deg, #f43f5e 0%, #be123c 100%)",
    "perceptron": "linear-gradient(135deg, #f59e0b 0%, #d97706 100%)",
    "perceptron_balanced": "linear-gradient(135deg, #f59e0b 0%, #b45309 100%)",
    "passive_aggressive": "linear-gradient(135deg, #f97316 0%, #ea580c 100%)",
    "passive_aggressive_balanced": "linear-gradient(135deg, #fb923c 0%, #c2410c 100%)",
    "knn": "linear-gradient(135deg, #22c55e 0%, #16a34a 100%)",
    "knn_cosine": "linear-gradient(135deg, #16a34a 0%, #15803d 100%)",
    "ridge": "linear-gradient(135deg, #38bdf8 0%, #0ea5e9 100%)",
    "adaboost": "linear-gradient(135deg, #f472b6 0%, #db2777 100%)",
    "naive_bayes": "linear-gradient(135deg, #64748b 0%, #334155 100%)",
    "naive_bayes_balanced": "linear-gradient(135deg, #475569 0%, #1e293b 100%)",
    "nearest_centroid": "linear-gradient(135deg, #8b5cf6 0%, #6d28d9 100%)",
    "nearest_centroid_balanced": "linear-gradient(135deg, #a855f7 0%, #7c3aed 100%)",
    "nearest_centroid_cosine": "linear-gradient(135deg, #d946ef 0%, #a21caf 100%)",
}

REPORT_TITLE = "ML Sweep 扩大研究：基础模型扩展与最新论文脉络"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--report-dir",
        type=Path,
        default=None,
        help="Path to the ml-sweep report directory. Defaults to the latest reports/ml-sweep-* directory.",
    )
    parser.add_argument(
        "--out",
        type=Path,
        default=Path("docs/ml-benchmark-presentation.html"),
        help="Output HTML file path.",
    )
    return parser.parse_args()


def latest_report_dir(base: Path) -> Path:
    candidates = [p for p in base.glob("ml-sweep-*") if p.is_dir()]
    if not candidates:
        raise FileNotFoundError(f"no ml-sweep report directories found under {base}")
    return max(candidates, key=lambda p: p.stat().st_mtime)


def read_csv_rows(path: Path) -> list[dict[str, Any]]:
    with path.open("r", encoding="utf-8", newline="") as f:
        return list(csv.DictReader(f))


def load_results(path: Path) -> list[dict[str, Any]]:
    rows = read_csv_rows(path)
    out: list[dict[str, Any]] = []
    for row in rows:
        out.append(
            {
                "profile": row["profile"],
                "modelType": row["modelType"],
                "xValue": int(row["xValue"]),
                "yValue": int(row["yValue"]),
                "configSummary": row["configSummary"],
                "trainAccuracy": float(row["trainAccuracy"]),
                "validationAccuracy": float(row["validationAccuracy"]),
                "allowPassRate": float(row.get("allowPassRate", "0") or 0),
                "durationSeconds": float(row["durationSeconds"]),
                "inferenceDurationSeconds": float(row.get("inferenceDurationSeconds", "0") or 0),
                "inferenceSamples": int(row.get("inferenceSamples", "0") or 0),
                "inferenceLatencyMs": float(row.get("inferenceLatencyMs", "0") or 0),
                "inferenceThroughput": float(row.get("inferenceThroughput", "0") or 0),
                "numSamples": int(row["numSamples"]),
                "trainSamples": int(row["trainSamples"]),
                "validationSamples": int(row["validationSamples"]),
                "error": row.get("error", ""),
            }
        )
    return out


def load_stability(path: Path) -> list[dict[str, Any]]:
    rows = read_csv_rows(path)
    out: list[dict[str, Any]] = []
    for row in rows:
        out.append(
            {
                "profile": row["profile"],
                "modelType": row["modelType"],
                "comparable": row["comparable"].lower() == "true",
                "xValue": int(row["xValue"]),
                "yValue": int(row["yValue"]),
                "configSummary": row["configSummary"],
                "runs": int(row["runs"]),
                "successRuns": int(row["successRuns"]),
                "failureRuns": int(row["failureRuns"]),
                "trainMean": float(row["trainMean"]),
                "trainStd": float(row["trainStd"]),
                "validationMean": float(row["validationMean"]),
                "validationStd": float(row["validationStd"]),
                "validationMin": float(row["validationMin"]),
                "validationMax": float(row["validationMax"]),
                "allowMean": float(row.get("allowMean", "0") or 0),
                "allowStd": float(row.get("allowStd", "0") or 0),
                "allowMin": float(row.get("allowMin", "0") or 0),
                "allowMax": float(row.get("allowMax", "0") or 0),
                "durationMean": float(row["durationMean"]),
                "durationStd": float(row["durationStd"]),
                "inferenceMean": float(row.get("inferenceMean", "0") or 0),
                "inferenceStd": float(row.get("inferenceStd", "0") or 0),
                "inferenceMin": float(row.get("inferenceMin", "0") or 0),
                "inferenceMax": float(row.get("inferenceMax", "0") or 0),
                "inferenceLatencyMean": float(row.get("inferenceLatencyMean", "0") or 0),
                "inferenceLatencyStd": float(row.get("inferenceLatencyStd", "0") or 0),
                "successRate": float(row["successRate"]),
            }
        )
    return out


def load_best(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def grouped(rows: list[dict[str, Any]], key: str) -> dict[str, list[dict[str, Any]]]:
    out: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for row in rows:
        out[row[key]].append(row)
    return out


def best_row(rows: list[dict[str, Any]]) -> dict[str, Any]:
    return sorted(
        rows,
        key=lambda r: (
            -r["validationAccuracy"],
            -r.get("allowPassRate", 0.0),
            -r["trainAccuracy"],
            -r.get("inferenceThroughput", 0.0),
            r["durationSeconds"],
            r["xValue"],
            r["yValue"],
        ),
    )[0]


def fmt_pct(value: float, digits: int = 2) -> str:
    return f"{value * 100:.{digits}f}%"


def fmt_seconds(value: float) -> str:
    if value < 0.1:
        return f"{value * 1000:.0f} ms"
    if value < 1.0:
        return f"{value * 1000:.0f} ms"
    return f"{value:.2f}s"


def fmt_ratio(value: float) -> str:
    return f"{value:.2f}×"


def fmt_rate(value: float) -> str:
    if value >= 1000:
        return f"{value / 1000.0:.2f}k/s"
    if value >= 100:
        return f"{value:.0f}/s"
    return f"{value:.1f}/s"


def fmt_latency_ms(value: float) -> str:
    if value >= 10:
        return f"{value:.1f} ms"
    if value >= 1:
        return f"{value:.2f} ms"
    return f"{value * 1000:.0f} µs"


def escape(value: Any) -> str:
    return html.escape(str(value))


def display_profile_title(profile: str) -> str:
    return PROFILE_TITLES.get(profile, profile.replace("_", " ").title())


def latest_rows_by_profile(rows: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    out: dict[str, dict[str, Any]] = {}
    for profile, items in grouped(rows, "profile").items():
        out[profile] = best_row(items)
    return out


def profile_range_summary(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    rows_by_profile = grouped(rows, "profile")
    summaries: list[dict[str, Any]] = []
    for profile, items in rows_by_profile.items():
        xs = sorted({r["xValue"] for r in items})
        ys = sorted({r["yValue"] for r in items if r["yValue"] != 0})
        combo_count = len(items)
        if ys:
            x_text = f"{xs[0]}–{xs[-1]} ({len(xs)} values)"
            y_text = f"{ys[0]}–{ys[-1]} ({len(ys)} values)"
        else:
            x_text = f"{xs[0]}–{xs[-1]} ({len(xs)} values)"
            y_text = "single-axis"
        summaries.append(
            {
                "profile": profile,
                "title": display_profile_title(profile),
                "xText": x_text,
                "yText": y_text,
                "combos": combo_count,
                "chart": f"{profile.replace('_', '-')}.svg",
            }
        )
    return summaries


def select_gallery_variants(rows: list[dict[str, Any]], per_profile: int = 2) -> list[dict[str, Any]]:
    rows_by_profile = grouped(rows, "profile")
    gallery: list[dict[str, Any]] = []
    for profile, items in rows_by_profile.items():
        if not items:
            continue
        ordered = sorted(
            items,
            key=lambda r: (
                -r["validationAccuracy"],
                -r.get("allowPassRate", 0.0),
                -r["trainAccuracy"],
                -r.get("inferenceThroughput", 0.0),
                r["durationSeconds"],
                r["xValue"],
                r["yValue"],
            ),
        )
        for rank, row in enumerate(ordered[:per_profile], start=1):
            copy = dict(row)
            copy["familyLabel"] = display_profile_title(profile)
            copy["familyRank"] = rank
            copy["familyAccent"] = PROFILE_ACCENTS.get(profile, "linear-gradient(135deg, #60a5fa, #2563eb)")
            copy["variantLabel"] = f"{copy['familyLabel']} #{rank}"
            gallery.append(copy)
    return gallery


def render_svg(report_dir: Path, filename: str) -> str:
    path = report_dir / filename
    if not path.exists():
        return f"<div class='missing'>Missing chart: {escape(filename)}</div>"
    text = path.read_text(encoding="utf-8")
    text = re.sub(r"^\s*<\?xml[^>]*>\s*", "", text)
    text = re.sub(r"^\s*<!DOCTYPE[^>]*>\s*", "", text)
    return text


def table(headers: list[str], rows: list[list[str]], class_name: str = "") -> str:
    thead = "".join(f"<th>{escape(h)}</th>" for h in headers)
    body = []
    for row in rows:
        cells = "".join(f"<td>{cell}</td>" for cell in row)
        body.append(f"<tr>{cells}</tr>")
    body_html = "".join(body)
    cls = f" class='{class_name}'" if class_name else ""
    return f"<table{cls}><thead><tr>{thead}</tr></thead><tbody>{body_html}</tbody></table>"


def stat_card(label: str, value: str, note: str = "", accent: str = "var(--accent)") -> str:
    note_html = f"<div class='stat-note'>{escape(note)}</div>" if note else ""
    return f"""
      <div class="stat-card">
        <div class="stat-accent" style="background:{accent};"></div>
        <div class="stat-label">{escape(label)}</div>
        <div class="stat-value">{escape(value)}</div>
        {note_html}
      </div>
    """


def render_variant_card(row: dict[str, Any]) -> str:
    note = "holdout-comparable" if any(row["profile"].startswith(prefix) for prefix in ("random_forest", "extra_trees", "logistic", "svm", "perceptron", "passive_aggressive", "nearest_centroid", "ensemble")) else "train-set / optimistic"
    return f"""
      <div class="variant-card" style="--variant-accent:{row['familyAccent']};">
        <div class="variant-top">
          <div class="variant-family">{escape(row['variantLabel'])}</div>
          <div class="variant-score">{fmt_pct(row['validationAccuracy'])}</div>
        </div>
        <div class="variant-config">{escape(row['configSummary'])}</div>
        <div class="variant-meta">
          <div><span>Train</span><strong>{fmt_pct(row['trainAccuracy'])}</strong></div>
          <div><span>ALLOW</span><strong>{fmt_pct(row.get('allowPassRate', 0.0))}</strong></div>
        </div>
        <div class="variant-note">{escape(note)} · {fmt_rate(row.get('inferenceThroughput', 0.0))} · {fmt_latency_ms(row.get('inferenceLatencyMs', 0.0))} · train {fmt_seconds(row['durationSeconds'])}</div>
      </div>
    """


def variant_gallery_slide(
    number: int,
    title: str,
    subtitle: str,
    variants: list[dict[str, Any]],
    accent: str,
) -> str:
    cards = "".join(render_variant_card(row) for row in variants)
    body = f"""
      <div class="panel">
        <div class="gallery-header">
          <h3>{escape(title)}</h3>
          <div class="gallery-summary">{escape(subtitle)}</div>
        </div>
        <div class="variant-grid">
          {cards}
        </div>
      </div>
    """
    return slide(number, "Model gallery", title, subtitle, body, accent, dense=True)


def slide(
    number: int,
    eyebrow: str,
    title: str,
    subtitle: str,
    body: str,
    accent: str = "var(--accent)",
    dense: bool = False,
) -> str:
    dense_class = " slide-dense" if dense else ""
    return f"""
    <section class="slide{dense_class}" id="slide-{number}">
      <div class="slide-shell" style="--slide-accent:{accent};">
        <div class="slide-badge">{number:02d}</div>
        <div class="slide-inner">
          <div class="slide-header">
            <div class="eyebrow">{escape(eyebrow)}</div>
            <h1>{escape(title)}</h1>
            <p class="subtitle">{subtitle}</p>
          </div>
          {body}
        </div>
      </div>
    </section>
    """


def slide_cover(report_dir: Path, best: dict[str, Any], stability_best: dict[str, Any], stats: dict[str, Any]) -> str:
    best_screen = best.get("screenBest", {})
    stable_best = best.get("stableBest", {})
    best_model = display_profile_title(best_screen.get("profile", ""))
    repeat_runs = int(best.get("repeats", stable_best.get("runs", 1)) or 1)
    subtitle = (
        f"本页基于最新 full sweep 报告 {escape(report_dir.name)} 生成，覆盖 {stats['profileCount']} 个模型族与 {stats['galleryCount']} 个代表性变体，并做 {repeat_runs} 次稳定性重复。"
    )
    body = f"""
      <div class="cover-layout">
        <div class="cover-copy">
          <div class="hero-pill">PPTX-style HTML · detailed analysis</div>
          <h2>扩大研究尺度与参数范围</h2>
          <p class="cover-text">
            本次将模型搜索空间从单一参数验证扩展为更宽的组合网格，并对最优配置做 {repeat_runs} 次稳定性重复。
            目标不是只找“最高的一次”，而是找在更大搜索空间里仍然稳定、可复现、可解释的方案。
            本页也额外覆盖了 <strong>{stats['galleryCount']}</strong> 个代表性模型变体，且同时对比了准确率、ALLOW 放行率、训练耗时与推理速度。
          </p>
          <div class="cover-metrics">
            {stat_card("数据集", f"{stats['datasetSize']} 条", "已标注样本", "linear-gradient(135deg, #60a5fa, #2563eb)")}
            {stat_card("模型族", f"{stats['profileCount']} 类", f"网格组合 {stats['comboCount']} 组", "linear-gradient(135deg, #34d399, #059669)")}
            {stat_card("代表性变体", f"{stats['galleryCount']} 个", "每个 family 取 top 4", "linear-gradient(135deg, #f59e0b, #d97706)")}
          </div>
        </div>
        <div class="cover-right">
          <div class="feature-card">
            <div class="feature-label">当前最佳单次结果</div>
            <div class="feature-title">{escape(best_model)}</div>
            <div class="feature-config">{escape(best_screen.get('configSummary', ''))}</div>
            <div class="feature-grid">
              <div><span>Train</span><strong>{fmt_pct(best_screen.get('trainAccuracy', 0.0))}</strong></div>
              <div><span>Validation</span><strong>{fmt_pct(best_screen.get('validationAccuracy', 0.0))}</strong></div>
              <div><span>ALLOW</span><strong>{fmt_pct(best_screen.get('allowPassRate', 0.0))}</strong></div>
              <div><span>Infer</span><strong>{fmt_rate(best_screen.get('inferenceThroughput', 0.0))}</strong></div>
            </div>
          </div>
          <div class="feature-card feature-card-secondary">
            <div class="feature-label">稳定最优配置</div>
            <div class="feature-title">{escape(display_profile_title(stable_best.get("profile", "")))}</div>
            <div class="feature-config">{escape(stable_best.get("configSummary", ""))}</div>
            <div class="feature-grid">
              <div><span>Mean ± Std</span><strong>{fmt_pct(stable_best.get('validationMean', 0.0))} ± {fmt_pct(stable_best.get('validationStd', 0.0))}</strong></div>
              <div><span>ALLOW</span><strong>{fmt_pct(stable_best.get('allowMean', 0.0))}</strong></div>
              <div><span>Speed</span><strong>{fmt_rate(stable_best.get('inferenceMean', 0.0))}</strong></div>
              <div><span>Latency</span><strong>{fmt_latency_ms(stable_best.get('inferenceLatencyMean', 0.0))}</strong></div>
              <div><span>Success</span><strong>{fmt_pct(stable_best.get('successRate', 0.0))}</strong></div>
            </div>
          </div>
        </div>
      </div>
    """
    return slide(1, "Cover", REPORT_TITLE, subtitle, body, "linear-gradient(135deg, #60a5fa, #8b5cf6)")


def slide_scope(report_dir: Path, rows: list[dict[str, Any]], summaries: list[dict[str, Any]], stats: dict[str, Any]) -> str:
    left = f"""
      <div class="panel">
        <h3>研究如何被扩大</h3>
        <ul class="bullet-list">
          <li>从单点验证扩展为 <strong>全模型横向 sweep</strong>，覆盖树模型、线性模型、KNN、Ridge、AdaBoost、Naive Bayes 与 Ensemble，并拆出更多变体家族。</li>
          <li>树模型参数改为更宽的 <strong>numTrees × maxDepth</strong> 网格；线性模型扩大 <strong>learning rate × iterations</strong> 的搜索范围。</li>
          <li>演示页额外抽取每个 family 的 top 4 配置，形成 <strong>{stats['galleryCount']} 个代表性模型变体</strong> 的可视化画廊。</li>
          <li>对每个模型族选出的最优点，再做 <strong>稳定性重复</strong> 观察均值、标准差和成功率。</li>
          <li>最终判断不再只看单次准确率，而是看 <strong>稳定均值、ALLOW 放行率、方差、推理速度、耗时</strong> 和数据集可编辑性。</li>
        </ul>
      </div>
    """
    rows_html = []
    for s in summaries:
        rows_html.append(
            [
                escape(s["title"]),
                escape(s["xText"]),
                escape(s["yText"]),
                str(s["combos"]),
            ]
        )
    right = f"""
      <div class="panel">
        <h3>参数范围总览</h3>
        {table(["模型", "X 轴范围", "Y 轴范围", "组合数"], rows_html, "compact")}
      </div>
    """
    body = f'<div class="grid grid-2">{left}{right}</div>'
    subtitle = f"本轮 sweep 统计 {len(rows)} 条单次训练结果，重点展示扩大的搜索空间和参数组合总量，同时在演示页中加入 {len(summaries)} 个模型族的代表性变体。"
    return slide(2, "Research scope", "更大的搜索空间，更严格的评价口径", subtitle, body, "linear-gradient(135deg, #34d399, #059669)")


def slide_literature() -> str:
    left = f"""
      <div class="panel">
        <h3>最新论文给了什么信号</h3>
        <ul class="bullet-list">
          <li><strong>CLIMB (2025)</strong>：类不平衡 tabular 任务里，单纯重采样不一定有效，ensemble 往往更稳；评价指标不能只看准确率。</li>
          <li><strong>PMLBmini (2024)</strong>：在低数据区间里，简单逻辑回归仍然是强基线，AutoML / deep learning 并不总能稳定压过它。</li>
          <li><strong>TabArena (2025)</strong>：living benchmark 的核心不是堆榜，而是持续更新数据集、模型与评测协议，并把 tuning + ensembling 看成真实上限的一部分。</li>
          <li><strong>综合基准 (2024)</strong>：传统树模型依旧很强，但在充分调参与合并后，深度模型在部分 tabular 任务上也能接近甚至超过它们。</li>
          <li><strong>对本仓库的直接启发</strong>：我把线性、树、近邻、朴素贝叶斯的“基础模型 + 变种”一起拉进来，并同时看准确率、ALLOW 放行率、推理速度与训练耗时。</li>
        </ul>
      </div>
    """
    rows_html = [
        [
            "CLIMB 2025",
            "不平衡数据上，单纯重采样未必提升；ensemble 更稳，指标不能只看 accuracy。",
            "增加 balanced 变体，并把 ALLOW 放行率纳入主表。",
        ],
        [
            "PMLBmini 2024",
            "小样本 tabular 里，逻辑回归仍然常常是强基线。",
            "保留并扩展 logistic / ridge / nearest centroid 这类轻量基线。",
        ],
        [
            "TabArena 2025",
            "living benchmark 要持续更新数据、模型和协议，而不是静态堆分数。",
            "把 sweep、repeat、图表、HTML 演示页做成可复用流程。",
        ],
        [
            "综合基准 2024",
            "树模型仍强，但调参 + 合并后，深度模型在部分 tabular 任务可竞争。",
            "对随机森林 / Extra Trees / ensemble 做更宽参数扫描。",
        ],
    ]
    right = f"""
      <div class="panel">
        <h3>论文 → 工程映射</h3>
        {table(["论文", "关键结论", "本仓库里的对应动作"], rows_html, "compact")}
        <div class="small" style="margin-top:10px;opacity:.82;">
          注：最后一列是基于论文结论做出的工程映射（推断），不是论文原文。
        </div>
      </div>
    """
    body = f'<div class="grid grid-2">{left}{right}</div>'
    subtitle = "这一页把近期 tabular benchmark 的共识，翻译成这次基础模型扩展与参数搜索的工程选择。"
    return slide(3, "Literature", "参考最新论文，继续扩展基础模型", subtitle, body, "linear-gradient(135deg, #f59e0b, #d97706)")


def slide_overall(rows: list[dict[str, Any]], best: dict[str, Any]) -> str:
    best_by_profile = latest_rows_by_profile(rows)
    ordered = sorted(
        best_by_profile.values(),
        key=lambda r: (-r["validationAccuracy"], -r.get("allowPassRate", 0.0), -r["trainAccuracy"], -r.get("inferenceThroughput", 0.0), r["durationSeconds"]),
    )
    table_rows = []
    for row in ordered:
        table_rows.append(
            [
                escape(display_profile_title(row["profile"])),
                escape(row["configSummary"]),
                fmt_pct(row["validationAccuracy"]),
                fmt_pct(row.get("allowPassRate", 0.0)),
                fmt_pct(row["trainAccuracy"]),
                fmt_seconds(row["durationSeconds"]),
                fmt_rate(row.get("inferenceThroughput", 0.0)),
                fmt_latency_ms(row.get("inferenceLatencyMs", 0.0)),
            ]
        )
    body = f"""
      <div class="stack">
        <div class="grid grid-2-wide">
          <div class="panel chart-panel">
            <h3>各模型单次最优准确率</h3>
            <div class="svg-card">{render_svg(best["outDirPath"], "overall_best.svg") if isinstance(best.get("outDirPath"), Path) else ""}</div>
          </div>
          <div class="panel chart-panel">
            <h3>各模型单次最优推理速度</h3>
            <div class="svg-card">{render_svg(best["outDirPath"], "overall_speed.svg") if isinstance(best.get("outDirPath"), Path) else ""}</div>
          </div>
        </div>
        <div class="panel">
          <h3>按验证准确率排序的最佳点</h3>
          {table(["模型", "最佳配置", "Val", "ALLOW", "Train", "Time", "Infer/s", "Latency"], table_rows, "compact")}
        </div>
      </div>
    """
    subtitle = "除了整体验证准确率，这一页也把 ALLOW 放行率摆到同一张表里，避免只看错误率而误伤正确命令。"
    return slide(4, "Model comparison", "所有模型的准确率与放行率横向对比", subtitle, body, "linear-gradient(135deg, #60a5fa, #2563eb)")


def slide_speed(rows: list[dict[str, Any]], best: dict[str, Any]) -> str:
    best_by_profile = latest_rows_by_profile(rows)
    ordered = sorted(
        best_by_profile.values(),
        key=lambda r: (-r.get("inferenceThroughput", 0.0), -r["validationAccuracy"], -r.get("allowPassRate", 0.0), r["durationSeconds"]),
    )
    table_rows = []
    for row in ordered:
        table_rows.append(
            [
                escape(display_profile_title(row["profile"])),
                escape(row["configSummary"]),
                fmt_pct(row["validationAccuracy"]),
                fmt_pct(row.get("allowPassRate", 0.0)),
                fmt_pct(row["trainAccuracy"]),
                fmt_seconds(row["durationSeconds"]),
                fmt_rate(row.get("inferenceThroughput", 0.0)),
                fmt_latency_ms(row.get("inferenceLatencyMs", 0.0)),
            ]
        )
    body = f"""
      <div class="stack">
        <div class="panel chart-panel">
          <h3>单次最优推理速度</h3>
          <div class="svg-card">{render_svg(best["outDirPath"], "overall_speed.svg") if isinstance(best.get("outDirPath"), Path) else ""}</div>
        </div>
        <div class="panel">
          <h3>按推理速度排序的最佳点</h3>
          {table(["模型", "最佳配置", "Val", "ALLOW", "Train", "Time", "Infer/s", "Latency"], table_rows, "compact")}
        </div>
      </div>
    """
    subtitle = "推理速度采用同一批缓存样本进行计时，因此可以跨模型家族横向比较。"
    return slide(5, "Inference speed", "推理速度横向对比", subtitle, body, "linear-gradient(135deg, #22c55e, #16a34a)")


def slide_gallery_one(variants: list[dict[str, Any]]) -> str:
    subtitle = f"每个 family 取 top 4 的前半部分，共 {len(variants)} 个代表性配置。"
    return variant_gallery_slide(
        6,
        "Representative variants I",
        subtitle,
        variants,
        "linear-gradient(135deg, #f59e0b, #d97706)",
    )


def slide_gallery_two(variants: list[dict[str, Any]]) -> str:
    subtitle = f"每个 family 取 top 4 的中后段，共 {len(variants)} 个代表性配置。"
    return variant_gallery_slide(
        7,
        "Representative variants II",
        subtitle,
        variants,
        "linear-gradient(135deg, #f97316, #ea580c)",
    )


def slide_gallery_three(variants: list[dict[str, Any]]) -> str:
    subtitle = f"每个 family 取 top 4 的收尾部分，共 {len(variants)} 个代表性配置。"
    return variant_gallery_slide(
        8,
        "Representative variants III",
        subtitle,
        variants,
        "linear-gradient(135deg, #0ea5e9, #0284c7)",
    )


def slide_stability(stability: list[dict[str, Any]], best: dict[str, Any]) -> str:
    repeat_runs = int(best.get("repeats", 1) or 1)
    comparable = [r for r in stability if r["comparable"]]
    comparable_sorted = sorted(
        comparable,
        key=lambda r: (-r["validationMean"], -r.get("allowMean", 0.0), -r["successRate"], r["validationStd"], -r.get("inferenceMean", 0.0), r["durationMean"]),
    )
    rows_html = []
    for row in comparable_sorted:
        rows_html.append(
            [
                escape(display_profile_title(row["profile"])),
                escape(row["configSummary"]),
                fmt_pct(row["validationMean"]),
                fmt_pct(row["validationStd"]),
                fmt_pct(row.get("allowMean", 0.0)),
                fmt_pct(row.get("allowStd", 0.0)),
                fmt_rate(row.get("inferenceMean", 0.0)),
                fmt_latency_ms(row.get("inferenceLatencyMean", 0.0)),
                fmt_pct(row["successRate"]),
            ]
        )
    body = f"""
      <div class="grid grid-2-wide">
        <div class="panel chart-panel">
          <h3>最优可比模型的稳定性</h3>
          <div class="chart-row">
            <div class="chart">{render_svg(best["outDirPath"], "stability_best.svg") if isinstance(best.get("outDirPath"), Path) else ""}</div>
            <div class="chart">{render_svg(best["outDirPath"], "stability_speed.svg") if isinstance(best.get("outDirPath"), Path) else ""}</div>
          </div>
        </div>
        <div class="panel">
          <h3>可比模型 {repeat_runs} 次重复统计</h3>
          {table(["模型", "最佳配置", "Mean", "Std", "ALLOW", "ALLOW Std", "Speed", "Latency", "Success"], rows_html, "compact")}
        </div>
      </div>
    """
    if repeat_runs > 1:
        title = f"稳定性分析：谁在 {repeat_runs} 次重复里站得住"
        subtitle = f"真正决定可用性的，不是一次最高值，而是 {repeat_runs} 次重复之后的均值、放行率、波动、成功率和推理速度。"
    else:
        title = "探索性稳定性分析：谁在这轮探索里站得住"
        subtitle = "真正决定可用性的，不是一次最高值，而是这轮探索里观察到的均值、放行率、波动、成功率和推理速度。"
    return slide(9, "Stability", title, subtitle, body, "linear-gradient(135deg, #a78bfa, #7c3aed)")


def top_configs_table(rows: list[dict[str, Any]], limit: int = 5) -> str:
    ordered = sorted(rows, key=lambda r: (-r["validationAccuracy"], -r.get("allowPassRate", 0.0), -r.get("inferenceThroughput", 0.0), r["durationSeconds"]))
    rows_html = []
    for row in ordered[:limit]:
        rows_html.append(
            [
                escape(row["configSummary"]),
                fmt_pct(row["validationAccuracy"]),
                fmt_pct(row.get("allowPassRate", 0.0)),
                fmt_pct(row["trainAccuracy"]),
                fmt_seconds(row["durationSeconds"]),
                fmt_rate(row.get("inferenceThroughput", 0.0)),
                fmt_latency_ms(row.get("inferenceLatencyMs", 0.0)),
            ]
        )
    return table(["配置", "Validation", "ALLOW", "Train", "耗时", "Infer/s", "Latency"], rows_html, "compact")


def slide_random_forest(rows: list[dict[str, Any]], stability: list[dict[str, Any]], best: dict[str, Any]) -> str:
    rf_rows = [r for r in rows if r["profile"] == "random_forest"]
    rf_stability = [r for r in stability if r["profile"] == "random_forest"]
    rf_best = best_row(rf_rows)
    rf_stable = sorted(rf_stability, key=lambda r: (-r["validationMean"], -r.get("allowMean", 0.0), r["validationStd"], -r.get("inferenceMean", 0.0), r["durationMean"]))[0]
    body = f"""
      <div class="stack">
        <div class="metric-row">
          {stat_card("单次最佳", rf_best["configSummary"], f"Validation {fmt_pct(rf_best['validationAccuracy'])}", "linear-gradient(135deg, #60a5fa, #2563eb)")}
          {stat_card("稳定最佳", rf_stable["configSummary"], f"{fmt_pct(rf_stable['validationMean'])} ± {fmt_pct(rf_stable['validationStd'])}", "linear-gradient(135deg, #34d399, #059669)")}
          {stat_card("推理速度", fmt_rate(rf_stable.get('inferenceMean', 0.0)), f"{fmt_latency_ms(rf_stable.get('inferenceLatencyMean', 0.0))} · {rf_stable['runs']} 次重复", "linear-gradient(135deg, #f59e0b, #d97706)")}
          {stat_card("ALLOW 放行", fmt_pct(rf_stable.get('allowMean', 0.0)), f"{fmt_pct(rf_stable.get('allowStd', 0.0))} · 正确命令放行率", "linear-gradient(135deg, #a78bfa, #7c3aed)")}
        </div>
        <div class="grid grid-2-wide">
          <div class="panel chart-panel">
            <h3>Random Forest 准确率热力图</h3>
            <div class="svg-card">{render_svg(best["outDirPath"], "random-forest.svg") if isinstance(best.get("outDirPath"), Path) else ""}</div>
          </div>
          <div class="panel chart-panel">
            <h3>Random Forest 推理速度</h3>
            <div class="svg-card">{render_svg(best["outDirPath"], "random-forest-inference.svg") if isinstance(best.get("outDirPath"), Path) else ""}</div>
          </div>
        </div>
        <div class="panel">
          <h3>Top 5 参数点</h3>
          {top_configs_table(rf_rows, 5)}
        </div>
      </div>
    """
    subtitle = "随机森林是本次 sweep 的核心候选，也是唯一在大多数重复里稳过 99% 的模型；这里同时看准确率、正确命令放行率、训练耗时和推理速度。"
    return slide(10, "Deep dive", "最好模型的参数准确率与耗时分析", subtitle, body, "linear-gradient(135deg, #22c55e, #16a34a)", dense=True)


def slide_tree_family(rows: list[dict[str, Any]], best: dict[str, Any]) -> str:
    best_map = latest_rows_by_profile(rows)
    rf = best_map.get("random_forest", {})
    et = best_map.get("extra_trees", {})
    body = f"""
      <div class="stack">
        <div class="grid grid-2-wide">
          <div class="panel chart-panel">
            <h3>Random Forest</h3>
            <div class="svg-card">{render_svg(best["outDirPath"], "random-forest.svg") if isinstance(best.get("outDirPath"), Path) else ""}</div>
          </div>
          <div class="panel chart-panel">
            <h3>Extra Trees</h3>
            <div class="svg-card">{render_svg(best["outDirPath"], "extra-trees.svg") if isinstance(best.get("outDirPath"), Path) else ""}</div>
          </div>
        </div>
        <div class="panel">
          <h3>树模型最佳点对比</h3>
          {table(["模型", "最佳配置", "Val", "ALLOW", "Train", "Time", "Infer/s", "Latency"], [
              [escape(display_profile_title(rf.get("profile", ""))), escape(rf.get("configSummary", "")), fmt_pct(rf.get("validationAccuracy", 0.0)), fmt_pct(rf.get("allowPassRate", 0.0)), fmt_pct(rf.get("trainAccuracy", 0.0)), fmt_seconds(rf.get("durationSeconds", 0.0)), fmt_rate(rf.get("inferenceThroughput", 0.0)), fmt_latency_ms(rf.get("inferenceLatencyMs", 0.0))],
              [escape(display_profile_title(et.get("profile", ""))), escape(et.get("configSummary", "")), fmt_pct(et.get("validationAccuracy", 0.0)), fmt_pct(et.get("allowPassRate", 0.0)), fmt_pct(et.get("trainAccuracy", 0.0)), fmt_seconds(et.get("durationSeconds", 0.0)), fmt_rate(et.get("inferenceThroughput", 0.0)), fmt_latency_ms(et.get("inferenceLatencyMs", 0.0))],
          ], "compact")}
        </div>
        <div class="panel">
          <ul class="bullet-list">
            <li>两者都属于树集成，但 Extra Trees 更随机，通常更快、但稳定性和上限都弱于随机森林。</li>
            <li>随机森林在更宽的树数/深度网格里依然能出现多处 100%，说明它对这批数据更匹配。</li>
            <li>树深过浅时容易漏掉少数类别，过深会让单次结果看似更高，但稳定性不一定更好。</li>
          </ul>
        </div>
      </div>
    """
    subtitle = "树模型是当前数据集上最值得继续挖掘的方向，随机森林表现明显领先 Extra Trees。"
    return slide(11, "Tree family", "树模型家族横向对比", subtitle, body, "linear-gradient(135deg, #8b5cf6, #7c3aed)", dense=True)


def slide_linear_family(rows: list[dict[str, Any]], best: dict[str, Any]) -> str:
    best_map = latest_rows_by_profile(rows)
    chart_names = [
        ("logistic.svg", "Logistic"),
        ("svm.svg", "SVM"),
        ("perceptron.svg", "Perceptron"),
        ("passive-aggressive.svg", "Passive Aggressive"),
    ]
    charts = []
    for filename, label in chart_names:
        charts.append(
            f"""
            <div class="panel chart-panel mini">
              <h3>{label}</h3>
              <div class="svg-card">{render_svg(best["outDirPath"], filename) if isinstance(best.get("outDirPath"), Path) else ""}</div>
            </div>
            """
        )
    body = f"""
      <div class="stack">
        <div class="grid grid-2-tight">{''.join(charts)}</div>
        <div class="panel">
          <h3>线性模型的结论</h3>
          <ul class="bullet-list">
            <li><strong>Logistic Regression</strong> 在扩大后的参数空间里是这组线性模型中最有竞争力的基线，但仍明显落后于随机森林。</li>
            <li><strong>SVM / Perceptron / Passive Aggressive</strong> 对当前特征空间较敏感，单次结果波动较大，稳定性不足。</li>
            <li>这组模型适合当作“轻量级可解释基线”，不适合作为最终高胜率方案。</li>
          </ul>
          <div class="mini-table">
            {table(["模型", "最佳配置", "Val", "ALLOW", "Train", "Time", "Infer/s", "Latency"], [
                [escape(display_profile_title("logistic")), escape(best_map.get("logistic", {}).get("configSummary", "")), fmt_pct(best_map.get("logistic", {}).get("validationAccuracy", 0.0)), fmt_pct(best_map.get("logistic", {}).get("allowPassRate", 0.0)), fmt_pct(best_map.get("logistic", {}).get("trainAccuracy", 0.0)), fmt_seconds(best_map.get("logistic", {}).get("durationSeconds", 0.0)), fmt_rate(best_map.get("logistic", {}).get("inferenceThroughput", 0.0)), fmt_latency_ms(best_map.get("logistic", {}).get("inferenceLatencyMs", 0.0))],
                [escape(display_profile_title("svm")), escape(best_map.get("svm", {}).get("configSummary", "")), fmt_pct(best_map.get("svm", {}).get("validationAccuracy", 0.0)), fmt_pct(best_map.get("svm", {}).get("allowPassRate", 0.0)), fmt_pct(best_map.get("svm", {}).get("trainAccuracy", 0.0)), fmt_seconds(best_map.get("svm", {}).get("durationSeconds", 0.0)), fmt_rate(best_map.get("svm", {}).get("inferenceThroughput", 0.0)), fmt_latency_ms(best_map.get("svm", {}).get("inferenceLatencyMs", 0.0))],
                [escape(display_profile_title("perceptron")), escape(best_map.get("perceptron", {}).get("configSummary", "")), fmt_pct(best_map.get("perceptron", {}).get("validationAccuracy", 0.0)), fmt_pct(best_map.get("perceptron", {}).get("allowPassRate", 0.0)), fmt_pct(best_map.get("perceptron", {}).get("trainAccuracy", 0.0)), fmt_seconds(best_map.get("perceptron", {}).get("durationSeconds", 0.0)), fmt_rate(best_map.get("perceptron", {}).get("inferenceThroughput", 0.0)), fmt_latency_ms(best_map.get("perceptron", {}).get("inferenceLatencyMs", 0.0))],
                [escape(display_profile_title("passive_aggressive")), escape(best_map.get("passive_aggressive", {}).get("configSummary", "")), fmt_pct(best_map.get("passive_aggressive", {}).get("validationAccuracy", 0.0)), fmt_pct(best_map.get("passive_aggressive", {}).get("allowPassRate", 0.0)), fmt_pct(best_map.get("passive_aggressive", {}).get("trainAccuracy", 0.0)), fmt_seconds(best_map.get("passive_aggressive", {}).get("durationSeconds", 0.0)), fmt_rate(best_map.get("passive_aggressive", {}).get("inferenceThroughput", 0.0)), fmt_latency_ms(best_map.get("passive_aggressive", {}).get("inferenceLatencyMs", 0.0))],
            ], "compact")}
          </div>
        </div>
      </div>
    """
    subtitle = "线性模型可以保留为轻量基线，但从结果上看并不是当前数据集的主力方案。"
    return slide(12, "Linear family", "线性模型家族分析", subtitle, body, "linear-gradient(135deg, #f97316, #ea580c)", dense=True)


def slide_baselines(rows: list[dict[str, Any]], best: dict[str, Any]) -> str:
    best_map = latest_rows_by_profile(rows)
    chart_names = [
        ("knn.svg", "KNN"),
        ("ridge.svg", "Ridge"),
        ("adaboost.svg", "AdaBoost"),
        ("naive-bayes.svg", "Naive Bayes"),
    ]
    charts = []
    for filename, label in chart_names:
        charts.append(
            f"""
            <div class="panel chart-panel mini">
              <h3>{label}</h3>
              <div class="svg-card">{render_svg(best["outDirPath"], filename) if isinstance(best.get("outDirPath"), Path) else ""}</div>
            </div>
            """
        )
    body = f"""
      <div class="stack">
        <div class="grid grid-2-tight">{''.join(charts)}</div>
        <div class="panel">
          <h3>基线模型观察</h3>
          <ul class="bullet-list">
            <li><strong>KNN</strong> 在训练集上很容易接近满分，但可比口径下不宜直接与 holdout 结果混合看待。</li>
            <li><strong>Ridge</strong> 与 <strong>Naive Bayes</strong> 是稳定、简单、便于解释的对照组。</li>
            <li><strong>AdaBoost</strong> 在这份数据上有一定提升，但仍未追上树集成主力。</li>
            <li>这组模型更适合做“下限基线”与回归监测，而不是最终选型。</li>
          </ul>
          <div class="mini-table">
            {table(["模型", "最佳配置", "Val", "ALLOW", "Train", "Time", "Infer/s", "Latency"], [
                [escape(display_profile_title("knn")), escape(best_map.get("knn", {}).get("configSummary", "")), fmt_pct(best_map.get("knn", {}).get("validationAccuracy", 0.0)), fmt_pct(best_map.get("knn", {}).get("allowPassRate", 0.0)), fmt_pct(best_map.get("knn", {}).get("trainAccuracy", 0.0)), fmt_seconds(best_map.get("knn", {}).get("durationSeconds", 0.0)), fmt_rate(best_map.get("knn", {}).get("inferenceThroughput", 0.0)), fmt_latency_ms(best_map.get("knn", {}).get("inferenceLatencyMs", 0.0))],
                [escape(display_profile_title("ridge")), escape(best_map.get("ridge", {}).get("configSummary", "")), fmt_pct(best_map.get("ridge", {}).get("validationAccuracy", 0.0)), fmt_pct(best_map.get("ridge", {}).get("allowPassRate", 0.0)), fmt_pct(best_map.get("ridge", {}).get("trainAccuracy", 0.0)), fmt_seconds(best_map.get("ridge", {}).get("durationSeconds", 0.0)), fmt_rate(best_map.get("ridge", {}).get("inferenceThroughput", 0.0)), fmt_latency_ms(best_map.get("ridge", {}).get("inferenceLatencyMs", 0.0))],
                [escape(display_profile_title("adaboost")), escape(best_map.get("adaboost", {}).get("configSummary", "")), fmt_pct(best_map.get("adaboost", {}).get("validationAccuracy", 0.0)), fmt_pct(best_map.get("adaboost", {}).get("allowPassRate", 0.0)), fmt_pct(best_map.get("adaboost", {}).get("trainAccuracy", 0.0)), fmt_seconds(best_map.get("adaboost", {}).get("durationSeconds", 0.0)), fmt_rate(best_map.get("adaboost", {}).get("inferenceThroughput", 0.0)), fmt_latency_ms(best_map.get("adaboost", {}).get("inferenceLatencyMs", 0.0))],
                [escape(display_profile_title("naive_bayes")), escape(best_map.get("naive_bayes", {}).get("configSummary", "")), fmt_pct(best_map.get("naive_bayes", {}).get("validationAccuracy", 0.0)), fmt_pct(best_map.get("naive_bayes", {}).get("allowPassRate", 0.0)), fmt_pct(best_map.get("naive_bayes", {}).get("trainAccuracy", 0.0)), fmt_seconds(best_map.get("naive_bayes", {}).get("durationSeconds", 0.0)), fmt_rate(best_map.get("naive_bayes", {}).get("inferenceThroughput", 0.0)), fmt_latency_ms(best_map.get("naive_bayes", {}).get("inferenceLatencyMs", 0.0))],
            ], "compact")}
          </div>
        </div>
      </div>
    """
    subtitle = "这些模型的价值在于提供下限和速度参考，而不是与随机森林争夺最终选型。"
    return slide(13, "Baselines", "KNN / Ridge / AdaBoost / Naive Bayes", subtitle, body, "linear-gradient(135deg, #64748b, #334155)", dense=True)


def slide_conclusion(best: dict[str, Any], stability: list[dict[str, Any]], report_dir: Path, gallery_count: int, profile_count: int) -> str:
    stable_best = best.get("stableBest", {})
    screen_best = best.get("screenBest", {})
    stable_runs = int(stable_best.get("runs", best.get("repeats", 1)) or 1)
    recommendation_label = "最终推荐" if stable_runs > 1 else "当前推荐"
    conclusion = []
    conclusion.append(
        f"<li><strong>{recommendation_label}：</strong> {escape(display_profile_title(stable_best.get('profile', '')))}，配置 {escape(stable_best.get('configSummary', ''))}。</li>"
    )
    conclusion.append(
        f"<li><strong>稳定性：</strong> {stable_runs} 次重复均值 {fmt_pct(stable_best.get('validationMean', 0.0))}，ALLOW 放行 {fmt_pct(stable_best.get('allowMean', 0.0))}，标准差 {fmt_pct(stable_best.get('validationStd', 0.0))}，成功率 {fmt_pct(stable_best.get('successRate', 0.0))}，推理速度 {fmt_rate(stable_best.get('inferenceMean', 0.0))}。</li>"
    )
    conclusion.append(
        f"<li><strong>单次峰值：</strong> {escape(display_profile_title(screen_best.get('profile', '')))} 的单次最佳可达 {fmt_pct(screen_best.get('validationAccuracy', 0.0))}，ALLOW 放行 {fmt_pct(screen_best.get('allowPassRate', 0.0))}，推理速度 {fmt_rate(screen_best.get('inferenceThroughput', 0.0))}。</li>"
    )
    conclusion.append(
        f"<li><strong>覆盖面：</strong> 当前 deck 覆盖 {profile_count} 个模型族，并在画廊里展示了 {gallery_count} 个代表性变体。</li>"
    )
    conclusion.append(
        "<li><strong>建议：</strong> 保留数据集编辑入口，继续增加少量少数类样本后再复测，以观察稳定均值是否还能再抬高。</li>"
    )
    conclusion.append(
        f"<li><strong>产物：</strong> 报告目录 {escape(report_dir.name)}，可视化已整理成 PPTX 风格 HTML，方便直接浏览或转述。</li>"
    )
    body = f"""
      <div class="grid grid-2">
        <div class="panel">
          <h3>结论</h3>
          <ul class="bullet-list">{''.join(conclusion)}</ul>
          <div class="callout">
            <div class="callout-title">一句话结论</div>
            <div class="callout-body">
              在更大的参数空间里，<strong>随机森林</strong> 仍然是最强的高精度候选；其它模型要么只在单次切分上偶尔冒尖，要么稳定性和上限都弱一些。现在我们还把 <strong>ALLOW 放行率</strong> 和推理速度一起放进了同一套横向比较里。
            </div>
          </div>
        </div>
        <div class="panel">
          <h3>下一步</h3>
          <ol class="bullet-list numbered">
            <li>把当前 HTML 放到前端 / docs 里作为演示页。</li>
            <li>继续扩大少数类样本，重点补充 ALERT / REWRITE 的边界样本。</li>
            <li>在保持稳定性前提下，再做更深一轮随机森林叶子深度/样本阈值微调。</li>
            <li>如果需要自动化，继续保留 sweep 脚本，后续一键重跑即可。</li>
          </ol>
          <div class="mini-table">
            {table(["文件", "用途"], [
                ["docs/ml-benchmark-presentation.html", "PPTX风格 HTML 演示"],
                ["docs/ml-benchmark-report.md", "文字版研究结论"],
                ["reports/ml-sweep-*/", "原始图表与 CSV 数据"],
            ], "compact")}
          </div>
        </div>
      </div>
    """
    subtitle = "完成扩大范围后的研究后，最终输出不仅是答案，还有可复用的展示和复测脚本。"
    return slide(14, "Summary", "最终结论与后续动作", subtitle, body, "linear-gradient(135deg, #0ea5e9, #0284c7)")


def build_html(report_dir: Path, best: dict[str, Any], rows: list[dict[str, Any]], stability: list[dict[str, Any]]) -> str:
    best["outDirPath"] = report_dir
    stability_best = best.get("stableBest", {})
    summaries = profile_range_summary(rows)
    gallery = select_gallery_variants(rows, per_profile=4)
    gallery_first = gallery[:14]
    gallery_second = gallery[14:27]
    gallery_third = gallery[27:]
    stats = {
        "datasetSize": best.get("datasetSize", 0),
        "profileCount": len(summaries),
        "comboCount": len(rows),
        "galleryCount": len(gallery),
    }

    cover = slide_cover(report_dir, best, stability_best, stats)
    scope = slide_scope(report_dir, rows, summaries, stats)
    literature = slide_literature()
    overall = slide_overall(rows, best)
    speed = slide_speed(rows, best)
    gallery_slide_1 = slide_gallery_one(gallery_first)
    gallery_slide_2 = slide_gallery_two(gallery_second)
    gallery_slide_3 = slide_gallery_three(gallery_third)
    stability_slide = slide_stability(stability, best)
    rf = slide_random_forest(rows, stability, best)
    tree_family = slide_tree_family(rows, best)
    linear_family = slide_linear_family(rows, best)
    baselines = slide_baselines(rows, best)
    conclusion = slide_conclusion(best, stability, report_dir, stats["galleryCount"], stats["profileCount"])

    toc_links = "".join(
        f'<a href="#slide-{i}" class="toc-chip">{i:02d} · {title}</a>'
        for i, title in [
            (1, "Cover"),
            (2, "Scope"),
            (3, "Literature"),
            (4, "Overall"),
            (5, "Speed"),
            (6, "Gallery I"),
            (7, "Gallery II"),
            (8, "Gallery III"),
            (9, "Stability"),
            (10, "Random Forest"),
            (11, "Tree Family"),
            (12, "Linear Family"),
            (13, "Baselines"),
            (14, "Conclusion"),
        ]
    )

    slides = "\n".join([
        cover,
        scope,
        literature,
        overall,
        speed,
        gallery_slide_1,
        gallery_slide_2,
        gallery_slide_3,
        stability_slide,
        rf,
        tree_family,
        linear_family,
        baselines,
        conclusion,
    ])

    css = """
    :root {
      --bg: #08111f;
      --bg2: #0f172a;
      --panel: rgba(15, 23, 42, 0.92);
      --panel-soft: rgba(30, 41, 59, 0.75);
      --line: rgba(148, 163, 184, 0.18);
      --text: #e5eefc;
      --muted: #9fb0c7;
      --accent: #60a5fa;
      --accent2: #34d399;
      --good: #22c55e;
      --warn: #f59e0b;
      --bad: #ef4444;
      --shadow: 0 30px 80px rgba(0, 0, 0, 0.35);
    }
    * { box-sizing: border-box; }
    html, body { height: 100%; }
    body {
      margin: 0;
      background:
        radial-gradient(circle at top left, rgba(96,165,250,0.16), transparent 38%),
        radial-gradient(circle at bottom right, rgba(167,139,250,0.12), transparent 40%),
        linear-gradient(180deg, var(--bg) 0%, var(--bg2) 100%);
      color: var(--text);
      font-family: "Microsoft YaHei", "Segoe UI", Arial, sans-serif;
      overflow-y: auto;
      scroll-snap-type: y mandatory;
    }
    a { color: inherit; text-decoration: none; }
    .deck-nav {
      position: fixed;
      top: 16px;
      right: 16px;
      z-index: 50;
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      max-width: min(520px, calc(100vw - 32px));
      justify-content: flex-end;
    }
    .toc-chip {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 8px 12px;
      border-radius: 999px;
      background: rgba(15, 23, 42, 0.72);
      border: 1px solid var(--line);
      color: #dbeafe;
      font-size: 12px;
      box-shadow: 0 10px 24px rgba(0,0,0,0.18);
      backdrop-filter: blur(12px);
    }
    .slide {
      min-height: 100vh;
      scroll-snap-align: start;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 28px;
    }
    .slide-shell {
      position: relative;
      width: min(1280px, calc(100vw - 56px));
      aspect-ratio: 16 / 9;
      border-radius: 28px;
      background: var(--panel);
      border: 1px solid rgba(255,255,255,0.08);
      box-shadow: var(--shadow);
      overflow: hidden;
    }
    .slide-shell::before {
      content: "";
      position: absolute;
      inset: 0;
      background: linear-gradient(135deg, color-mix(in srgb, var(--slide-accent) 18%, transparent) 0%, transparent 35%);
      pointer-events: none;
    }
    .slide-inner {
      position: relative;
      z-index: 1;
      height: 100%;
      padding: 32px 36px 28px;
      display: flex;
      flex-direction: column;
      gap: 16px;
    }
    .slide-badge {
      position: absolute;
      top: 18px;
      right: 20px;
      z-index: 2;
      width: 44px;
      height: 44px;
      display: grid;
      place-items: center;
      border-radius: 50%;
      background: linear-gradient(135deg, var(--slide-accent), rgba(255,255,255,0.1));
      color: #fff;
      font-weight: 700;
      box-shadow: 0 14px 28px rgba(0,0,0,0.28);
    }
    .slide-header h1 {
      margin: 4px 0 0;
      font-size: 34px;
      line-height: 1.08;
      letter-spacing: -0.02em;
    }
    .slide-header .eyebrow {
      color: #93c5fd;
      text-transform: uppercase;
      letter-spacing: 0.14em;
      font-size: 12px;
      font-weight: 700;
    }
    .subtitle {
      margin: 8px 0 0;
      color: var(--muted);
      font-size: 15px;
      line-height: 1.5;
      max-width: 1080px;
    }
    .grid { display: grid; gap: 16px; }
    .grid-2 { grid-template-columns: 1fr 1fr; }
    .grid-2-wide { grid-template-columns: 1.05fr 0.95fr; }
    .grid-2-tight { grid-template-columns: 1fr 1fr; }
    .stack { display: flex; flex-direction: column; gap: 14px; }
    .panel {
      border-radius: 20px;
      background: rgba(255,255,255,0.04);
      border: 1px solid var(--line);
      padding: 16px 18px;
      overflow: hidden;
    }
    .panel h3 {
      margin: 0 0 12px;
      font-size: 18px;
      line-height: 1.2;
      letter-spacing: -0.01em;
    }
    .chart-panel { display: flex; flex-direction: column; }
    .chart-panel .svg-card {
      flex: 1;
      min-height: 0;
      background: rgba(2, 6, 23, 0.30);
      border-radius: 16px;
      padding: 10px;
      border: 1px solid rgba(148,163,184,0.12);
      overflow: hidden;
    }
    .svg-card svg { width: 100%; height: auto; display: block; }
    .compact { font-size: 12px; }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: 12px;
      overflow: hidden;
    }
    th, td {
      padding: 10px 8px;
      border-bottom: 1px solid rgba(148,163,184,0.12);
      vertical-align: top;
      text-align: left;
    }
    th {
      color: #cbd5e1;
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      background: rgba(15, 23, 42, 0.48);
      position: sticky;
      top: 0;
      z-index: 1;
    }
    tbody tr:hover { background: rgba(255,255,255,0.03); }
    .bullet-list {
      margin: 0;
      padding-left: 18px;
      color: #dbe7f7;
      line-height: 1.55;
      font-size: 14px;
    }
    .bullet-list li { margin: 8px 0; }
    .bullet-list.numbered { list-style: decimal; }
    .cover-layout {
      display: grid;
      grid-template-columns: 1.15fr 0.85fr;
      gap: 18px;
      height: 100%;
    }
    .cover-copy {
      display: flex;
      flex-direction: column;
      justify-content: center;
      gap: 16px;
      padding-right: 16px;
    }
    .hero-pill {
      display: inline-flex;
      align-self: flex-start;
      padding: 8px 14px;
      border-radius: 999px;
      border: 1px solid rgba(255,255,255,0.15);
      background: rgba(255,255,255,0.06);
      color: #bfdbfe;
      font-size: 12px;
      letter-spacing: 0.06em;
    }
    .cover-copy h2 {
      margin: 0;
      font-size: 52px;
      line-height: 1.02;
      letter-spacing: -0.04em;
      max-width: 10ch;
    }
    .cover-text {
      margin: 0;
      color: var(--muted);
      font-size: 16px;
      line-height: 1.7;
      max-width: 70ch;
    }
    .cover-metrics {
      display: grid;
      grid-template-columns: repeat(3, minmax(0, 1fr));
      gap: 12px;
    }
    .stat-card {
      position: relative;
      border-radius: 18px;
      padding: 16px 16px 14px 16px;
      background: rgba(255,255,255,0.04);
      border: 1px solid var(--line);
      min-height: 102px;
      overflow: hidden;
    }
    .stat-accent {
      position: absolute;
      inset: 0 auto 0 0;
      width: 5px;
    }
    .stat-label {
      font-size: 12px;
      color: #cbd5e1;
      text-transform: uppercase;
      letter-spacing: 0.08em;
    }
    .stat-value {
      margin-top: 8px;
      font-size: 24px;
      font-weight: 700;
      line-height: 1.15;
    }
    .stat-note {
      margin-top: 6px;
      color: var(--muted);
      font-size: 12px;
      line-height: 1.4;
    }
    .cover-right {
      display: grid;
      gap: 14px;
      align-content: center;
    }
    .feature-card {
      border-radius: 22px;
      padding: 20px;
      background: linear-gradient(180deg, rgba(15,23,42,0.95), rgba(30,41,59,0.78));
      border: 1px solid rgba(255,255,255,0.10);
      box-shadow: 0 18px 42px rgba(0,0,0,0.20);
    }
    .feature-card-secondary {
      background: linear-gradient(180deg, rgba(17,24,39,0.86), rgba(15,23,42,0.66));
    }
    .feature-label {
      color: #cbd5e1;
      font-size: 12px;
      text-transform: uppercase;
      letter-spacing: 0.08em;
    }
    .feature-title {
      margin-top: 8px;
      font-size: 28px;
      font-weight: 700;
    }
    .feature-config {
      margin-top: 6px;
      color: var(--muted);
      font-size: 14px;
      line-height: 1.5;
    }
    .feature-grid {
      margin-top: 14px;
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 12px;
    }
    .feature-grid div {
      padding: 12px 14px;
      border-radius: 16px;
      background: rgba(255,255,255,0.05);
      border: 1px solid rgba(255,255,255,0.06);
    }
    .feature-grid span {
      display: block;
      color: var(--muted);
      font-size: 12px;
      margin-bottom: 6px;
    }
    .feature-grid strong {
      font-size: 15px;
      line-height: 1.35;
    }
    .metric-row {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 12px;
    }
    .callout {
      margin-top: 14px;
      padding: 16px 18px;
      border-radius: 18px;
      background: linear-gradient(135deg, rgba(96,165,250,0.12), rgba(139,92,246,0.10));
      border: 1px solid rgba(96,165,250,0.22);
    }
    .callout-title {
      color: #bfdbfe;
      font-size: 12px;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      margin-bottom: 6px;
    }
    .callout-body {
      font-size: 15px;
      line-height: 1.65;
      color: #eef2ff;
    }
    .gallery-header {
      display: flex;
      justify-content: space-between;
      align-items: baseline;
      gap: 12px;
      margin-bottom: 12px;
    }
    .gallery-summary {
      color: var(--muted);
      font-size: 12px;
      line-height: 1.4;
      max-width: 42ch;
      text-align: right;
    }
    .variant-grid {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 12px;
    }
    .variant-card {
      position: relative;
      border-radius: 18px;
      padding: 12px 12px 10px;
      background: linear-gradient(180deg, rgba(15,23,42,0.95), rgba(30,41,59,0.72));
      border: 1px solid rgba(255,255,255,0.08);
      overflow: hidden;
      min-height: 122px;
    }
    .variant-card::before {
      content: "";
      position: absolute;
      inset: 0 auto 0 0;
      width: 4px;
      background: var(--variant-accent);
    }
    .variant-top {
      display: flex;
      justify-content: space-between;
      gap: 10px;
      align-items: flex-start;
    }
    .variant-family {
      font-size: 12px;
      font-weight: 700;
      letter-spacing: 0.02em;
      color: #e2e8f0;
      line-height: 1.2;
    }
    .variant-score {
      font-size: 18px;
      font-weight: 800;
      color: #fff;
      line-height: 1;
      white-space: nowrap;
    }
    .variant-config {
      margin-top: 8px;
      color: #cbd5e1;
      font-size: 11px;
      line-height: 1.4;
      min-height: 2.8em;
    }
    .variant-meta {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 8px;
      margin-top: 10px;
    }
    .variant-meta div {
      border-radius: 12px;
      padding: 8px 10px;
      background: rgba(255,255,255,0.05);
      border: 1px solid rgba(255,255,255,0.06);
    }
    .variant-meta span {
      display: block;
      color: var(--muted);
      font-size: 10px;
      margin-bottom: 4px;
      text-transform: uppercase;
      letter-spacing: 0.06em;
    }
    .variant-meta strong {
      font-size: 12px;
      line-height: 1.2;
    }
    .variant-note {
      margin-top: 6px;
      color: var(--muted);
      font-size: 10px;
      line-height: 1.25;
      text-transform: uppercase;
      letter-spacing: 0.04em;
    }
    .mini { min-height: 0; }
    .mini-table { margin-top: 12px; }
    .slide-dense .slide-header h1 { font-size: 28px; }
    .slide-dense .subtitle { font-size: 13px; }
    .slide-dense .panel h3 { font-size: 16px; }
    .slide-dense .bullet-list { font-size: 12px; }
    .slide-dense .variant-grid { gap: 10px; }
    .slide-dense .variant-card { min-height: 118px; }
    @media (max-width: 1200px) {
      .variant-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
      .gallery-header { flex-direction: column; align-items: flex-start; }
      .gallery-summary { text-align: left; max-width: none; }
    }
    @media print {
      body { scroll-snap-type: none; background: #08111f; }
      .deck-nav { display: none; }
      .slide { page-break-after: always; min-height: auto; padding: 0; }
      .slide-shell { width: 100%; box-shadow: none; border-radius: 0; }
    }
    """

    html_doc = f"""<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>{escape(REPORT_TITLE)}</title>
  <style>{css}</style>
</head>
<body>
  <div class="deck-nav">{toc_links}</div>
  {slides}
</body>
</html>
"""
    return html_doc


def main() -> int:
    args = parse_args()
    base = Path.cwd()
    report_dir = args.report_dir or latest_report_dir(base / "reports")
    if not report_dir.exists():
        raise FileNotFoundError(report_dir)

    best = load_best(report_dir / "best.json")
    if not best:
        raise FileNotFoundError(f"missing best.json in {report_dir}")
    rows = load_results(report_dir / "results.csv")
    stability = load_stability(report_dir / "stability-summary.csv")

    html_out = build_html(report_dir, best, rows, stability)
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(html_out, encoding="utf-8")

    print(f"[ml-presentation] report_dir={report_dir}")
    print(f"[ml-presentation] wrote {args.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
