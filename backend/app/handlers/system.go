package handlers

import (
	"bytes"
	"context"
	"debug/elf"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
)

// ---- moved from app/handlers_system.go ----

func HandleSystemLs(c *gin.Context) {
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

func HandleFilePreview(c *gin.Context) {
	targetPath := strings.TrimSpace(c.Query("path"))
	if targetPath == "" {
		c.JSON(400, gin.H{"error": "path is required"})
		return
	}
	preview, err := Deps.BuildFilePreview(targetPath)
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

func HandleFilePreviewStream(c *gin.Context) {
	targetPath := strings.TrimSpace(c.Query("path"))
	if targetPath == "" {
		c.JSON(400, gin.H{"error": "path is required"})
		return
	}

	previewRaw, err := Deps.BuildFilePreview(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(404, gin.H{"error": "path not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	preview, ok := previewRaw.(*FilePreviewResponse)
	if !ok {
		c.JSON(500, gin.H{"error": "invalid preview response"})
		return
	}
	if preview.IsDir || preview.PreviewType != "text" {
		c.JSON(415, gin.H{"error": "only text files can be streamed"})
		return
	}

	file, err := os.Open(preview.Path)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer file.Close()

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "no-store")
	c.Header("X-File-Size", strconv.FormatInt(preview.Size, 10))
	c.Status(http.StatusOK)

	flusher, _ := c.Writer.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-c.Request.Context().Done():
			return
		default:
		}

		n, readErr := file.Read(buf)
		if n > 0 {
			if _, writeErr := c.Writer.Write(buf[:n]); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr == io.EOF {
			return
		}
		if readErr != nil {
			return
		}
	}
}

func printableASCII(b byte) string {
	if b >= 32 && b <= 126 && unicode.IsPrint(rune(b)) {
		return string([]byte{b})
	}
	return "."
}

func HandleFileHex(c *gin.Context) {
	targetPath := strings.TrimSpace(c.Query("path"))
	if targetPath == "" {
		c.JSON(400, gin.H{"error": "path is required"})
		return
	}
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "4096"))
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 64*1024 {
		limit = 4096
	}

	absPath, err := filepath.Abs(filepath.Clean(targetPath))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		c.JSON(404, gin.H{"error": "file not found"})
		return
	}
	file, err := os.Open(absPath)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer file.Close()

	buf := make([]byte, limit)
	n, err := file.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	buf = buf[:n]
	rows := []gin.H{}
	for i := 0; i < len(buf); i += 16 {
		end := i + 16
		if end > len(buf) {
			end = len(buf)
		}
		chunk := buf[i:end]
		hexCells := make([]string, 0, 16)
		ascii := strings.Builder{}
		for _, b := range chunk {
			hexCells = append(hexCells, fmt.Sprintf("%02x", b))
			ascii.WriteString(printableASCII(b))
		}
		rows = append(rows, gin.H{"offset": offset + int64(i), "hex": hexCells, "ascii": ascii.String()})
	}
	nextOffset := offset + int64(n)
	c.JSON(200, gin.H{
		"path":       absPath,
		"size":       info.Size(),
		"offset":     offset,
		"limit":      limit,
		"bytesRead":  n,
		"nextOffset": nextOffset,
		"eof":        nextOffset >= info.Size(),
		"rows":       rows,
	})
}

