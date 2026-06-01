package app

import (
	"log"
)

// ---- moved from backend/zz_merged_backend.go section capturestartuptls.go ----

type tlsCaptureRuntimeBundle struct {
	store       *TLSCaptureStore
	rules       *TLSCaptureRuleStore
	broadcaster *tlsCaptureBroadcaster
	controller  *TLSCaptureController
}

func startTLSCaptureRuntime(settings RuntimeSettings) *tlsCaptureRuntimeBundle {
	store := NewTLSCaptureStore(2000)
	rules := NewTLSCaptureRuleStore()
	broadcaster := newTLSCaptureBroadcaster()
	controller := NewTLSCaptureController(store, rules, broadcaster)
	if settings.TlsCaptureEnabled {
		if err := controller.AttachDefaults(); err != nil {
			log.Printf("[TLS] static library attach completed with warnings: %v", err)
		}
	}
	return &tlsCaptureRuntimeBundle{
		store:       store,
		rules:       rules,
		broadcaster: broadcaster,
		controller:  controller,
	}
}
