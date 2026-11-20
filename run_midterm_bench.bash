#!/usr/bin/env bash
set -uo pipefail

# Agent eBPF Filter — 中期审查 & 大挑复赛本地一键验证与数据采集脚本
# 用法:
#   chmod +x run_midterm_bench.sh
#   ./run_midterm_bench.sh              # 运行无特权测试、Replay 基准与环境/GPU探测
#   RUN_SUDO=1 ./run_midterm_bench.sh   # 额外运行需要 root/sudo 的 LSM/cgroup 与 syscall 开销基准

TS="$(date +%Y%m%d_%H%M%S)"
OUT_DIR="logs/midterm_${TS}"
REPORT="${OUT_DIR}/summary.md"
mkdir -p "${OUT_DIR}"

if [[ ! -f "Makefile" && ! -f "go.work" ]]; then
  echo "错误：请在 agent-ebpf-filter 项目根目录下运行此脚本。" >&2
  exit 1
fi

log_step() {
  echo ""
  echo "============================================================"
  echo "[*] $1"
  echo "============================================================"
}

run_cmd() {
  local name="$1"
  local cmd="$2"
  local logfile="${OUT_DIR}/${name}.log"
  echo "-> 执行: ${cmd}"
  if eval "${cmd}" >"${logfile}" 2>&1; then
    echo "   [PASS] 日志已保存至 ${logfile}"
    echo "| ${name} | PASS | \`${cmd}\` |" >> "${REPORT}.tmp"
  else
    local rc=$?
    echo "   [FAIL/SKIP rc=${rc}] 详见 ${logfile}"
    echo "| ${name} | FAIL (rc=${rc}) | \`${cmd}\` |" >> "${REPORT}.tmp"
  fi
}

has_make_target() {
  grep -qE "^$1:" Makefile 2>/dev/null
}

# 1. 收集系统与仓库元信息
log_step "1. 收集仓库与硬件/内核环境信息"
GIT_COMMITS="$(git rev-list --count HEAD 2>/dev/null || echo 'N/A')"
GIT_HEAD="$(git log -1 --format='%h (%cd) %s' --date=short 2>/dev/null || echo 'N/A')"
KERNEL_VER="$(uname -r)"
BTF_STATUS="未检测到"
[[ -f /sys/kernel/btf/vmlinux ]] && BTF_STATUS="已启用 (/sys/kernel/btf/vmlinux)"
GO_VER="$(go version 2>/dev/null || echo '未安装')"
GPU_INFO="$(nvidia-smi --query-gpu=name,driver_version,memory.total --format=csv,noheader 2>/dev/null || echo '未检测到 NVIDIA GPU / 驱动')"
CUDA_VER="$(nvcc --version 2>/dev/null | grep -oE 'release [0-9.]+' || echo '未检测到 nvcc')"

cat > "${REPORT}" <<EOF
# Agent eBPF Filter 中期审查实测报告 (${TS})

## 1. 环境与仓库基线
- **Git 提交总数**: ${GIT_COMMITS}
- **最新提交**: ${GIT_HEAD}
- **Linux 内核**: ${KERNEL_VER}
- **CO-RE BTF 支持**: ${BTF_STATUS}
- **Go 工具链**: ${GO_VER}
- **GPU 设备**: ${GPU_INFO}
- **CUDA 工具链**: ${CUDA_VER}

## 2. 自动化测试与基准结果
| 检查项 | 状态 | 命令 |
| :--- | :--- | :--- |
EOF

: > "${REPORT}.tmp"

# 2. 无特权静态检查与单元测试
log_step "2. 运行无特权静态检查与 Go 单元测试"
if has_make_target "os-enforcement-check"; then
  run_cmd "os_enforcement_check" "make os-enforcement-check"
fi
if [[ -f "go.work" ]]; then
  run_cmd "go_test_workspace" "go test ./backend/... ./wrapper/... 2>/dev/null || go test ./..."
else
  run_cmd "go_test_all" "go test ./..."
fi

# 3. Runtime Replay 基准测试
log_step "3. 运行 Runtime Replay 基准测试"
if has_make_target "runtime-benchmark"; then
  run_cmd "runtime_benchmark" "make runtime-benchmark"
elif [[ -d "benchmarks/runtime-replay" ]]; then
  run_cmd "runtime_replay_go" "go test -v ./benchmarks/runtime-replay/..."
fi

# 4. 可选模块探测 (kernel-ml / AstrLink / Magpie)
log_step "4. 探测 kernel-ml (GPGPU/CUDA) 与 Magpie/AstrLink 模块"
{
  echo "=== kernel-ml 目录 ==="
  ls -la kernel-ml 2>/dev/null || echo "无 kernel-ml 目录"
  echo "=== Magpie / AstrLink 相关文件 ==="
  find . -maxdepth 4 \( -iname "*magpie*" -o -iname "*astrlink*" \) 2>/dev/null || true
} > "${OUT_DIR}/modules_probe.log"
echo "   模块探测结果已保存至 ${OUT_DIR}/modules_probe.log"

if [[ -d "kernel-ml" && -f "kernel-ml/Makefile" ]]; then
  run_cmd "kernel_ml_check" "make -C kernel-ml check 2>/dev/null || make -C kernel-ml"
fi

# 5. 需要 root/sudo 的内核与开销基准 (仅当 RUN_SUDO=1 时执行)
if [[ "${RUN_SUDO:-0}" == "1" ]]; then
  log_step "5. 运行特权级 BPF LSM / cgroup 检查与 Syscall 开销基准"
  if has_make_target "os-enforcement-preflight"; then
    run_cmd "os_enforcement_preflight" "sudo make os-enforcement-preflight"
  fi
  if has_make_target "os-enforcement-smoke"; then
    run_cmd "os_enforcement_smoke" "sudo make os-enforcement-smoke"
  fi
  if [[ -x "scripts/ebpf-overhead-benchmark.sh" ]]; then
    run_cmd "ebpf_overhead_benchmark" "sudo bash scripts/ebpf-overhead-benchmark.sh"
  elif [[ -f "scripts/benchmark-syscalls.go" ]]; then
    run_cmd "syscall_microbenchmark" "go run scripts/benchmark-syscalls.go"
  fi
else
  echo ""
  echo "[提示] 已跳过特权级测试（如需运行 BPF LSM/cgroup 冒烟与 Syscall 开销基准，请使用: RUN_SUDO=1 ./run_midterm_bench.sh）"
fi

cat "${REPORT}.tmp" >> "${REPORT}"
rm -f "${REPORT}.tmp"

echo ""
echo "============================================================"
echo "完成！汇总报告已生成：${REPORT}"
echo "你可以直接把 ${REPORT} 和 ${OUT_DIR}/ 下的关键日志发给我，我帮你更新 PPT 中的最新实测数据。"
echo "============================================================"