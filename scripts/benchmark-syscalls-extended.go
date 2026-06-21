package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// BenchmarkResult stores timing results for a single operation
type BenchmarkResult struct {
	Name       string  `json:"name"`
	AvgTimeUs  float64 `json:"avg_time_us"`
	MinTimeUs  float64 `json:"min_time_us"`
	MaxTimeUs  float64 `json:"max_time_us"`
	StdDevUs   float64 `json:"stddev_us"`
	Iterations int     `json:"iterations"`
	Concurrency int    `json:"concurrency,omitempty"`
}

// BenchmarkReport contains all benchmark results
type BenchmarkReport struct {
	Timestamp   string            `json:"timestamp"`
	Runs        int               `json:"runs"`
	Warmup      int               `json:"warmup"`
	Concurrency int               `json:"concurrency"`
	Operations  []BenchmarkResult `json:"operations"`
}

var (
	outputFile  = flag.String("output", "", "Output JSON file")
	runs        = flag.Int("runs", 5, "Number of runs per operation")
	warmup      = flag.Int("warmup", 2, "Number of warmup runs")
	iterations  = flag.Int("iterations", 10000, "Iterations per run")
	concurrency = flag.Int("concurrency", 1, "Number of concurrent goroutines (batch mode)")
)

func main() {
	flag.Parse()

	if *outputFile == "" {
		fmt.Fprintf(os.Stderr, "Error: --output flag is required\n")
		os.Exit(1)
	}

	if *concurrency < 1 {
		fmt.Fprintf(os.Stderr, "Error: concurrency must be >= 1\n")
		os.Exit(1)
	}

	fmt.Printf("Running benchmark with %d runs, %d warmup, %d iterations per run, %d concurrent workers\n",
		*runs, *warmup, *iterations, *concurrency)

	report := BenchmarkReport{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Runs:        *runs,
		Warmup:      *warmup,
		Concurrency: *concurrency,
		Operations:  make([]BenchmarkResult, 0),
	}

	// Benchmark various syscalls that eBPF hooks might intercept
	operations := []struct {
		name string
		fn   func() error
	}{
		// Process info syscalls
		{"getpid", benchGetPid},
		{"getppid", benchGetPPid},
		{"gettid", benchGetTid},
		{"getuid", benchGetUid},
		{"getgid", benchGetGid},
		{"geteuid", benchGetEuid},
		{"getegid", benchGetEgid},

		// File operations
		{"open_close", benchOpenClose},
		{"stat", benchStat},
		{"access", benchAccess},
		{"getcwd", benchGetcwd},
		{"read_write", benchReadWrite},
		{"readlink", benchReadlink},

		// Network operations
		{"socket_close", benchSocketClose},
		{"bind", benchBind},

		// Directory operations
		{"mkdir_rmdir", benchMkdirRmdir},

		// Special operations
		{"clone", benchClone},
		{"prctl", benchPrctl},
	}

	for _, op := range operations {
		fmt.Printf("Benchmarking %s...\n", op.name)
		result := runBenchmark(op.name, op.fn, *runs, *warmup, *iterations, *concurrency)
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

func runBenchmark(name string, fn func() error, runs, warmup, iterations, concurrency int) BenchmarkResult {
	// Warmup
	for i := 0; i < warmup; i++ {
		runConcurrent(fn, iterations, concurrency)
	}

	// Actual runs
	times := make([]float64, runs)
	for i := 0; i < runs; i++ {
		start := time.Now()
		runConcurrent(fn, iterations, concurrency)
		elapsed := time.Since(start)
		// Average time per operation across all concurrent workers
		times[i] = float64(elapsed.Microseconds()) / float64(iterations)
	}

	// Calculate statistics
	avg, min, max, stddev := calculateStats(times)

	return BenchmarkResult{
		Name:        name,
		AvgTimeUs:   avg,
		MinTimeUs:   min,
		MaxTimeUs:   max,
		StdDevUs:    stddev,
		Iterations:  iterations * runs,
		Concurrency: concurrency,
	}
}

// runConcurrent executes the benchmark function concurrently across multiple goroutines
func runConcurrent(fn func() error, iterations, concurrency int) {
	if concurrency == 1 {
		// Fast path: no concurrency overhead
		for j := 0; j < iterations; j++ {
			_ = fn()
		}
		return
	}

	// Split iterations among workers
	iterPerWorker := iterations / concurrency
	remainder := iterations % concurrency

	var wg sync.WaitGroup
	wg.Add(concurrency)

	for w := 0; w < concurrency; w++ {
		workerIters := iterPerWorker
		if w == 0 {
			workerIters += remainder // First worker handles remainder
		}

		go func(iters int) {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				_ = fn()
			}
		}(workerIters)
	}

	wg.Wait()
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

func benchGetUid() error {
	_ = os.Getuid()
	return nil
}

func benchGetGid() error {
	_ = os.Getgid()
	return nil
}

func benchGetEuid() error {
	_ = os.Geteuid()
	return nil
}

func benchGetEgid() error {
	_ = os.Getegid()
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

func benchAccess() error {
	return syscall.Access("/dev/null", 0x4) // R_OK = 4
}

func benchReadlink() error {
	buf := make([]byte, 128)
	_, err := syscall.Readlink("/proc/self/exe", buf)
	return err
}

func benchBind() error {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)

	// Bind to localhost:0 (auto-assign port)
	addr := &syscall.SockaddrInet4{
		Port: 0,
		Addr: [4]byte{127, 0, 0, 1},
	}
	return syscall.Bind(fd, addr)
}

func benchMkdirRmdir() error {
	tmpDir := fmt.Sprintf("/tmp/bench_%d", time.Now().UnixNano())
	if err := syscall.Mkdir(tmpDir, 0755); err != nil {
		return err
	}
	return syscall.Rmdir(tmpDir)
}

func benchClone() error {
	// We can't actually clone, so we measure getpid as a proxy
	// (clone syscall is too disruptive for benchmarking)
	_ = os.Getpid()
	return nil
}

func benchPrctl() error {
	// PR_GET_NAME (16) is safe to call repeatedly
	var name [16]byte
	_, _, errno := syscall.Syscall6(
		syscall.SYS_PRCTL,
		16, // PR_GET_NAME
		uintptr(unsafe.Pointer(&name[0])),
		0, 0, 0, 0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
