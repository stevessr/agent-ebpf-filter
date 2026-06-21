package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"syscall"
	"time"
)

// BenchmarkResult stores timing results for a single operation
type BenchmarkResult struct {
	Name       string  `json:"name"`
	AvgTimeUs  float64 `json:"avg_time_us"`
	MinTimeUs  float64 `json:"min_time_us"`
	MaxTimeUs  float64 `json:"max_time_us"`
	StdDevUs   float64 `json:"stddev_us"`
	Iterations int     `json:"iterations"`
}

// BenchmarkReport contains all benchmark results
type BenchmarkReport struct {
	Timestamp  string            `json:"timestamp"`
	Runs       int               `json:"runs"`
	Warmup     int               `json:"warmup"`
	Operations []BenchmarkResult `json:"operations"`
}

var (
	outputFile = flag.String("output", "", "Output JSON file")
	runs       = flag.Int("runs", 5, "Number of runs per operation")
	warmup     = flag.Int("warmup", 2, "Number of warmup runs")
	iterations = flag.Int("iterations", 10000, "Iterations per run")
)

func main() {
	flag.Parse()

	if *outputFile == "" {
		fmt.Fprintf(os.Stderr, "Error: --output flag is required\n")
		os.Exit(1)
	}

	fmt.Printf("Running benchmark with %d runs, %d warmup, %d iterations per run\n", *runs, *warmup, *iterations)

	report := BenchmarkReport{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Runs:       *runs,
		Warmup:     *warmup,
		Operations: make([]BenchmarkResult, 0),
	}

	// Benchmark various syscalls that eBPF hooks might intercept
	operations := []struct {
		name string
		fn   func() error
	}{
		{"getpid", benchGetPid},
		{"getppid", benchGetPPid},
		{"gettid", benchGetTid},
		{"open_close", benchOpenClose},
		{"stat", benchStat},
		{"read_write", benchReadWrite},
		{"socket_close", benchSocketClose},
		{"getcwd", benchGetcwd},
		{"getuid", benchGetUid},
		{"access", benchAccess},
	}

	for _, op := range operations {
		fmt.Printf("Benchmarking %s...\n", op.name)
		result := runBenchmark(op.name, op.fn, *runs, *warmup, *iterations)
		report.Operations = append(report.Operations, result)
	}

	// Write results
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling results: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*outputFile, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Results written to %s\n", *outputFile)
}

func runBenchmark(name string, fn func() error, runs, warmup, iterations int) BenchmarkResult {
	// Warmup
	for i := 0; i < warmup; i++ {
		for j := 0; j < iterations; j++ {
			_ = fn()
		}
	}

	// Actual runs
	times := make([]float64, runs)
	for i := 0; i < runs; i++ {
		start := time.Now()
		for j := 0; j < iterations; j++ {
			if err := fn(); err != nil {
				// Ignore errors for benchmarking purposes
			}
		}
		elapsed := time.Since(start)
		times[i] = float64(elapsed.Microseconds()) / float64(iterations)
	}

	// Calculate statistics
	avg, min, max, stddev := calculateStats(times)

	return BenchmarkResult{
		Name:       name,
		AvgTimeUs:  avg,
		MinTimeUs:  min,
		MaxTimeUs:  max,
		StdDevUs:   stddev,
		Iterations: iterations * runs,
	}
}

func calculateStats(times []float64) (avg, min, max, stddev float64) {
	if len(times) == 0 {
		return 0, 0, 0, 0
	}

	sum := 0.0
	min = times[0]
	max = times[0]

	for _, t := range times {
		sum += t
		if t < min {
			min = t
		}
		if t > max {
			max = t
		}
	}

	avg = sum / float64(len(times))

	// Calculate standard deviation
	variance := 0.0
	for _, t := range times {
		diff := t - avg
		variance += diff * diff
	}
	variance /= float64(len(times))
	stddev = 0.0
	if variance > 0 {
		// Simple square root approximation
		x := variance
		for i := 0; i < 10; i++ {
			x = (x + variance/x) / 2
		}
		stddev = x
	}

	return avg, min, max, stddev
}

// Benchmark functions

func benchGetPid() error {
	_ = os.Getpid()
	return nil
}

func benchGetPPid() error {
	_ = os.Getppid()
	return nil
}

func benchGetTid() error {
	_ = syscall.Gettid()
	return nil
}

func benchOpenClose() error {
	fd, err := syscall.Open("/dev/null", syscall.O_RDONLY, 0)
	if err != nil {
		return err
	}
	return syscall.Close(fd)
}

func benchStat() error {
	var stat syscall.Stat_t
	return syscall.Stat("/dev/null", &stat)
}

func benchReadWrite() error {
	// Create a pipe for read/write testing
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	defer r.Close()
	defer w.Close()

	data := []byte("x")
	buf := make([]byte, 1)

	// Write
	if _, err := w.Write(data); err != nil {
		return err
	}

	// Read
	if _, err := r.Read(buf); err != nil {
		return err
	}

	return nil
}

func benchSocketClose() error {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return err
	}
	return syscall.Close(fd)
}

func benchGetcwd() error {
	_, err := os.Getwd()
	return err
}

func benchGetUid() error {
	_ = os.Getuid()
	return nil
}

func benchAccess() error {
	return syscall.Access("/dev/null", 0x4) // R_OK = 4
}
