package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/pb"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/shirou/gopsutil/v3/cpu"
)

func getGPUMetrics() (map[int32]gpuInfo, []*pb.GPUStatus) {
	procMap := make(map[int32]gpuInfo)
	var globalStats []*pb.GPUStatus

	// 1. NVIDIA (Native)
	if nvmlInitialized {
		count, _ := nvml.DeviceGetCount()
		for i := 0; i < count; i++ {
			device, _ := nvml.DeviceGetHandleByIndex(i)
			name, _ := device.GetName()
			util, _ := device.GetUtilizationRates()
			mInfo, _ := device.GetMemoryInfo()
			temp, _ := device.GetTemperature(nvml.TEMPERATURE_GPU)

			gs := &pb.GPUStatus{
				Index: uint32(i), Name: name, UtilGpu: util.Gpu, UtilMem: util.Memory,
				MemTotal: uint32(mInfo.Total / 1024 / 1024), MemUsed: uint32(mInfo.Used / 1024 / 1024), Temp: temp,
			}

			// Detailed NVIDIA engine stats
			if encUtil, _, ret := device.GetEncoderUtilization(); ret == nvml.SUCCESS {
				gs.EncUtil = encUtil
			}
			if decUtil, _, ret := device.GetDecoderUtilization(); ret == nvml.SUCCESS {
				gs.DecUtil = decUtil
			}
			if smClock, ret := device.GetClockInfo(nvml.CLOCK_SM); ret == nvml.SUCCESS {
				gs.SmClockMhz = smClock
			}
			if memClock, ret := device.GetClockInfo(nvml.CLOCK_MEM); ret == nvml.SUCCESS {
				gs.MemClockMhz = memClock
			}
			if gfxClock, ret := device.GetClockInfo(nvml.CLOCK_GRAPHICS); ret == nvml.SUCCESS {
				gs.GfxClockMhz = gfxClock
			}
			if powerMw, ret := device.GetPowerUsage(); ret == nvml.SUCCESS {
				gs.PowerW = powerMw / 1000
			}
			if limitMw, ret := device.GetPowerManagementLimit(); ret == nvml.SUCCESS {
				gs.PowerLimitW = limitMw / 1000
			}
			if fan, ret := device.GetFanSpeed(); ret == nvml.SUCCESS {
				gs.FanSpeed = fan
			}
			if gen, ret := device.GetCurrPcieLinkGeneration(); ret == nvml.SUCCESS {
				gs.PcieGen = int32(gen)
			}
			if width, ret := device.GetCurrPcieLinkWidth(); ret == nvml.SUCCESS {
				gs.PcieWidth = int32(width)
			}

			globalStats = append(globalStats, gs)

			procs, ret := device.GetComputeRunningProcesses()
			if ret == nvml.SUCCESS {
				for _, p := range procs {
					procMap[int32(p.Pid)] = gpuInfo{mem: uint32(p.UsedGpuMemory / 1024 / 1024), gpu: uint32(i), util: 0}
				}
			}
		}
	}

	// 2. Generic DRM (Intel/AMD via fdinfo)
	scanFdinfo(procMap, &globalStats)

	return procMap, globalStats
}

func readVMFaultCounters() (vmFaultCounters, error) {
	data, err := os.ReadFile("/proc/vmstat")
	if err != nil {
		return vmFaultCounters{}, err
	}

	counters := vmFaultCounters{}
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
			counters.pageFaults = val
		case "pgmajfault":
			counters.majorFaults = val
		case "pswpin":
			counters.swapIn = val
		case "pswpout":
			counters.swapOut = val
		}
	}

	if err := scanner.Err(); err != nil {
		return vmFaultCounters{}, err
	}

	return counters, nil
}

func scanFdinfo(procMap map[int32]gpuInfo, globalStats *[]*pb.GPUStatus) {
	now := time.Now()
	fdinfoHistoryMu.Lock()
	dt := now.Sub(fdinfoTime).Nanoseconds()
	fdinfoTime = now
	fdinfoHistoryMu.Unlock()

	// Track seen client IDs to avoid overcounting VRAM (some drivers provide drm-client-id)
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

			if driver == "" || (driver == "nvidia" && nvmlInitialized) {
				continue
			}

			// Normalize driver name for UI
			if driver == "i915" || driver == "xe" {
				driver = "Intel Graphics"
			}
			if driver == "amdgpu" {
				driver = "AMD Radeon"
			}

			// Utilization calculation
			histKey := fmt.Sprintf("%d:%s", pid, fd.Name())
			util := uint32(0)
			fdinfoHistoryMu.RLock()
			prev, ok := fdinfoHistory[histKey]
			fdinfoHistoryMu.RUnlock()
			if ok && dt > 0 {
				diff := enginesNs - prev
				util = uint32((diff * 100) / uint64(dt))
			}
			fdinfoHistoryMu.Lock()
			fdinfoHistory[histKey] = enginesNs
			fdinfoHistoryMu.Unlock()

			// Aggregate per process
			p := int32(pid)
			ckey := clientKey{pid, clientId}

			cur := procMap[p]
			if !seenClients[ckey] {
				cur.mem += uint32(memKb / 1024)
				seenClients[ckey] = true
			}
			if util > cur.util {
				cur.util = util
			}
			procMap[p] = cur

			// Global aggregation
			found := false
			for _, gs := range *globalStats {
				if gs.Name == driver {
					gs.UtilGpu += util // Summing across all processes' engine usage is correct for global util.
					if gs.UtilGpu > 100 {
						gs.UtilGpu = 100
					}
					gs.MemUsed = cur.mem // This is tricky. Let's just track drivers.
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

func getCoreTypes() []pb.CPUInfo_Core_Type {
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
