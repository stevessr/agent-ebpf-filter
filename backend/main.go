package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/pb"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/creack/pty/v2"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	gnet "github.com/shirou/gopsutil/v3/net"
	ps "github.com/shirou/gopsutil/v3/process"
	"google.golang.org/protobuf/proto"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

const udsPath = "/tmp/agent-ebpf.sock"
const ebpfPinRoot = "/sys/fs/bpf/agent-ebpf"
const ebpfPinMapsDir = ebpfPinRoot + "/maps"
const ebpfPinLinksDir = ebpfPinRoot + "/links"

type bpfEvent struct {
	PID, PPID, UID, Type, TagID uint32
	Comm                        [16]byte
	Path                        [256]byte
}

type WrapperRule struct {
	Comm         string   `json:"comm"`
	Action       string   `json:"action"`
	RewrittenCmd []string `json:"rewritten_cmd,omitempty"`
}

type shellControlMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

type trackerMapSet struct {
	AgentPids    *ebpf.Map
	TrackedComms *ebpf.Map
	TrackedPaths *ebpf.Map
	Events       *ebpf.Map
}

type ExportConfig struct {
	Tags  []string          `json:"tags"`
	Comms map[string]string `json:"comms"`
	Paths map[string]string `json:"paths"`
}

// HookType distinguishes how the hook intercepts the agent CLI.
// "native" = write into the agent CLI's own config file (preferred).
// "wrapper" = install a shell alias that routes through agent-wrapper.
type HookType string

const (
	HookTypeNative  HookType = "native"
	HookTypeWrapper HookType = "wrapper"
)

type HookDef struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	TargetCmd   string   `json:"target_cmd"`
	HookType    HookType `json:"hook_type"`
	// NativeConfigPath is the path to the agent CLI's config file (for native hooks).
	NativeConfigPath string `json:"-"`
}

var availableHooks = []HookDef{
	{
		ID: "claude", Name: "Claude Code", HookType: HookTypeNative,
		Description:      "Uses Claude Code's built-in PreToolUse hook to intercept all tool calls (recommended)",
		TargetCmd:        "claude",
		NativeConfigPath: func() string { h, _ := os.UserHomeDir(); return filepath.Join(h, ".claude", "settings.json") }(),
	},
	{
		ID: "gemini", Name: "Gemini CLI", HookType: HookTypeWrapper,
		Description: "Intercepts gemini / rtk commands via shell alias wrapper",
		TargetCmd:   "gemini",
	},
	{
		ID: "copilot", Name: "GitHub Copilot", HookType: HookTypeWrapper,
		Description: "Intercepts gh copilot commands via shell alias wrapper",
		TargetCmd:   "gh",
	},
	{
		ID: "cursor", Name: "Cursor", HookType: HookTypeWrapper,
		Description: "Intercepts cursor execution via shell alias wrapper",
		TargetCmd:   "cursor",
	},
}

func getShellConfigPath() string {
	home, _ := os.UserHomeDir()
	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "zsh") {
		return filepath.Join(home, ".zshrc")
	}
	return filepath.Join(home, ".bashrc")
}

// isNativeHookInstalled checks whether the agent-ebpf PreToolUse hook is present
// in the Claude Code settings.json.
func isNativeHookInstalled(cfgPath string) bool {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "agent-ebpf-hook")
}

func isWrapperHookInstalled(cmd string) bool {
	p := getShellConfigPath()
	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), fmt.Sprintf("alias %s=", cmd))
}

func isHookInstalled(h HookDef) bool {
	if h.HookType == HookTypeNative {
		return isNativeHookInstalled(h.NativeConfigPath)
	}
	return isWrapperHookInstalled(h.TargetCmd)
}

