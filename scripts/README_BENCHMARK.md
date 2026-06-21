# eBPF Performance Overhead Benchmark

This directory contains tools for measuring the performance overhead introduced by eBPF hooks in the agent-ebpf-filter system.

## Overview

The benchmark suite compares the execution time of common syscalls with and without eBPF tracepoint hooks attached. This helps quantify the runtime cost of observability.

## Tools

### 1. `ebpf-overhead-benchmark.sh`

Main orchestration script that:
- Builds the benchmark tool if needed
- Runs baseline tests (no eBPF)
- Loads eBPF programs and runs tests again
- Generates comparison reports with overhead percentages
- Restores original system state

**Requirements:**
- Must run as root (to load/unload eBPF programs)
- Python 3 (for report generation)
- Go toolchain (to build benchmark tool)

**Usage:**
```bash
sudo ./scripts/ebpf-overhead-benchmark.sh
```

**Output:**
- `reports/ebpf-overhead-<timestamp>/baseline.json` - Results without eBPF
- `reports/ebpf-overhead-<timestamp>/ebpf.json` - Results with eBPF
- `reports/ebpf-overhead-<timestamp>/summary.json` - Comparison summary

**Environment Variables:**
- `EBPF_BENCH_OUTDIR` - Override output directory

### 2. `benchmark-syscalls.go`

Go program that measures syscall execution times.

**Benchmarked Operations:**
- `getpid` - Get process ID
- `getppid` - Get parent process ID
- `gettid` - Get thread ID
- `open_close` - Open and close file descriptor
- `stat` - File stat syscall
- `read_write` - Read/write to pipe
- `socket_close` - Create and close socket
- `getcwd` - Get current working directory
- `getuid` - Get user ID
- `access` - File access check

**Flags:**
- `--output` (required) - Output JSON file path
- `--runs` (default: 5) - Number of runs per operation
- `--warmup` (default: 2) - Number of warmup runs
- `--iterations` (default: 10000) - Iterations per run

**Direct Usage:**
```bash
cd scripts
go build -o benchmark-syscalls ./benchmark-syscalls.go
./benchmark-syscalls --output results.json --runs 10 --iterations 50000
```

## Interpreting Results

### Summary Report Format

```
eBPF Performance Overhead Report
======================================================================
Operation            Baseline (μs)   eBPF (μs)       Overhead
----------------------------------------------------------------------
getpid              0.05            0.08            +60.00%
open_close          2.30            2.45            +6.52%
stat                1.80            1.92            +6.67%
...
----------------------------------------------------------------------
Average overhead: +15.23%
```

### What the Numbers Mean

- **Baseline (μs)**: Average time per operation without eBPF hooks
- **eBPF (μs)**: Average time per operation with eBPF hooks attached
- **Overhead**: Percentage increase in execution time

### Expected Overhead Ranges

Based on typical eBPF tracepoint overhead:

- **Very light syscalls** (getpid, getuid): 20-100% overhead
  - These are extremely fast (<0.1μs), so any overhead is proportionally large
  - Absolute overhead is still negligible (~0.05μs)

- **Light syscalls** (stat, access): 5-20% overhead
  - Moderate baseline cost (1-5μs)
  - eBPF overhead is more balanced

- **Heavy syscalls** (open, socket, read/write): 2-10% overhead
  - Higher baseline cost (>5μs)
  - eBPF overhead becomes proportionally smaller

- **Average across all operations**: 10-30% typical

### When to Be Concerned

- **>50% average overhead**: May indicate:
  - Heavy event processing in eBPF programs
  - Excessive map operations
  - Ring buffer contention
  - Need for optimization

- **Variance >20% between runs**: May indicate:
  - System load affecting results
  - CPU frequency scaling
  - Need for more warmup iterations

## Advanced Usage

### Custom Operation Subset

Modify `benchmark-syscalls.go` to focus on specific operations:

```go
operations := []struct {
    name string
    fn   func() error
}{
    {"open_close", benchOpenClose},
    {"stat", benchStat},
    // Add or remove operations as needed
}
```

### Higher Precision

For more precise measurements:

```bash
sudo ./scripts/ebpf-overhead-benchmark.sh
# Edit the script to increase iterations
# Change: --iterations 10000
# To: --iterations 100000
```

