//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"runtime"
	"sort"
	"syscall"
	"time"
	"unsafe"
)

const (
	winTH32CS_SNAPPROCESS = 0x00000002
	winPROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	winPROCESS_VM_READ = 0x0010
	winAF_INET = 2
	winAF_INET6 = 23
	winTCP_TABLE_OWNER_PID_ALL = 5
	winTCP_STATE_ESTAB = 5
	winERROR_INSUFFICIENT_BUFFER = syscall.Errno(122)
	winEpochOffset = uint64(116444736000000000)
)

var (
	winKernel32 = syscall.NewLazyDLL("kernel32.dll")
	winPSAPI = syscall.NewLazyDLL("psapi.dll")
	winIPHLPAPI = syscall.NewLazyDLL("iphlpapi.dll")
	winCreateToolhelp32Snapshot = winKernel32.NewProc("CreateToolhelp32Snapshot")
	winProcess32First = winKernel32.NewProc("Process32FirstW")
	winProcess32Next = winKernel32.NewProc("Process32NextW")
	winOpenProcess = winKernel32.NewProc("OpenProcess")
	winCloseHandle = winKernel32.NewProc("CloseHandle")
	winGetProcessTimes = winKernel32.NewProc("GetProcessTimes")
	winGetSystemTimes = winKernel32.NewProc("GetSystemTimes")
	winGlobalMemoryStatusEx = winKernel32.NewProc("GlobalMemoryStatusEx")
	winGetProcessMemoryInfo = winPSAPI.NewProc("GetProcessMemoryInfo")
	winGetExtendedTcpTable = winIPHLPAPI.NewProc("GetExtendedTcpTable")
)

type winFiletime struct { Low uint32; High uint32 }
func (f winFiletime) ticks() uint64 { return uint64(f.High)<<32 | uint64(f.Low) }
func (f winFiletime) unixMillis() int64 {
	if f.ticks() < winEpochOffset { return 0 }
	return int64((f.ticks()-winEpochOffset)/10000)
}

type winProcessEntry struct {
	Size uint32
	Usage uint32
	ProcessID uint32
	DefaultHeapID uintptr
	ModuleID uint32
	Threads uint32
	ParentProcessID uint32
	PriClassBase int32
	Flags uint32
	ExeFile [260]uint16
}

type winMemoryStatus struct {
	Length uint32
	MemoryLoad uint32
	TotalPhys uint64
	AvailPhys uint64
	TotalPageFile uint64
	AvailPageFile uint64
	TotalVirtual uint64
	AvailVirtual uint64
	AvailExtendedVirtual uint64
}

type winProcessMemoryCounters struct {
	Size uint32
	PageFaultCount uint32
	PeakWorkingSetSize uintptr
	WorkingSetSize uintptr
	QuotaPeakPagedPoolUsage uintptr
	QuotaPagedPoolUsage uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage uintptr
	PagefileUsage uintptr
	PeakPagefileUsage uintptr
	PrivateUsage uintptr
}

func winAPIError(name string, err error) error {
	if err == nil || err == syscall.Errno(0) {
		return fmt.Errorf("%s failed", name)
	}
	return fmt.Errorf("%s: %w", name, err)
}

