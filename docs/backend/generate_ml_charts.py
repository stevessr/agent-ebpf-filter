#!/usr/bin/env python3
"""
生成 ML 模型对比 SVG 图表
用于替换文档中的 ASCII 艺术图
"""

import matplotlib.pyplot as plt
import matplotlib.patches as mpatches
from matplotlib.patches import FancyBboxPatch
import numpy as np

# 设置中文字体
plt.rcParams['font.sans-serif'] = ['SimHei', 'DejaVu Sans', 'Arial Unicode MS']
plt.rcParams['axes.unicode_minus'] = False


def generate_accuracy_vs_speed():
    """生成准确率 vs 速度散点图"""
    fig, ax = plt.subplots(figsize=(10, 8))

    # 模型数据：(速度分数，准确率分数，名称，颜色)
    models = [
        (9, 2, 'Logistic\nRegression', '#f39c12'),
        (8, 3, 'SVM', '#2ecc71'),
        (5, 5, 'Random\nForest', '#3498db'),
        (3, 4.5, 'Ensemble', '#e74c3c'),
        (4, 5, 'Neural\nNetwork', '#e74c3c'),
    ]

    # 绘制散点
    for speed, accuracy, name, color in models:
        ax.scatter(speed, accuracy, s=500, c=color, alpha=0.7, edgecolors='white', linewidth=2, zorder=3)
        ax.annotate(name, (speed, accuracy), fontsize=11, ha='center', va='center',
                   weight='bold', color='white')

    # 设置坐标轴
    ax.set_xlim(0, 10)
    ax.set_ylim(0, 6)
    ax.set_xlabel('推理速度 →', fontsize=14, weight='bold')
    ax.set_ylabel('准确率 ↑', fontsize=14, weight='bold')
    ax.set_title('ML 模型：准确率 vs 速度权衡', fontsize=16, weight='bold', pad=20)

    # 添加网格
    ax.grid(True, alpha=0.3, linestyle='--')
    ax.set_axisbelow(True)

    # 添加注释
    ax.text(0.5, 5.5, '(慢)', fontsize=10, color='gray')
    ax.text(9.5, 5.5, '(快)', fontsize=10, color='gray')

    # 美化
    ax.spines['top'].set_visible(False)
    ax.spines['right'].set_visible(False)

    plt.tight_layout()
    plt.savefig('ml-accuracy-vs-speed.svg', dpi=150, bbox_inches='tight')
    print("✓ 生成：ml-accuracy-vs-speed.svg")


def generate_latency_bars():
    """生成推理延迟条形图"""
    fig, ax = plt.subplots(figsize=(10, 6))

    models = ['Logistic\nRegression', 'SVM', 'Neural\nNetwork', 'Random\nForest']
    latencies = [1, 2, 5, 10]
    colors = ['#f39c12', '#2ecc71', '#e74c3c', '#3498db']

    bars = ax.barh(models, latencies, color=colors, alpha=0.8, edgecolor='white', linewidth=2)

    # 添加数值标签
    for i, (bar, latency) in enumerate(zip(bars, latencies)):
        ax.text(latency + 0.3, i, f'{latency} μs', va='center', fontsize=11, weight='bold')

    ax.set_xlabel('推理延迟 (微秒)', fontsize=14, weight='bold')
    ax.set_title('内核态模型推理延迟对比', fontsize=16, weight='bold', pad=20)
    ax.set_xlim(0, 12)

    # 美化
    ax.spines['top'].set_visible(False)
    ax.spines['right'].set_visible(False)
    ax.grid(axis='x', alpha=0.3, linestyle='--')
    ax.set_axisbelow(True)

    plt.tight_layout()
    plt.savefig('ml-latency-comparison.svg', dpi=150, bbox_inches='tight')
    print("✓ 生成：ml-latency-comparison.svg")


