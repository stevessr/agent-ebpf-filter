package app

import (
	"log"
)

// ---- moved from backend/zz_merged_backend.go section capturestartuptls.go ----

type tlsCaptureRuntimeBundle struct {
	store         *TLSCaptureStore
	rules         *TLSCaptureRuleStore
	broadcaster   *tlsCaptureBroadcaster
	controller    *TLSCaptureController
	codexTracker  *CodexSyscallTracker
}

func startTLSCaptureRuntime(settings RuntimeSettings) *tlsCaptureRuntimeBundle {
	store := NewTLSCaptureStore(2000)
	rules := NewTLSCaptureRuleStore()
	broadcaster := newTLSCaptureBroadcaster()
	controller := NewTLSCaptureController(store, rules, broadcaster)

	var codexTracker *CodexSyscallTracker
	if settings.TlsCaptureEnabled {
		if err := controller.AttachDefaults(); err != nil {
			log.Printf("[TLS] static library attach completed with warnings: %v", err)
		}

		// 启动 Codex syscall tracker
		if tracker, err := NewCodexSyscallTracker(store); err == nil {
			if err := tracker.Attach(); err == nil {
				codexTracker = tracker
				go func() {
					if err := tracker.ReadLoop(); err != nil {
						log.Printf("[Codex] tracker read loop error: %v", err)
					}
				}()
				log.Printf("[Codex] syscall tracker started")
			} else {
				log.Printf("[Codex] failed to attach: %v", err)
			}
		}
	}

	return &tlsCaptureRuntimeBundle{
		store:        store,
		rules:        rules,
		broadcaster:  broadcaster,
		controller:   controller,
		codexTracker: codexTracker,
	}
}
