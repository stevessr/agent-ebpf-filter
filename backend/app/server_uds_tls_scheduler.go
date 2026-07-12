package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	errWrapperTLSAttachSchedulerClosed = errors.New("wrapper TLS attach scheduler is closed")
	errWrapperTLSAttachQueueFull       = errors.New("wrapper TLS attach queue is full")
	errWrapperTLSAttachDuplicate       = errors.New("wrapper TLS attach is already pending")
)

type wrapperTLSAttachRequest struct {
	PID        uint32
	Comm       string
	BinaryPath string
}

type wrapperTLSAttachScheduler struct {
	ctx     context.Context
	cancel  context.CancelFunc
	queue   chan wrapperTLSAttachRequest
	run     func(context.Context, wrapperTLSAttachRequest)
	done    chan struct{}
	mu      sync.Mutex
	pending map[uint32]struct{}
	stopped bool
}

func newWrapperTLSAttachScheduler(parent context.Context, queueSize int, run func(context.Context, wrapperTLSAttachRequest)) *wrapperTLSAttachScheduler {
	if parent == nil {
		parent = context.Background()
	}
	if queueSize <= 0 {
		queueSize = udsTLSAttachQueueSize
	}
	ctx, cancel := context.WithCancel(parent)
	scheduler := &wrapperTLSAttachScheduler{
		ctx:     ctx,
		cancel:  cancel,
		queue:   make(chan wrapperTLSAttachRequest, queueSize),
		run:     run,
		done:    make(chan struct{}),
		pending: make(map[uint32]struct{}),
	}
	go scheduler.loop()
	return scheduler
}

func (scheduler *wrapperTLSAttachScheduler) Submit(req wrapperTLSAttachRequest) error {
	if scheduler == nil || req.PID == 0 {
		return errWrapperTLSAttachSchedulerClosed
	}
	req.Comm = strings.TrimSpace(req.Comm)
	req.BinaryPath = strings.TrimSpace(req.BinaryPath)
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.stopped || scheduler.ctx.Err() != nil {
		return errWrapperTLSAttachSchedulerClosed
	}
	if _, exists := scheduler.pending[req.PID]; exists {
		return errWrapperTLSAttachDuplicate
	}
	select {
	case scheduler.queue <- req:
		scheduler.pending[req.PID] = struct{}{}
		return nil
	default:
		return errWrapperTLSAttachQueueFull
	}
}

func (scheduler *wrapperTLSAttachScheduler) Stop() {
	if scheduler == nil {
		return
	}
	scheduler.mu.Lock()
	if !scheduler.stopped {
		scheduler.stopped = true
		scheduler.cancel()
	}
	done := scheduler.done
	scheduler.mu.Unlock()
	<-done
}

func (scheduler *wrapperTLSAttachScheduler) loop() {
	defer close(scheduler.done)
	for {
		if scheduler.ctx.Err() != nil {
			return
		}
		select {
		case <-scheduler.ctx.Done():
			return
		case req := <-scheduler.queue:
			scheduler.runSafely(req)
			scheduler.mu.Lock()
			delete(scheduler.pending, req.PID)
			scheduler.mu.Unlock()
		}
	}
}

func (scheduler *wrapperTLSAttachScheduler) runSafely(req wrapperTLSAttachRequest) {
	if scheduler.run == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[WARN] wrapper TLS attach worker recovered from panic for PID %d: %v", req.PID, recovered)
		}
	}()
	scheduler.run(scheduler.ctx, req)
}

func runWrapperTLSAttach(ctx context.Context, req wrapperTLSAttachRequest) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil || tlsCaptureController == nil || !runtimeSettingsStore.Snapshot().TlsCaptureEnabled {
		return
	}
	binPath := strings.TrimSpace(req.BinaryPath)
	if binPath == "" {
		timer := time.NewTimer(500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		var err error
		binPath, err = os.Readlink(fmt.Sprintf("/proc/%d/exe", req.PID))
		if err != nil || binPath == "" {
			log.Printf("[tls] wrapper-attach: PID %d (%s): cannot read exe after exec: %v", req.PID, req.Comm, err)
			return
		}
	}
	if ctx.Err() != nil || !runtimeSettingsStore.Snapshot().TlsCaptureEnabled {
		return
	}
	result := tlsCaptureController.AttachExecutable(binPath, int(req.PID), "")
	if result.Error != "" {
		log.Printf("[tls] wrapper-attach: PID %d (%s, %s): %s", req.PID, req.Comm, binPath, result.Error)
		return
	}
	log.Printf("[tls] wrapper-attach: PID %d (%s) attached via %s/%s (library=%s)", req.PID, req.Comm, result.TargetKind, result.Library, binPath)
}
