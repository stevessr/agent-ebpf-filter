package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

const bootstrapFlag = "--ebpf-bootstrap"

// mapNames defines the required pinned eBPF maps.
var mapNames = []string{"agent_pids", "events", "tracked_comms", "tracked_paths"}

var trackerAttachSpecs = []struct {
	category, name, pinName string
	program                 func(*bpf.AgentTrackerObjects) *ebpf.Program
}{
	{"syscalls", "sys_enter_execve", "sys_enter_execve", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterExecve }},
	{"syscalls", "sys_enter_openat", "sys_enter_openat", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterOpenat }},
	{"syscalls", "sys_enter_connect", "sys_enter_connect", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterConnect }},
	{"syscalls", "sys_enter_mkdirat", "sys_enter_mkdirat", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterMkdirat }},
	{"syscalls", "sys_enter_unlinkat", "sys_enter_unlinkat", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterUnlinkat }},
	{"syscalls", "sys_enter_ioctl", "sys_enter_ioctl", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterIoctl }},
	{"syscalls", "sys_enter_bind", "sys_enter_bind", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterBind }},
}

// ── mode detection ────────────────────────────────────────────────────────────

func isBootstrapMode() bool {
	for _, a := range os.Args[1:] {
		if a == bootstrapFlag {
			return true
		}
	}
	return os.Getenv("AGENT_EBPF_BOOTSTRAP") == "1"
}

// ── privileged bootstrap mode ─────────────────────────────────────────────────

// bootstrapTrackerMaps runs as root and ensures maps/links are pinned in bpffs.
func bootstrapTrackerMaps() error {
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("raise memlock: %w", err)
	}
	maps, err := doBootstrap()
	if err != nil {
		return err
	}
	defer closeMapHandles(maps)
	return nil
}

func doBootstrap() (map[string]*ebpf.Map, error) {
	if replacements, err := loadPinnedMapHandles(); err == nil {
		var objs bpf.AgentTrackerObjects
		if err := bpf.LoadAgentTrackerObjects(&objs, &ebpf.CollectionOptions{MapReplacements: replacements}); err == nil {
			defer objs.Close()
			_ = os.RemoveAll(ebpfPinLinksDir)
			_ = os.MkdirAll(ebpfPinLinksDir, 0755)
			if err := pinLinks(&objs); err != nil {
				closeMapHandles(replacements)
				return nil, err
			}
			if err := ensurePinnedMapPermissions(); err != nil {
				closeMapHandles(replacements)
				return nil, err
			}
			return replacements, nil
		}
		closeMapHandles(replacements)
	}

	_ = os.RemoveAll(ebpfPinRoot)
	for _, d := range []string{ebpfPinMapsDir, ebpfPinLinksDir} {
		_ = os.MkdirAll(d, 0755)
	}
	var objs bpf.AgentTrackerObjects
	if err := bpf.LoadAgentTrackerObjects(&objs, nil); err != nil {
		return nil, fmt.Errorf("load eBPF objects: %w", err)
	}
	defer objs.Close()
	if err := pinMaps(&objs); err != nil {
		return nil, err
	}
	if err := pinLinks(&objs); err != nil {
		return nil, err
	}
	if err := ensurePinnedMapPermissions(); err != nil {
		return nil, err
	}
	return loadPinnedMapHandles()
}

func pinMaps(objs *bpf.AgentTrackerObjects) error {
	for name, m := range map[string]*ebpf.Map{
		"agent_pids": objs.AgentPids, "events": objs.Events,
		"tracked_comms": objs.TrackedComms, "tracked_paths": objs.TrackedPaths,
	} {
		if err := m.Pin(filepath.Join(ebpfPinMapsDir, name)); err != nil {
			return fmt.Errorf("pin map %s: %w", name, err)
		}
	}
	return nil
}

func pinLinks(objs *bpf.AgentTrackerObjects) error {
	for _, s := range trackerAttachSpecs {
		l, err := link.Tracepoint(s.category, s.name, s.program(objs), nil)
		if err != nil {
			return fmt.Errorf("attach %s/%s: %w", s.category, s.name, err)
		}
		if err := l.Pin(filepath.Join(ebpfPinLinksDir, s.pinName)); err != nil {
			_ = l.Close()
			return fmt.Errorf("pin link %s: %w", s.pinName, err)
		}
		_ = l.Close()
	}
	return nil
}

