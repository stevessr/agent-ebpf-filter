package events

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
)

type kernelRiskFeedbackAction struct {
	Kind     string
	Target   string
	Score    float64
	Decision string
	Reason   string
}

type KernelRiskFeedbackAction = kernelRiskFeedbackAction

var (
	kernelRiskFeedbackQueue  = make(chan kernelRiskFeedbackAction, 256)
	kernelRiskFeedbackWorker = struct {
		lifecycleMu sync.Mutex
		mu          sync.Mutex
		cancel      context.CancelFunc
		done        chan struct{}
		started     bool
	}{}
	kernelRiskFeedbackDedup = &kernelRiskFeedbackState{
		seen: make(map[string]time.Time),
	}
)

type kernelRiskFeedbackState struct {
	mu          sync.Mutex
	seen        map[string]time.Time
	windowStart time.Time
	windowCount int
}

func StartKernelRiskFeedbackWorker(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	kernelRiskFeedbackWorker.lifecycleMu.Lock()
	defer kernelRiskFeedbackWorker.lifecycleMu.Unlock()
	kernelRiskFeedbackWorker.mu.Lock()
	if kernelRiskFeedbackWorker.started || kernelRiskFeedbackWorker.done != nil {
		kernelRiskFeedbackWorker.mu.Unlock()
		return
	}
	drainKernelRiskFeedbackQueue()
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	kernelRiskFeedbackWorker.cancel = cancel
	kernelRiskFeedbackWorker.done = done
	kernelRiskFeedbackWorker.started = true
	kernelRiskFeedbackWorker.mu.Unlock()

	go runKernelRiskFeedbackWorker(workerCtx, done)
}

func runKernelRiskFeedbackWorker(ctx context.Context, done chan struct{}) {
	defer func() {
		close(done)
		kernelRiskFeedbackWorker.mu.Lock()
		if kernelRiskFeedbackWorker.done == done {
			kernelRiskFeedbackWorker.cancel = nil
			kernelRiskFeedbackWorker.done = nil
			kernelRiskFeedbackWorker.started = false
		}
		kernelRiskFeedbackWorker.mu.Unlock()
	}()
	for {
		select {
		case <-ctx.Done():
			drainKernelRiskFeedbackQueue()
			return
		case action := <-kernelRiskFeedbackQueue:
			settings := Deps.RuntimeSettingsSnapshot()
			if !settings.PolicyManagementEnabled || !settings.KernelRiskFeedback.Enabled {
				Deps.CollectorMetrics.RecordKernelRiskFeedback(false, errors.New("kernel risk feedback disabled or policy management gate closed"))
				continue
			}
			if !kernelRiskFeedbackDedup.Allow(action, settings.KernelRiskFeedback, time.Now()) {
				Deps.CollectorMetrics.RecordKernelRiskFeedback(false, nil)
				continue
			}
			if err := applyKernelRiskFeedbackAction(action); err != nil {
				Deps.CollectorMetrics.RecordKernelRiskFeedback(false, err)
				continue
			}
			Deps.CollectorMetrics.RecordKernelRiskFeedback(true, nil)
		}
	}
}

