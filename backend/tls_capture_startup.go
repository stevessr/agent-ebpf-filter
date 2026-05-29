package main

import "log"

type tlsCaptureRuntime struct {
	store       *TLSCaptureStore
	rules       *TLSCaptureRuleStore
	broadcaster *tlsCaptureBroadcaster
	controller  *TLSCaptureController
}

func startTLSCaptureRuntime(settings RuntimeSettings) *tlsCaptureRuntime {
	store := NewTLSCaptureStore(2000)
	rules := NewTLSCaptureRuleStore()
	broadcaster := newTLSCaptureBroadcaster()
	controller := NewTLSCaptureController(store, rules, broadcaster)
	if settings.TlsCaptureEnabled {
		if err := controller.AttachDefaults(); err != nil {
			log.Printf("[TLS] static library attach completed with warnings: %v", err)
		}
	}
	return &tlsCaptureRuntime{
		store:       store,
		rules:       rules,
		broadcaster: broadcaster,
		controller:  controller,
	}
}
