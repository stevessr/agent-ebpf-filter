package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

const bootstrapFlag = "--ebpf-bootstrap"

type trackerAttachSpec struct {
	category string
	name     string
	pinName  string
	program  func(*bpf.AgentTrackerObjects) *ebpf.Program
}

var trackerAttachSpecs = []trackerAttachSpec{
	{category: "syscalls", name: "sys_enter_execve", pinName: "sys_enter_execve", program: func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterExecve }},
	{category: "syscalls", name: "sys_enter_openat", pinName: "sys_enter_openat", program: func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterOpenat }},
	{category: "syscalls", name: "sys_enter_connect", pinName: "sys_enter_connect", program: func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterConnect }},
	{category: "syscalls", name: "sys_enter_mkdirat", pinName: "sys_enter_mkdirat", program: func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterMkdirat }},
	{category: "syscalls", name: "sys_enter_unlinkat", pinName: "sys_enter_unlinkat", program: func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterUnlinkat }},
	{category: "syscalls", name: "sys_enter_ioctl", pinName: "sys_enter_ioctl", program: func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterIoctl }},
	{category: "syscalls", name: "sys_enter_bind", pinName: "sys_enter_bind", program: func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterBind }},
}

func isBootstrapMode() bool {
	if os.Getenv("AGENT_EBPF_BOOTSTRAP") == "1" {
		return true
	}
	for _, arg := range os.Args[1:] {
		if arg == bootstrapFlag {
			return true
		}
	}
	return false
}

func ensureTrackerMapsLoaded() error {
	if err := loadPinnedTrackerMaps(); err == nil {
		return nil
	}
	if err := bootstrapTrackerMaps(); err != nil {
		return err
	}
	return loadPinnedTrackerMaps()
}

func bootstrapTrackerMaps() error {
	if os.Geteuid() == 0 {
		return performTrackerBootstrap()
	}
	return launchPrivilegedTrackerBootstrap()
}

func launchPrivilegedTrackerBootstrap() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	command, err := privilegeBootstrapCommand()
	if err != nil {
		return err
	}

	cmd := exec.Command(command, executable, bootstrapFlag)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("launch privileged eBPF bootstrap: %w", err)
	}
	return nil
}

func privilegeBootstrapCommand() (string, error) {
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		if path, err := exec.LookPath("pkexec"); err == nil {
			return path, nil
		}
	}
	if path, err := exec.LookPath("sudo"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("pkexec"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("eBPF bootstrap requires root privileges; install sudo or pkexec")
}

func performTrackerBootstrap() (err error) {
	if err = rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("raise memlock limit: %w", err)
	}

	if replacements, loadErr := loadPinnedTrackerMapReplacements(); loadErr == nil {
		var objs bpf.AgentTrackerObjects
		opts := &ebpf.CollectionOptions{MapReplacements: replacements}
		if err = bpf.LoadAgentTrackerObjects(&objs, opts); err == nil {
			defer objs.Close()
			defer closeTrackerMapReplacements(replacements)
			if err = resetTrackerLinkPins(); err != nil {
				return err
			}
			defer func() {
				if err != nil {
					_ = os.RemoveAll(ebpfPinLinksDir)
				}
			}()
			if err = pinTrackerLinks(&objs); err == nil {
				return nil
			}
			return err
		}
		closeTrackerMapReplacements(replacements)
		return fmt.Errorf("load AgentTracker eBPF objects with pinned maps: %w", err)
	}

	if err = resetTrackerRootPins(); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(ebpfPinRoot)
		}
	}()

	var objs bpf.AgentTrackerObjects
	if err = bpf.LoadAgentTrackerObjects(&objs, nil); err != nil {
		return fmt.Errorf("load AgentTracker eBPF objects: %w", err)
	}
	defer objs.Close()

	if err = pinTrackerObjects(&objs); err != nil {
		return err
	}
	if err = resetTrackerLinkPins(); err != nil {
		return err
	}
	if err = pinTrackerLinks(&objs); err != nil {
		return err
	}

	return nil
}

func ensureTrackerPinDirs() error {
	if err := os.MkdirAll(ebpfPinMapsDir, 0755); err != nil {
		return fmt.Errorf("create eBPF map pin dir: %w", err)
	}
	if err := os.MkdirAll(ebpfPinLinksDir, 0755); err != nil {
		return fmt.Errorf("create eBPF link pin dir: %w", err)
	}
	return nil
}

func resetTrackerRootPins() error {
	if err := os.RemoveAll(ebpfPinRoot); err != nil {
		return fmt.Errorf("reset eBPF pin root: %w", err)
	}
	if err := ensureTrackerPinDirs(); err != nil {
		return err
	}
	if err := os.Chmod(ebpfPinRoot, 0755); err != nil {
		return fmt.Errorf("chmod eBPF pin root: %w", err)
	}
	if err := os.Chmod(ebpfPinMapsDir, 0755); err != nil {
		return fmt.Errorf("chmod eBPF map pin dir: %w", err)
	}
	if err := os.Chmod(ebpfPinLinksDir, 0755); err != nil {
		return fmt.Errorf("chmod eBPF link pin dir: %w", err)
	}
	return nil
}

func resetTrackerLinkPins() error {
	if err := os.RemoveAll(ebpfPinLinksDir); err != nil {
		return fmt.Errorf("reset eBPF link pins: %w", err)
	}
	if err := os.MkdirAll(ebpfPinLinksDir, 0755); err != nil {
		return fmt.Errorf("create eBPF link pin dir: %w", err)
	}
	if err := os.Chmod(ebpfPinLinksDir, 0755); err != nil {
		return fmt.Errorf("chmod eBPF link pin dir: %w", err)
	}
	return nil
}

