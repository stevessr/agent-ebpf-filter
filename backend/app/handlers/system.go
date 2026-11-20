package handlers

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"
)

func HandleRun(c *gin.Context) {
	var r struct {
		Comm string   `json:"comm"`
		Args []string `json:"args"`
		User string   `json:"user"`
		Cwd  string   `json:"cwd"`
	}
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	wb := Deps.ResolveWrapperPath()
	if wb == "" {
		c.JSON(500, gin.H{"error": "wrapper not found"})
		return
	}

	wrapperArgs := []string{}
	if r.User != "" {
		wrapperArgs = append(wrapperArgs, "--user", r.User)
	}
	if r.Cwd != "" {
		wrapperArgs = append(wrapperArgs, "--cwd", r.Cwd)
	}
	wrapperArgs = append(wrapperArgs, r.Comm)
	wrapperArgs = append(wrapperArgs, r.Args...)

	cmd := exec.Command(wb, wrapperArgs...)
	cmd.Env = os.Environ()
	Deps.DropPrivileges(cmd)
	if err := cmd.Start(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "started", "pid": cmd.Process.Pid})
}

func HandleUserInfo(c *gin.Context) {
	u, err := user.Current()
	if err != nil {
		c.JSON(200, gin.H{"username": "unknown", "home": "/tmp", "uid": "0"})
		return
	}
	c.JSON(200, gin.H{"username": u.Username, "home": u.HomeDir, "uid": u.Uid})
}

func HandleUsers(c *gin.Context) {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	users := []gin.H{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		uid, _ := strconv.Atoi(parts[2])
		// Include real users (uid >= 1000) and root
		if uid >= 1000 || uid == 0 {
			users = append(users, gin.H{
				"username": parts[0],
				"uid":      uid,
				"home":     parts[5],
				"shell":    parts[6],
			})
		}
	}
	c.JSON(200, users)
}

func HandleProcessExe(c *gin.Context) {
	pidStr := c.Query("pid")
	if pidStr == "" {
		c.JSON(400, gin.H{"error": "pid required"})
		return
	}
	exePath, err := os.Readlink(fmt.Sprintf("/proc/%s/exe", pidStr))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"path": exePath})
}

func HandleProcessIO(c *gin.Context) {
	pidStr := c.Query("pid")
	if pidStr == "" {
		c.JSON(400, gin.H{"error": "pid required"})
		return
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%s/io", pidStr))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	info := gin.H{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			info[key] = val
		}
	}
	c.JSON(200, info)
}

func HandleSystemdServices(c *gin.Context) {
	scope := c.DefaultQuery("scope", "system")
	args := []string{"list-units", "--type=service", "--all", "--no-legend", "--no-pager"}
	if scope == "user" {
		args = append([]string{"--user"}, args...)
	}
	cmd := exec.Command("systemctl", args...)
	if scope == "user" {
		if uid, _, ok := Deps.OriginalInvokerIDs(); ok {
			uidStr := strconv.FormatUint(uint64(uid), 10)
			cmd.Env = append(os.Environ(),
				"XDG_RUNTIME_DIR=/run/user/"+uidStr,
				"DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/"+uidStr+"/bus",
			)
		}
		Deps.ConfigureCommandForRealUser(cmd)
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

func HandleSystemdControl(c *gin.Context) {
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
		if uid, _, ok := Deps.OriginalInvokerIDs(); ok {
			uidStr := strconv.FormatUint(uint64(uid), 10)
			cmd.Env = append(os.Environ(),
				"XDG_RUNTIME_DIR=/run/user/"+uidStr,
				"DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/"+uidStr+"/bus",
			)
		}
		Deps.ConfigureCommandForRealUser(cmd)
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

func HandleSystemdLogs(c *gin.Context) {
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
		if uid, _, ok := Deps.OriginalInvokerIDs(); ok {
			uidStr := strconv.FormatUint(uint64(uid), 10)
			cmd.Env = append(os.Environ(),
				"XDG_RUNTIME_DIR=/run/user/"+uidStr,
				"DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/"+uidStr+"/bus",
			)
		}
		Deps.ConfigureCommandForRealUser(cmd)
	}
	out, err := cmd.Output()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"unit": unit, "logs": string(out)})
}

func HandleTrackedComms(c *gin.Context) {
	items := []string{}
	if Deps.TrackerMaps == nil {
		c.JSON(200, items)
		return
	}
	iter := Deps.TrackerMaps.TrackedCommsIterate()
	var k [16]byte
	var tid uint32
	for iter.Next(&k, &tid) {
		items = append(items, string(bytes.TrimRight(k[:], "\x00")))
	}
	c.JSON(200, items)
}

func HandleProcessSignal(c *gin.Context) {
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

func HandleProcessMaps(c *gin.Context) {
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

func RegisterSystemRoutes(rg *gin.RouterGroup, fileAccessMiddleware ...gin.HandlerFunc) {
	fileRoute := func(handler gin.HandlerFunc) []gin.HandlerFunc {
		handlers := make([]gin.HandlerFunc, 0, len(fileAccessMiddleware)+1)
		handlers = append(handlers, fileAccessMiddleware...)
		return append(handlers, handler)
	}
	rg.GET("/ls", fileRoute(HandleSystemLs)...)
	rg.GET("/file-preview", fileRoute(HandleFilePreview)...)
	rg.GET("/file-preview/stream", fileRoute(HandleFilePreviewStream)...)
	rg.GET("/file-hex", fileRoute(HandleFileHex)...)
	rg.GET("/file-elf", fileRoute(HandleFileELF)...)
	rg.GET("/home", HandleSystemHome)
	rg.GET("/download", fileRoute(HandleDownload)...)
	rg.POST("/upload", fileRoute(HandleUpload)...)
	rg.POST("/benchmark", HandleRunBenchmark)
	rg.GET("/benchmark", HandleGetBenchmarkResults)
	rg.GET("/tracked-comms", HandleTrackedComms)
	rg.POST("/process/signal", HandleProcessSignal)
	rg.GET("/process/maps", HandleProcessMaps)
	rg.GET("/sensors", HandleSensors)
	rg.GET("/cameras", HandleCameras)
	rg.GET("/camera/snapshot", HandleCameraSnapshot)
	rg.GET("/microphones", HandleMicrophones)
	rg.GET("/user-info", HandleUserInfo)
	rg.GET("/users", HandleUsers)
	rg.GET("/process/exe", HandleProcessExe)
	rg.GET("/process/io", HandleProcessIO)
}

// RegisterSystemRunRoute keeps the critical command runner separate from the
// general system routes so the app layer must explicitly attach its build and
// runtime security gates when registering the endpoint.
func RegisterSystemRunRoute(rg *gin.RouterGroup, middleware ...gin.HandlerFunc) {
	handlers := make([]gin.HandlerFunc, 0, len(middleware)+1)
	handlers = append(handlers, middleware...)
	handlers = append(handlers, HandleRun)
	rg.POST("/run", handlers...)
}