func winReadProcesses() (map[int]windowsProcessSample, error) {
	snapshot, _, callErr := winCreateToolhelp32Snapshot.Call(winTH32CS_SNAPPROCESS, 0)
	if snapshot == ^uintptr(0) { return nil, winAPIError("CreateToolhelp32Snapshot", callErr) }
	defer winCloseHandle.Call(snapshot)
	entry := winProcessEntry{Size: uint32(unsafe.Sizeof(winProcessEntry{}))}
	ok, _, callErr := winProcess32First.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	if ok == 0 { return nil, winAPIError("Process32FirstW", callErr) }
	processes := make(map[int]windowsProcessSample)
	for {
		if entry.ProcessID != 0 {
			pid := int(entry.ProcessID)
			p := windowsProcessSample{PID: pid, PPID: int(entry.ParentProcessID), Name: syscall.UTF16ToString(entry.ExeFile[:])}
			handle, _, _ := winOpenProcess.Call(winPROCESS_QUERY_LIMITED_INFORMATION|winPROCESS_VM_READ, 0, uintptr(entry.ProcessID))
			if handle == 0 {
				// Protected processes can still appear in the inventory even
				// though their resource counters are inaccessible.
				handle, _, _ = winOpenProcess.Call(winPROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(entry.ProcessID))
			}
			if handle != 0 {
				var created, exited, kernel, user winFiletime
				r, _, _ := winGetProcessTimes.Call(handle,
					uintptr(unsafe.Pointer(&created)), uintptr(unsafe.Pointer(&exited)),
					uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
				if r != 0 { p.Start = created.ticks(); p.CPU = kernel.ticks()+user.ticks(); p.HasCPU = true }
				var mem winProcessMemoryCounters
				mem.Size = uint32(unsafe.Sizeof(mem))
				r, _, _ = winGetProcessMemoryInfo.Call(handle, uintptr(unsafe.Pointer(&mem)), uintptr(mem.Size))
				if r != 0 { p.WorkingSet = uint64(mem.WorkingSetSize); p.HasMemory = true }
				winCloseHandle.Call(handle)
			}
			processes[pid] = p
		}
		ok, _, _ = winProcess32Next.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
		if ok == 0 { break }
	}
	return processes, nil
}

func winReadSystem() (systemSnapshot, uint64, uint64, error) {
	var mem winMemoryStatus
	mem.Length = uint32(unsafe.Sizeof(mem))
	ok, _, callErr := winGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem)))
	if ok == 0 { return systemSnapshot{}, 0, 0, winAPIError("GlobalMemoryStatusEx", callErr) }
	var idle, kernel, user winFiletime
	ok, _, callErr = winGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if ok == 0 { return systemSnapshot{}, 0, 0, winAPIError("GetSystemTimes", callErr) }
	total := mem.TotalPhys
	used := uint64(0)
	if total >= mem.AvailPhys { used = total-mem.AvailPhys }
	s := systemSnapshot{MemTotal: total, MemUsed: used}
	if total > 0 { s.MemPercent = 100*float64(used)/float64(total) }
	return s, idle.ticks(), kernel.ticks()+user.ticks(), nil
}

// GetExtendedTcpTable returns a point-in-time TCP table, not ETW connection
// events. Only ESTABLISHED rows are reported as observed peer addresses.
// Local listening sockets and transient connections are not mislabeled.
func winTCPTable(family uintptr) ([]byte, error) {
	var size uint32
	r, _, _ := winGetExtendedTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, family, winTCP_TABLE_OWNER_PID_ALL, 0)
	if r != 0 && syscall.Errno(r) != winERROR_INSUFFICIENT_BUFFER {
		return nil, fmt.Errorf("GetExtendedTcpTable (%d) size: %w", family, syscall.Errno(r))
	}
	if size < 4 || size > 16<<20 {
		return nil, fmt.Errorf("GetExtendedTcpTable (%d) invalid response length %d", family, size)
	}
	buf := make([]byte, size)
	r, _, _ = winGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)),
		0, family, winTCP_TABLE_OWNER_PID_ALL, 0)
	if r != 0 { return nil, fmt.Errorf("GetExtendedTcpTable (%d): %w", family, syscall.Errno(r)) }
	if size > uint32(len(buf)) || size < 4 { return nil, fmt.Errorf("GetExtendedTcpTable invalid table size") }
	return buf[:size], nil
}

func winParseTCPRows(data []byte, family uintptr) (map[string]windowsTCPSample, error) {
	if len(data) < 4 { return nil, fmt.Errorf("TCP table header truncated") }
	count := binary.LittleEndian.Uint32(data[:4])
	rowLen := 24
	if family == winAF_INET6 { rowLen = 56 }
	if uint64(count)*uint64(rowLen) > uint64(len(data)-4) {
		return nil, fmt.Errorf("TCP table length inconsistent with row count")
	}
	out := make(map[string]windowsTCPSample)
	for i:=uint32(0);i<count;i++ {
		row := data[4+int(i)*rowLen : 4+(int(i)+1)*rowLen]
		var local, remote string
		var portLocal, portRemote uint16
		var state, owner uint32
		if family == winAF_INET {
			state = binary.LittleEndian.Uint32(row[0:4])
			owner = binary.LittleEndian.Uint32(row[20:24])
			local = net.IP(row[4:8]).String()
			remote = net.IP(row[12:16]).String()
			portLocal = binary.BigEndian.Uint16(row[8:10])
			portRemote = binary.BigEndian.Uint16(row[16:18])
		} else {
			state = binary.LittleEndian.Uint32(row[48:52])
			owner = binary.LittleEndian.Uint32(row[52:56])
			local = net.IP(row[0:16]).String()
			remote = net.IP(row[24:40]).String()
			portLocal = binary.BigEndian.Uint16(row[20:22])
			portRemote = binary.BigEndian.Uint16(row[44:46])
		}
		if state != winTCP_STATE_ESTAB || owner == 0 || portRemote == 0 { continue }
		from := net.JoinHostPort(local, fmt.Sprint(portLocal))
		to := net.JoinHostPort(remote, fmt.Sprint(portRemote))
		key := fmt.Sprintf("%d|%s|%s", owner, from, to)
		out[key] = windowsTCPSample{PID: int(owner), Local: from, Remote: to}
	}
	return out, nil
}

