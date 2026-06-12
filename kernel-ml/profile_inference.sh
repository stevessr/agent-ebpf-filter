#!/bin/bash
# Profile kernel-ml inference with perf or Nsight Systems.
#
# Examples:
#   sudo ./profile_inference.sh perf 10000
#   ./profile_inference.sh nsight

set -euo pipefail

MODE="${1:-perf}"
COUNT="${2:-10000}"
PROC_PREDICT="${PROC_PREDICT:-/proc/ml_predict}"
HELPER="${HELPER:-./kernel_ml_cuda_helper}"

run_predict_writer() {
  python3 - "$COUNT" "$PROC_PREDICT" <<'PY'
import os
import struct
import sys

count = int(sys.argv[1])
path = sys.argv[2]
fv = struct.pack("<128qII16s", *([0] * 128), os.getpid(), 257, b"profile\0")
fd = os.open(path, os.O_WRONLY)
try:
    for _ in range(count):
        os.write(fd, fv)
finally:
    os.close(fd)
PY
}

case "$MODE" in
  perf)
    if [ ! -w "$PROC_PREDICT" ]; then
      echo "Need write access to $PROC_PREDICT. Load kernel_ml.ko and run with sudo if required." >&2
      exit 1
    fi
    if command -v perf >/dev/null 2>&1; then
      perf stat -e cycles,instructions,cache-misses,context-switches -- \
        bash -c "$(declare -f run_predict_writer); COUNT='$COUNT'; PROC_PREDICT='$PROC_PREDICT'; run_predict_writer"
    else
      echo "perf not found; running uninstrumented writer" >&2
      run_predict_writer
    fi
    ;;
  nsight|nsys)
    if [ ! -x "$HELPER" ]; then
      echo "CUDA helper not found/executable at $HELPER; run make cuda-helper first." >&2
      exit 1
    fi
    if command -v nsys >/dev/null 2>&1; then
      nsys profile -o kernel_ml_cuda_helper_selftest --force-overwrite=true "$HELPER" --self-test
    else
      echo "nsys not found; falling back to helper self-test" >&2
      "$HELPER" --self-test
    fi
    ;;
  *)
    echo "Usage: $0 perf [count] | nsight" >&2
    exit 2
    ;;
esac