// installNativeHook injects a PreToolUse hook into the Claude Code settings.json
// that POSTs every tool call to our backend for inspection.
func installNativeHook(cfgPath string) error {
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return err
	}

	// Read existing config (may not exist yet).
	var cfg map[string]interface{}
	if b, err := os.ReadFile(cfgPath); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	// Build the hook entry.
	hookEntry := map[string]interface{}{
		"type":          "command",
		"command":       `curl -s -X POST http://localhost:8080/hooks/event -H 'Content-Type: application/json' -d @- || true`,
		"statusMessage": "agent-ebpf-hook: inspecting...",
		"async":         true,
	}
	matcher := map[string]interface{}{
		"matcher": "",
		"hooks":   []interface{}{hookEntry},
	}

	// Merge into existing hooks.PreToolUse array.
	hooks, _ := cfg["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = make(map[string]interface{})
	}
	preToolUse, _ := hooks["PreToolUse"].([]interface{})

	// Remove any existing agent-ebpf-hook entry to avoid duplicates.
	filtered := []interface{}{}
	for _, m := range preToolUse {
		if mm, ok := m.(map[string]interface{}); ok {
			hs, _ := mm["hooks"].([]interface{})
			isOurs := false
			for _, h := range hs {
				if hm, ok := h.(map[string]interface{}); ok {
					if cmd, _ := hm["command"].(string); strings.Contains(cmd, "agent-ebpf-hook") {
						isOurs = true
					}
				}
			}
			if !isOurs {
				filtered = append(filtered, m)
			}
		}
	}
	hooks["PreToolUse"] = append(filtered, matcher)
	cfg["hooks"] = hooks

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, b, 0644)
}

// uninstallNativeHook removes the agent-ebpf PreToolUse hook from settings.json.
func uninstallNativeHook(cfgPath string) error {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil // nothing to do
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return err
	}
	hooks, _ := cfg["hooks"].(map[string]interface{})
	if hooks == nil {
		return nil
	}
	preToolUse, _ := hooks["PreToolUse"].([]interface{})
	filtered := []interface{}{}
	for _, m := range preToolUse {
		if mm, ok := m.(map[string]interface{}); ok {
			hs, _ := mm["hooks"].([]interface{})
			isOurs := false
			for _, h := range hs {
				if hm, ok := h.(map[string]interface{}); ok {
					if cmd, _ := hm["command"].(string); strings.Contains(cmd, "agent-ebpf-hook") {
						isOurs = true
					}
				}
			}
			if !isOurs {
				filtered = append(filtered, m)
			}
		}
	}
	if len(filtered) == 0 {
		delete(hooks, "PreToolUse")
	} else {
		hooks["PreToolUse"] = filtered
	}
	if len(hooks) == 0 {
		delete(cfg, "hooks")
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0644)
}

var (
	trackerMaps trackerMapSet

	tagsMu      sync.RWMutex
	tagMap             = map[uint32]string{0: "Unknown", 1: "AI Agent", 2: "Git", 3: "Build Tool", 4: "Package Manager", 5: "Runtime", 6: "System Tool", 7: "Network Tool", 8: "Security"}
	tagNameToID        = map[string]uint32{"AI Agent": 1, "Git": 2, "Build Tool": 3, "Package Manager": 4, "Runtime": 5, "System Tool": 6, "Network Tool": 7, "Security": 8}
	nextTagID   uint32 = 9

	rulesMu      sync.RWMutex
	wrapperRules = make(map[string]WrapperRule)

	nvmlInitialized bool

	// For non-NVIDIA GPU tracking (Intel/AMD via fdinfo)
	fdinfoHistory = make(map[string]uint64) // pid:fd -> last_engine_ns
	fdinfoTime    time.Time
)

func init() {
	if ret := nvml.Init(); ret == nvml.SUCCESS {
		nvmlInitialized = true
	} else {
		log.Printf("NVML Init failed: %v", nvml.ErrorString(ret))
	}
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if os.Getenv("GIN_MODE") != "release" && (os.Getenv("DISABLE_AUTH") == "true" || os.Getenv("DISABLE_AUTH") == "") {
			c.Next()
			return
		}
		apiKey := c.GetHeader("X-API-KEY")
		expectedKey := os.Getenv("AGENT_API_KEY")
		if expectedKey == "" {
			expectedKey = "agent-secret-123"
		}
		if apiKey != expectedKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		c.Next()
	}
}

