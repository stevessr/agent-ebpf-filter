#!/bin/bash
# Comprehensive ML Report Generator
# Reads the sweep CSV outputs and generates markdown + HTML reports
set -euo pipefail

REPORT_DIR="${1:-}"
if [ -z "${REPORT_DIR}" ]; then
    # Find latest comprehensive report
    cd "$(dirname "$0")/.."
    REPORT_DIR=$(ls -td reports/ml-sweep-comprehensive* 2>/dev/null | head -1)
fi
if [ -z "${REPORT_DIR}" ] || [ ! -d "${REPORT_DIR}" ]; then
    echo "Usage: $0 <report-dir>"
    echo "No comprehensive sweep report found in reports/"
    exit 1
fi

cd "$(dirname "$0")/.."
REPORT_DIR=$(realpath "${REPORT_DIR}")
OUT="${REPORT_DIR}/full-report.md"

echo "Generating comprehensive report from ${REPORT_DIR}..."

cat > "${OUT}" << 'HEADER'
# ML 模型综合对比报告（完整版）

> 包含准确率、训练耗时、推理速度、内存占用

## 1. 各模型最佳配置对比

HEADER

# Parse results CSV for best configs
CSV="${REPORT_DIR}/results.csv"
if [ -f "${CSV}" ]; then
    echo "" >> "${OUT}"
    echo "| 模型 | 最佳参数 | 验证准确率 | 训练耗时 | 推理速度 | 内存占用 |" >> "${OUT}"
    echo "|:----|:---------|:----------:|:--------:|:--------:|:--------:|" >> "${OUT}"
    
    # Group by profile, find best validation accuracy for each
    tail -n +2 "${CSV}" 2>/dev/null | awk -F',' '{
        profile=$1; valAcc=$7; duration=$8; throughput=$12; memory=$13
        cfg=$5; trainAcc=$6
        if (valAcc > bestVal[profile]) {
            bestVal[profile]=valAcc
            bestCfg[profile]=cfg
            bestDur[profile]=duration
            bestTP[profile]=throughput
            bestMem[profile]=memory
        }
    } END {
        for (p in bestVal) {
            printf "%s|%s|%.2f%%|%ss|%s/s|%s bytes\n", p, bestCfg[p], bestVal[p]*100, bestDur[p], bestTP[p], bestMem[p]
        }
    }' | sort -t'|' -k3 -rn | while IFS='|' read -r m cfg acc dur tp mem; do
        printf "| %s | %s | %s | %s | %s | %s |\n" "${m}" "${cfg}" "${acc}" "${dur}" "${tp}" "${mem}"
    done
fi

# Memory analysis
echo "" >> "${OUT}"
echo "## 2. 内存占用分析" >> "${OUT}"
echo "" >> "${OUT}"
echo "| 模型 | 平均内存(MB) | 最小内存(MB) | 最大内存(MB) | 配置 |" >> "${OUT}"
echo "|:----|:------------:|:------------:|:------------:|:-----|" >> "${OUT}"

STABLE="${REPORT_DIR}/stability-summary.csv"
if [ -f "${STABLE}" ]; then
    tail -n +2 "${STABLE}" 2>/dev/null | awk -F',' '{
        printf "%s|%s|%s|%s|%s\n", $2, $24/1048576, $26/1048576, $27/1048576, $6
    }' | sort -t'|' -k2 -rn | while IFS='|' read -r m memMean memMin memMax cfg; do
        printf "| %s | %.2f | %.2f | %.2f | %s |\n" "${m}" "${memMean}" "${memMin}" "${memMax}" "${cfg}"
    done
    
    echo "" >> "${OUT}"
    echo "## 3. 稳定性分析（${ML_SWEEP_REPEATS:-3}次重复）" >> "${OUT}"
    echo "" >> "${OUT}"
    echo "| 模型 | 配置 | 平均准确率 | 标准差 | 推理速度(μs) | 内存(MB) |" >> "${OUT}"
    echo "|:----|:-----|:----------:|:-----:|:------------:|:--------:|" >> "${OUT}"
    
    tail -n +2 "${STABLE}" 2>/dev/null | awk -F',' '{
        # $6=config, $13=valMean, $14=valStd, $21=infLatMean, $24=memMean
        printf "%s|%s|%.2f%%|%.2f%%|%.3f|%.2f\n", $2, $6, $13*100, $14*100, $21*1000, $24/1048576
    }' | sort -t'|' -k3 -rn | while IFS='|' read -r m cfg acc std lat mem; do
        printf "| %s | %s | %s | %s | %s | %s |\n" "${m}" "${cfg}" "${acc}" "${std}" "${lat}" "${mem}"
    done
fi

echo "" >> "${OUT}"
echo "## 4. 全参数扫描数据" >> "${OUT}"
echo "" >> "${OUT}"
echo "完整CSV数据: ${CSV}" >> "${OUT}"
echo "稳定性数据: ${STABLE}" >> "${OUT}"
echo "HTML报告: ${REPORT_DIR}/index.html" >> "${OUT}"
echo "" >> "${OUT}"
echo "---" >> "${OUT}"
echo "*报告生成于 $(date '+%Y-%m-%d %H:%M:%S')*" >> "${OUT}"

echo "Report written to: ${OUT}"
echo ""
echo "Quick summary:"
grep "^| " "${OUT}" | head -20
