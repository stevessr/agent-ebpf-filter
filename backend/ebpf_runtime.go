package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

const bootstrapFlag = "--ebpf-bootstrap"

// mapNames defines the required pinned eBPF maps.
var mapNames = []string{"agent_pids", "events", "tracked_comms", "tracked_paths", "tracked_prefixes", "exit_ctx", "exit_path_buf", "exit_path_ctx"}

var trackerAttachSpecs = []struct {
	category, name, pinName string
	program                 func(*bpf.AgentTrackerObjects) *ebpf.Program
}{
	// Existing sys_enter
	{"syscalls", "sys_enter_execve", "sys_enter_execve", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterExecve }},
	{"syscalls", "sys_enter_openat", "sys_enter_openat", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterOpenat }},
	{"syscalls", "sys_enter_connect", "sys_enter_connect", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterConnect }},
	{"syscalls", "sys_enter_mkdirat", "sys_enter_mkdirat", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterMkdirat }},
	{"syscalls", "sys_enter_unlinkat", "sys_enter_unlinkat", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterUnlinkat }},
	{"syscalls", "sys_enter_ioctl", "sys_enter_ioctl", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterIoctl }},
	{"syscalls", "sys_enter_bind", "sys_enter_bind", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterBind }},
	{"syscalls", "sys_enter_sendto", "sys_enter_sendto", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterSendto }},
	{"syscalls", "sys_enter_recvfrom", "sys_enter_recvfrom", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterRecvfrom }},
	// New sys_enter
	{"syscalls", "sys_enter_read", "sys_enter_read", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterRead }},
	{"syscalls", "sys_enter_write", "sys_enter_write", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterWrite }},
	{"syscalls", "sys_enter_open", "sys_enter_open", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterOpen }},
	{"syscalls", "sys_enter_chmod", "sys_enter_chmod", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterChmod }},
	{"syscalls", "sys_enter_chown", "sys_enter_chown", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterChown }},
	{"syscalls", "sys_enter_rename", "sys_enter_rename", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterRename }},
	{"syscalls", "sys_enter_link", "sys_enter_link", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterLink }},
	{"syscalls", "sys_enter_symlink", "sys_enter_symlink", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterSymlink }},
	{"syscalls", "sys_enter_mknod", "sys_enter_mknod", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterMknod }},
	{"syscalls", "sys_enter_clone", "sys_enter_clone", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterClone }},
	{"syscalls", "sys_enter_exit_group", "sys_enter_exit_group", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterExitGroup }},
	{"syscalls", "sys_enter_socket", "sys_enter_socket", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterSocket }},
	{"syscalls", "sys_enter_accept", "sys_enter_accept", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterAccept }},
	{"syscalls", "sys_enter_accept4", "sys_enter_accept4", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysEnterAccept4 }},
	// sys_exit for ALL 22 syscalls
	{"syscalls", "sys_exit_execve", "sys_exit_execve", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitExecve }},
	{"syscalls", "sys_exit_openat", "sys_exit_openat", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitOpenat }},
	{"syscalls", "sys_exit_connect", "sys_exit_connect", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitConnect }},
	{"syscalls", "sys_exit_mkdirat", "sys_exit_mkdirat", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitMkdirat }},
	{"syscalls", "sys_exit_unlinkat", "sys_exit_unlinkat", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitUnlinkat }},
	{"syscalls", "sys_exit_ioctl", "sys_exit_ioctl", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitIoctl }},
	{"syscalls", "sys_exit_bind", "sys_exit_bind", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitBind }},
	{"syscalls", "sys_exit_sendto", "sys_exit_sendto", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitSendto }},
	{"syscalls", "sys_exit_recvfrom", "sys_exit_recvfrom", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitRecvfrom }},
	{"syscalls", "sys_exit_read", "sys_exit_read", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitRead }},
	{"syscalls", "sys_exit_write", "sys_exit_write", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitWrite }},
	{"syscalls", "sys_exit_open", "sys_exit_open", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitOpen }},
	{"syscalls", "sys_exit_chmod", "sys_exit_chmod", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitChmod }},
	{"syscalls", "sys_exit_chown", "sys_exit_chown", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitChown }},
	{"syscalls", "sys_exit_rename", "sys_exit_rename", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitRename }},
	{"syscalls", "sys_exit_link", "sys_exit_link", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitLink }},
	{"syscalls", "sys_exit_symlink", "sys_exit_symlink", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitSymlink }},
	{"syscalls", "sys_exit_mknod", "sys_exit_mknod", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitMknod }},
	{"syscalls", "sys_exit_clone", "sys_exit_clone", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitClone }},
	{"syscalls", "sys_exit_exit_group", "sys_exit_exit_group", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitExitGroup }},
	{"syscalls", "sys_exit_socket", "sys_exit_socket", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitSocket }},
	{"syscalls", "sys_exit_accept", "sys_exit_accept", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitAccept }},
	{"syscalls", "sys_exit_accept4", "sys_exit_accept4", func(o *bpf.AgentTrackerObjects) *ebpf.Program { return o.TracepointSyscallsSysExitAccept4 }},
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
		// Preserve tracked data before closing old map handles
		backup := extractTrackedData(replacements)
		closeMapHandles(replacements)
		// Fall through to fresh bootstrap but restore backed-up data
		defer restoreTrackedData(backup)
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
		"tracked_prefixes": objs.TrackedPrefixes, "exit_ctx": objs.ExitCtx,
		"exit_path_buf": objs.ExitPathBuf, "exit_path_ctx": objs.ExitPathCtx,
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
	cmd.Env = os.Environ()

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
		AgentPids:       maps["agent_pids"],
		Events:          maps["events"],
		TrackedComms:    maps["tracked_comms"],
		TrackedPaths:    maps["tracked_paths"],
		TrackedPrefixes: maps["tracked_prefixes"],
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
		sudoArgs := []string{"--preserve-env=AGENT_WRAPPER_PATH,DISPLAY,WAYLAND_DISPLAY,XAUTHORITY,USER,HOME,AGENT_REAL_HOME,GIN_MODE,DISABLE_AUTH", exe}
		sudoArgs = append(sudoArgs, args...)
		return exec.Command(priv, sudoArgs...)
	}

	cmdArgs := append([]string{exe}, args...)
	cmd := exec.Command(priv, cmdArgs...)

	// Manual environment inheritance for non-sudo escalators (like pkexec)
	// We want to pass down only selected safe/required variables.
	whitelist := map[string]bool{
		"AGENT_WRAPPER_PATH": true,
		"AGENT_REAL_HOME":    true,
		"GIN_MODE":           true,
		"DISABLE_AUTH":       true,
		"DISPLAY":            true,
		"WAYLAND_DISPLAY":    true,
		"XAUTHORITY":         true,
	}

	inherited := os.Environ()
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 && whitelist[parts[0]] {
			inherited = setEnvValue(inherited, parts[0], parts[1])
		}
	}
	cmd.Env = inherited

	return cmd
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

