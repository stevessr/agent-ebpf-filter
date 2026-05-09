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



from render_ml_presentation_render import *

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
