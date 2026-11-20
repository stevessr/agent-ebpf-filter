package app

import (
	"agent-ebpf-filter/app/types"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

// loadedEBPFPlugin tracks an attached user plugin so we can detach on disable/delete.
type loadedEBPFPlugin struct {
	collection *ebpf.Collection
	links      []link.Link
	loadError  string
}

var (
	loadedPluginsMu sync.Mutex
	loadedPlugins   = make(map[string]*loadedEBPFPlugin)
	loadingPlugins  = make(map[string]*ebpfPluginLoadReservation)
)

type ebpfPluginLoadReservation struct {
	canceled bool
}

// LoadEBPFPlugin reads the object file from disk and attaches the requested program.
func LoadEBPFPlugin(m *PluginManifest) error {
	return LoadEBPFPluginContext(context.Background(), m)
}

// LoadEBPFPluginContext propagates request cancellation while waiting for
// artifact access and between the non-cancelable kernel load/attach steps.
func LoadEBPFPluginContext(ctx context.Context, m *PluginManifest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateLoadableEBPFPluginManifest(m); err != nil {
		return err
	}
	if m.ObjectSHA256 == "" {
		return errors.New("compiled object checksum is missing; compile the plugin before loading")
	}
	reservation, err := reserveEBPFPluginLoad(m.ID)
	if err != nil {
		return err
	}
	installed := false
	defer func() {
		if !installed {
			releaseEBPFPluginLoad(m.ID, reservation)
		}
	}()

	releaseArtifacts, err := acquirePluginArtifactLock(ctx, m.ID)
	if err != nil {
		return fmt.Errorf("wait for plugin artifact lock: %w", err)
	}
	object, err := readPluginFile(m.ID, "program.o", maxUserBPFObjectBytes)
	releaseArtifacts()
	if err != nil {
		return fmt.Errorf("plugin object missing or unsafe: %w", err)
	}
	if sha256Hex(object) != m.ObjectSHA256 {
		return errors.New("plugin object checksum does not match manifest")
	}
	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(object))
	if err != nil {
		return fmt.Errorf("load collection spec: %w", err)
	}
	programSpec, err := validateUserBPFCollectionSpec(spec, m)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("plugin load canceled before kernel verification: %w", err)
	}
	// Loading unselected programs needlessly consumes verifier time and kernel
	// memory. Keep only the explicitly requested entry point.
	spec.Programs = map[string]*ebpf.ProgramSpec{m.ProgramName: programSpec}
	coll, err := ebpf.NewCollectionWithOptions(spec, ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{LogDisabled: true},
	})
	if err != nil {
		return fmt.Errorf("instantiate collection: %w", err)
	}
	if err := ctx.Err(); err != nil {
		coll.Close()
		return fmt.Errorf("plugin load canceled after kernel verification: %w", err)
	}
	prog, ok := coll.Programs[m.ProgramName]
	if !ok {
		coll.Close()
		names := make([]string, 0, len(coll.Programs))
		for n := range coll.Programs {
			names = append(names, n)
		}
		return fmt.Errorf("program %q not found in object (available: %s)", m.ProgramName, strings.Join(names, ", "))
	}

	var attached link.Link
	switch m.AttachKind {
	case PluginAttachTracepoint:
		category, name, err := splitTracepointTarget(m.AttachTarget)
		if err != nil {
			coll.Close()
			return err
		}
		attached, err = link.Tracepoint(category, name, prog, nil)
		if err != nil {
			coll.Close()
			return fmt.Errorf("attach tracepoint: %w", err)
		}
	case PluginAttachKprobe:
		attached, err = link.Kprobe(m.AttachTarget, prog, nil)
		if err != nil {
			coll.Close()
			return fmt.Errorf("attach kprobe: %w", err)
		}
	case PluginAttachKretprobe:
		attached, err = link.Kretprobe(m.AttachTarget, prog, nil)
		if err != nil {
			coll.Close()
			return fmt.Errorf("attach kretprobe: %w", err)
		}
	case PluginAttachLSM:
		attached, err = link.AttachLSM(link.LSMOptions{Program: prog})
		if err != nil {
			coll.Close()
			target := strings.TrimSpace(m.AttachTarget)
			if target != "" {
				return fmt.Errorf("attach lsm %s: %w", target, err)
			}
			return fmt.Errorf("attach lsm: %w", err)
		}
	default:
		coll.Close()
		return fmt.Errorf("unsupported attach kind %q", m.AttachKind)
	}
	if err := ctx.Err(); err != nil {
		if attached != nil {
			_ = attached.Close()
		}
		coll.Close()
		return fmt.Errorf("plugin load canceled after attach: %w", err)
	}

	entry := &loadedEBPFPlugin{
		collection: coll,
		links:      []link.Link{attached},
	}
	old, err := installLoadedEBPFPlugin(m.ID, reservation, entry)
	if err != nil {
		closeLoadedPlugin(entry)
		return err
	}
	installed = true
	closeLoadedPlugin(old)
	return nil
}

func reserveEBPFPluginLoad(id string) (*ebpfPluginLoadReservation, error) {
	loadedPluginsMu.Lock()
	defer loadedPluginsMu.Unlock()
	if _, exists := loadingPlugins[id]; exists {
		return nil, fmt.Errorf("plugin %q is already loading", id)
	}
	ids := make(map[string]struct{}, len(loadedPlugins)+len(loadingPlugins))
	for loadedID, entry := range loadedPlugins {
		if loadedEBPFPluginActive(entry) {
			ids[loadedID] = struct{}{}
		}
	}
	for loadingID := range loadingPlugins {
		ids[loadingID] = struct{}{}
	}
	if _, alreadyCounted := ids[id]; !alreadyCounted && len(ids) >= maxLoadedEBPFPlugins {
		return nil, fmt.Errorf("loaded eBPF plugin limit (%d) reached", maxLoadedEBPFPlugins)
	}
	reservation := &ebpfPluginLoadReservation{}
	loadingPlugins[id] = reservation
	return reservation, nil
}