// ── tracked data preservation ─────────────────────────────────────────────────

type trackedDataBackup struct {
	Comms    map[[16]byte]uint32
	Paths    map[[256]byte]uint32
	Prefixes []prefixBackupEntry
}

type prefixBackupEntry struct {
	Data      [64]byte
	PrefixLen uint32
	Value     uint32
}

func extractTrackedData(maps map[string]*ebpf.Map) *trackedDataBackup {
	b := &trackedDataBackup{
		Comms: make(map[[16]byte]uint32),
		Paths: make(map[[256]byte]uint32),
	}

	if m, ok := maps["tracked_comms"]; ok && m != nil {
		iter := m.Iterate()
		var k [16]byte
		var v uint32
		for iter.Next(&k, &v) {
			b.Comms[k] = v
		}
	}

	if m, ok := maps["tracked_paths"]; ok && m != nil {
		iter := m.Iterate()
		var k [256]byte
		var v uint32
		for iter.Next(&k, &v) {
			b.Paths[k] = v
		}
	}

	if m, ok := maps["tracked_prefixes"]; ok && m != nil {
		iter := m.Iterate()
		for {
			var k struct {
				PrefixLen uint32
				Data      [64]byte
			}
			var v uint32
			if !iter.Next(&k, &v) {
				break
			}
			b.Prefixes = append(b.Prefixes, prefixBackupEntry{
				Data:      k.Data,
				PrefixLen: k.PrefixLen,
				Value:     v,
			})
		}
	}

	return b
}

func restoreTrackedData(backup *trackedDataBackup) {
	if backup == nil {
		return
	}

	maps, err := loadPinnedMapHandles()
	if err != nil {
		return
	}
	defer closeMapHandles(maps)

	if m, ok := maps["tracked_comms"]; ok && m != nil {
		for k, v := range backup.Comms {
			_ = m.Put(k, v)
		}
	}

	if m, ok := maps["tracked_paths"]; ok && m != nil {
		for k, v := range backup.Paths {
			_ = m.Put(k, v)
		}
	}

	if m, ok := maps["tracked_prefixes"]; ok && m != nil {
		for _, entry := range backup.Prefixes {
			k := struct {
				PrefixLen uint32
				Data      [64]byte
			}{
				PrefixLen: entry.PrefixLen,
				Data:      entry.Data,
			}
			_ = m.Put(k, entry.Value)
		}
	}
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
	for _, mp := range []*(*ebpf.Map){&set.AgentPids, &set.Events, &set.TrackedComms, &set.TrackedPaths, &set.TrackedPrefixes} {
		if *mp != nil {
			_ = (*mp).Close()
			*mp = nil
		}
	}
}
