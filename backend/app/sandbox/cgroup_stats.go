package sandbox

import (
	"fmt"
	"net"
	"sort"

	"github.com/cilium/ebpf"
)

func ListBlockedCIDRs(cidr4, cidr6 *ebpf.Map) []string {
	items := []string{}
	if cidr4 != nil {
		iter := cidr4.Iterate()
		var key ipv4LPMKey
		var val uint32
		for iter.Next(&key, &val) {
			if val == 0 || key.PrefixLen > 32 {
				continue
			}
			ipNet := &net.IPNet{
				IP:   net.IPv4(key.Addr[0], key.Addr[1], key.Addr[2], key.Addr[3]),
				Mask: net.CIDRMask(int(key.PrefixLen), 32),
			}
			items = append(items, ipNet.String())
		}
	}
	if cidr6 != nil {
		iter := cidr6.Iterate()
		var key ipv6LPMKey
		var val uint32
		for iter.Next(&key, &val) {
			if val == 0 || key.PrefixLen > 128 {
				continue
			}
			ip := make(net.IP, 16)
			copy(ip, key.Addr[:])
			ipNet := &net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(int(key.PrefixLen), 128),
			}
			items = append(items, ipNet.String())
		}
	}
	sort.Strings(items)
	return items
}

func ListBlockedCgroups(blocklist *ebpf.Map) []string {
	if blocklist == nil {
		return nil
	}
	items := []string{}
	iter := blocklist.Iterate()
	var key uint64
	var val uint32
	for iter.Next(&key, &val) {
		if val == 0 {
			continue
		}
		items = append(items, fmt.Sprintf("%d", key))
	}
	sort.Slice(items, func(i, j int) bool {
		if len(items[i]) == len(items[j]) {
			return items[i] < items[j]
		}
		return len(items[i]) < len(items[j])
	})
	return items
}

func ListBlockedIPs(blocklist, ip6Blocklist *ebpf.Map) []string {
	items := []string{}
	if blocklist != nil {
		iter := blocklist.Iterate()
		var key uint32
		var val uint32
		for iter.Next(&key, &val) {
			if val == 0 {
				continue
			}
			items = append(items, ipv4StringFromBlockKey(key))
		}
	}
	if ip6Blocklist != nil {
		iter := ip6Blocklist.Iterate()
		var key ip6BlockKey
		var val uint32
		for iter.Next(&key, &val) {
			if val == 0 {
				continue
			}
			items = append(items, ip6StringFromBlockKey(key))
		}
	}
	sort.Strings(items)
	return items
}

func ListBlockedPorts(blocklist *ebpf.Map) []uint16 {
	if blocklist == nil {
		return nil
	}
	items := []uint16{}
	iter := blocklist.Iterate()
	var key uint32
	var val uint32
	for iter.Next(&key, &val) {
		if val == 0 || key == 0 || key > 65535 {
			continue
		}
		items = append(items, uint16(key))
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	return items
}

type CgroupSandboxStats struct {
	ConnectChecked uint64 `json:"connectChecked"`
	ConnectBlocked uint64 `json:"connectBlocked"`
	ConnectAllowed uint64 `json:"connectAllowed"`
	Checked        uint64 `json:"checked"`
	Blocked        uint64 `json:"blocked"`
	Allowed        uint64 `json:"allowed"`
}

func GetCgroupSandboxStats(statsMap *ebpf.Map) (CgroupSandboxStats, error) {
	if statsMap == nil {
		return CgroupSandboxStats{}, fmt.Errorf("stats map not loaded")
	}

	cpuCount, err := ebpf.PossibleCPU()
	if err != nil || cpuCount <= 0 {
		return CgroupSandboxStats{}, err
	}

	type rawStats struct {
		ConnectChecked uint64
		ConnectBlocked uint64
		ConnectAllowed uint64
	}

	values := make([]rawStats, cpuCount)
	key := uint32(0)
	if err := statsMap.Lookup(&key, &values); err != nil {
		return CgroupSandboxStats{}, err
	}

	var total CgroupSandboxStats
	for _, s := range values {
		total.ConnectChecked += s.ConnectChecked
		total.ConnectBlocked += s.ConnectBlocked
		total.ConnectAllowed += s.ConnectAllowed
	}
	total.Checked = total.ConnectChecked
	total.Blocked = total.ConnectBlocked
	total.Allowed = total.ConnectAllowed
	return total, nil
}

// Apply cgroup sandbox to a specific agent run (block all outbound for that cgroup)
func sandboxCgroupForAgent(cgroupID uint64) error {
	if CurrentCgroupSandboxSnapshot().CgroupBlocklist == nil {
		return fmt.Errorf("cgroup sandbox not available")
	}
	return BlockCgroup(cgroupID)
}

// Release cgroup sandbox (allow outbound again)
func releaseCgroupSandbox(cgroupID uint64) error {
	if CurrentCgroupSandboxSnapshot().CgroupBlocklist == nil {
		return nil
	}
	return UnblockCgroup(cgroupID)
}