func releaseEBPFPluginLoad(id string, reservation *ebpfPluginLoadReservation) {
	loadedPluginsMu.Lock()
	if loadingPlugins[id] == reservation {
		delete(loadingPlugins, id)
	}
	loadedPluginsMu.Unlock()
}

func installLoadedEBPFPlugin(id string, reservation *ebpfPluginLoadReservation, entry *loadedEBPFPlugin) (*loadedEBPFPlugin, error) {
	loadedPluginsMu.Lock()
	defer loadedPluginsMu.Unlock()
	if loadingPlugins[id] != reservation || reservation.canceled {
		if loadingPlugins[id] == reservation {
			delete(loadingPlugins, id)
		}
		return nil, fmt.Errorf("plugin %q load was canceled", id)
	}
	old := loadedPlugins[id]
	loadedPlugins[id] = entry
	delete(loadingPlugins, id)
	return old, nil
}

// UnloadEBPFPlugin detaches and frees resources for a plugin.
func UnloadEBPFPlugin(id string) {
	loadedPluginsMu.Lock()
	if reservation := loadingPlugins[id]; reservation != nil {
		reservation.canceled = true
	}
	entry := loadedPlugins[id]
	delete(loadedPlugins, id)
	loadedPluginsMu.Unlock()
	closeLoadedPlugin(entry)
}

func closeLoadedPlugin(entry *loadedEBPFPlugin) {
	if entry == nil {
		return
	}
	for _, l := range entry.links {
		_ = l.Close()
	}
	if entry.collection != nil {
		entry.collection.Close()
	}
}

func splitTracepointTarget(target string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(target), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("tracepoint target must be category/name, got %q", target)
	}
	if !userBPFTracepointComponentPattern.MatchString(parts[0]) ||
		!userBPFTracepointComponentPattern.MatchString(parts[1]) ||
		strings.Contains(parts[0], "..") || strings.Contains(parts[1], "..") {
		return "", "", fmt.Errorf("invalid tracepoint target %q", target)
	}
	return parts[0], parts[1], nil
}

func loadedEBPFPluginActive(entry *loadedEBPFPlugin) bool {
	return entry != nil && (entry.collection != nil || len(entry.links) != 0)
}

func ebpfPluginRuntimeState(id string) (bool, string) {
	loadedPluginsMu.Lock()
	defer loadedPluginsMu.Unlock()
	entry, ok := loadedPlugins[id]
	if !ok {
		return false, ""
	}
	return loadedEBPFPluginActive(entry), entry.loadError
}

// ReapplyEBPFPluginsOnBoot brings up all enabled eBPF plugins after startup.
func ReapplyEBPFPluginsOnBoot() {
	for _, m := range pluginRegistry.List() {
		if m.Kind != PluginKindEBPF || !m.Enabled {
			continue
		}
		copyM := m
		if err := LoadEBPFPlugin(&copyM); err != nil {
			loadedPluginsMu.Lock()
			loadedPlugins[m.ID] = &loadedEBPFPlugin{loadError: err.Error()}
			loadedPluginsMu.Unlock()
		}
	}
}

type BPFTemplate = types.BPFTemplate

// bpfTemplates returns a fixed set of starter snippets for the online builder.
func bpfTemplates() []BPFTemplate {
	return []BPFTemplate{
		{
			ID:           "trace-execve",
			Name:         "Trace execve",
			Description:  "Logs every process exec via the sys_enter_execve tracepoint.",
			AttachKind:   PluginAttachTracepoint,
			AttachTarget: "syscalls/sys_enter_execve",
			ProgramName:  "trace_execve",
			Source: `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";

SEC("tracepoint/syscalls/sys_enter_execve")
int trace_execve(void *ctx) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    bpf_printk("execve by %s\n", comm);
    return 0;
}
`,
		},
		{
			ID:           "count-openat",
			Name:         "Count openat",
			Description:  "Increments a per-CPU array entry every time openat is invoked.",
			AttachKind:   PluginAttachTracepoint,
			AttachTarget: "syscalls/sys_enter_openat",
			ProgramName:  "count_openat",
			Source: `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>

char LICENSE[] SEC("license") = "GPL";

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __type(key, u32);
    __type(value, u64);
    __uint(max_entries, 1);
} openat_count SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_openat")
int count_openat(void *ctx) {
    u32 key = 0;
    u64 *val = bpf_map_lookup_elem(&openat_count, &key);
    if (val) __sync_fetch_and_add(val, 1);
    return 0;
}
`,
		},
		{
			ID:           "kprobe-unlink",
			Name:         "Kprobe do_unlinkat",
			Description:  "Prints the comm of every process that calls do_unlinkat.",
			AttachKind:   PluginAttachKprobe,
			AttachTarget: "do_unlinkat",
			ProgramName:  "trace_unlink",
			Source: `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";

SEC("kprobe/do_unlinkat")
int trace_unlink(struct pt_regs *ctx) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    bpf_printk("unlink by %s\n", comm);
    return 0;
}
`,
		},
	}
}
