package ebpf

import (
	"testing"

	"github.com/cilium/ebpf"
)

// CollectionSpec parsing needs no privileges. Catch compiler-emitted libc
// calls before NewCollection reaches the kernel (or requires root).
func TestAgentTrackerHasNoUnresolvedCalls(t *testing.T) {
	spec, err := LoadAgentTracker()
	if err != nil {
		t.Fatalf("parse generated tracker: %v", err)
	}
	checkProgramCalls(t, spec)
}

func checkProgramCalls(t *testing.T, spec *ebpf.CollectionSpec) {
	t.Helper()
	for name, program := range spec.Programs {
		t.Run(name, func(t *testing.T) {
			symbols, err := program.Instructions.SymbolOffsets()
			if err != nil {
				t.Fatal(err)
			}
			for i, instruction := range program.Instructions {
				if !instruction.IsFunctionCall() {
					continue
				}
				reference := instruction.Reference()
				if reference == "" {
					continue // Already resolved relative BPF-to-BPF call.
				}
				if _, ok := symbols[reference]; !ok {
					t.Errorf("instruction %d: unresolved function %q", i, reference)
				}
			}
		})
	}
}
