//go:build cgo

package observability

import (
	"agent-ebpf-filter/pb"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

func appendNVIDIAMetrics(procMap map[int32]GpuInfo, globalStats *[]*pb.GPUStatus) {
	if !deps.NvmlInitialized {
		return
	}
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

		*globalStats = append(*globalStats, gs)

		procs, ret := device.GetComputeRunningProcesses()
		if ret == nvml.SUCCESS {
			for _, p := range procs {
				procMap[int32(p.Pid)] = GpuInfo{
					Mem: uint32(p.UsedGpuMemory / 1024 / 1024),
					GPU: uint32(i),
				}
			}
		}
	}
}
