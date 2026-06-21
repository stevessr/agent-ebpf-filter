#!/usr/bin/env python3
"""
生成 ML 模型对比 SVG 图表（纯 SVG，无需 matplotlib）
"""

def generate_accuracy_vs_speed_svg():
    """生成准确率 vs 速度散点图"""
    svg = '''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 600">
  <defs>
    <style>
      .axis { stroke: #333; stroke-width: 2; }
      .grid { stroke: #ddd; stroke-width: 1; stroke-dasharray: 5,5; }
      .label { font-family: Arial, sans-serif; font-size: 14px; fill: #333; }
      .title { font-family: Arial, sans-serif; font-size: 20px; font-weight: bold; fill: #333; }
      .model-text { font-family: Arial, sans-serif; font-size: 12px; font-weight: bold; fill: white; text-anchor: middle; }
    </style>
  </defs>

  <!-- 标题 -->
  <text x="400" y="30" class="title" text-anchor="middle">ML 模型：准确率 vs 速度权衡</text>

  <!-- 网格 -->
  <line x1="100" y1="100" x2="100" y2="500" class="axis"/>
  <line x1="100" y1="500" x2="700" y2="500" class="axis"/>

  <line x1="100" y1="200" x2="700" y2="200" class="grid"/>
  <line x1="100" y1="300" x2="700" y2="300" class="grid"/>
  <line x1="100" y1="400" x2="700" y2="400" class="grid"/>

  <line x1="250" y1="100" x2="250" y2="500" class="grid"/>
  <line x1="400" y1="100" x2="400" y2="500" class="grid"/>
  <line x1="550" y1="100" x2="550" y2="500" class="grid"/>

  <!-- 坐标轴标签 -->
  <text x="400" y="540" class="label" text-anchor="middle">推理速度 →</text>
  <text x="50" y="300" class="label" text-anchor="middle" transform="rotate(-90 50 300)">准确率 ↑</text>
  <text x="120" y="520" class="label" style="fill: #999;">(慢)</text>
  <text x="680" y="520" class="label" style="fill: #999;">(快)</text>

  <!-- Logistic Regression: 速度 9, 准确率 2 -->
  <circle cx="640" cy="420" r="40" fill="#f39c12" opacity="0.8" stroke="white" stroke-width="3"/>
  <text x="640" y="420" class="model-text" dy="0.3em">Logistic</text>
  <text x="640" y="435" class="model-text" dy="0.3em">Regression</text>

  <!-- SVM: 速度 8, 准确率 3 -->
  <circle cx="580" cy="340" r="40" fill="#2ecc71" opacity="0.8" stroke="white" stroke-width="3"/>
  <text x="580" y="345" class="model-text" dy="0.3em">SVM</text>

  <!-- Random Forest: 速度 5, 准确率 5 -->
  <circle cx="400" cy="180" r="40" fill="#3498db" opacity="0.8" stroke="white" stroke-width="3"/>
  <text x="400" y="175" class="model-text" dy="0.3em">Random</text>
  <text x="400" y="190" class="model-text" dy="0.3em">Forest</text>

  <!-- Neural Network: 速度 4, 准确率 5 -->
  <circle cx="340" cy="180" r="40" fill="#e74c3c" opacity="0.8" stroke="white" stroke-width="3"/>
  <text x="340" y="175" class="model-text" dy="0.3em">Neural</text>
  <text x="340" y="190" class="model-text" dy="0.3em">Network</text>

  <!-- Ensemble: 速度 3, 准确率 4.5 -->
  <circle cx="280" cy="196" r="40" fill="#9b59b6" opacity="0.8" stroke="white" stroke-width="3"/>
  <text x="280" y="201" class="model-text" dy="0.3em">Ensemble</text>
</svg>'''

    with open('ml-accuracy-vs-speed.svg', 'w', encoding='utf-8') as f:
        f.write(svg)
    print("✓ 生成：ml-accuracy-vs-speed.svg")