// ── service mode ──────────────────────────────────────────────────────────────

func ensureTrackerMapsLoaded() error {
	maps, err := loadPinnedMapHandles()
	if err != nil {
		if os.Geteuid() != 0 {
			return fmt.Errorf("load pinned maps requires elevated backend privileges: %w", err)
		}
		if err := bootstrapTrackerMaps(); err != nil {
			return fmt.Errorf("bootstrap eBPF components: %w", err)
		}
		maps, err = loadPinnedMapHandles()
		if err != nil {
			return fmt.Errorf("load pinned maps after bootstrap: %w", err)
		}
	}

	loaded, err := toTrackerMapSet(maps)
	if err != nil {
		closeMapHandles(maps)
		return err
	}

	closeTrackerMapSet(&trackerMaps)
	trackerMaps = loaded
	return nil
}

func ensureBackendPrivileges() (bool, error) {
	if os.Geteuid() == 0 {
		return false, nil
	}

	realHome, _ := os.UserHomeDir()

	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	priv, err := privilegeEscalationCmd()
	if err != nil {
		return false, err
	}
	cmd := privilegedCommand(priv, exe, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr

	if realHome != "" {
		cmd.Env = setEnvValue(cmd.Env, "AGENT_REAL_HOME", realHome)
	}

	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("start privileged backend: %w", err)
	}
	return true, nil
}

func toTrackerMapSet(maps map[string]*ebpf.Map) (trackerMapSet, error) {
	missing := make([]string, 0, len(mapNames))
	for _, name := range mapNames {
		if maps[name] == nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return trackerMapSet{}, fmt.Errorf("missing pinned maps: %v", missing)
	}
	return trackerMapSet{
		AgentPids:    maps["agent_pids"],
		Events:       maps["events"],
		TrackedComms: maps["tracked_comms"],
		TrackedPaths: maps["tracked_paths"],
	}, nil
}

func ensurePinnedMapPermissions() error {
	for _, dir := range []string{ebpfPinRoot, ebpfPinMapsDir, ebpfPinLinksDir} {
		if err := os.Chmod(dir, 0755); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("chmod %s: %w", dir, err)
		}
	}
	for _, name := range mapNames {
		path := filepath.Join(ebpfPinMapsDir, name)
		if err := os.Chmod(path, 0666); err != nil {
			return fmt.Errorf("chmod map %s: %w", name, err)
		}
	}
	return nil
}

func privilegedCommand(priv, exe string, args ...string) *exec.Cmd {
	if filepath.Base(priv) == "sudo" {
		sudoArgs := []string{"--preserve-env=AGENT_WRAPPER_PATH,DISPLAY,WAYLAND_DISPLAY,USER,HOME,AGENT_REAL_HOME", exe}
		sudoArgs = append(sudoArgs, args...)
		return exec.Command(priv, sudoArgs...)
	}
	cmdArgs := append([]string{exe}, args...)
	return exec.Command(priv, cmdArgs...)
}

func privilegeEscalationCmd() (string, error) {
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		if p, err := exec.LookPath("pkexec"); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("sudo"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("pkexec"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("need sudo or pkexec for privileged backend startup")
}

// ── shared helpers ────────────────────────────────────────────────────────────

func loadPinnedMapHandles() (map[string]*ebpf.Map, error) {
	out := make(map[string]*ebpf.Map, len(mapNames))
	for _, n := range mapNames {
		m, err := ebpf.LoadPinnedMap(filepath.Join(ebpfPinMapsDir, n), nil)
		if err != nil {
			closeMapHandles(out)
			return nil, err
		}
		out[n] = m
	}
	return out, nil
}

func closeMapHandles(maps map[string]*ebpf.Map) {
	for _, m := range maps {
		if m != nil {
			_ = m.Close()
		}
	}
}

func closeTrackerMapSet(set *trackerMapSet) {
	if set == nil {
		return
	}
	for _, mp := range []*(*ebpf.Map){&set.AgentPids, &set.Events, &set.TrackedComms, &set.TrackedPaths} {
		if *mp != nil {
			_ = (*mp).Close()
			*mp = nil
		}
	}
}
