package handlers

import (
	"fmt"
	"math"
	"runtime"
	"time"

	"agent-ebpf-filter/app/wsstream"
	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	gnet "github.com/shirou/gopsutil/v3/net"
	ps "github.com/shirou/gopsutil/v3/process"
	"google.golang.org/protobuf/proto"
)

// ---- moved from app/handlersstatssystem.go ----

const (
	systemStatsMaxProcesses        = 8192
	systemStatsMaxNameBytes        = 512
	systemStatsMaxUserBytes        = 512
	systemStatsMaxCommandLineBytes = 4096
)

func ServeSystemStatsWS(c *gin.Context) {
	conn, err := Deps.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(wsstream.ControlReadLimit)
	iv := wsstream.IntervalMilliseconds(c.Query("interval"), 2*time.Second, wsstream.MinStreamInterval, wsstream.MaxStreamInterval)
	ticker := time.NewTicker(iv)
	defer ticker.Stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	coreTypes := Deps.GetCoreTypes()
	lastFaults, err := Deps.ReadVMFaultCounters()
	if err != nil {
		lastFaults = Deps.VMFaultCountersZero()
	}
	lastNetIO, _ := gnet.IOCounters(true)
	lastDiskIO, _ := disk.IOCounters()
	lastIOStatTime := time.Now()
	lastFaultTime := time.Now()
	type procCPUSample struct {
		createTime int64
		totalCPU   float64
		sampleTime time.Time
	}
	procCPUSamples := make(map[int32]procCPUSample)
	cpuScale := float64(runtime.NumCPU())
	if cpuScale <= 0 {
		cpuScale = 1
	}
	for {
		var now time.Time
		select {
		case <-done:
			return
		case <-c.Request.Context().Done():
			return
		case now = <-ticker.C:
		}
		gm, gs := Deps.GetGPUMetrics()
		vm, _ := mem.VirtualMemory()
		if vm == nil {
			vm = &mem.VirtualMemoryStat{}
		}
		sm, _ := mem.SwapMemory()
		if sm == nil {
			sm = &mem.SwapMemoryStat{}
		}
		cc, _ := cpu.Percent(0, false)
		cp, _ := cpu.Percent(0, true)
		netIO, _ := gnet.IOCounters(true)
		diskIO, _ := disk.IOCounters()
		pbIO := &pb.IOInfo{}
		dtIO := now.Sub(lastIOStatTime).Seconds()
		vmFaults, faultErr := Deps.ReadVMFaultCounters()
		faultInfo := &pb.FaultInfo{}
		currentPIDs := make(map[int32]struct{})
		emittedSystemMetric := false
		if faultErr == nil {
			pageFaults := vmFaults.PageFaults
			majorFaults := vmFaults.MajorFaults
			minorFaults := uint64(0)
			if pageFaults >= majorFaults {
				minorFaults = pageFaults - majorFaults
			}
			faultInfo.PageFaults = pageFaults
			faultInfo.MajorFaults = majorFaults
			faultInfo.MinorFaults = minorFaults
			dt := now.Sub(lastFaultTime).Seconds()
			if dt > 0 {
				pageDelta := deltaUint64(pageFaults, lastFaults.PageFaults)
				majorDelta := deltaUint64(majorFaults, lastFaults.MajorFaults)
				swapInDelta := deltaUint64(vmFaults.SwapIn, lastFaults.SwapIn)
				swapOutDelta := deltaUint64(vmFaults.SwapOut, lastFaults.SwapOut)
				faultInfo.PageFaultRate = float64(pageDelta) / dt
				faultInfo.MajorFaultRate = float64(majorDelta) / dt
				faultInfo.MinorFaultRate = faultInfo.PageFaultRate - faultInfo.MajorFaultRate
				if faultInfo.MinorFaultRate < 0 {
					faultInfo.MinorFaultRate = 0
				}
				faultInfo.SwapIn = vmFaults.SwapIn
				faultInfo.SwapOut = vmFaults.SwapOut
				faultInfo.SwapInRate = float64(swapInDelta) / dt
				faultInfo.SwapOutRate = float64(swapOutDelta) / dt
			}
			lastFaults = vmFaults
			lastFaultTime = now
		}

		if dtIO > 0 {
			for _, n := range netIO {
				var rb, sb uint64
				for _, prev := range lastNetIO {
					if prev.Name == n.Name {
						rb = deltaUint64(n.BytesRecv, prev.BytesRecv)
						sb = deltaUint64(n.BytesSent, prev.BytesSent)
						break
					}
				}
				pbIO.Networks = append(pbIO.Networks, &pb.NetworkInterface{
					Name:      n.Name,
					RecvBytes: uint64(float64(rb) / dtIO),
					SentBytes: uint64(float64(sb) / dtIO),
				})
				pbIO.TotalNetRecvBytes += uint64(float64(rb) / dtIO)
				pbIO.TotalNetSentBytes += uint64(float64(sb) / dtIO)
			}
			for name, d := range diskIO {
				var rb, wb uint64
				if prev, ok := lastDiskIO[name]; ok {
					rb = deltaUint64(d.ReadBytes, prev.ReadBytes)
					wb = deltaUint64(d.WriteBytes, prev.WriteBytes)
				}
				pbIO.Disks = append(pbIO.Disks, &pb.DiskDevice{
					Name:       name,
					ReadBytes:  uint64(float64(rb) / dtIO),
					WriteBytes: uint64(float64(wb) / dtIO),
				})
				pbIO.TotalReadBytes += uint64(float64(rb) / dtIO)
				pbIO.TotalWriteBytes += uint64(float64(wb) / dtIO)
			}
		}
		lastNetIO = netIO
		lastDiskIO = diskIO
		lastIOStatTime = now
		totalCPU := 0.0
		if len(cc) > 0 {
			totalCPU = cc[0]
		}
		cpuInfo := &pb.CPUInfo{Total: totalCPU, Cores: cp}
		for i, usage := range cp {
			ct := pb.CPUInfo_Core_PERFORMANCE
			if i < len(coreTypes) {
				ct = coreTypes[i]
			}
			cpuInfo.CoreDetails = append(cpuInfo.CoreDetails, &pb.CPUInfo_Core{Index: uint32(i), Usage: usage, Type: ct})
		}
		zused, ztotal := Deps.GetZramStats()
		stats := &pb.SystemStats{Gpus: gs, Cpu: cpuInfo, Memory: &pb.MemoryInfo{
			Total:     vm.Total,
			Used:      vm.Used,
			Percent:   float32(vm.UsedPercent),
			Cached:    vm.Cached,
			Buffers:   vm.Buffers,
			Shared:    vm.Shared,
			ZramUsed:  zused,
			ZramTotal: ztotal,
			SwapTotal: sm.Total,
			SwapUsed:  sm.Used,
		}, Io: pbIO, Faults: faultInfo}
		psList, _ := ps.Processes()
		for _, p := range psList {
			if len(stats.Processes) >= systemStatsMaxProcesses {
				break
			}
			select {
			case <-c.Request.Context().Done():
				return
			default:
			}
			n, _ := p.Name()
			pp, _ := p.Ppid()
			ct, _ := p.CreateTime()
			ccp := 0.0
			if times, err := p.Times(); err == nil {
				totalCPU := times.Total()
				if prev, ok := procCPUSamples[p.Pid]; ok && prev.createTime == ct {
					dt := now.Sub(prev.sampleTime).Seconds()
					if dt > 0 {
						ccp = ((totalCPU - prev.totalCPU) / dt) * 100 / cpuScale
						if ccp < 0 || math.IsNaN(ccp) || math.IsInf(ccp, 0) {
							ccp = 0
						}
					}
				}
				if ct > 0 {
					procCPUSamples[p.Pid] = procCPUSample{createTime: ct, totalCPU: totalCPU, sampleTime: now}
				}
			}
			mp, _ := p.MemoryPercent()
			u, _ := p.Username()
			cmdl, _ := p.Cmdline()
			gmem, gid, gutil := uint32(0), uint32(0), uint32(0)
			if info, ok := gm[p.Pid]; ok {
				gmem, gid, gutil = info.Mem, info.GPU, info.Util
			}
			minF, majF := uint64(0), uint64(0)
			if faults, err := p.PageFaults(); err == nil && faults != nil {
				minF = faults.MinorFaults
				majF = faults.MajorFaults
			}
			currentPIDs[p.Pid] = struct{}{}
			n = boundedHandlerText(n, systemStatsMaxNameBytes)
			u = boundedHandlerText(u, systemStatsMaxUserBytes)
			cmdl = boundedHandlerText(cmdl, systemStatsMaxCommandLineBytes)
			stats.Processes = append(stats.Processes, &pb.Process{Pid: p.Pid, Ppid: pp, Name: n, Cpu: ccp, Mem: mp, User: u, GpuMem: gmem, GpuId: gid, GpuUtil: gutil, Cmdline: cmdl, CreateTime: ct, MinorFaults: minF, MajorFaults: majF})
			if !emittedSystemMetric && (ccp >= 80 || mp >= 20) {
				emitSystemMetricEvent(p.Pid, pp, n, ccp, mp, uint64(float64(vm.Total)*float64(mp)/100), "threshold")
				emittedSystemMetric = true
			}
		}
		for pid := range procCPUSamples {
			if _, ok := currentPIDs[pid]; !ok {
				delete(procCPUSamples, pid)
			}
		}
		data, err := proto.Marshal(stats)
		if err != nil {
			return
		}
		if err := wsstream.WriteMessage(conn, websocket.BinaryMessage, data); err != nil {
			return
		}
	}
}