func getTagName(id uint32) string {
	tagsMu.RLock()
	defer tagsMu.RUnlock()
	if name, ok := tagMap[id]; ok {
		return name
	}
	return fmt.Sprintf("Tag-%d", id)
}

func getTagID(name string) uint32 {
	tagsMu.Lock()
	defer tagsMu.Unlock()
	if id, ok := tagNameToID[name]; ok {
		return id
	}
	id := nextTagID
	tagMap[id] = name
	tagNameToID[name] = id
	nextTagID++
	return id
}

type gpuInfo struct{ mem, gpu, util uint32 }

type vmFaultCounters struct {
	pageFaults  uint64
	majorFaults uint64
	swapIn      uint64
	swapOut     uint64
}

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
			globalStats = append(globalStats, &pb.GPUStatus{
				Index: uint32(i), Name: name, UtilGpu: util.Gpu, UtilMem: util.Memory,
				MemTotal: uint32(mInfo.Total / 1024 / 1024), MemUsed: uint32(mInfo.Used / 1024 / 1024), Temp: temp,
			})
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

func deltaUint64(current, previous uint64) uint64 {
	if current >= previous {
		return current - previous
	}
	return 0
}

func resolveWrapperPath() string {
	if override := os.Getenv("AGENT_WRAPPER_PATH"); override != "" {
		if info, err := os.Stat(override); err == nil && !info.IsDir() {
			return override
		}
	}

	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		sourceDir := filepath.Dir(sourceFile)
		candidates := []string{
			filepath.Join(sourceDir, "..", "agent-wrapper"),
			filepath.Join(sourceDir, "agent-wrapper"),
			filepath.Join(sourceDir, "..", "..", "agent-wrapper"),
		}
		for _, cnd := range candidates {
			if info, err := os.Stat(cnd); err == nil && !info.IsDir() {
				return cnd
			}
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		for _, rel := range []string{
			"agent-wrapper",
			"../agent-wrapper",
			"../../agent-wrapper",
			"../../../agent-wrapper",
		} {
			cnd := filepath.Clean(filepath.Join(cwd, rel))
			if info, err := os.Stat(cnd); err == nil && !info.IsDir() {
				return cnd
			}
		}
	}

	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		for _, rel := range []string{
			"agent-wrapper",
			"../agent-wrapper",
			"../../agent-wrapper",
		} {
			cnd := filepath.Clean(filepath.Join(execDir, rel))
			if info, err := os.Stat(cnd); err == nil && !info.IsDir() {
				return cnd
			}
		}
	}

	return ""
}

func resolveShellCandidate(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}

	if strings.ContainsRune(candidate, os.PathSeparator) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate
		}
		return ""
	}

	if resolved, err := exec.LookPath(candidate); err == nil {
		return resolved
	}

	for _, prefix := range []string{"/bin/", "/usr/bin/", "/usr/local/bin/"} {
		path := prefix + candidate
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return path
		}
	}

	return ""
}

func resolveShellPath(requested string) string {
	requested = strings.TrimSpace(requested)

	switch strings.ToLower(requested) {
	case "", "auto":
		for _, candidate := range []string{"fish", "zsh", "bash", "ash", "sh"} {
			if resolved := resolveShellCandidate(candidate); resolved != "" {
				return resolved
			}
		}
		return ""
	case "system", "env":
		if resolved := resolveShellCandidate(os.Getenv("SHELL")); resolved != "" {
			return resolved
		}
		for _, candidate := range []string{"fish", "zsh", "bash", "ash", "sh"} {
			if resolved := resolveShellCandidate(candidate); resolved != "" {
				return resolved
			}
		}
		return ""
	default:
		return resolveShellCandidate(requested)
	}
}