def generate_memory_bars():
    """生成内存占用条形图"""
    fig, ax = plt.subplots(figsize=(10, 6))

    models = ['Logistic\nRegression', 'SVM', 'Neural\nNetwork', 'Random\nForest']
    memory = [1, 1, 16, 50]
    colors = ['#f39c12', '#2ecc71', '#e74c3c', '#3498db']

    bars = ax.barh(models, memory, color=colors, alpha=0.8, edgecolor='white', linewidth=2)

    # 添加数值标签
    for i, (bar, mem) in enumerate(zip(bars, memory)):
        ax.text(mem + 1.5, i, f'{mem} KB', va='center', fontsize=11, weight='bold')

    ax.set_xlabel('内存占用 (KB)', fontsize=14, weight='bold')
    ax.set_title('内核态模型内存占用对比', fontsize=16, weight='bold', pad=20)
    ax.set_xlim(0, 60)

    # 美化
    ax.spines['top'].set_visible(False)
    ax.spines['right'].set_visible(False)
    ax.grid(axis='x', alpha=0.3, linestyle='--')
    ax.set_axisbelow(True)

    plt.tight_layout()
    plt.savefig('ml-memory-comparison.svg', dpi=150, bbox_inches='tight')
    print("✓ 生成：ml-memory-comparison.svg")


def generate_model_comparison_radar():
    """生成模型综合对比雷达图"""
    fig, ax = plt.subplots(figsize=(10, 10), subplot_kw=dict(projection='polar'))

    # 5 个维度
    categories = ['推理速度', '内存效率', '准确率', '可解释性', '训练速度']
    N = len(categories)

    # 模型数据 (归一化到 0-5)
    models_data = {
        'Random Forest': [3, 2, 4, 4, 3],
        'SVM': [4.5, 5, 3, 3.5, 3.5],
        'Logistic Regression': [5, 5, 2, 4.5, 4.5],
        'Neural Network': [4, 3, 5, 1.5, 2],
    }

    colors = {
        'Random Forest': '#3498db',
        'SVM': '#2ecc71',
        'Logistic Regression': '#f39c12',
        'Neural Network': '#e74c3c',
    }

    # 计算角度
    angles = [n / float(N) * 2 * np.pi for n in range(N)]
    angles += angles[:1]

    # 绘制每个模型
    for model, values in models_data.items():
        values += values[:1]
        ax.plot(angles, values, 'o-', linewidth=2, label=model, color=colors[model])
        ax.fill(angles, values, alpha=0.15, color=colors[model])

    # 设置标签
    ax.set_xticks(angles[:-1])
    ax.set_xticklabels(categories, fontsize=12)
    ax.set_ylim(0, 5)
    ax.set_yticks([1, 2, 3, 4, 5])
    ax.set_yticklabels(['1', '2', '3', '4', '5'], fontsize=10, color='gray')
    ax.grid(True, alpha=0.3)

    # 标题和图例
    ax.set_title('ML 模型综合性能对比', fontsize=16, weight='bold', pad=30)
    ax.legend(loc='upper right', bbox_to_anchor=(1.3, 1.1), fontsize=11)

    plt.tight_layout()
    plt.savefig('ml-radar-comparison.svg', dpi=150, bbox_inches='tight')
    print("✓ 生成：ml-radar-comparison.svg")


def generate_performance_heatmap():
    """生成性能指标热力图"""
    fig, ax = plt.subplots(figsize=(12, 6))

    models = ['Logistic\nRegression', 'SVM', 'Neural\nNetwork', 'Random\nForest']
    metrics = ['推理延迟', '内存占用', '准确率', '可解释性', '训练速度']

    # 数据矩阵 (5=最好，1=最差)
    data = np.array([
        [5, 5, 2, 4, 4],  # Logistic
        [4, 5, 3, 3, 3],  # SVM
        [3, 3, 5, 1, 2],  # NN
        [2, 1, 4, 4, 3],  # RF
    ])

    im = ax.imshow(data, cmap='RdYlGn', aspect='auto', vmin=1, vmax=5)

    # 设置刻度
    ax.set_xticks(np.arange(len(metrics)))
    ax.set_yticks(np.arange(len(models)))
    ax.set_xticklabels(metrics, fontsize=12)
    ax.set_yticklabels(models, fontsize=12)

    # 旋转 x 轴标签
    plt.setp(ax.get_xticklabels(), rotation=45, ha="right", rotation_mode="anchor")

    # 添加数值
    for i in range(len(models)):
        for j in range(len(metrics)):
            text = ax.text(j, i, f'{data[i, j]:.0f}',
                          ha="center", va="center", color="black", fontsize=12, weight='bold')

    # 添加颜色条
    cbar = plt.colorbar(im, ax=ax)
    cbar.set_label('评分 (5=最好)', rotation=270, labelpad=20, fontsize=12)

    ax.set_title('ML 模型性能矩阵热力图', fontsize=16, weight='bold', pad=20)

    plt.tight_layout()
    plt.savefig('ml-performance-heatmap.svg', dpi=150, bbox_inches='tight')
    print("✓ 生成：ml-performance-heatmap.svg")


