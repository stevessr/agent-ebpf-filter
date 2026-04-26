package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"agent-ebpf-filter/pb"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	gnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/disk"
	ps "github.com/shirou/gopsutil/v3/process"
	"google.golang.org/protobuf/proto"
)
// ... keeping other imports intact, just ensuring bufio is there. Wait, I will just rewrite the func.

func handleSystemLs(c *gin.Context) {
	p := c.DefaultQuery("path", "/")
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "0"))
	showHidden := c.Query("showHidden") == "true"

	e, err := os.ReadDir(p)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// Filter
	var filtered []os.DirEntry
	for _, v := range e {
		if !showHidden && strings.HasPrefix(v.Name(), ".") {
			continue
		}
		filtered = append(filtered, v)
	}

	total := len(filtered)
	start := offset
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}

	end := total
	if limit > 0 && start+limit < total {
		end = start + limit
	}

	slice := filtered[start:end]
	l := []gin.H{}
	for _, v := range slice {
		fp := filepath.Join(p, v.Name())
		mType := ""
		var size int64
		var modTime string

		info, err := v.Info()
		if err == nil {
			size = info.Size()
			modTime = info.ModTime().Format("2006-01-02T15:04:05Z07:00")
			if !v.IsDir() {
				mType = mime.TypeByExtension(filepath.Ext(v.Name()))
			}
		}

		l = append(l, gin.H{
			"name":     v.Name(),
			"isDir":    v.IsDir(),
			"path":     fp,
			"mimeType": mType,
			"size":     size,
			"modTime":  modTime,
		})
	}
	c.JSON(200, gin.H{
		"items":  l,
		"total":  total,
		"offset": start,
		"limit":  limit,
	})
}

func handleFilePreview(c *gin.Context) {
	targetPath := strings.TrimSpace(c.Query("path"))
	if targetPath == "" {
		c.JSON(400, gin.H{"error": "path is required"})
		return
	}

	preview, err := buildFilePreview(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(404, gin.H{"error": "path not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, preview)
}

func handleSystemHome(c *gin.Context) {
	c.JSON(200, gin.H{"path": getRealHomeDir()})
}

func handleDownload(c *gin.Context) {
	p := c.Query("path")
	if p == "" {
		c.JSON(400, gin.H{"error": "path is required"})
		return
	}
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		c.JSON(404, gin.H{"error": "file not found"})
		return
	}
	c.File(p)
}

func handleUpload(c *gin.Context) {
	dir := c.Query("path")
	if dir == "" {
		dir = getRealHomeDir()
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "no file uploaded"})
		return
	}
	dst := filepath.Join(dir, file.Filename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok", "path": dst})
}

func handleRun(c *gin.Context) {
	var r struct {
		Comm string   `json:"comm"`
		Args []string `json:"args"`
	}
	if err := c.ShouldBindJSON(&r); err == nil {
		wb := resolveWrapperPath()
		if wb == "" {
			c.JSON(500, gin.H{"error": "wrapper not found"})
			return
		}
		cmd := exec.Command(wb, append([]string{r.Comm}, r.Args...)...)
		cmd.Env = os.Environ()
		dropPrivileges(cmd)
		if err := cmd.Start(); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"status": "started", "pid": cmd.Process.Pid})
	}
}

func serveSystemStatsWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	intervalStr := c.DefaultQuery("interval", "2000")
	iv, _ := time.ParseDuration(intervalStr + "ms")
	if iv < 500*time.Millisecond {
		iv = 500 * time.Millisecond
	}
	ticker := time.NewTicker(iv)
	defer ticker.Stop()

	coreTypes := getCoreTypes()
	lastFaults, err := readVMFaultCounters()
	if err != nil {
		lastFaults = vmFaultCounters{}
	}
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
	for range ticker.C {
		now := time.Now()
		gm, gs := getGPUMetrics()
		vm, _ := mem.VirtualMemory()
		cc, _ := cpu.Percent(0, false)
		cp, _ := cpu.Percent(0, true)
		netIO, _ := gnet.IOCounters(true)
		diskIO, _ := disk.IOCounters()
		pbIO := &pb.IOInfo{}
		vmFaults, faultErr := readVMFaultCounters()
		faultInfo := &pb.FaultInfo{}
		currentPIDs := make(map[int32]struct{})
		if faultErr == nil {
			pageFaults := vmFaults.pageFaults
			majorFaults := vmFaults.majorFaults
			minorFaults := uint64(0)
			if pageFaults >= majorFaults {
				minorFaults = pageFaults - majorFaults
			}
			faultInfo.PageFaults = pageFaults
			faultInfo.MajorFaults = majorFaults
			faultInfo.MinorFaults = minorFaults

			dt := now.Sub(lastFaultTime).Seconds()
			if dt > 0 {
				pageDelta := deltaUint64(pageFaults, lastFaults.pageFaults)
				majorDelta := deltaUint64(majorFaults, lastFaults.majorFaults)
				swapInDelta := deltaUint64(vmFaults.swapIn, lastFaults.swapIn)
				swapOutDelta := deltaUint64(vmFaults.swapOut, lastFaults.swapOut)

				faultInfo.PageFaultRate = float64(pageDelta) / dt
				faultInfo.MajorFaultRate = float64(majorDelta) / dt
				faultInfo.MinorFaultRate = faultInfo.PageFaultRate - faultInfo.MajorFaultRate
				if faultInfo.MinorFaultRate < 0 {
					faultInfo.MinorFaultRate = 0
				}
				faultInfo.SwapIn = vmFaults.swapIn
				faultInfo.SwapOut = vmFaults.swapOut
				faultInfo.SwapInRate = float64(swapInDelta) / dt
				faultInfo.SwapOutRate = float64(swapOutDelta) / dt
			}
			lastFaults = vmFaults
			lastFaultTime = now
		}

		for _, n := range netIO {
			pbIO.Networks = append(pbIO.Networks, &pb.NetworkInterface{Name: n.Name, RecvBytes: n.BytesRecv, SentBytes: n.BytesSent})
			pbIO.TotalNetRecvBytes += n.BytesRecv
			pbIO.TotalNetSentBytes += n.BytesSent
		}
		for name, d := range diskIO {
			pbIO.Disks = append(pbIO.Disks, &pb.DiskDevice{Name: name, ReadBytes: d.ReadBytes, WriteBytes: d.WriteBytes})
			pbIO.TotalReadBytes += d.ReadBytes
			pbIO.TotalWriteBytes += d.WriteBytes
		}
		cpuInfo := &pb.CPUInfo{Total: cc[0], Cores: cp}
		for i, usage := range cp {
			ct := pb.CPUInfo_Core_PERFORMANCE
			if i < len(coreTypes) {
				ct = coreTypes[i]
			}
			cpuInfo.CoreDetails = append(cpuInfo.CoreDetails, &pb.CPUInfo_Core{Index: uint32(i), Usage: usage, Type: ct})
		}
		stats := &pb.SystemStats{Gpus: gs, Cpu: cpuInfo, Memory: &pb.MemoryInfo{Total: vm.Total, Used: vm.Used, Percent: float32(vm.UsedPercent)}, Io: pbIO, Faults: faultInfo}
		psList, _ := ps.Processes()
		for _, p := range psList {
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
				gmem, gid, gutil = info.mem, info.gpu, info.util
			}
			minorFaults, majorFaults := uint64(0), uint64(0)
			if faults, err := p.PageFaults(); err == nil && faults != nil {
				minorFaults = faults.MinorFaults
				majorFaults = faults.MajorFaults
			}
			currentPIDs[p.Pid] = struct{}{}
			stats.Processes = append(stats.Processes, &pb.Process{Pid: p.Pid, Ppid: pp, Name: n, Cpu: ccp, Mem: mp, User: u, GpuMem: gmem, GpuId: gid, GpuUtil: gutil, Cmdline: cmdl, CreateTime: ct, MinorFaults: minorFaults, MajorFaults: majorFaults})
		}
		for pid := range procCPUSamples {
			if _, ok := currentPIDs[pid]; !ok {
				delete(procCPUSamples, pid)
			}
		}
		data, _ := proto.Marshal(stats)
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			return
		}
	}
}

func handleSystemdServices(c *gin.Context) {
	scope := c.DefaultQuery("scope", "system")
	args := []string{"list-units", "--type=service", "--all", "--no-legend", "--no-pager"}
	if scope == "user" {
		args = append([]string{"--user"}, args...)
	}

	cmd := exec.Command("systemctl", args...)
	// In user scope, we might need to point to the correct user bus if running as root
	if scope == "user" {
		if uid := os.Getenv("AGENT_REAL_UID"); uid != "" {
			// This is a common way to talk to user session from sudo/root
			cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR=/run/user/"+uid)
		}
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("%v: %s", err, string(out))})
		return
	}

	services := []gin.H{}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		services = append(services, gin.H{
			"unit":        fields[0],
			"load":        fields[1],
			"active":      fields[2],
			"sub":         fields[3],
			"description": strings.Join(fields[4:], " "),
		})
	}
	c.JSON(200, services)
}