func winReadTCPConnections() (map[string]windowsTCPSample, error) {
	result := make(map[string]windowsTCPSample)
	for _, family := range []uintptr{winAF_INET, winAF_INET6} {
		raw, err := winTCPTable(family)
		if err != nil { return nil, err }
		connections, err := winParseTCPRows(raw, family)
		if err != nil { return nil, err }
		for key, conn := range connections { result[key] = conn }
	}
	return result, nil
}

func collectWindowsObservation(ctx context.Context, prev windowsObservation, hasBaseline bool, elapsed time.Duration) (windowsObservation, error) {
	if err := ctx.Err(); err != nil { return windowsObservation{}, err }
	processes, err := winReadProcesses()
	if err != nil { return windowsObservation{}, err }
	connections, err := winReadTCPConnections()
	if err != nil { return windowsObservation{}, err }
	system, idle, total, err := winReadSystem()
	if err != nil { return windowsObservation{}, err }
	result := windowsObservation{
		Processes: processes, Connections: connections, System: system,
		CPUIdle: idle, CPUTotal: total,
	}
	system.Processes = make([]systemProcess, 0, len(processes))
	for _, p := range processes {
		cpu, mem := -1.0, -1.0
		if hasBaseline && p.HasCPU {
			if old, exists := prev.Processes[p.PID]; exists && old.HasCPU {
				cpu = windowsProcessCPUPercent(old, p, elapsed, runtime.NumCPU())
			}
		}
		if system.MemTotal > 0 && p.HasMemory { mem = 100*float64(p.WorkingSet)/float64(system.MemTotal) }
		system.Processes = append(system.Processes, systemProcess{
			PID: p.PID, PPID: p.PPID, Name: p.Name, CPU: cpu,
			MemPercent: mem, CreateTime: 0,
		})
	}
	sort.Slice(system.Processes, func(i,j int) bool { return system.Processes[i].PID < system.Processes[j].PID })
	if hasBaseline { system.CPUTotal = windowsCPUPercent(prev, result) }
	system.FetchedAt = time.Now()
	result.System = system
	return result, nil
}

func (a *renewApp) runLocalMonitor(ctx context.Context) {
	a.startEventUIBatcher(ctx)
	a.update(func() {
		a.starting = false
		a.backend = "Windows Win32 / IP Helper API（本机只读采样）"
		a.connected = false
		a.eventStreamConnected = false
		a.historyInitialized = true
		a.historyCursor = ""
		a.runtimeCfg.Runtime.PolicyManagementEnabled = false
	})
	var previous windowsObservation
	var previousAt time.Time
	hasBaseline := false
	var dropped int64
	ticker := time.NewTicker(2*time.Second)
	defer ticker.Stop()
	for {
		now := time.Now()
		snapshot, err := collectWindowsObservation(ctx, previous, hasBaseline, now.Sub(previousAt))
		if err != nil {
			if ctx.Err() != nil { return }
			a.update(func() {
				a.connected = false
				a.systemConnected = false
				a.eventStreamConnected = false
				a.healthReady = true
				a.health.CaptureHealthy = false
				a.lastErr = "Windows 本机采集失败：" + err.Error()
				a.systemErr = err.Error()
			})
		} else {
			events, lost := windowsObservationEvents(previous, snapshot, hasBaseline, now)
			connections := make([]windowsTCPSample, 0, len(snapshot.Connections))
			for _, conn := range snapshot.Connections { connections = append(connections, conn) }
			sort.Slice(connections, func(i, j int) bool {
				if connections[i].PID != connections[j].PID { return connections[i].PID < connections[j].PID }
				if connections[i].Remote != connections[j].Remote { return connections[i].Remote < connections[j].Remote }
				return connections[i].Local < connections[j].Local
			})
			dropped += int64(lost)
			previous, previousAt, hasBaseline = snapshot, now, true
			a.update(func() {
				a.connected = true
				a.systemConnected = true
				a.eventStreamConnected = true
				a.healthReady = true
				a.health.CaptureHealthy = true
				a.health.RingbufDroppedTotal = dropped // Windows UI explicitly labels this as sampled-summary overflow
				a.system = snapshot.System
				a.windowsConnections = connections
				a.systemErr = ""
				a.lastErr = ""
				a.lastSync = now
			})
			if len(events) > 0 { a.queueEventSummaries(events) }
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

