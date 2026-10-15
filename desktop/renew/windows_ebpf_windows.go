//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type windowsEBPFProbe struct {
	Available bool
	Connected bool
	CountValid bool
	IPv4 uint64
	IPv6 uint64
	Message string
	CheckedAt time.Time
}

// A read-only inventory of *our* kernel hooks. This is not proof of their
// event-by-event accuracy. The map counter requires bpftool availability.
func probeWindowsEBPF(parent context.Context) windowsEBPFProbe {
	result := windowsEBPFProbe{CheckedAt: time.Now()}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "netsh", "ebpf", "show", "programs")
	data, err := cmd.Output()
	if err != nil {
		result.Message = "eBPF for Windows 未就绪（netsh ebpf show programs 不可用）"
		return result
	}
	result.Available = true
	// A registered program with zero links is not an active kernel hook.
	result.Connected = windowsProgramHasAttachedLink(string(data), "renew_connect4") &&
		windowsProgramHasAttachedLink(string(data), "renew_connect6")
	if !result.Connected {
		result.Message = "eBPF 运行时已发现；Renew connect4/connect6 Hook 未全部附加（需要 Links > 0）"
		return result
	}
	result.Message = "eBPF for Windows connect4/connect6 Hook 已注册（只读计数）"
	ctxMap, cancelMap := context.WithTimeout(parent, 2*time.Second)
	defer cancelMap()
	out, err := exec.CommandContext(ctxMap, "bpftool", "-j", "map", "dump", "name", "renew_conn").Output()
	if err != nil {
		result.Message += "；未读取 map（bpftool 不可用或权限不足）"
		return result
	}
	ipv4, ipv6, ok := parseWindowsEBPFMap(out)
	if ok {
		result.IPv4, result.IPv6, result.CountValid = ipv4, ipv6, true
	} else {
		result.Message += "；map 内容无法解码"
	}
	return result
}

// bpftool -j emits map array bytes as little-endian hex string arrays.
// Reject unknown formats rather than showing fabricated zero counters.
func parseWindowsEBPFMap(data []byte) (uint64, uint64, bool) {
	var rows []struct {
		Key json.RawMessage   `json:"key"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return 0, 0, false
	}
	var counts [2]uint64
	var found [2]bool
	for _, row := range rows {
		key, kOK := decodeBPFToolBytes(row.Key, 4)
		val, vOK := decodeBPFToolBytes(row.Value, 8)
		if !kOK || !vOK {
			return 0, 0, false
		}
		i := binary.LittleEndian.Uint32(key)
		if i > 1 || found[i] {
			return 0, 0, false
		}
		found[i] = true
		counts[i] = binary.LittleEndian.Uint64(val)
	}
	return counts[0], counts[1], found[0] && found[1]
}

func decodeBPFToolBytes(data json.RawMessage, required int) ([]byte, bool) {
	var hexStrings []string
	if err := json.Unmarshal(data, &hexStrings); err != nil || len(hexStrings) != required {
		return nil, false
	}
	result := make([]byte, 0, required)
	for _, str := range hexStrings {
		n, err := strconv.ParseUint(strings.TrimPrefix(strings.TrimSpace(str), "0x"), 16, 8)
		if err != nil {
			return nil, false
		}
		result = append(result, byte(n))
	}
	return result, true
}

func (p windowsEBPFProbe) Summary() string {
	if p.Message == "" {
		return "eBPF 状态尚未检测"
	}
	if p.CountValid {
		return fmt.Sprintf("%s；IPv4 %d 次，IPv6 %d 次（累计 Hook 调用次数，不代表已放行/已阻断连接数）", p.Message, p.IPv4, p.IPv6)
	}
	return p.Message
}

func windowsProgramHasAttachedLink(table, name string) bool {
    for _, line := range strings.Split(table, "\n") {
        fields := strings.Fields(line)
        // netsh ebpf show programs: ID Pins Links Mode Type Name
        if len(fields) < 6 || !strings.EqualFold(fields[len(fields)-1], name) {
            continue
        }
        links, err := strconv.ParseUint(fields[2], 10, 32)
        if err == nil && links > 0 {
            return true
        }
    }
    return false
}
