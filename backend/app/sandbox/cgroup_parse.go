package sandbox

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func unifiedCgroupRelativePath(data []byte) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) == 3 && parts[0] == "0" && parts[1] == "" {
			if strings.TrimSpace(parts[2]) == "" {
				return "", fmt.Errorf("empty cgroup v2 path")
			}
			return parts[2], nil
		}
	}
	return "", fmt.Errorf("no cgroup v2 entry")
}

func resolveCgroupPath(root, rel string) (string, error) {
	cleanRoot := filepath.Clean(root)
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("empty cgroup path")
	}
	cleanRel := filepath.Clean("/" + strings.TrimPrefix(rel, "/"))
	if cleanRel == "/" {
		return cleanRoot, nil
	}
	return filepath.Join(cleanRoot, strings.TrimPrefix(cleanRel, "/")), nil
}

func cgroupIDFromPath(cgroupPath string) (uint64, error) {
	st, err := os.Stat(cgroupPath)
	if err != nil {
		return 0, fmt.Errorf("stat cgroup path %q: %w", cgroupPath, err)
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("stat cgroup path %q: missing Stat_t", cgroupPath)
	}
	if sys.Ino == 0 {
		return 0, fmt.Errorf("stat cgroup path %q: zero inode", cgroupPath)
	}
	return uint64(sys.Ino), nil
}

type ip6BlockKey struct {
	Addr [4]uint32
}

func ip6BlockKeyFromIP(ip net.IP) (ip6BlockKey, error) {
	var key ip6BlockKey
	ip16 := ip.To16()
	if ip16 == nil || ip.To4() != nil {
		return key, fmt.Errorf("invalid IPv6 address: %s", ip.String())
	}
	for i := 0; i < 4; i++ {
		key.Addr[i] = binary.BigEndian.Uint32(ip16[i*4 : (i+1)*4])
	}
	return key, nil
}

func ParseIP(ipStr string) (net.IP, string, error) {
	trimmed := strings.TrimSpace(ipStr)
	if trimmed == "" {
		return nil, "", fmt.Errorf("empty IP")
	}
	ip := net.ParseIP(trimmed)
	if ip == nil {
		return nil, "", fmt.Errorf("invalid IP: %s", ipStr)
	}
	return ip, canonicalCgroupSandboxIPText(ip), nil
}

func canonicalCgroupSandboxIPText(ip net.IP) string {
	if ip4 := ip.To4(); ip4 != nil {
		return net.IP(ip4).String()
	}
	return ip.String()
}

type ipv4LPMKey struct {
	PrefixLen uint32
	Addr      [4]byte
}

type ipv6LPMKey struct {
	PrefixLen uint32
	Addr      [16]byte
}

// ParseCIDR validates and canonicalizes an IPv4, IPv6, or IPv4-mapped IPv6 CIDR.
// IPv4-mapped IPv6 CIDRs (::ffff:a.b.c.d/p with p >= 96) normalize to IPv4 CIDRs
// so dual-stack policies share one canonical trie entry.
func ParseCIDR(cidrStr string) (*net.IPNet, string, error) {
	trimmed := strings.TrimSpace(cidrStr)
	if trimmed == "" {
		return nil, "", fmt.Errorf("empty CIDR")
	}
	_, ipNet, err := net.ParseCIDR(trimmed)
	if err != nil || ipNet == nil {
		return nil, "", fmt.Errorf("invalid CIDR: %s", cidrStr)
	}
	ones, bits := ipNet.Mask.Size()
	if bits == 0 {
		return nil, "", fmt.Errorf("non-canonical CIDR mask: %s", cidrStr)
	}
	if bits == 32 {
		ip4 := ipNet.IP.To4()
		if ip4 == nil {
			return nil, "", fmt.Errorf("invalid IPv4 CIDR: %s", cidrStr)
		}
		mask := net.CIDRMask(ones, 32)
		canon := &net.IPNet{IP: ip4.Mask(mask), Mask: mask}
		return canon, canon.String(), nil
	}
	if bits == 128 && ones >= 96 {
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			v4Bits := ones - 96
			mask := net.CIDRMask(v4Bits, 32)
			canon := &net.IPNet{IP: ip4.Mask(mask), Mask: mask}
			return canon, canon.String(), nil
		}
	}
	ip6 := ipNet.IP.To16()
	if ip6 == nil {
		return nil, "", fmt.Errorf("invalid IPv6 CIDR: %s", cidrStr)
	}
	mask := net.CIDRMask(ones, 128)
	canon := &net.IPNet{IP: ip6.Mask(mask), Mask: mask}
	return canon, canon.String(), nil
}