func handleSystemdControl(c *gin.Context) {
	var req struct {
		Unit   string `json:"unit"`
		Action string `json:"action"` // start, stop, restart
		Scope  string `json:"scope"`  // system, user
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	validActions := map[string]bool{"start": true, "stop": true, "restart": true}
	if !validActions[req.Action] {
		c.JSON(400, gin.H{"error": "invalid action"})
		return
	}

	args := []string{req.Action, req.Unit}
	if req.Scope == "user" {
		args = append([]string{"--user"}, args...)
		cmd := exec.Command("systemctl", args...)
		if err := cmd.Run(); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	} else {
		// Use pkexec or sudo if available for systemctl actions
		fullArgs := append([]string{"systemctl"}, args...)
		cmd := exec.Command("pkexec", fullArgs...)
		if err := cmd.Run(); err != nil {
			cmd = exec.Command("systemctl", args...)
			if err := cmd.Run(); err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
		}
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func handleSystemdLogs(c *gin.Context) {
	unit := c.Query("unit")
	lines := c.DefaultQuery("lines", "100")
	scope := c.DefaultQuery("scope", "system")
	if unit == "" {
		c.JSON(400, gin.H{"error": "unit is required"})
		return
	}

	args := []string{"-u", unit, "-n", lines, "--no-pager"}
	if scope == "user" {
		args = append([]string{"--user"}, args...)
	}

	cmd := exec.Command("journalctl", args...)
	out, err := cmd.Output()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"unit": unit,
		"logs": string(out),
	})
}

func handleSensors(c *gin.Context) {
	temps, _ := host.SensorsTemperatures()
	// You could also add fan speeds or voltages here if available via other means
	c.JSON(200, temps)
}

func handleCameras(c *gin.Context) {
	matches, _ := filepath.Glob("/dev/video*")
	// Filter out non-capture devices (usually even numbers are capture, odd are metadata/etc)
	// For simplicity, return all and let user choose
	c.JSON(200, matches)
}

func handleCameraSnapshot(c *gin.Context) {
	dev := c.Query("device")
	if dev == "" {
		dev = "/dev/video0"
	}

	// Try to grab a single frame using ffmpeg (fast and common)
	// Use -f v4l2 to specify input format
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("camera_snap_%d.jpg", time.Now().UnixNano()))
	cmd := exec.Command("ffmpeg", "-y", "-f", "v4l2", "-video_size", "640x480", "-i", dev, "-frames:v", "1", tmpFile)
	
	if err := cmd.Run(); err != nil {
		c.JSON(500, gin.H{"error": "Failed to capture frame: " + err.Error() + ". Ensure ffmpeg and v4l2 are installed."})
		return
	}
	defer os.Remove(tmpFile)

	c.File(tmpFile)
}

func handleTrackedComms(c *gin.Context) {
	items := []string{}
	iter := trackerMaps.TrackedComms.Iterate()
	var k [16]byte
	var tid uint32
	for iter.Next(&k, &tid) {
		items = append(items, string(bytes.TrimRight(k[:], "\x00")))
	}
	c.JSON(200, items)
}

func handleProcessSignal(c *gin.Context) {
	var req struct {
		PID    int    `json:"pid"`
		Signal string `json:"signal"` // stop, cont, kill, term
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	p, err := os.FindProcess(req.PID)
	if err != nil {
		c.JSON(404, gin.H{"error": "process not found"})
		return
	}

	var sig os.Signal
	switch strings.ToLower(req.Signal) {
	case "stop":
		sig = syscall.SIGSTOP
	case "cont":
		sig = syscall.SIGCONT
	case "kill":
		sig = syscall.SIGKILL
	case "term":
		sig = syscall.SIGTERM
	default:
		c.JSON(400, gin.H{"error": "unsupported signal"})
		return
	}

	if err := p.Signal(sig); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func serveCameraWS(c *gin.Context) {
	dev := c.Query("device")
	if dev == "" {
		dev = "/dev/video0"
	}
	
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	cmd := exec.Command("ffmpeg", "-y", "-f", "v4l2", "-video_size", "640x480", "-framerate", "15", "-i", dev, "-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "5", "-")
	
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Error: Failed to create stdout pipe"))
		return
	}

	if err := cmd.Start(); err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Error: Failed to start ffmpeg: "+err.Error()))
		return
	}
	defer cmd.Process.Kill()

	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 5*1024*1024)
	
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		soi := bytes.Index(data, []byte{0xff, 0xd8})
		if soi < 0 {
			if len(data) > 0 && data[len(data)-1] == 0xff {
				return len(data) - 1, nil, nil
			}
			return len(data), nil, nil
		}
		eoi := bytes.Index(data[soi:], []byte{0xff, 0xd9})
		if eoi < 0 {
			if atEOF {
				return 0, nil, io.ErrUnexpectedEOF
			}
			return soi, nil, nil
		}
		frameLen := eoi + 2
		return soi + frameLen, data[soi : soi+frameLen], nil
	})

	for scanner.Scan() {
		frame := scanner.Bytes()
		if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			break
		}
	}
}

func registerSystemRoutes(rg *gin.RouterGroup) {
	rg.GET("/ls", handleSystemLs)
	rg.GET("/file-preview", handleFilePreview)
	rg.GET("/home", handleSystemHome)
	rg.GET("/download", handleDownload)
	rg.POST("/upload", handleUpload)
	rg.GET("/env", handleListLaunchEnvEntries)
	rg.POST("/run", handleRun)
	rg.GET("/systemd", handleSystemdServices)
	rg.POST("/systemd/control", handleSystemdControl)
	rg.GET("/systemd/logs", handleSystemdLogs)
	rg.GET("/sensors", handleSensors)
	rg.GET("/cameras", handleCameras)
	rg.GET("/camera/snapshot", handleCameraSnapshot)
	rg.GET("/tracked-comms", handleTrackedComms)
	rg.POST("/process/signal", handleProcessSignal)
}