func emitSystemMetricEvent(pid, ppid int32, comm string, cpuPercent float64, memoryPercent float32, memoryBytes uint64, alert string) {
	if Deps.BroadcastCh == nil {
		return
	}
	event := &pb.Event{
		Pid:           uint32(pid),
		Ppid:          uint32(maxInt(int(ppid), 0)),
		Type:          "system_metric",
		EventType:     pb.EventType_SYSTEM_METRIC,
		Tag:           "System Metric",
		Comm:          comm,
		ExtraInfo:     formatSystemMetricExtra(cpuPercent, memoryPercent, memoryBytes, alert),
		Bytes:         memoryBytes,
		RiskScore:     math.Max(float64(cpuPercent)/100, float64(memoryPercent)/100),
		Decision:      "ALERT",
		SchemaVersion: Deps.EventSchemaVersion,
	}
	Deps.SendTLSBridge(Deps.BroadcastCh, event)
}

func formatSystemMetricExtra(cpuPercent float64, memoryPercent float32, memoryBytes uint64, alert string) string {
	return fmt.Sprintf("cpu_percent=%.2f memory_percent=%.2f memory_bytes=%d alert=%s", cpuPercent, memoryPercent, memoryBytes, alert)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func deltaUint64(current, previous uint64) uint64 {
	if current >= previous {
		return current - previous
	}
	return 0
}
