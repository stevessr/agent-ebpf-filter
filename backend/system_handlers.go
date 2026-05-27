package main

import (
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"
)

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
		l = append(l, gin.H{"name": v.Name(), "isDir": v.IsDir(), "path": fp, "mimeType": mType, "size": size, "modTime": modTime})
	}
	c.JSON(200, gin.H{"items": l, "total": total, "offset": start, "limit": limit})
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

func handleSystemdServices(c *gin.Context) {
	scope := c.DefaultQuery("scope", "system")
	args := []string{"list-units", "--type=service", "--all", "--no-legend", "--no-pager"}
	if scope == "user" {
		args = append([]string{"--user"}, args...)
	}
	cmd := exec.Command("systemctl", args...)
	if scope == "user" {
		if uid, _, ok := originalInvokerIDs(); ok {
			uidStr := strconv.FormatUint(uint64(uid), 10)
			cmd.Env = append(os.Environ(),
				"XDG_RUNTIME_DIR=/run/user/"+uidStr,
				"DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/"+uidStr+"/bus",
			)
		}
		configureCommandForRealUser(cmd)
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
		services = append(services, gin.H{"unit": fields[0], "load": fields[1], "active": fields[2], "sub": fields[3], "description": strings.Join(fields[4:], " ")})
	}
	c.JSON(200, services)
}

func handleSystemdControl(c *gin.Context) {
	var req struct {
		Unit   string `json:"unit"`
		Action string `json:"action"`
		Scope  string `json:"scope"`
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
		if uid, _, ok := originalInvokerIDs(); ok {
			uidStr := strconv.FormatUint(uint64(uid), 10)
			cmd.Env = append(os.Environ(),
				"XDG_RUNTIME_DIR=/run/user/"+uidStr,
				"DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/"+uidStr+"/bus",
			)
		}
		configureCommandForRealUser(cmd)
		if err := cmd.Run(); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	} else {
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
	if scope == "user" {
		if uid, _, ok := originalInvokerIDs(); ok {
			uidStr := strconv.FormatUint(uint64(uid), 10)
			cmd.Env = append(os.Environ(),
				"XDG_RUNTIME_DIR=/run/user/"+uidStr,
				"DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/"+uidStr+"/bus",
			)
		}
		configureCommandForRealUser(cmd)
	}
	out, err := cmd.Output()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"unit": unit, "logs": string(out)})
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
		Signal string `json:"signal"`
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

func handleProcessMaps(c *gin.Context) {
	pidStr := c.Query("pid")
	if pidStr == "" {
		c.JSON(400, gin.H{"error": "pid required"})
		return
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%s/maps", pidStr))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"maps": string(data)})
}

func registerSystemRoutes(rg *gin.RouterGroup) {
	rg.GET("/ls", handleSystemLs)
	rg.GET("/file-preview", handleFilePreview)
	rg.GET("/home", handleSystemHome)
	rg.GET("/download", handleDownload)
	rg.POST("/upload", handleUpload)
	rg.GET("/env", handleListLaunchEnvEntries)
	rg.GET("/bootstrap-health", handleBootstrapHealth)
	rg.GET("/collector-health", handleCollectorHealth)
	rg.POST("/benchmark", handleRunBenchmark)
	rg.GET("/benchmark", handleGetBenchmarkResults)
	rg.GET("/otel-health", handleOTelHealth)
	rg.GET("/domain-forward/status", handleDomainForwardProxyStatus)
	rg.POST("/run", systemRunEnabledMiddleware(), handleRun)
	rg.GET("/systemd", handleSystemdServices)
	rg.POST("/systemd/control", handleSystemdControl)
	rg.GET("/systemd/logs", handleSystemdLogs)
	rg.GET("/sensors", handleSensors)
	rg.GET("/cameras", handleCameras)
	rg.GET("/microphones", handleMicrophones)
	rg.GET("/camera/snapshot", handleCameraSnapshot)
	rg.GET("/tracked-comms", handleTrackedComms)
	rg.POST("/process/signal", handleProcessSignal)
	rg.GET("/process/maps", handleProcessMaps)
}
