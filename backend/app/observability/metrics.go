package observability

import (
	"agent-ebpf-filter/pb"
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

func GetGPUMetrics() (map[int32]GpuInfo, []*pb.GPUStatus) {
	procMap := make(map[int32]GpuInfo)
	var globalStats []*pb.GPUStatus

	// 1. NVIDIA NVML when CGO is available. Pure-Go desktop builds
	// intentionally skip this helper and still collect generic DRM fdinfo.
	appendNVIDIAMetrics(procMap, &globalStats)

	// 2. Generic DRM (Intel/AMD via fdinfo)
	scanFdinfo(procMap, &globalStats)

	return procMap, globalStats
}

func ReadVMFaultCounters() (VmFaultCounters, error) {
	data, err := os.ReadFile("/proc/vmstat")
	if err != nil {
		return VmFaultCounters{}, err
	}

	counters := VmFaultCounters{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}

		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		switch fields[0] {
		case "pgfault":
			counters.PageFaults = val
		case "pgmajfault":
			counters.MajorFaults = val
		case "pswpin":
			counters.SwapIn = val
		case "pswpout":
			counters.SwapOut = val
		}
	}

	if err := scanner.Err(); err != nil {
		return VmFaultCounters{}, err
	}

	return counters, nil
}

func scanFdinfo(procMap map[int32]GpuInfo, globalStats *[]*pb.GPUStatus) {
	now := time.Now()
	deps.FdinfoHistoryMu.Lock()
	dt := now.Sub(*deps.FdinfoTime).Nanoseconds()
	*deps.FdinfoTime = now
	deps.FdinfoHistoryMu.Unlock()

	type clientKey struct {
		pid int
		id  string
	}
	seenClients := make(map[clientKey]bool)

	procDirs, _ := os.ReadDir("/proc")
	for _, pd := range procDirs {
		pid, err := strconv.Atoi(pd.Name())
		if err != nil {
			continue
		}

		fdDir := fmt.Sprintf("/proc/%d/fdinfo", pid)
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			fpath := filepath.Join(fdDir, fd.Name())
			file, err := os.Open(fpath)
			if err != nil {
				continue
			}

			scanner := bufio.NewScanner(file)
			var driver, clientId string
			var memKb, enginesNs uint64

			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "drm-driver:") {
					driver = strings.TrimSpace(line[11:])
				} else if strings.HasPrefix(line, "drm-client-id:") {
					clientId = strings.TrimSpace(line[14:])
				} else if strings.HasPrefix(line, "drm-total-") || strings.HasPrefix(line, "drm-memory-") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						v, _ := strconv.ParseUint(parts[1], 10, 64)
						memKb += v
					}
				} else if strings.HasPrefix(line, "drm-engine-") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						v, _ := strconv.ParseUint(parts[1], 10, 64)
						enginesNs += v
					}
				}
			}
			file.Close()

			if driver == "" || (driver == "nvidia" && deps.NvmlInitialized) {
				continue
			}

			if driver == "i915" || driver == "xe" {
				driver = "Intel Graphics"
			}
			if driver == "amdgpu" {
				driver = "AMD Radeon"
			}

			histKey := fmt.Sprintf("%d:%s", pid, fd.Name())
			util := uint32(0)
			deps.FdinfoHistoryMu.RLock()
			prev, ok := deps.FdinfoHistory[histKey]
			deps.FdinfoHistoryMu.RUnlock()
			if ok && dt > 0 {
				diff := enginesNs - prev
				util = uint32((diff * 100) / uint64(dt))
			}
			deps.FdinfoHistoryMu.Lock()
			deps.FdinfoHistory[histKey] = enginesNs
			deps.FdinfoHistoryMu.Unlock()

			p := int32(pid)
			ckey := clientKey{pid, clientId}

			cur := procMap[p]
			if !seenClients[ckey] {
				cur.Mem += uint32(memKb / 1024)
				seenClients[ckey] = true
			}
			if util > cur.Util {
				cur.Util = util
			}
			procMap[p] = cur

			found := false
			for _, gs := range *globalStats {
				if gs.Name == driver {
					gs.UtilGpu += util
					if gs.UtilGpu > 100 {
						gs.UtilGpu = 100
					}
					gs.MemUsed = cur.Mem
					found = true
					break
				}
			}
			if !found {
				*globalStats = append(*globalStats, &pb.GPUStatus{
					Index: uint32(len(*globalStats)), Name: driver, UtilGpu: util, MemUsed: uint32(memKb / 1024),
				})
			}
		}
	}
}

func GetCoreTypes() []pb.CPUInfo_Core_Type {
	cores, _ := cpu.Counts(true)
	types := make([]pb.CPUInfo_Core_Type, cores)
	maxFreqs := make([]int64, cores)
	overallMax := int64(0)
	for i := 0; i < cores; i++ {
		data, err := os.ReadFile(fmt.Sprintf("/sys/devices/system/cpu/cpu%d/topology/core_type", i))
		if err == nil {
			val := strings.TrimSpace(string(data))
			if val == "intel_atom" {
				types[i] = pb.CPUInfo_Core_EFFICIENCY
				continue
			}
			if val == "intel_core" {
				types[i] = pb.CPUInfo_Core_PERFORMANCE
				continue
			}
		}
		freqData, err := os.ReadFile(fmt.Sprintf("/sys/devices/system/cpu/cpu%d/cpufreq/cpuinfo_max_freq", i))
		if err == nil {
			fmt.Sscanf(string(freqData), "%d", &maxFreqs[i])
			if maxFreqs[i] > overallMax {
				overallMax = maxFreqs[i]
			}
		}
	}
	if overallMax > 0 {
		for i := 0; i < cores; i++ {
			if types[i] != 0 {
				continue
			}
			if maxFreqs[i] < (overallMax * 8 / 10) {
				types[i] = pb.CPUInfo_Core_EFFICIENCY
			} else {
				types[i] = pb.CPUInfo_Core_PERFORMANCE
			}
		}
	}
	return types
}