### Multiple Backend Configurations

Test different eBPF hook configurations:

```bash
# Test with minimal hooks
sudo ./scripts/ebpf-overhead-benchmark.sh

# Modify backend/ebpf/agent_tracker.c to add/remove hooks
make backend

# Test again
sudo ./scripts/ebpf-overhead-benchmark.sh
```

## Integration with CI/CD

### Regression Detection

```bash
#!/bin/bash
# Run benchmark and check for regressions

./scripts/ebpf-overhead-benchmark.sh

LATEST_REPORT=$(ls -t reports/ebpf-overhead-*/summary.json | head -1)

# Check average overhead
AVG_OVERHEAD=$(python3 -c "
import json
with open('$LATEST_REPORT') as f:
    data = json.load(f)
    overheads = [op['overhead_percent'] for op in data['operations']]
    print(sum(overheads) / len(overheads) if overheads else 0)
")

if (( $(echo "$AVG_OVERHEAD > 40" | bc -l) )); then
    echo "ERROR: Average overhead $AVG_OVERHEAD% exceeds threshold of 40%"
    exit 1
fi

echo "OK: Average overhead $AVG_OVERHEAD% is within acceptable range"
```

## Troubleshooting

### "This script must be run as root"

The script needs root privileges to load/unload eBPF programs. Run with `sudo`.

### "Command not found: benchmark-syscalls"

The benchmark tool will be built automatically. If this fails, manually build:

```bash
cd scripts
go build -o benchmark-syscalls ./benchmark-syscalls.go
```

### Inconsistent Results

- Ensure system is idle during benchmarking
- Disable CPU frequency scaling: `sudo cpupower frequency-set -g performance`
- Run with more iterations: edit script and increase `--iterations`
- Close unnecessary applications

### eBPF Programs Not Loading

- Check kernel support: `cat /sys/kernel/btf/vmlinux | head`
- Ensure bpffs is mounted: `mount | grep bpf`
- Check backend compiled: `ls -la backend/ebpf/*.o`

## Example Output

```
[benchmark] Starting eBPF overhead benchmark
[benchmark] Output directory: reports/ebpf-overhead-20260621-083045
[benchmark] Building benchmark tool...
[benchmark] Unloading eBPF programs...
[benchmark] eBPF programs unloaded
[benchmark] Running baseline benchmark (no eBPF)...
Running benchmark with 5 runs, 2 warmup, 10000 iterations per run
Benchmarking getpid...
Benchmarking getppid...
...
[benchmark] Baseline benchmark completed
[benchmark] Loading eBPF programs...
[benchmark] eBPF programs loaded
[benchmark] Running eBPF benchmark (with hooks)...
...
[benchmark] Generating comparison report...

======================================================================
eBPF Performance Overhead Report
======================================================================
Timestamp: 20260621-083045
Output Directory: reports/ebpf-overhead-20260621-083045
======================================================================

Operation            Baseline (μs)   eBPF (μs)       Overhead
----------------------------------------------------------------------
getpid              0.05            0.08            +60.00%
getppid             0.05            0.08            +60.00%
gettid              0.05            0.09            +80.00%
open_close          2.30            2.45            +6.52%
stat                1.80            1.92            +6.67%
read_write          3.50            3.71            +6.00%
socket_close        2.10            2.24            +6.67%
getcwd              0.80            0.88            +10.00%
getuid              0.05            0.08            +60.00%
access              1.60            1.70            +6.25%
----------------------------------------------------------------------

Total operations: 10
Runs per operation: 5
Warmup runs: 2

Average overhead: +26.21%

======================================================================
Summary saved to: reports/ebpf-overhead-20260621-083045/summary.json

[benchmark] Report generated
[benchmark] Benchmark completed successfully
[benchmark] Results available in: reports/ebpf-overhead-20260621-083045
```

## Contributing

To add new benchmark operations:

1. Add benchmark function to `benchmark-syscalls.go`:
   ```go
   func benchMyOperation() error {
       // Your syscall here
       return nil
   }
   ```

2. Add to operations list in `main()`:
   ```go
   {"my_operation", benchMyOperation},
   ```

3. Rebuild and run:
   ```bash
   sudo ./scripts/ebpf-overhead-benchmark.sh
   ```