func loadPinnedTrackerMapReplacements() (map[string]*ebpf.Map, error) {
	replacements := map[string]*ebpf.Map{}
	cleanup := func() {
		closeTrackerMapReplacements(replacements)
	}

	for _, entry := range []struct {
		name string
		path string
	}{
		{name: "agent_pids", path: filepath.Join(ebpfPinMapsDir, "agent_pids")},
		{name: "events", path: filepath.Join(ebpfPinMapsDir, "events")},
		{name: "tracked_comms", path: filepath.Join(ebpfPinMapsDir, "tracked_comms")},
		{name: "tracked_paths", path: filepath.Join(ebpfPinMapsDir, "tracked_paths")},
	} {
		m, err := ebpf.LoadPinnedMap(entry.path, nil)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("load pinned map %s: %w", entry.name, err)
		}
		replacements[entry.name] = m
	}

	return replacements, nil
}

func pinTrackerObjects(objs *bpf.AgentTrackerObjects) error {
	pins := []struct {
		name string
		mapv *ebpf.Map
	}{
		{name: "agent_pids", mapv: objs.AgentPids},
		{name: "events", mapv: objs.Events},
		{name: "tracked_comms", mapv: objs.TrackedComms},
		{name: "tracked_paths", mapv: objs.TrackedPaths},
	}

	for _, pin := range pins {
		if pin.mapv == nil {
			return fmt.Errorf("eBPF map %s is nil", pin.name)
		}
		path := filepath.Join(ebpfPinMapsDir, pin.name)
		if err := pin.mapv.Pin(path); err != nil {
			return fmt.Errorf("pin eBPF map %s: %w", pin.name, err)
		}
		makePinnedObjectAccessible(path)
	}

	return nil
}

func pinTrackerLinks(objs *bpf.AgentTrackerObjects) error {
	for _, spec := range trackerAttachSpecs {
		prog := spec.program(objs)
		if prog == nil {
			return fmt.Errorf("eBPF program for %s/%s is nil", spec.category, spec.name)
		}

		l, err := link.Tracepoint(spec.category, spec.name, prog, nil)
		if err != nil {
			return fmt.Errorf("attach tracepoint %s/%s: %w", spec.category, spec.name, err)
		}

		path := filepath.Join(ebpfPinLinksDir, spec.pinName)
		if err := l.Pin(path); err != nil {
			_ = l.Close()
			return fmt.Errorf("pin tracepoint link %s: %w", spec.pinName, err)
		}
		makePinnedObjectAccessible(path)
		_ = l.Close()
	}
	return nil
}

func makePinnedObjectAccessible(path string) {
	if uid, gid, ok := resolveBootstrapOwner(); ok {
		if err := os.Chown(path, uid, gid); err == nil {
			_ = os.Chmod(path, 0600)
			return
		}
	}
	_ = os.Chmod(path, 0666)
}

func resolveBootstrapOwner() (int, int, bool) {
	if uidStr := os.Getenv("SUDO_UID"); uidStr != "" {
		uid, err := strconv.Atoi(uidStr)
		if err != nil {
			return -1, -1, false
		}
		gid := -1
		if gidStr := os.Getenv("SUDO_GID"); gidStr != "" {
			if parsed, err := strconv.Atoi(gidStr); err == nil {
				gid = parsed
			}
		}
		return uid, gid, true
	}
	if uidStr := os.Getenv("PKEXEC_UID"); uidStr != "" {
		uid, err := strconv.Atoi(uidStr)
		if err != nil {
			return -1, -1, false
		}
		return uid, -1, true
	}
	return -1, -1, false
}

func loadPinnedTrackerMaps() (err error) {
	loaded := trackerMapSet{}
	defer func() {
		if err != nil {
			closeTrackerMapSet(&loaded)
		}
	}()

	if loaded.AgentPids, err = ebpf.LoadPinnedMap(filepath.Join(ebpfPinMapsDir, "agent_pids"), nil); err != nil {
		return fmt.Errorf("load pinned map agent_pids: %w", err)
	}
	if loaded.Events, err = ebpf.LoadPinnedMap(filepath.Join(ebpfPinMapsDir, "events"), nil); err != nil {
		return fmt.Errorf("load pinned map events: %w", err)
	}
	if loaded.TrackedComms, err = ebpf.LoadPinnedMap(filepath.Join(ebpfPinMapsDir, "tracked_comms"), nil); err != nil {
		return fmt.Errorf("load pinned map tracked_comms: %w", err)
	}
	if loaded.TrackedPaths, err = ebpf.LoadPinnedMap(filepath.Join(ebpfPinMapsDir, "tracked_paths"), nil); err != nil {
		return fmt.Errorf("load pinned map tracked_paths: %w", err)
	}

	closeTrackerMapSet(&trackerMaps)
	trackerMaps = loaded
	return nil
}

func closeTrackerMapSet(set *trackerMapSet) {
	if set == nil {
		return
	}
	if set.AgentPids != nil {
		_ = set.AgentPids.Close()
		set.AgentPids = nil
	}
	if set.Events != nil {
		_ = set.Events.Close()
		set.Events = nil
	}
	if set.TrackedComms != nil {
		_ = set.TrackedComms.Close()
		set.TrackedComms = nil
	}
	if set.TrackedPaths != nil {
		_ = set.TrackedPaths.Close()
		set.TrackedPaths = nil
	}
}

func closeTrackerMapReplacements(replacements map[string]*ebpf.Map) {
	for name, m := range replacements {
		if m != nil {
			_ = m.Close()
			replacements[name] = nil
		}
	}
}
