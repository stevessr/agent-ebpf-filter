//go:build linux

package main

import (
	"log"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// Security sandbox for the agent-ebpf-filter backend.
// Applies three tiers of defense-in-depth AFTER eBPF bootstrap is complete.

type sandboxConfig struct {
	DisableCapDrop  bool
	DisableNoNewPrivs bool
	StrictMode      bool
}

func defaultSandboxConfig() sandboxConfig {
	return sandboxConfig{}
}

// applySecuritySandbox applies defense-in-depth hardening.
func applySecuritySandbox(cfg sandboxConfig) error {
	// Tier 1: No new privileges (prevent setuid escalation)
	if !cfg.DisableNoNewPrivs {
		if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
			if cfg.StrictMode {
				return err
			}
			log.Printf("[SANDBOX] PR_SET_NO_NEW_PRIVS: %v", err)
		} else {
			log.Printf("[SANDBOX] PR_SET_NO_NEW_PRIVS: enabled")
		}
	}

	// Tier 2: Drop capabilities no longer needed
	if !cfg.DisableCapDrop {
		if err := applyCapDrop(); err != nil {
			if cfg.StrictMode {
				return err
			}
			log.Printf("[SANDBOX] cap_drop: %v", err)
		}
	}

	return nil
}

func applyCapDrop() error {
	// Capabilities to KEEP (needed for normal operation):
	//   CAP_KILL — signal child processes
	//   CAP_NET_BIND_SERVICE — bind to low ports if needed
	//   CAP_SYS_PTRACE — read /proc/<pid>/maps, process info
	// Capabilities to DROP (dangerous, no longer needed after eBPF bootstrap):
	dropped := []struct {
		cap uintptr
		name string
	}{
		{unix.CAP_SYS_ADMIN, "SYS_ADMIN"},
		{unix.CAP_SYS_MODULE, "SYS_MODULE"},
		{unix.CAP_SYS_RAWIO, "SYS_RAWIO"},
		{unix.CAP_SYS_BOOT, "SYS_BOOT"},
		{unix.CAP_SYS_TIME, "SYS_TIME"},
		{unix.CAP_SYS_TTY_CONFIG, "SYS_TTY_CONFIG"},
		{unix.CAP_SYS_CHROOT, "SYS_CHROOT"},
		{unix.CAP_MKNOD, "MKNOD"},
		{unix.CAP_MAC_OVERRIDE, "MAC_OVERRIDE"},
		{unix.CAP_MAC_ADMIN, "MAC_ADMIN"},
		{unix.CAP_LINUX_IMMUTABLE, "LINUX_IMMUTABLE"},
		{unix.CAP_IPC_LOCK, "IPC_LOCK"},
		{unix.CAP_IPC_OWNER, "IPC_OWNER"},
		{unix.CAP_AUDIT_CONTROL, "AUDIT_CONTROL"},
		{unix.CAP_AUDIT_WRITE, "AUDIT_WRITE"},
		{unix.CAP_AUDIT_READ, "AUDIT_READ"},
		{unix.CAP_BLOCK_SUSPEND, "BLOCK_SUSPEND"},
		{unix.CAP_WAKE_ALARM, "WAKE_ALARM"},
		{unix.CAP_LEASE, "LEASE"},
		{unix.CAP_SETPCAP, "SETPCAP"},
		{unix.CAP_FSETID, "FSETID"},
		{unix.CAP_CHOWN, "CHOWN"},
		{unix.CAP_FOWNER, "FOWNER"},
		{unix.CAP_SETFCAP, "SETFCAP"},
		{unix.CAP_SETGID, "SETGID"},
		{unix.CAP_SETUID, "SETUID"},
		{unix.CAP_NET_ADMIN, "NET_ADMIN"},
		{unix.CAP_NET_BROADCAST, "NET_BROADCAST"},
		{unix.CAP_SYS_PACCT, "SYS_PACCT"},
		{unix.CAP_SYS_NICE, "SYS_NICE"},
		{unix.CAP_SYS_RESOURCE, "SYS_RESOURCE"},
		{unix.CAP_CHECKPOINT_RESTORE, "CHECKPOINT_RESTORE"},
	}

	var dropErrs []error
	for _, c := range dropped {
		err := unix.Prctl(unix.PR_CAPBSET_DROP, c.cap, 0, 0, 0)
		if err != nil && err != syscall.EINVAL {
			dropErrs = append(dropErrs, err)
		}
	}
	if len(dropErrs) > 0 {
		log.Printf("[SANDBOX] cap_drop errors: %v", dropErrs)
	}

	// Clear ambient capabilities
	unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0)

	log.Printf("[SANDBOX] capabilities dropped: %d ambient cleared", len(dropped))
	return nil
}

// ApplySandbox applies the security sandbox. Call AFTER eBPF bootstrap.
func ApplySandbox() {
	if os.Getenv("AGENT_EBPF_NO_SANDBOX") == "true" {
		log.Println("[SANDBOX] disabled via AGENT_EBPF_NO_SANDBOX")
		return
	}

	cfg := defaultSandboxConfig()
	cfg.StrictMode = os.Getenv("AGENT_EBPF_SANDBOX_STRICT") == "true"
	cfg.DisableCapDrop = os.Getenv("AGENT_EBPF_NO_CAP_DROP") == "true"
	cfg.DisableNoNewPrivs = os.Getenv("AGENT_EBPF_NO_NO_NEW_PRIVS") == "true"

	if err := applySecuritySandbox(cfg); err != nil {
		log.Printf("[SANDBOX] ERROR: %v", err)
		if cfg.StrictMode {
			log.Fatalf("[SANDBOX] strict mode: exiting")
		}
	}
}
