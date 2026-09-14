package core

import (
	"testing"
	"unsafe"
)

func TestBpfEventAuditABIGuard(t *testing.T) {
	var event BpfEvent
	if got := unsafe.Sizeof(event); got != 688 {
		t.Fatalf("BpfEvent size = %d, want 688", got)
	}
	if got := unsafe.Offsetof(event.KernelAuditGeneration); got != 656 {
		t.Fatalf("KernelAuditGeneration offset = %d, want 656", got)
	}
	if got := unsafe.Offsetof(event.KernelDroppedSinceLast); got != 664 {
		t.Fatalf("KernelDroppedSinceLast offset = %d, want 664", got)
	}
	if got := unsafe.Offsetof(event.KernelReserveFailuresTotal); got != 672 {
		t.Fatalf("KernelReserveFailuresTotal offset = %d, want 672", got)
	}
}
