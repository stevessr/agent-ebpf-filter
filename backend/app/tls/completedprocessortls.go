package tls

import (
	"strings"
	"time"
)

type tlsCompletedProcessResult struct {
	HTTPEvents int
	RawEvents  int
}

// tlsCompletedEventProcessor owns protocol-reassembly state above the kernel
// transport. Both perf-fragment capture and bpf-ts ringbuf capture can feed the
// same CompletedTLSFragment representation into this layer.
type tlsCompletedEventProcessor struct {
	http1       *TLSHTTPStreamAssembler
	http2       *TLSHTTP2StreamAssembler
	store       *TLSCaptureStore
	rules       *TLSCaptureRuleStore
	broadcaster *TLSBroadcaster
}

func newTLSCompletedEventProcessor(
	store *TLSCaptureStore,
	rules *TLSCaptureRuleStore,
	broadcaster *TLSBroadcaster,
) *tlsCompletedEventProcessor {
	if store == nil {
		store = NewTLSCaptureStore(1000)
	}
	if rules == nil {
		rules = NewTLSCaptureRuleStore()
	}
	if broadcaster == nil {
		broadcaster = NewTLSCaptureBroadcaster()
	}
	return &tlsCompletedEventProcessor{
		http1:       NewTLSHTTPStreamAssembler(10 * time.Second),
		http2:       NewTLSHTTP2StreamAssembler(10 * time.Second),
		store:       store,
		rules:       rules,
		broadcaster: broadcaster,
	}
}

func (processor *tlsCompletedEventProcessor) Process(completed CompletedTLSFragment) tlsCompletedProcessResult {
	if processor == nil || processor.http1 == nil || processor.http2 == nil || processor.store == nil || processor.broadcaster == nil {
		return tlsCompletedProcessResult{}
	}

	parsedEvents, http1Recognized := processor.http1.AddRecognized(completed)
	http2Recognized := false
	if len(parsedEvents) == 0 && !http1Recognized {
		parsedEvents, http2Recognized = processor.http2.Add(completed)
	}

	result := tlsCompletedProcessResult{}
	if len(parsedEvents) == 0 {
		if http1Recognized || http2Recognized {
			// A protocol assembler owns these bytes and is waiting for more input or
			// intentionally suppressed a capture gap. Never emit a raw duplicate.
			return result
		}
		raw := completedToPlaintextEvent(completed)
		if processor.rules == nil || processor.rules.Allows(raw) {
			processor.broadcaster.Broadcast(raw)
			processor.store.Add(raw)
			result.RawEvents++
		}
		return result
	}

	for _, event := range parsedEvents {
		if processor.rules != nil && !processor.rules.Allows(event) {
			continue
		}
		if strings.HasPrefix(event.Type, "http2") || isTLSHTTPDisplayEvent(event) {
			DispatchTLSAgentEvent(&event, tlsAgentLoopDetector, deps.Broadcast)
			result.HTTPEvents++
		} else {
			result.RawEvents++
		}
		processor.store.Add(event)
		processor.broadcaster.Broadcast(event)
	}
	return result
}
