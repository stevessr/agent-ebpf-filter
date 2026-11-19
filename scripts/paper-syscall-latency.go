// Standalone Linux syscall-latency sampler for paper concurrency evidence.
// Timing is per read(2) on an already opened /dev/zero FD. Measurement includes
// userspace timing/instrumentation cost. It is NOT eBPF-only overhead.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
)

func main() {
	concurrency := flag.Int("concurrency", 1, "concurrent workers")
	samples := flag.Int("samples", 50000, "total independently timed pread operations")
	warmup := flag.Int("warmup", 1000, "untimed pread calls per worker")
	path := flag.String("path", "/dev/zero", "existing readable stream (default /dev/zero)")
	output := flag.String("output", "", "destination CSV containing latency_us")
	flag.Parse()

	if *concurrency < 1 || *concurrency > 1024 || *samples < *concurrency || *warmup < 0 || *output == "" {
		fmt.Fprintln(os.Stderr, "invalid options: 1 <= concurrency <= min(samples,1024), warmup >= 0, --output required")
		os.Exit(2)
	}
	info, err := os.Stat(*path)
	if err != nil || info.IsDir() {
		fmt.Fprintf(os.Stderr, "input file must be an existing readable file: %s (%v)\n", *path, err)
		os.Exit(2)
	}

	runtime.GOMAXPROCS(runtime.NumCPU())
	durations := make([]float64, *samples)
	errs := make(chan error, *concurrency)
	var wg sync.WaitGroup

	// Give each worker its own fd and byte buffer. Every timed sample is
	// a single read syscall; open/close do not contaminate its latency.
	for worker := 0; worker < *concurrency; worker++ {
		start := (*samples * worker) / *concurrency
		end := (*samples * (worker + 1)) / *concurrency
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			fd, err := syscall.Open(*path, syscall.O_RDONLY, 0)
			if err != nil {
				errs <- err
				return
			}
			defer syscall.Close(fd)
			var buf [1]byte
			for i := 0; i < *warmup; i++ {
				if _, err := syscall.Read(fd, buf[:]); err != nil {
					errs <- err
					return
				}
			}
			for i := start; i < end; i++ {
				before := time.Now()
				n, err := syscall.Read(fd, buf[:])
				ns := time.Since(before).Nanoseconds()
				if err != nil || n != 1 {
					errs <- fmt.Errorf("read: bytes=%d, err=%w", n, err)
					return
				}
				durations[i] = float64(ns) / 1000
			}
		}(start, end)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		fmt.Fprintf(os.Stderr, "sample collection failed: %v\n", err)
		os.Exit(1)
	}

	file, err := os.Create(*output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create CSV: %v\n", err)
		os.Exit(1)
	}
	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"latency_us"}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		file.Close()
		os.Exit(1)
	}
	for _, duration := range durations {
		if err := writer.Write([]string{strconv.FormatFloat(duration, 'f', 6, 64)}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			file.Close()
			os.Exit(1)
		}
	}
	writer.Flush()
	err = writer.Error()
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "flush CSV: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("collected %d read(2) timings at concurrency=%d with %d available CPUs; results=%s\n",
		*samples, *concurrency, runtime.NumCPU(), *output)
}
