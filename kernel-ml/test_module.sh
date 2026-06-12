#!/bin/bash
# Test script for kernel ML module

set -e

MODULE_PATH="./kernel_ml.ko"
PROC_LOAD="/proc/ml_load"
PROC_PREDICT="/proc/ml_predict"
PROC_STATS="/proc/ml_stats"

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
echo "5. Module info:"
lsmod | grep kernel_ml

echo
echo "=== Test Complete ==="
echo "To unload: sudo rmmod kernel_ml"
echo "To load model: cat model.bin > /proc/ml_load"
echo "To predict: <write feature_vector binary to /proc/ml_predict>"
