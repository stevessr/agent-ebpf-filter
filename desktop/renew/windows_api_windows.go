//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	windowsTCPTableOwnerPIDAll = 5
	windowsTCPStateEstablished = 5
	windowsTCPStateSynSent     = 3
	windowsMaxTCPTableBytes    = 8 << 20
)

var (
	windowsIPHelper       = syscall.NewLazyDLL("iphlpapi.dll")
	windowsGetTCPTable    = windowsIPHelper.NewProc("GetExtendedTcpTable")
	windowsKernel32       = syscall.NewLazyDLL("kernel32.dll")
	windowsGlobalMemStats = windowsKernel32.NewProc("GlobalMemoryStatusEx")
)

// windowsTCPConnection is a point-in-time observation, not a socket-open event.
// No DNS hostname, destination process, or byte count is inferred from this API.
type windowsTCPConnection struct {
	PID    int
	Local  string
	Remote string
	State  uint32
}

func windowsProcessSnapshot() (map[int]systemProcess, error) {
	handle, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("CreateToolhelp32Snapshot: %w", err)
	}
	defer syscall.CloseHandle(handle)

	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	if err := syscall.Process32First(handle, &entry); err != nil {
		if err == syscall.ERROR_NO_MORE_FILES {
			return map[int]systemProcess{}, nil
		}
		return nil, fmt.Errorf("Process32First: %w", err)
	}
	result := make(map[int]systemProcess)
	for {
		pid := int(entry.ProcessID)
		result[pid] = systemProcess{
			PID:  pid,
			PPID: int(entry.ParentProcessID),
			Name: syscall.UTF16ToString(entry.ExeFile[:]),
		}
		entry.Size = uint32(unsafe.Sizeof(entry))
		err = syscall.Process32Next(handle, &entry)
		if err == syscall.ERROR_NO_MORE_FILES {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("Process32Next: %w", err)
		}
	}
	return result, nil
}

// The structure layout matches MEMORYSTATUSEX (DWORD fields + ULONGLONGs).
type windowsMemoryStatus struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func windowsMemorySnapshot() (total, used uint64, percentage float64, err error) {
	status := windowsMemoryStatus{Length: uint32(unsafe.Sizeof(windowsMemoryStatus{}))}
	r, _, callErr := windowsGlobalMemStats.Call(uintptr(unsafe.Pointer(&status)))
	if r == 0 {
		return 0, 0, 0, fmt.Errorf("GlobalMemoryStatusEx: %v", callErr)
	}
	if status.TotalPhys >= status.AvailPhys {
		used = status.TotalPhys - status.AvailPhys
	}
	return status.TotalPhys, used, float64(status.MemoryLoad), nil
}

func windowsTCPSnapshot() ([]windowsTCPConnection, error) {
	var result []windowsTCPConnection
	var failures []string
	for _, family := range []int{syscall.AF_INET, syscall.AF_INET6} {
		rows, err := windowsTCPRows(family)
		if err != nil {
			failures = append(failures, fmt.Sprintf("AF=%d: %v", family, err))
			continue
		}
		result = append(result, rows...)
	}
	if len(failures) == 2 {
		return nil, fmt.Errorf("GetExtendedTcpTable: %s", strings.Join(failures, "; "))
	}
	// Preserve partial IPv4/IPv6 telemetry rather than inventing completeness.
	if len(failures) != 0 {
		return result, fmt.Errorf("GetExtendedTcpTable incomplete: %s", strings.Join(failures, "; "))
	}
	return result, nil
}

func windowsTCPRows(family int) ([]windowsTCPConnection, error) {
	var size uint32
	// First call establishes the required buffer size (normally ERROR_INSUFFICIENT_BUFFER).
	_, _, _ = windowsGetTCPTable.Call(0, uintptr(unsafe.Pointer(&size)), 0,
		uintptr(family), windowsTCPTableOwnerPIDAll, 0)
	if size < 4 || size > windowsMaxTCPTableBytes {
		return nil, fmt.Errorf("invalid TCP table buffer size %d", size)
	}
	for attempt := 0; attempt < 3; attempt++ {
		buffer := make([]byte, size)
		code, _, _ := windowsGetTCPTable.Call(uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(unsafe.Pointer(&size)), 0, uintptr(family), windowsTCPTableOwnerPIDAll, 0)
		if code == uintptr(syscall.ERROR_INSUFFICIENT_BUFFER) {
			if size <= 4 || size > windowsMaxTCPTableBytes {
				return nil, fmt.Errorf("TCP table size exceeds bound (%d)", size)
			}
			continue
		}
		if code != 0 {
			return nil, syscall.Errno(code)
		}
		return decodeWindowsTCPRows(buffer, family)
	}
	return nil, fmt.Errorf("TCP table resized too frequently")
}

func windowsPort(raw uint32) string {
	// IP Helper stores the network-order port in the low two bytes of a DWORD.
	return strconv.Itoa(int(uint16(raw&0xff)<<8 | uint16((raw>>8)&0xff)))
}

func decodeWindowsTCPRows(buf []byte, family int) ([]windowsTCPConnection, error) {
	if len(buf) < 4 {
		return nil, fmt.Errorf("short TCP table header")
	}
	rowSize := 24 // MIB_TCPROW_OWNER_PID
	if family == syscall.AF_INET6 {
		rowSize = 56 // MIB_TCP6ROW_OWNER_PID
	} else if family != syscall.AF_INET {
		return nil, fmt.Errorf("unsupported IP family %d", family)
	}
	count := uint64(binary.LittleEndian.Uint32(buf[:4]))
	if count > uint64((len(buf)-4)/rowSize) {
		return nil, fmt.Errorf("TCP table count exceeds buffer")
	}
	rows := make([]windowsTCPConnection, 0, count)
	for i := uint64(0); i < count; i++ {
		offset := 4 + int(i)*rowSize
		row := buf[offset : offset+rowSize]
		var state, pid uint32
		var local, remote string
		if family == syscall.AF_INET {
			state = binary.LittleEndian.Uint32(row[0:4])
			local = net.JoinHostPort(net.IP(row[4:8]).String(),
				windowsPort(binary.LittleEndian.Uint32(row[8:12])))
			remote = net.JoinHostPort(net.IP(row[12:16]).String(),
				windowsPort(binary.LittleEndian.Uint32(row[16:20])))
			pid = binary.LittleEndian.Uint32(row[20:24])
		} else {
			local = net.JoinHostPort(net.IP(row[0:16]).String(),
				windowsPort(binary.LittleEndian.Uint32(row[20:24])))
			remote = net.JoinHostPort(net.IP(row[24:40]).String(),
				windowsPort(binary.LittleEndian.Uint32(row[44:48])))
			state = binary.LittleEndian.Uint32(row[48:52])
			pid = binary.LittleEndian.Uint32(row[52:56])
		}
		if pid == 0 || (state != windowsTCPStateEstablished && state != windowsTCPStateSynSent) {
			continue
		}
		rows = append(rows, windowsTCPConnection{
			PID: int(pid), Local: local, Remote: remote, State: state,
		})
	}
	return rows, nil
}
