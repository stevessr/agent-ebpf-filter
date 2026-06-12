#!/bin/bash
# Test script for kernel ML module

set -e

MODULE_PATH="./kernel_ml.ko"
PROC_LOAD="/proc/ml_load"
PROC_PREDICT="/proc/ml_predict"
PROC_STATS="/proc/ml_stats"
PROC_BACKEND="/proc/ml_backend"

echo "=== Kernel ML Module Test ==="
echo

# Check if module exists
if [ ! -f "$MODULE_PATH" ]; then
    echo "Error: Module not found at $MODULE_PATH"
    exit 1
fi

echo "1. Loading kernel module..."
sudo insmod "$MODULE_PATH"
sleep 1

echo "2. Checking dmesg for load message..."
dmesg | tail -3 | grep "kernel-ml"

echo
echo "3. Checking proc interfaces..."
ls -l /proc/ml_* 2>/dev/null || echo "No proc files found!"

echo
echo "4. Reading stats (before any inference)..."
cat "$PROC_STATS"

echo
echo "5. Checking backend control..."
cat "$PROC_BACKEND"
echo auto | sudo tee "$PROC_BACKEND" >/dev/null
grep -q '^backend=auto' "$PROC_BACKEND"
echo kernel | sudo tee "$PROC_BACKEND" >/dev/null
grep -q '^backend=kernel' "$PROC_BACKEND"

echo
echo "6. Module info:"
lsmod | grep kernel_ml

echo
echo "=== Test Complete ==="
echo "To unload: sudo rmmod kernel_ml"
echo "To load model: cat model.bin > /proc/ml_load"
echo "To predict: <write feature_vector binary to /proc/ml_predict>"
echo "To use CUDA: make cuda-helper && sudo ./kernel_ml_cuda_helper && echo cuda > /proc/ml_backend"
