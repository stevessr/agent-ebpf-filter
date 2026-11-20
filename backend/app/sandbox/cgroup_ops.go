package sandbox

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ── Management operations ─────────────────────────────────────────────

func ensureCgroupSandboxSnapshot() (CgroupSandboxSnapshot, error) {
	snap := CurrentCgroupSandboxSnapshot()
	if !snap.Available() || !snap.Attached() {
		if err := ensureCgroupSandboxLoaded(); err != nil {
			return CgroupSandboxSnapshot{}, err
		}
		snap = CurrentCgroupSandboxSnapshot()
	}
	return snap, nil
}

func BlockCgroup(cgroupID uint64) error {
	snap, err := ensureCgroupSandboxSnapshot()
	if err != nil {
		return err
	}
	if snap.CgroupBlocklist == nil {
		return fmt.Errorf("cgroup sandbox not loaded")
	}
	val := uint32(1)
	return snap.CgroupBlocklist.Put(&cgroupID, &val)
}

func UnblockCgroup(cgroupID uint64) error {
	snap, err := ensureCgroupSandboxSnapshot()
	if err != nil {
		return err
	}
	if snap.CgroupBlocklist == nil {
		return fmt.Errorf("cgroup sandbox not loaded")
	}
	return ignoreMissingMapKey(snap.CgroupBlocklist.Delete(&cgroupID))
}

func CgroupIDForPID(pid int, root string) (uint64, string, error) {
	if pid <= 0 {
		return 0, "", fmt.Errorf("invalid pid: %d", pid)
	}
	if strings.TrimSpace(root) == "" {
		var err error
		root, err = cgroupSandboxAttachPath()
		if err != nil {
			return 0, "", err
		}
	}

	data, err := os.ReadFile(filepath.Join("/proc", fmt.Sprintf("%d", pid), "cgroup"))
	if err != nil {
		return 0, "", fmt.Errorf("read pid %d cgroup: %w", pid, err)
	}
	rel, err := unifiedCgroupRelativePath(data)
	if err != nil {
		return 0, "", fmt.Errorf("pid %d unified cgroup: %w", pid, err)
	}
	cgroupPath, err := resolveCgroupPath(root, rel)
	if err != nil {
		return 0, "", err
	}
	id, err := cgroupIDFromPath(cgroupPath)
	if err != nil {
		return 0, "", err
	}
	return id, cgroupPath, nil
}

func BlockIP(ipStr string) error {
	ip, _, err := ParseIP(ipStr)
	if err != nil {
		return err
	}
	snap, err := ensureCgroupSandboxSnapshot()
	if err != nil {
		return err
	}
	val := uint32(1)
	if ip4 := ip.To4(); ip4 != nil {
		if snap.IPBlocklist == nil {
			return fmt.Errorf("cgroup sandbox IPv4 blocklist not loaded")
		}
		ipU32 := binary.BigEndian.Uint32(ip4) // host-order IPv4 key; BPF uses bpf_ntohl(ctx->user_ip4)
		return snap.IPBlocklist.Put(&ipU32, &val)
	}
	key, err := ip6BlockKeyFromIP(ip)
	if err != nil {
		return err
	}
	if snap.IP6Blocklist == nil {
		return fmt.Errorf("cgroup sandbox IPv6 blocklist not loaded")
	}
	return snap.IP6Blocklist.Put(&key, &val)
}

func UnblockIP(ipStr string) error {
	ip, _, err := ParseIP(ipStr)
	if err != nil {
		return err
	}
	snap, err := ensureCgroupSandboxSnapshot()
	if err != nil {
		return err
	}
	if ip4 := ip.To4(); ip4 != nil {
		if snap.IPBlocklist == nil {
			return fmt.Errorf("cgroup sandbox IPv4 blocklist not loaded")
		}
		ipU32 := binary.BigEndian.Uint32(ip4)
		return ignoreMissingMapKey(snap.IPBlocklist.Delete(&ipU32))
	}
	key, err := ip6BlockKeyFromIP(ip)
	if err != nil {
		return err
	}
	if snap.IP6Blocklist == nil {
		return fmt.Errorf("cgroup sandbox IPv6 blocklist not loaded")
	}
	return ignoreMissingMapKey(snap.IP6Blocklist.Delete(&key))
}

func BlockPort(port uint16) error {
	if err := ValidatePort(port); err != nil {
		return err
	}
	snap, err := ensureCgroupSandboxSnapshot()
	if err != nil {
		return err
	}
	if snap.PortBlocklist == nil {
		return fmt.Errorf("cgroup sandbox port blocklist not loaded")
	}
	portU32 := uint32(port)
	val := uint32(1)
	return snap.PortBlocklist.Put(&portU32, &val)
}

func UnblockPort(port uint16) error {
	if err := ValidatePort(port); err != nil {
		return err
	}
	snap, err := ensureCgroupSandboxSnapshot()
	if err != nil {
		return err
	}
	if snap.PortBlocklist == nil {
		return fmt.Errorf("cgroup sandbox port blocklist not loaded")
	}
	portU32 := uint32(port)
	return ignoreMissingMapKey(snap.PortBlocklist.Delete(&portU32))
}

func BlockCIDR(cidrStr string) error {
	ipNet, _, err := ParseCIDR(cidrStr)
	if err != nil {
		return err
	}
	snap, err := ensureCgroupSandboxSnapshot()
	if err != nil {
		return err
	}
	val := uint32(1)
	if _, bits := ipNet.Mask.Size(); bits == 32 {
		if snap.CIDRBlocklist == nil {
			return fmt.Errorf("cgroup sandbox IPv4 CIDR blocklist not loaded")
		}
		key, err := ipv4LPMKeyFromNet(ipNet)
		if err != nil {
			return err
		}
		return snap.CIDRBlocklist.Put(&key, &val)
	}
	if snap.CIDR6Blocklist == nil {
		return fmt.Errorf("cgroup sandbox IPv6 CIDR blocklist not loaded")
	}
	key, err := ipv6LPMKeyFromNet(ipNet)
	if err != nil {
		return err
	}
	return snap.CIDR6Blocklist.Put(&key, &val)
}

func UnblockCIDR(cidrStr string) error {
	ipNet, _, err := ParseCIDR(cidrStr)
	if err != nil {
		return err
	}
	snap, err := ensureCgroupSandboxSnapshot()
	if err != nil {
		return err
	}
	if _, bits := ipNet.Mask.Size(); bits == 32 {
		if snap.CIDRBlocklist == nil {
			return fmt.Errorf("cgroup sandbox IPv4 CIDR blocklist not loaded")
		}
		key, err := ipv4LPMKeyFromNet(ipNet)
		if err != nil {
			return err
		}
		return ignoreMissingMapKey(snap.CIDRBlocklist.Delete(&key))
	}
	if snap.CIDR6Blocklist == nil {
		return fmt.Errorf("cgroup sandbox IPv6 CIDR blocklist not loaded")
	}
	key, err := ipv6LPMKeyFromNet(ipNet)
	if err != nil {
		return err
	}
	return ignoreMissingMapKey(snap.CIDR6Blocklist.Delete(&key))
}