def generate_latency_bars_svg():
    """生成推理延迟条形图"""
    svg = '''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 500">
  <defs>
    <style>
      .label { font-family: Arial, sans-serif; font-size: 14px; fill: #333; }
      .title { font-family: Arial, sans-serif; font-size: 20px; font-weight: bold; fill: #333; }
      .value { font-family: Arial, sans-serif; font-size: 14px; font-weight: bold; fill: #333; }
    </style>
  </defs>

  <text x="400" y="30" class="title" text-anchor="middle">内核态模型推理延迟对比</text>

  <!-- Logistic Regression: 1 μs -->
  <rect x="250" y="80" width="50" height="40" fill="#f39c12" opacity="0.8" stroke="white" stroke-width="2" rx="4"/>
  <text x="150" y="105" class="label">Logistic Regression</text>
  <text x="310" y="105" class="value">1 μs</text>

  <!-- SVM: 2 μs -->
  <rect x="250" y="150" width="100" height="40" fill="#2ecc71" opacity="0.8" stroke="white" stroke-width="2" rx="4"/>
  <text x="150" y="175" class="label">SVM</text>
  <text x="360" y="175" class="value">2 μs</text>

  <!-- Neural Network: 5 μs -->
  <rect x="250" y="220" width="250" height="40" fill="#e74c3c" opacity="0.8" stroke="white" stroke-width="2" rx="4"/>
  <text x="150" y="245" class="label">Neural Network</text>
  <text x="510" y="245" class="value">5 μs</text>

  <!-- Random Forest: 10 μs -->
  <rect x="250" y="290" width="500" height="40" fill="#3498db" opacity="0.8" stroke="white" stroke-width="2" rx="4"/>
  <text x="150" y="315" class="label">Random Forest</text>
  <text x="760" y="315" class="value">10 μs</text>

  <!-- 坐标轴 -->
  <line x1="250" y1="360" x2="750" y2="360" stroke="#333" stroke-width="2"/>
  <text x="500" y="390" class="label" text-anchor="middle">推理延迟 (微秒)</text>

  <!-- 刻度 -->
  <line x1="250" y1="355" x2="250" y2="365" stroke="#333" stroke-width="2"/>
  <text x="250" y="380" class="label" text-anchor="middle">0</text>

  <line x1="500" y1="355" x2="500" y2="365" stroke="#333" stroke-width="2"/>
  <text x="500" y="380" class="label" text-anchor="middle">5</text>

  <line x1="750" y1="355" x2="750" y2="365" stroke="#333" stroke-width="2"/>
  <text x="750" y="380" class="label" text-anchor="middle">10</text>
</svg>'''

    with open('ml-latency-comparison.svg', 'w', encoding='utf-8') as f:
        f.write(svg)
    print("✓ 生成：ml-latency-comparison.svg")


def generate_memory_bars_svg():
    """生成内存占用条形图"""
    svg = '''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 500">
  <defs>
    <style>
      .label { font-family: Arial, sans-serif; font-size: 14px; fill: #333; }
      .title { font-family: Arial, sans-serif; font-size: 20px; font-weight: bold; fill: #333; }
      .value { font-family: Arial, sans-serif; font-size: 14px; font-weight: bold; fill: #333; }
    </style>
  </defs>

  <text x="400" y="30" class="title" text-anchor="middle">内核态模型内存占用对比</text>

  <!-- Logistic Regression: 1 KB -->
  <rect x="250" y="80" width="10" height="40" fill="#f39c12" opacity="0.8" stroke="white" stroke-width="2" rx="2"/>
  <text x="150" y="105" class="label">Logistic Regression</text>
  <text x="270" y="105" class="value">1 KB</text>

  <!-- SVM: 1 KB -->
  <rect x="250" y="150" width="10" height="40" fill="#2ecc71" opacity="0.8" stroke="white" stroke-width="2" rx="2"/>
  <text x="150" y="175" class="label">SVM</text>
  <text x="270" y="175" class="value">1 KB</text>

  <!-- Neural Network: 16 KB -->
  <rect x="250" y="220" width="160" height="40" fill="#e74c3c" opacity="0.8" stroke="white" stroke-width="2" rx="2"/>
  <text x="150" y="245" class="label">Neural Network</text>
  <text x="420" y="245" class="value">16 KB</text>

  <!-- Random Forest: 50 KB -->
  <rect x="250" y="290" width="500" height="40" fill="#3498db" opacity="0.8" stroke="white" stroke-width="2" rx="2"/>
  <text x="150" y="315" class="label">Random Forest</text>
  <text x="760" y="315" class="value">50 KB</text>

  <!-- 坐标轴 -->
  <line x1="250" y1="360" x2="750" y2="360" stroke="#333" stroke-width="2"/>
  <text x="500" y="390" class="label" text-anchor="middle">内存占用 (KB)</text>

  <!-- 刻度 -->
  <line x1="250" y1="355" x2="250" y2="365" stroke="#333" stroke-width="2"/>
  <text x="250" y="380" class="label" text-anchor="middle">0</text>

  <line x1="500" y1="355" x2="500" y2="365" stroke="#333" stroke-width="2"/>
  <text x="500" y="380" class="label" text-anchor="middle">25</text>

  <line x1="750" y1="355" x2="750" y2="365" stroke="#333" stroke-width="2"/>
  <text x="750" y="380" class="label" text-anchor="middle">50</text>
</svg>'''

    with open('ml-memory-comparison.svg', 'w', encoding='utf-8') as f:
        f.write(svg)
    print("✓ 生成：ml-memory-comparison.svg")


