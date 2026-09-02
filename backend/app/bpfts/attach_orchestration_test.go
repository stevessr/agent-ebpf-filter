package bpfts

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cilium/ebpf"
)

type fakeRuntimeLink struct {
	name   string
	closed *[]string
	err    error
}

func (link *fakeRuntimeLink) Close() error {
	*link.closed = append(*link.closed, link.name)
	return link.err
}

func TestAttachProbeSetUsesManifestOrder(t *testing.T) {
	manifest := Manifest{
		Version: ManifestVersion,
		Source:  "order.ts",
		Probes: []ManifestProbe{
			{Name: "entry", Kind: "uprobe", Section: "uprobe", Target: "SSL_read"},
			{Name: "exit", Kind: "uretprobe", Section: "uretprobe", Target: "SSL_read"},
			{Name: "kernel", Kind: "kretprobe", Section: "kretprobe/do_sys_open", Target: "do_sys_open"},
		},
	}
	programs := map[string]*ebpf.Program{
		"entry":  new(ebpf.Program),
		"exit":   new(ebpf.Program),
		"kernel": new(ebpf.Program),
	}
	var attached []string
	var closed []string
	attach := func(probe ManifestProbe, _ *ebpf.Program, _ LoadOptions) (runtimeLink, error) {
		attached = append(attached, probe.Name+":"+probe.Kind)
		return &fakeRuntimeLink{name: probe.Name, closed: &closed}, nil
	}
	links, err := attachProbeSet(programs, manifest, LoadOptions{}, attach)
	if err != nil {
		t.Fatalf("attachProbeSet() error = %v", err)
	}
	if got, want := attached, []string{"entry:uprobe", "exit:uretprobe", "kernel:kretprobe"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("attach order = %v, want %v", got, want)
	}
	if len(links) != 3 {
		t.Fatalf("attached link count = %d, want 3", len(links))
	}
	if len(closed) != 0 {
		t.Fatalf("successful attach unexpectedly closed links: %v", closed)
	}
}

func TestAttachProbeSetRollsBackInReverseOrder(t *testing.T) {
	manifest := Manifest{
		Version: ManifestVersion,
		Source:  "rollback.ts",
		Probes: []ManifestProbe{
			{Name: "first", Kind: "kprobe", Section: "kprobe/a", Target: "a"},
			{Name: "second", Kind: "kretprobe", Section: "kretprobe/b", Target: "b"},
			{Name: "third", Kind: "tracepoint", Section: "tracepoint/syscalls/sys_enter_openat", Category: "syscalls", Event: "sys_enter_openat"},
		},
	}
	programs := map[string]*ebpf.Program{
		"first":  new(ebpf.Program),
		"second": new(ebpf.Program),
		"third":  new(ebpf.Program),
	}
	var closed []string
	attach := func(probe ManifestProbe, _ *ebpf.Program, _ LoadOptions) (runtimeLink, error) {
		if probe.Name == "third" {
			return nil, errors.New("synthetic attach failure")
		}
		return &fakeRuntimeLink{name: probe.Name, closed: &closed}, nil
	}
	links, err := attachProbeSet(programs, manifest, LoadOptions{}, attach)
	if err == nil || !strings.Contains(err.Error(), "synthetic attach failure") {
		t.Fatalf("expected attach failure, got links=%v err=%v", links, err)
	}
	if links != nil {
		t.Fatalf("failed attach returned live links: %v", links)
	}
	if got, want := closed, []string{"second", "first"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rollback close order = %v, want %v", got, want)
	}
}

func TestAttachProbeSetJoinsCleanupErrors(t *testing.T) {
	manifest := Manifest{
		Version: ManifestVersion,
		Source:  "cleanup.ts",
		Probes: []ManifestProbe{
			{Name: "first", Kind: "kprobe", Section: "kprobe/a", Target: "a"},
			{Name: "second", Kind: "kprobe", Section: "kprobe/b", Target: "b"},
		},
	}
	programs := map[string]*ebpf.Program{
		"first":  new(ebpf.Program),
		"second": new(ebpf.Program),
	}
	var closed []string
	attach := func(probe ManifestProbe, _ *ebpf.Program, _ LoadOptions) (runtimeLink, error) {
		if probe.Name == "second" {
			return nil, errors.New("attach failed")
		}
		return &fakeRuntimeLink{name: probe.Name, closed: &closed, err: errors.New("close failed")}, nil
	}
	_, err := attachProbeSet(programs, manifest, LoadOptions{}, attach)
	if err == nil || !strings.Contains(err.Error(), "attach failed") || !strings.Contains(err.Error(), "close failed") {
		t.Fatalf("expected joined attach/cleanup error, got %v", err)
	}
	if got, want := closed, []string{"first"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanup close order = %v, want %v", got, want)
	}
}