func resolveShellWorkDir() string {
	if override := os.Getenv("AGENT_SHELL_DIR"); override != "" {
		if info, err := os.Stat(override); err == nil && info.IsDir() {
			return override
		}
	}

	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		repoRoot := filepath.Dir(filepath.Dir(sourceFile))
		if info, err := os.Stat(repoRoot); err == nil && info.IsDir() {
			return repoRoot
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}

	if home, err := os.UserHomeDir(); err == nil {
		return home
	}

	return "/"
}

func setEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	replaced := false
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			replaced = true
			break
		}
	}
	if !replaced {
		env = append(env, prefix+value)
	}
	return env
}

func serveShellWS(c *gin.Context) {
	if sessionID := strings.TrimSpace(c.Query("session_id")); sessionID != "" {
		shellSessions.AttachWS(c)
		return
	}

	serveLegacyShellWS(c)
}

func serveLegacyShellWS(c *gin.Context) {
	shellPath := resolveShellPath(c.DefaultQuery("shell", "auto"))
	if shellPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "shell not found"})
		return
	}

	cols, rows := 80, 24
	if v, err := strconv.Atoi(c.DefaultQuery("cols", "80")); err == nil && v > 0 {
		cols = v
	}
	if v, err := strconv.Atoi(c.DefaultQuery("rows", "24")); err == nil && v > 0 {
		rows = v
	}

	cmd := exec.Command(shellPath)
	cmd.Dir = resolveShellWorkDir()
	cmd.Env = setEnvValue(os.Environ(), "TERM", "xterm-256color")

	// Disable fish shell's query-terminal feature to prevent 10s wait warnings
	ff := os.Getenv("fish_features")
	if ff == "" {
		ff = "no-query-term"
	} else if !strings.Contains(ff, "no-query-term") {
		ff = ff + ",no-query-term"
	}
	cmd.Env = setEnvValue(cmd.Env, "fish_features", ff)

	dropPrivileges(cmd)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return
	}
	defer func() {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	go func() {
		defer conn.Close()
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				payload := append([]byte(nil), buf[:n]...)
				if writeErr := conn.WriteMessage(websocket.BinaryMessage, payload); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		switch messageType {
		case websocket.BinaryMessage:
			if len(data) == 0 {
				continue
			}
			if _, err := ptmx.Write(data); err != nil {
				return
			}
		case websocket.TextMessage:
			var ctrl shellControlMessage
			if err := json.Unmarshal(data, &ctrl); err == nil && ctrl.Type == "resize" {
				if ctrl.Cols > 0 && ctrl.Rows > 0 {
					_ = pty.Setsize(ptmx, &pty.Winsize{
						Cols: uint16(ctrl.Cols),
						Rows: uint16(ctrl.Rows),
					})
				}
				continue
			}
			if _, err := ptmx.Write(data); err != nil {
				return
			}
		}
	}
}

func writePortFile(actualPort int) {
	data := []byte(fmt.Sprintf("%d", actualPort))
	_ = os.WriteFile(".port", data, 0644)

	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		backendDir := filepath.Dir(sourceFile)
		_ = os.WriteFile(filepath.Join(backendDir, ".port"), data, 0644)
	}
}

func scanFdinfo(procMap map[int32]gpuInfo, globalStats *[]*pb.GPUStatus) {
	now := time.Now()
	dt := now.Sub(fdinfoTime).Nanoseconds()
	fdinfoTime = now

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
			if prev, ok := fdinfoHistory[histKey]; ok && dt > 0 {
				diff := enginesNs - prev
				util = uint32((diff * 100) / uint64(dt))
			}
			fdinfoHistory[histKey] = enginesNs

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
					gs.UtilGpu += util // Sum up for global? Actually usually we want the max or avg.
					// For DRM, summing across all processes' engine usage is correct for global util.
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

func startUDSServer(broadcast chan *pb.Event) {
	_ = os.Remove(udsPath)
	l, err := net.Listen("unix", udsPath)
	if err != nil {
		return
	}
	_ = os.Chmod(udsPath, 0666)
	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			buf := make([]byte, 4096)
			for {
				n, err := c.Read(buf)
				if err != nil {
					return
				}
				req := &pb.WrapperRequest{}
				if err := proto.Unmarshal(buf[:n], req); err != nil {
					continue
				}
				resp := &pb.WrapperResponse{Action: pb.WrapperResponse_ALLOW}
				rulesMu.RLock()
				rule, ok := wrapperRules[req.Comm]
				rulesMu.RUnlock()
				if ok {
					switch rule.Action {
					case "BLOCK":
						resp.Action = pb.WrapperResponse_BLOCK
						resp.Message = "Blocked by policy"
					case "ALERT":
						resp.Action = pb.WrapperResponse_ALERT
						resp.Message = "Security alert"
					case "REWRITE":
						resp.Action = pb.WrapperResponse_REWRITE
						resp.RewrittenArgs = rule.RewrittenCmd
					}
				}
				broadcast <- &pb.Event{Pid: req.Pid, Comm: req.Comm, Type: "wrapper_intercept", Tag: "Wrapper", Path: strings.Join(append([]string{req.Comm}, req.Args...), " ")}
				out, _ := proto.Marshal(resp)
				_, _ = c.Write(out)
			}
		}(conn)
	}
}