def generate_performance_summary_svg():
    """生成性能总览对比图"""
    svg = '''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1000 600">
  <defs>
    <style>
      .label { font-family: Arial, sans-serif; font-size: 13px; fill: #333; }
      .title { font-family: Arial, sans-serif; font-size: 22px; font-weight: bold; fill: #333; }
      .model-name { font-family: Arial, sans-serif; font-size: 14px; font-weight: bold; fill: #333; }
      .metric { font-family: Arial, sans-serif; font-size: 12px; fill: #666; }
      .star { fill: #f39c12; }
    </style>
  </defs>

  <text x="500" y="35" class="title" text-anchor="middle">ML 模型性能总览对比</text>

  <!-- 表头 -->
  <rect x="50" y="60" width="900" height="40" fill="#ecf0f1" rx="4"/>
  <text x="150" y="85" class="label" text-anchor="middle" font-weight="bold">模型</text>
  <text x="300" y="85" class="label" text-anchor="middle" font-weight="bold">推理延迟</text>
  <text x="450" y="85" class="label" text-anchor="middle" font-weight="bold">内存占用</text>
  <text x="600" y="85" class="label" text-anchor="middle" font-weight="bold">准确率</text>
  <text x="750" y="85" class="label" text-anchor="middle" font-weight="bold">适用场景</text>

  <!-- Logistic Regression -->
  <rect x="50" y="110" width="900" height="80" fill="#fff9e6" rx="4"/>
  <circle cx="100" cy="150" r="25" fill="#f39c12" opacity="0.8"/>
  <text x="150" y="155" class="model-name">Logistic Regression</text>
  <text x="300" y="145" class="metric" text-anchor="middle">~1 μs</text>
  <text x="300" y="165" class="label" text-anchor="middle" style="fill:#2ecc71; font-weight:bold;">极速</text>
  <text x="450" y="155" class="metric" text-anchor="middle">1 KB</text>
  <text x="600" y="155" class="metric" text-anchor="middle">★★☆☆☆</text>
  <text x="750" y="145" class="label" text-anchor="middle">极速响应</text>
  <text x="750" y="165" class="label" text-anchor="middle">实时防御</text>

  <!-- SVM -->
  <rect x="50" y="200" width="900" height="80" fill="#e8f8f5" rx="4"/>
  <circle cx="100" cy="240" r="25" fill="#2ecc71" opacity="0.8"/>
  <text x="150" y="245" class="model-name">SVM</text>
  <text x="300" y="235" class="metric" text-anchor="middle">~2 μs</text>
  <text x="300" y="255" class="label" text-anchor="middle" style="fill:#2ecc71; font-weight:bold;">快速</text>
  <text x="450" y="245" class="metric" text-anchor="middle">1 KB</text>
  <text x="600" y="245" class="metric" text-anchor="middle">★★★☆☆</text>
  <text x="750" y="235" class="label" text-anchor="middle">低延迟</text>
  <text x="750" y="255" class="label" text-anchor="middle">边界清晰</text>

  <!-- Neural Network -->
  <rect x="50" y="290" width="900" height="80" fill="#fadbd8" rx="4"/>
  <circle cx="100" cy="330" r="25" fill="#e74c3c" opacity="0.8"/>
  <text x="150" y="335" class="model-name">Neural Network</text>
  <text x="300" y="335" class="metric" text-anchor="middle">~5 μs</text>
  <text x="450" y="335" class="metric" text-anchor="middle">16 KB</text>
  <text x="600" y="335" class="metric" text-anchor="middle">★★★★★</text>
  <text x="750" y="325" class="label" text-anchor="middle">高准确率</text>
  <text x="750" y="345" class="label" text-anchor="middle">复杂模式</text>

  <!-- Random Forest -->
  <rect x="50" y="380" width="900" height="80" fill="#d6eaf8" rx="4"/>
  <circle cx="100" cy="420" r="25" fill="#3498db" opacity="0.8"/>
  <text x="150" y="425" class="model-name">Random Forest</text>
  <text x="300" y="425" class="metric" text-anchor="middle">~10 μs</text>
  <text x="450" y="425" class="metric" text-anchor="middle">50 KB</text>
  <text x="600" y="425" class="metric" text-anchor="middle">★★★★☆</text>
  <text x="750" y="415" class="label" text-anchor="middle">生产默认</text>
  <text x="750" y="435" class="label" text-anchor="middle">稳健通用</text>

  <!-- 图例 -->
  <text x="100" y="520" class="label" style="font-weight:bold;">图例：</text>
  <text x="100" y="545" class="metric">★ = 准确率评级 (5 星最高)</text>
  <circle cx="350" cy="535" r="8" fill="#2ecc71" opacity="0.8"/>
  <text x="365" y="540" class="metric">= 速度优势</text>
  <circle cx="550" cy="535" r="8" fill="#e74c3c" opacity="0.8"/>
  <text x="565" y="540" class="metric">= 准确率优势</text>
</svg>'''

    with open('ml-performance-summary.svg', 'w', encoding='utf-8') as f:
        f.write(svg)
    print("✓ 生成：ml-performance-summary.svg")


if __name__ == '__main__':
    print("开始生成 ML 模型对比图表（纯 SVG）...\n")

    generate_accuracy_vs_speed_svg()
    generate_latency_bars_svg()
    generate_memory_bars_svg()
    generate_performance_summary_svg()

    print("\n✅ 所有图表生成完成！")
    print("\n生成的 SVG 文件：")
    print("  - ml-accuracy-vs-speed.svg       (准确率 vs 速度散点图)")
    print("  - ml-latency-comparison.svg      (推理延迟条形图)")
    print("  - ml-memory-comparison.svg       (内存占用条形图)")
    print("  - ml-performance-summary.svg     (性能总览对比表)")
