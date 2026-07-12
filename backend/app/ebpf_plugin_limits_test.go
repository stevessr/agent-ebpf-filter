package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
)

func testTracepointManifest() *PluginManifest {
	return &PluginManifest{
		ID:           "safe-plugin",
		Name:         "Safe plugin",
		Kind:         PluginKindEBPF,
		AttachKind:   PluginAttachTracepoint,
		AttachTarget: "syscalls/sys_enter_execve",
		ProgramName:  "test_prog",
	}
}

func testUserBPFCollectionSpec() *ebpf.CollectionSpec {
	return &ebpf.CollectionSpec{
		Programs: map[string]*ebpf.ProgramSpec{
			"test_prog": {
				Type:         ebpf.TracePoint,
				Instructions: make(asm.Instructions, 1),
			},
		},
		Maps: map[string]*ebpf.MapSpec{
			"counts": {
				Type:       ebpf.Array,
				KeySize:    4,
				ValueSize:  8,
				MaxEntries: 1,
			},
		},
	}
}

func TestValidateUserBPFCollectionSpecAcceptsBoundedSelection(t *testing.T) {
	spec := testUserBPFCollectionSpec()
	program, err := validateUserBPFCollectionSpec(spec, testTracepointManifest())
	if err != nil {
		t.Fatalf("validateUserBPFCollectionSpec() error = %v", err)
	}
	if program != spec.Programs["test_prog"] {
		t.Fatal("validator did not return selected program")
	}
}

func TestValidateUserBPFCollectionSpecRejectsResourceAbuse(t *testing.T) {
	tests := map[string]func(*ebpf.CollectionSpec){
		"instructions": func(spec *ebpf.CollectionSpec) {
			spec.Programs["test_prog"].Instructions = make(asm.Instructions, maxUserBPFInstructionsPerProgram+1)
		},
		"program count": func(spec *ebpf.CollectionSpec) {
			for i := 0; i < maxUserBPFPrograms; i++ {
				spec.Programs[string(rune('a'+i))+"_prog"] = &ebpf.ProgramSpec{Type: ebpf.TracePoint, Instructions: make(asm.Instructions, 1)}
			}
		},
		"map pinning": func(spec *ebpf.CollectionSpec) {
			spec.Maps["counts"].Pinning = ebpf.PinByName
		},
		"inner map": func(spec *ebpf.CollectionSpec) {
			spec.Maps["counts"].InnerMap = &ebpf.MapSpec{Type: ebpf.Array, MaxEntries: 1}
		},
		"map memory": func(spec *ebpf.CollectionSpec) {
			spec.Maps["counts"].ValueSize = maxUserBPFMapValueBytes
			spec.Maps["counts"].MaxEntries = maxUserBPFMapEntries
		},
		"ring buffer": func(spec *ebpf.CollectionSpec) {
			spec.Maps["counts"] = &ebpf.MapSpec{Type: ebpf.RingBuf, MaxEntries: uint32(maxUserBPFMapEstimatedBytes + 1)}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			spec := testUserBPFCollectionSpec()
			mutate(spec)
			if _, err := validateUserBPFCollectionSpec(spec, testTracepointManifest()); err == nil {
				t.Fatal("unsafe collection spec was accepted")
			}
		})
	}
}

func TestValidateUserBPFCollectionSpecRejectsProgramMismatch(t *testing.T) {
	spec := testUserBPFCollectionSpec()
	spec.Programs["test_prog"].Type = ebpf.Kprobe
	if _, err := validateUserBPFCollectionSpec(spec, testTracepointManifest()); err == nil || !strings.Contains(err.Error(), "expected TracePoint") {
		t.Fatalf("program type mismatch error = %v", err)
	}

	manifest := testTracepointManifest()
	manifest.ProgramName = "missing"
	if _, err := validateUserBPFCollectionSpec(testUserBPFCollectionSpec(), manifest); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing program error = %v", err)
	}
}

func TestValidateLoadableEBPFPluginManifestRejectsUnsafeTargets(t *testing.T) {
	for _, target := range []string{
		"../sys_enter_execve",
		"syscalls/../escape",
		"syscalls/name/extra",
		"syscalls/name\nextra",
	} {
		manifest := testTracepointManifest()
		manifest.AttachTarget = target
		if err := validateLoadableEBPFPluginManifest(manifest); err == nil {
			t.Fatalf("unsafe tracepoint target %q accepted", target)
		}
	}
	manifest := testTracepointManifest()
	manifest.AttachKind = PluginAttachKprobe
	manifest.AttachTarget = "../../kernel/symbol"
	if err := validateLoadableEBPFPluginManifest(manifest); err == nil {
		t.Fatal("unsafe kprobe target accepted")
	}
}

func withIsolatedLoadedPlugins(t *testing.T) {
	t.Helper()
	loadedPluginsMu.Lock()
	oldLoaded := loadedPlugins
	oldLoading := loadingPlugins
	loadedPlugins = make(map[string]*loadedEBPFPlugin)
	loadingPlugins = make(map[string]*ebpfPluginLoadReservation)
	loadedPluginsMu.Unlock()
	t.Cleanup(func() {
		loadedPluginsMu.Lock()
		loadedPlugins = oldLoaded
		loadingPlugins = oldLoading
		loadedPluginsMu.Unlock()
	})
}

func TestEBPFPluginLoadReservationsAreBoundedAndCancelable(t *testing.T) {
	withIsolatedLoadedPlugins(t)
	loadedPluginsMu.Lock()
	for i := 0; i < maxLoadedEBPFPlugins; i++ {
		loadingPlugins[fmt.Sprintf("plugin-%02d", i)] = &ebpfPluginLoadReservation{}
	}
	loadedPluginsMu.Unlock()
	if _, err := reserveEBPFPluginLoad("overflow-plugin"); err == nil {
		t.Fatal("load reservation limit was not enforced")
	}

	loadedPluginsMu.Lock()
	loadingPlugins = make(map[string]*ebpfPluginLoadReservation)
	loadedPluginsMu.Unlock()
	reservation, err := reserveEBPFPluginLoad("safe-plugin")
	if err != nil {
		t.Fatal(err)
	}
	UnloadEBPFPlugin("safe-plugin")
	if _, err := installLoadedEBPFPlugin("safe-plugin", reservation, &loadedEBPFPlugin{}); err == nil {
		t.Fatal("canceled plugin load was installed")
	}
}

func TestEBPFPluginFailurePlaceholderIsNotReportedLoaded(t *testing.T) {
	withIsolatedLoadedPlugins(t)
	loadedPlugins["safe-plugin"] = &loadedEBPFPlugin{loadError: "load failed"}
	loaded, loadError := ebpfPluginRuntimeState("safe-plugin")
	if loaded || loadError != "load failed" {
		t.Fatalf("runtime state = loaded=%v error=%q", loaded, loadError)
	}
}

func TestLoadEBPFPluginContextCancelsArtifactWaitAndReleasesReservation(t *testing.T) {
	withIsolatedLoadedPlugins(t)
	release, err := acquirePluginArtifactLock(context.Background(), "safe-plugin")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	manifest := testTracepointManifest()
	manifest.ObjectSHA256 = strings.Repeat("a", 64)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = LoadEBPFPluginContext(ctx, manifest)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("load cancellation error = %v", err)
	}
	loadedPluginsMu.Lock()
	_, stillLoading := loadingPlugins[manifest.ID]
	loadedPluginsMu.Unlock()
	if stillLoading {
		t.Fatal("canceled load leaked reservation")
	}
}