func HandleFileELF(c *gin.Context) {
	targetPath := strings.TrimSpace(c.Query("path"))
	if targetPath == "" {
		c.JSON(400, gin.H{"error": "path is required"})
		return
	}
	absPath, err := filepath.Abs(filepath.Clean(targetPath))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		c.JSON(404, gin.H{"error": "file not found"})
		return
	}
	f, err := elf.Open(absPath)
	if err != nil {
		c.JSON(415, gin.H{"error": "not an ELF file"})
		return
	}
	defer f.Close()

	sections := []gin.H{}
	for i, s := range f.Sections {
		if i >= 128 {
			break
		}
		sections = append(sections, gin.H{"name": s.Name, "type": s.Type.String(), "flags": s.Flags.String(), "addr": s.Addr, "offset": s.Offset, "size": s.Size})
	}
	programs := []gin.H{}
	for i, p := range f.Progs {
		if i >= 64 {
			break
		}
		programs = append(programs, gin.H{"type": p.Type.String(), "flags": p.Flags.String(), "vaddr": p.Vaddr, "off": p.Off, "filesz": p.Filesz, "memsz": p.Memsz})
	}
	dynlibs, _ := f.DynString(elf.DT_NEEDED)
	dynSyms, _ := f.DynamicSymbols()
	staticSyms, _ := f.Symbols()
	limitSymbols := func(in []elf.Symbol) []gin.H {
		out := []gin.H{}
		for i, s := range in {
			if i >= 120 {
				break
			}
			out = append(out, gin.H{"name": s.Name, "info": s.Info, "other": s.Other, "section": s.Section.String(), "value": s.Value, "size": s.Size})
		}
		return out
	}

	disassembly := ""
	disassemblyError := ""
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "objdump", "-d", "--demangle", "--no-show-raw-insn", absPath)
	if out, err := cmd.CombinedOutput(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 240 {
			lines = lines[:240]
			disassemblyError = "Disassembly preview truncated to first 240 lines."
		}
		disassembly = strings.Join(lines, "\n")
	} else {
		disassemblyError = strings.TrimSpace(fmt.Sprintf("%v: %s", err, string(out)))
	}

	c.JSON(200, gin.H{
		"path": absPath,
		"size": info.Size(),
		"header": gin.H{
			"class": f.Class.String(), "data": f.Data.String(), "version": f.Version.String(), "osabi": f.OSABI.String(),
			"abiVersion": f.ABIVersion, "type": f.Type.String(), "machine": f.Machine.String(), "entry": f.Entry,
		},
		"sections":           sections,
		"programs":           programs,
		"dynamicLibraries":   dynlibs,
		"dynamicSymbols":     limitSymbols(dynSyms),
		"staticSymbols":      limitSymbols(staticSyms),
		"disassembly":        disassembly,
		"disassemblyError":   disassemblyError,
		"dynamicSymbolCount": len(dynSyms),
		"staticSymbolCount":  len(staticSyms),
	})
}

func HandleSystemHome(c *gin.Context) {
	c.JSON(200, gin.H{"path": Deps.GetRealHomeDir()})
}

func HandleDownload(c *gin.Context) {
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

func HandleUpload(c *gin.Context) {
	dir := c.Query("path")
	if dir == "" {
		dir = Deps.GetRealHomeDir()
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

func RegisterSystemRoutes(rg *gin.RouterGroup) {
	rg.GET("/ls", HandleSystemLs)
	rg.GET("/file-preview", HandleFilePreview)
	rg.GET("/file-preview/stream", HandleFilePreviewStream)
	rg.GET("/file-hex", HandleFileHex)
	rg.GET("/file-elf", HandleFileELF)
	rg.GET("/home", HandleSystemHome)
	rg.GET("/download", HandleDownload)
	rg.POST("/upload", HandleUpload)
	rg.POST("/benchmark", HandleRunBenchmark)
	rg.GET("/benchmark", HandleGetBenchmarkResults)
	rg.GET("/tracked-comms", HandleTrackedComms)
	rg.POST("/process/signal", HandleProcessSignal)
	rg.GET("/process/maps", HandleProcessMaps)
	rg.GET("/sensors", HandleSensors)
	rg.GET("/cameras", HandleCameras)
	rg.GET("/camera/snapshot", HandleCameraSnapshot)
	rg.GET("/microphones", HandleMicrophones)
	rg.POST("/run", HandleRun)
	rg.GET("/user-info", HandleUserInfo)
	rg.GET("/users", HandleUsers)
	rg.GET("/process/io", HandleProcessIO)
}