package tls

import (
	"log"
)

// ---- moved from backend/zz_merged_backend.go section capturestartuptls.go ----

type RuntimeBundle struct {
	Store       *TLSCaptureStore
	Rules       *TLSCaptureRuleStore
	Broadcaster *TLSBroadcaster
	Controller  *TLSCaptureController
}

func StartTLSCaptureRuntime(settings RuntimeSettings) *RuntimeBundle {
	store := NewTLSCaptureStore(2000)
	rules := NewTLSCaptureRuleStore()
	broadcaster := NewTLSCaptureBroadcaster()
	controller := NewTLSCaptureController(store, rules, broadcaster)

	if settings.TlsCaptureEnabled {
		if err := controller.AttachDefaults(); err != nil {
			log.Printf("[TLS] static library attach completed with warnings: %v", err)
		}
	}

	return &RuntimeBundle{
		Store:       store,
		Rules:       rules,
		Broadcaster: broadcaster,
		Controller:  controller,
	}
}