func ShutdownKernelRiskFeedbackWorker(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	kernelRiskFeedbackWorker.lifecycleMu.Lock()
	defer kernelRiskFeedbackWorker.lifecycleMu.Unlock()
	kernelRiskFeedbackWorker.mu.Lock()
	if kernelRiskFeedbackWorker.done == nil {
		kernelRiskFeedbackWorker.mu.Unlock()
		return nil
	}
	cancel := kernelRiskFeedbackWorker.cancel
	done := kernelRiskFeedbackWorker.done
	kernelRiskFeedbackWorker.started = false
	kernelRiskFeedbackWorker.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done == nil {
		return nil
	}
	select {
	case <-done:
		kernelRiskFeedbackWorker.mu.Lock()
		if kernelRiskFeedbackWorker.done == done {
			kernelRiskFeedbackWorker.cancel = nil
			kernelRiskFeedbackWorker.done = nil
		}
		kernelRiskFeedbackWorker.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func kernelRiskFeedbackWorkerStarted() bool {
	kernelRiskFeedbackWorker.mu.Lock()
	defer kernelRiskFeedbackWorker.mu.Unlock()
	return kernelRiskFeedbackWorker.started
}

func drainKernelRiskFeedbackQueue() {
	for {
		select {
		case <-kernelRiskFeedbackQueue:
		default:
			return
		}
	}
}

func (s *kernelRiskFeedbackState) Allow(action kernelRiskFeedbackAction, settings KernelRiskFeedbackSettings, now time.Time) bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.seen == nil {
		s.seen = make(map[string]time.Time)
	}
	key := action.Kind + "\x00" + action.Target
	if last, ok := s.seen[key]; ok && now.Sub(last) < 5*time.Minute {
		return false
	}
	if s.windowStart.IsZero() || now.Sub(s.windowStart) >= time.Minute {
		s.windowStart = now
		s.windowCount = 0
	}
	limit := settings.MaxActionsPerMinute
	if limit <= 0 {
		limit = 30
	}
	if s.windowCount >= limit {
		return false
	}
	s.windowCount++
	s.seen[key] = now
	return true
}

func queueKernelRiskFeedback(event *pb.Event, decision kernelRiskDecision) {
	if !kernelRiskFeedbackWorkerStarted() {
		return
	}
	settings := Deps.RuntimeSettingsSnapshot()
	actions := kernelRiskFeedbackActions(settings, event, decision)
	for _, action := range actions {
		select {
		case kernelRiskFeedbackQueue <- action:
		default:
			Deps.CollectorMetrics.RecordKernelRiskFeedback(false, errors.New("kernel risk feedback queue full"))
		}
	}
}

func kernelRiskFeedbackActions(settings RuntimeSettings, event *pb.Event, decision kernelRiskDecision) []kernelRiskFeedbackAction {
	if event == nil || !settings.PolicyManagementEnabled || !settings.KernelRiskFeedback.Enabled {
		return nil
	}
	feedback := settings.KernelRiskFeedback
	if feedback.MinRiskScore <= 0 {
		feedback.MinRiskScore = 85
	}
	if decision.Score < feedback.MinRiskScore {
		return nil
	}

	actions := make([]kernelRiskFeedbackAction, 0, 3)
	add := func(kind, target, reason string) {
		target = strings.TrimSpace(target)
		if kind == "" || target == "" {
			return
		}
		actions = append(actions, kernelRiskFeedbackAction{
			Kind:     kind,
			Target:   target,
			Score:    decision.Score,
			Decision: Deps.StringsTrimDefault(decision.Decision, "OBSERVE"),
			Reason:   reason,
		})
	}

	if feedback.EnforceNetwork && isKernelRiskNetworkEvent(event.GetType()) {
		host, port := kernelRiskEndpoint(event)
		if ip := net.ParseIP(host); ip != nil && !isPrivateKernelRiskIP(ip) {
			add(kernelRiskFeedbackKindNetworkIP, ip.String(), "risk_scored_public_endpoint")
		}
		if decision.Score >= 95 && port > 0 && port <= 65535 && isHighRiskKernelPort(port) {
			add(kernelRiskFeedbackKindNetworkPort, strconv.FormatUint(uint64(port), 10), "risk_scored_high_risk_port")
		}
	}

	path := platform.FirstNonEmpty(event.GetPath(), event.GetExtraPath())
	if feedback.EnforceFileNames && isKernelRiskFileEvent(event.GetType()) && (isSensitiveKernelRiskPath(strings.ToLower(path)) || isSecretKernelRiskPath(strings.ToLower(path))) {
		if name := safeKernelRiskBasename(path); name != "" {
			add(kernelRiskFeedbackKindLSMFileName, name, "risk_scored_file_basename")
		}
	}

	if feedback.EnforceExec && (event.GetType() == "execve" || event.GetType() == "process_exec") {
		if path != "" && filepath.IsAbs(path) && isTmpExecutableKernelRiskPath(strings.ToLower(path)) {
			add(kernelRiskFeedbackKindLSMExecPath, path, "risk_scored_tmp_exec_path")
		} else if decision.Score >= 95 {
			if name := safeKernelRiskBasename(platform.FirstNonEmpty(event.GetComm(), path)); name != "" {
				add(kernelRiskFeedbackKindLSMExecName, name, "risk_scored_exec_name")
			}
		}
	}

	return actions
}

func KernelRiskFeedbackActions(settings RuntimeSettings, event *pb.Event, decision KernelRiskDecision) []KernelRiskFeedbackAction {
	return kernelRiskFeedbackActions(settings, event, decision)
}

func applyKernelRiskFeedbackAction(action kernelRiskFeedbackAction) error {
	switch action.Kind {
	case kernelRiskFeedbackKindNetworkIP:
		return Deps.BlockIP(action.Target)
	case kernelRiskFeedbackKindNetworkPort:
		port, err := strconv.ParseUint(action.Target, 10, 16)
		if err != nil || port == 0 {
			return fmt.Errorf("invalid kernel-risk feedback port %q", action.Target)
		}
		return Deps.BlockPort(uint16(port))
	case kernelRiskFeedbackKindLSMFileName:
		return Deps.BlockLsmFileName(action.Target)
	case kernelRiskFeedbackKindLSMExecPath:
		return Deps.BlockLsmExecPath(action.Target)
	case kernelRiskFeedbackKindLSMExecName:
		return Deps.BlockLsmExecName(action.Target)
	default:
		return fmt.Errorf("unknown kernel-risk feedback action kind %q", action.Kind)
	}
}

func isKernelRiskNetworkEvent(eventType string) bool {
	switch eventType {
	case "network_connect", "network_sendto", "tcp_connect", "dns_query":
		return true
	default:
		return false
	}
}

func isKernelRiskFileEvent(eventType string) bool {
	switch eventType {
	case "open", "openat", "read", "write", "chmod", "chown", "mknod", "link", "symlink", "rename", "unlink", "unlinkat":
		return true
	default:
		return false
	}
}

func safeKernelRiskBasename(path string) string {
	name := filepath.Base(strings.TrimSpace(path))
	switch name {
	case "", ".", string(filepath.Separator):
		return ""
	default:
		return name
	}
}