func getZramStats() (used, total uint64) {
	zramDevices, _ := filepath.Glob("/sys/block/zram*")
	for _, dev := range zramDevices {
		compr, _ := os.ReadFile(filepath.Join(dev, "compr_data_size"))
		orig, _ := os.ReadFile(filepath.Join(dev, "orig_data_size"))
		var c, o uint64
		fmt.Sscanf(string(compr), "%d", &c)
		fmt.Sscanf(string(orig), "%d", &o)
		used += c
		total += o
	}
	return
}

func main() {
	if isBootstrapMode() {
		if err := bootstrapTrackerMaps(); err != nil {
			log.Fatalf("failed to bootstrap eBPF components: %v", err)
		}
		return
	}
	if relaunched, err := ensureBackendPrivileges(); err != nil {
		log.Fatalf("failed to elevate backend privileges: %v", err)
	} else if relaunched {
		return
	}

	procsList, _ := ps.Processes()
	curr := int32(os.Getpid())
	for _, p := range procsList {
		if p.Pid != curr {
			if n, _ := p.Name(); n == "agent-ebpf-filter" || n == "main" {
				_ = p.Kill()
			}
		}
	}

	if err := ensureTrackerMapsLoaded(); err != nil {
		log.Fatalf("failed to initialize eBPF components: %v", err)
	}
	objs := &trackerMaps

	rd, _ := ringbuf.NewReader(objs.Events)
	defer rd.Close()

	broadcast := make(chan *pb.Event, 100)
	go func() {
		var event bpfEvent
		types := map[uint32]string{0: "execve", 1: "openat", 2: "network_connect", 3: "mkdir", 4: "unlink", 5: "ioctl", 6: "network_bind"}
		for {
			record, err := rd.Read()
			if err != nil {
				return
			}
			_ = binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event)
			broadcast <- &pb.Event{Pid: event.PID, Ppid: event.PPID, Uid: event.UID, Type: types[event.Type], Tag: getTagName(event.TagID), Comm: string(bytes.TrimRight(event.Comm[:], "\x00")), Path: string(bytes.TrimRight(event.Path[:], "\x00"))}
		}
	}()

	go startUDSServer(broadcast)

	r := gin.Default()
	clients := make(map[*websocket.Conn]bool)
	var clientsMu sync.Mutex
	go func() {
		for event := range broadcast {
			data, _ := proto.Marshal(event)
			clientsMu.Lock()
			for c := range clients {
				if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
					c.Close()
					delete(clients, c)
				}
			}
			clientsMu.Unlock()
		}
	}()

	r.GET("/ws", func(c *gin.Context) {
		conn, _ := upgrader.Upgrade(c.Writer, c.Request, nil)
		clientsMu.Lock()
		clients[conn] = true
		clientsMu.Unlock()
	})

	r.GET("/ws/system", func(c *gin.Context) {
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
				ccp, _ := p.CPUPercent()
				mp, _ := p.MemoryPercent()
				u, _ := p.Username()
				cmdl, _ := p.Cmdline()
				ct, _ := p.CreateTime()
				gmem, gid, gutil := uint32(0), uint32(0), uint32(0)
				if info, ok := gm[p.Pid]; ok {
					gmem, gid, gutil = info.mem, info.gpu, info.util
				}
				minorFaults, majorFaults := uint64(0), uint64(0)
				if faults, err := p.PageFaults(); err == nil && faults != nil {
					minorFaults = faults.MinorFaults
					majorFaults = faults.MajorFaults
				}
				stats.Processes = append(stats.Processes, &pb.Process{Pid: p.Pid, Ppid: pp, Name: n, Cpu: ccp, Mem: mp, User: u, GpuMem: gmem, GpuId: gid, GpuUtil: gutil, Cmdline: cmdl, CreateTime: ct, MinorFaults: minorFaults, MajorFaults: majorFaults})
			}
			data, _ := proto.Marshal(stats)
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		}
	})

	r.POST("/shell-sessions", handleCreateShellSession)
	r.GET("/shell-sessions", handleListShellSessions)
	r.DELETE("/shell-sessions/:id", handleDeleteShellSession)
	r.GET("/ws/shell", serveShellWS)

	r.POST("/hooks/event", func(c *gin.Context) {
		// Receives PreToolUse events from Claude Code's native hook mechanism.
		var payload map[string]interface{}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
			return
		}
		toolName, _ := payload["tool_name"].(string)
		hookEvent, _ := payload["hook_event_name"].(string)
		toolInput, _ := payload["tool_input"].(map[string]interface{})
		path := ""
		if toolInput != nil {
			if cmd, ok := toolInput["command"].(string); ok {
				path = cmd
			} else if fp, ok := toolInput["file_path"].(string); ok {
				path = fp
			}
		}
		broadcast <- &pb.Event{
			Type: "native_hook",
			Tag:  "Claude Code",
			Comm: fmt.Sprintf("%s:%s", hookEvent, toolName),
			Path: path,
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.POST("/register", func(c *gin.Context) {
		if trackerMaps.AgentPids == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "agent pid map not initialized"})
			return
		}
		var req struct {
			PID uint32 `json:"pid"`
			Tag string `json:"tag,omitempty"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.PID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pid"})
			return
		}
		tag := req.Tag
		if tag == "" {
			tag = "AI Agent"
		}
		if err := trackerMaps.AgentPids.Put(req.PID, getTagID(tag)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.POST("/unregister", func(c *gin.Context) {
		if trackerMaps.AgentPids == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "agent pid map not initialized"})
			return
		}
		var req struct {
			PID uint32 `json:"pid"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.PID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pid"})
			return
		}
		_ = trackerMaps.AgentPids.Delete(req.PID)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/", authMiddleware())
	{
		config := api.Group("/config")
		{
			config.GET("/tags", func(c *gin.Context) {
				tagsMu.RLock()
				defer tagsMu.RUnlock()
				t := []string{}
				for _, n := range tagMap {
					t = append(t, n)
				}
				c.JSON(200, t)
			})
			config.POST("/tags", func(c *gin.Context) {
				var r struct {
					Name string `json:"name"`
				}
				_ = c.ShouldBindJSON(&r)
				getTagID(r.Name)
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.GET("/comms", func(c *gin.Context) {
				items := []gin.H{}
				iter := objs.TrackedComms.Iterate()
				var k [16]byte
				var tid uint32
				for iter.Next(&k, &tid) {
					items = append(items, gin.H{"comm": string(bytes.TrimRight(k[:], "\x00")), "tag": getTagName(tid)})
				}
				c.JSON(200, items)
			})
			config.POST("/comms", func(c *gin.Context) {
				var r struct {
					Comm, Tag string `json:"comm" json:"tag"`
				}
				_ = c.ShouldBindJSON(&r)
				var k [16]byte
				copy(k[:], r.Comm)
				_ = objs.TrackedComms.Put(k, getTagID(r.Tag))
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.DELETE("/comms/:comm", func(c *gin.Context) {
				var k [16]byte
				copy(k[:], c.Param("comm"))
				_ = objs.TrackedComms.Delete(k)
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.GET("/paths", func(c *gin.Context) {
				items := []gin.H{}
				iter := objs.TrackedPaths.Iterate()
				var k [256]byte
				var tid uint32
				for iter.Next(&k, &tid) {
					items = append(items, gin.H{"path": string(bytes.TrimRight(k[:], "\x00")), "tag": getTagName(tid)})
				}
				c.JSON(200, items)
			})
			config.POST("/paths", func(c *gin.Context) {
				var r struct {
					Path, Tag string `json:"path" json:"tag"`
				}
				_ = c.ShouldBindJSON(&r)
				var k [256]byte
				copy(k[:], r.Path)
				_ = objs.TrackedPaths.Put(k, getTagID(r.Tag))
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.DELETE("/paths/*path", func(c *gin.Context) {
				p := c.Param("path")
				if len(p) > 0 && p[0] == '/' {
					p = p[1:]
				}
				var k [256]byte
				copy(k[:], p)
				_ = objs.TrackedPaths.Delete(k)
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.GET("/rules", func(c *gin.Context) { rulesMu.RLock(); defer rulesMu.RUnlock(); c.JSON(200, wrapperRules) })
			config.POST("/rules", func(c *gin.Context) {
				var r WrapperRule
				_ = c.ShouldBindJSON(&r)
				rulesMu.Lock()
				wrapperRules[r.Comm] = r
				rulesMu.Unlock()
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.DELETE("/rules/:comm", func(c *gin.Context) {
				rulesMu.Lock()
				delete(wrapperRules, c.Param("comm"))
				rulesMu.Unlock()
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.GET("/export", func(c *gin.Context) {
				cfg := ExportConfig{Comms: make(map[string]string), Paths: make(map[string]string)}
				tagsMu.RLock()
				for _, n := range tagMap {
					cfg.Tags = append(cfg.Tags, n)
				}
				tagsMu.RUnlock()
				var k16 [16]byte
				var k256 [256]byte
				var tid uint32
				i1 := objs.TrackedComms.Iterate()
				for i1.Next(&k16, &tid) {
					cfg.Comms[string(bytes.TrimRight(k16[:], "\x00"))] = getTagName(tid)
				}
				i2 := objs.TrackedPaths.Iterate()
				for i2.Next(&k256, &tid) {
					cfg.Paths[string(bytes.TrimRight(k256[:], "\x00"))] = getTagName(tid)
				}
				c.JSON(200, cfg)
			})
			config.POST("/import", func(c *gin.Context) {
				var cfg ExportConfig
				_ = c.ShouldBindJSON(&cfg)
				for _, t := range cfg.Tags {
					getTagID(t)
				}
				for comm, tag := range cfg.Comms {
					var k [16]byte
					copy(k[:], comm)
					_ = objs.TrackedComms.Put(k, getTagID(tag))
				}
				for p, tag := range cfg.Paths {
					var k [256]byte
					copy(k[:], p)
					_ = objs.TrackedPaths.Put(k, getTagID(tag))
				}
				c.JSON(200, gin.H{"status": "ok"})
			})
			hooks := config.Group("/hooks")
			{
				hooks.GET("", func(c *gin.Context) {
					res := []gin.H{}
					for _, h := range availableHooks {
						res = append(res, gin.H{
							"id": h.ID, "name": h.Name, "description": h.Description,
							"target_cmd": h.TargetCmd, "hook_type": h.HookType,
							"installed": isHookInstalled(h),
						})
					}
					c.JSON(200, res)
				})
				hooks.POST("", func(c *gin.Context) {
					var req struct {
						ID         string `json:"id"`
						Install    bool   `json:"install"`
						UseWrapper bool   `json:"use_wrapper"` // override: force wrapper even for native-capable CLIs
					}
					if err := c.ShouldBindJSON(&req); err != nil {
						c.JSON(400, gin.H{"error": "invalid request"})
						return
					}
					var target HookDef
					found := false
					for _, h := range availableHooks {
						if h.ID == req.ID {
							target = h
							found = true
							break
						}
					}
					if !found {
						c.JSON(404, gin.H{"error": "hook not found"})
						return
					}

					// Determine effective hook type: user can opt into wrapper even for native CLIs.
					effectiveType := target.HookType
					if req.UseWrapper {
						effectiveType = HookTypeWrapper
					}

					if req.Install {
						if effectiveType == HookTypeNative {
							if err := installNativeHook(target.NativeConfigPath); err != nil {
								c.JSON(500, gin.H{"error": err.Error()})
								return
							}
						} else {
							// Wrapper: add shell alias
							p := getShellConfigPath()
							b, _ := os.ReadFile(p)
							content := string(b)
							aliasLine := fmt.Sprintf("\nalias %s='agent-wrapper %s' # agent-ebpf-hook\n", target.TargetCmd, target.TargetCmd)
							if !strings.Contains(content, fmt.Sprintf("alias %s=", target.TargetCmd)) {
								f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
								if err != nil {
									c.JSON(500, gin.H{"error": err.Error()})
									return
								}
								f.WriteString(aliasLine)
								f.Close()
							}
						}
					} else {
						// Uninstall both types to ensure clean state.
						if target.HookType == HookTypeNative {
							_ = uninstallNativeHook(target.NativeConfigPath)
						}
						// Also remove wrapper alias if present.
						p := getShellConfigPath()
						b, _ := os.ReadFile(p)
						lines := strings.Split(string(b), "\n")
						newLines := []string{}
						for _, l := range lines {
							if !strings.Contains(l, fmt.Sprintf("alias %s=", target.TargetCmd)) {
								newLines = append(newLines, l)
							}
						}
						_ = os.WriteFile(p, []byte(strings.Join(newLines, "\n")), 0644)
					}
					c.JSON(200, gin.H{"status": "ok"})
				})
			}
		}
		system := api.Group("/system")
		{
			system.GET("/ls", func(c *gin.Context) {
				p := c.DefaultQuery("path", "/")
				e, _ := os.ReadDir(p)
				l := []gin.H{}
				for _, v := range e {
					fp := p
					if fp == "/" {
						fp = "/" + v.Name()
					} else {
						fp = fp + "/" + v.Name()
					}
					l = append(l, gin.H{"name": v.Name(), "isDir": v.IsDir(), "path": fp})
				}
				c.JSON(200, l)
			})
			system.POST("/run", func(c *gin.Context) {
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
			})
		}
	}
	staticDir := "../frontend/dist"
	if _, err := os.Stat(staticDir); err != nil {
		staticDir = "./frontend/dist"
	}
	r.StaticFile("/", filepath.Join(staticDir, "index.html"))
	r.Static("/assets", filepath.Join(staticDir, "assets"))
	r.NoRoute(func(c *gin.Context) { c.File(filepath.Join(staticDir, "index.html")) })
	commonCLIs := map[string]string{"git": "Git", "npm": "Package Manager", "bun": "Package Manager", "pnpm": "Package Manager", "yarn": "Package Manager", "node": "Runtime", "python": "Runtime", "python3": "Runtime", "go": "Build Tool", "cargo": "Build Tool", "rustc": "Build Tool", "gcc": "Build Tool", "g++": "Build Tool", "clang": "Build Tool", "make": "Build Tool", "cmake": "Build Tool", "docker": "System Tool", "kubectl": "Network Tool"}
	for cl, t := range commonCLIs {
		var k [16]byte
		copy(k[:], cl)
		_ = objs.TrackedComms.Put(k, getTagID(t))
	}
	startPort, maxTries, actualPort := 8080, 10, 8080
	for i := 0; i < maxTries; i++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", startPort+i))
		if err == nil {
			actualPort = startPort + i
			l.Close()
			break
		}
	}
	writePortFile(actualPort)
	_ = r.Run(fmt.Sprintf(":%d", actualPort))
}