def generate_model_family_tree():
    """生成模型家族树状图"""
    fig, ax = plt.subplots(figsize=(14, 10))

    # 隐藏坐标轴
    ax.set_xlim(0, 100)
    ax.set_ylim(0, 100)
    ax.axis('off')

    # 根节点
    root_box = FancyBboxPatch((35, 85), 30, 8, boxstyle="round,pad=0.5",
                              edgecolor='#34495e', facecolor='#ecf0f1', linewidth=2)
    ax.add_patch(root_box)
    ax.text(50, 89, 'ML 模型 (51 种)', ha='center', va='center', fontsize=14, weight='bold')

    # 第一层：内核态 vs 用户态
    kernel_box = FancyBboxPatch((5, 70), 20, 6, boxstyle="round,pad=0.3",
                               edgecolor='#e74c3c', facecolor='#fadbd8', linewidth=2)
    ax.add_patch(kernel_box)
    ax.text(15, 73, '内核态 (4)', ha='center', va='center', fontsize=11, weight='bold')

    user_box = FancyBboxPatch((75, 70), 20, 6, boxstyle="round,pad=0.3",
                             edgecolor='#3498db', facecolor='#d6eaf8', linewidth=2)
    ax.add_patch(user_box)
    ax.text(85, 73, '用户态 (47)', ha='center', va='center', fontsize=11, weight='bold')

    # 连线
    ax.plot([50, 15], [85, 76], 'k-', linewidth=1.5, alpha=0.6)
    ax.plot([50, 85], [85, 76], 'k-', linewidth=1.5, alpha=0.6)

    # 内核态模型
    kernel_models = [
        ('RF', 55, '#3498db'),
        ('SVM', 60, '#2ecc71'),
        ('LR', 65, '#f39c12'),
        ('NN', 70, '#e74c3c'),
    ]

    for name, y, color in kernel_models:
        box = FancyBboxPatch((3, y-2), 8, 3, boxstyle="round,pad=0.2",
                            edgecolor=color, facecolor='white', linewidth=1.5)
        ax.add_patch(box)
        ax.text(7, y-0.5, name, ha='center', va='center', fontsize=9)
        ax.plot([15, 7], [70, y-2], 'k-', linewidth=1, alpha=0.4)

    # 用户态家族
    user_families = [
        ('树模型\n(18)', 60, '#2ecc71'),
        ('线性\n(12)', 55, '#9b59b6'),
        ('在线\n(6)', 50, '#e67e22'),
        ('近邻\n(8)', 45, '#1abc9c'),
        ('其他\n(3)', 40, '#34495e'),
    ]

    for name, y, color in user_families:
        box = FancyBboxPatch((75, y-2), 12, 4, boxstyle="round,pad=0.2",
                            edgecolor=color, facecolor='white', linewidth=1.5)
        ax.add_patch(box)
        ax.text(81, y, name, ha='center', va='center', fontsize=9, weight='bold')
        ax.plot([85, 81], [70, y+2], 'k-', linewidth=1, alpha=0.4)

    # 添加标题
    ax.text(50, 97, 'ML 模型家族树', ha='center', fontsize=18, weight='bold')

    plt.tight_layout()
    plt.savefig('ml-family-tree.svg', dpi=150, bbox_inches='tight')
    print("✓ 生成：ml-family-tree.svg")


if __name__ == '__main__':
    print("开始生成 ML 模型对比图表...\n")

    generate_accuracy_vs_speed()
    generate_latency_bars()
    generate_memory_bars()
    generate_model_comparison_radar()
    generate_performance_heatmap()
    generate_model_family_tree()

    print("\n✅ 所有图表生成完成！")
    print("\n生成的 SVG 文件：")
    print("  - ml-accuracy-vs-speed.svg       (准确率 vs 速度)")
    print("  - ml-latency-comparison.svg      (推理延迟对比)")
    print("  - ml-memory-comparison.svg       (内存占用对比)")
    print("  - ml-radar-comparison.svg        (雷达图)")
    print("  - ml-performance-heatmap.svg     (性能热力图)")
    print("  - ml-family-tree.svg             (模型家族树)")