func ipv4LPMKeyFromNet(ipNet *net.IPNet) (ipv4LPMKey, error) {
	var key ipv4LPMKey
	ones, bits := ipNet.Mask.Size()
	ip4 := ipNet.IP.To4()
	if bits != 32 || ip4 == nil {
		return key, fmt.Errorf("not an IPv4 CIDR: %v", ipNet)
	}
	key.PrefixLen = uint32(ones)
	copy(key.Addr[:], ip4.Mask(ipNet.Mask).To4())
	return key, nil
}

func ipv6LPMKeyFromNet(ipNet *net.IPNet) (ipv6LPMKey, error) {
	var key ipv6LPMKey
	ones, bits := ipNet.Mask.Size()
	ip6 := ipNet.IP.To16()
	if bits != 128 || ip6 == nil {
		return key, fmt.Errorf("not an IPv6 CIDR: %v", ipNet)
	}
	key.PrefixLen = uint32(ones)
	copy(key.Addr[:], ip6.Mask(ipNet.Mask).To16())
	return key, nil
}

// LPMTriePrefixMatchesIP checks whether ipStr matches cidrStr using the exact
// network-byte-order LPM key encoding and IPv4-mapped IPv6 handling used in
// cgroup_sandbox.c.
func LPMTriePrefixMatchesIP(cidrStr, ipStr string) (bool, error) {
	ipNet, _, err := ParseCIDR(cidrStr)
	if err != nil {
		return false, err
	}
	ip, _, err := ParseIP(ipStr)
	if err != nil {
		return false, err
	}
	_, cidrBits := ipNet.Mask.Size()
	if cidrBits == 32 {
		ip4 := ip.To4()
		if ip4 == nil {
			return false, nil
		}
		key, err := ipv4LPMKeyFromNet(ipNet)
		if err != nil {
			return false, err
		}
		return lpmPrefixMatchBytes(key.Addr[:], ip4, int(key.PrefixLen)), nil
	}
	if ip.To4() != nil {
		return false, nil
	}
	ip6 := ip.To16()
	if ip6 == nil {
		return false, nil
	}
	key, err := ipv6LPMKeyFromNet(ipNet)
	if err != nil {
		return false, err
	}
	return lpmPrefixMatchBytes(key.Addr[:], ip6, int(key.PrefixLen)), nil
}

func lpmPrefixMatchBytes(prefixAddr, candidate []byte, prefixLen int) bool {
	if len(prefixAddr) != len(candidate) || prefixLen < 0 || prefixLen > len(prefixAddr)*8 {
		return false
	}
	fullBytes := prefixLen / 8
	for i := 0; i < fullBytes; i++ {
		if prefixAddr[i] != candidate[i] {
			return false
		}
	}
	remBits := prefixLen % 8
	if remBits > 0 {
		mask := byte(0xff << (8 - remBits))
		if (prefixAddr[fullBytes] & mask) != (candidate[fullBytes] & mask) {
			return false
		}
	}
	return true
}

func ValidatePort(port uint16) error {
	if port == 0 {
		return fmt.Errorf("invalid destination port: 0")
	}
	return nil
}

func ipv4StringFromBlockKey(key uint32) string {
	return net.IPv4(byte(key>>24), byte(key>>16), byte(key>>8), byte(key)).String()
}

func ip6StringFromBlockKey(key ip6BlockKey) string {
	var raw [16]byte
	for i, part := range key.Addr {
		binary.BigEndian.PutUint32(raw[i*4:(i+1)*4], part)
	}
	return net.IP(raw[:]).String()
}

type cgroupIDRequest struct {
	CgroupID json.RawMessage `json:"cgroupId"`
}

func parseCgroupID(raw json.RawMessage) (uint64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, fmt.Errorf("missing cgroupId")
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		id, parseErr := strconv.ParseUint(strings.TrimSpace(asString), 10, 64)
		if parseErr != nil || id == 0 {
			return 0, fmt.Errorf("invalid cgroupId: %s", asString)
		}
		return id, nil
	}

	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err != nil {
		return 0, fmt.Errorf("invalid cgroupId")
	}
	id, err := strconv.ParseUint(asNumber.String(), 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("invalid cgroupId: %s", asNumber.String())
	}
	return id, nil
}

// ── Statistics ────────────────────────────────────────────────────────

// ParseID parses a cgroup id from its textual form.
func ParseID(raw string) (uint64, error) { return parseCgroupID(json.RawMessage(raw)) }
