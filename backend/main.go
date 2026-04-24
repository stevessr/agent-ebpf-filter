package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"agent-ebpf-filter/pb"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/creack/pty/v2"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/pelletier/go-toml/v2"
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
	NetFamily                   uint32
	NetDirection                uint32
	NetBytes                    uint32
	NetPort                     uint32
	NetAddr                     [16]byte
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
	Tags    []string               `json:"tags"`
	Comms   map[string]string      `json:"comms"`
	Paths   map[string]string      `json:"paths"`
	Rules   map[string]WrapperRule `json:"rules"`
	Runtime *RuntimeSettings       `json:"runtime,omitempty"`
}

type RuntimeConfigResponse struct {
	Runtime                RuntimeSettings `json:"runtime"`
	MCPEndpoint            string          `json:"mcpEndpoint"`
	AuthHeaderName         string          `json:"authHeaderName"`
	BearerAuthHeaderName   string          `json:"bearerAuthHeaderName"`
	PersistedEventLogPath  string          `json:"persistedEventLogPath"`
	PersistedEventLogAlive bool            `json:"persistedEventLogAlive"`
}

type kiroHookState struct {
	PreviousDefaultAgent string `json:"previous_default_agent,omitempty"`
}

type FilePreviewResponse struct {
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	ParentDir   string    `json:"parentDir"`
	IsDir       bool      `json:"isDir"`
	Size        int64     `json:"size"`
	Mode        string    `json:"mode"`
	ModTime     time.Time `json:"modTime"`
	MimeType    string    `json:"mimeType,omitempty"`
	PreviewType string    `json:"previewType"`
	Content     string    `json:"content,omitempty"`
	DataURL     string    `json:"dataUrl,omitempty"`
	Truncated   bool      `json:"truncated,omitempty"`
}

// HookType distinguishes how the hook intercepts the agent CLI.
// "native" = write into the agent CLI's own config file (preferred).
// "wrapper" = install a shell alias that routes through agent-wrapper.
type HookType string

const (
	HookTypeNative  HookType = "native"
	HookTypeWrapper HookType = "wrapper"
)

type ConfigFormat string

const (
	ConfigFormatJSON ConfigFormat = "json"
	ConfigFormatTOML ConfigFormat = "toml"
)

type HookDef struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	TargetCmd   string   `json:"target_cmd"`
	HookType    HookType `json:"hook_type"`
	// NativeConfigPath is the path to the agent CLI's config file (for native hooks).
	NativeConfigPath string `json:"-"`
	// NativeFeatureConfigPath is an optional companion config file used to enable hook support.
	NativeFeatureConfigPath string `json:"-"`
	// NativeHookEvent is the event name for native hooks (e.g. "PreToolUse" or "BeforeTool").
	NativeHookEvent string `json:"-"`
	// NativeMatcher is an optional default matcher to inject for native hooks.
	NativeMatcher string `json:"-"`
	// ConfigFormat defines if the config is JSON or TOML.
	ConfigFormat ConfigFormat `json:"-"`
}

var availableHooks = []HookDef{
	{
		ID: "claude", Name: "Claude Code", HookType: HookTypeNative,
		Description:     "Uses Claude Code's built-in PreToolUse hook to intercept all tool calls (recommended)",
		TargetCmd:       "claude",
		NativeHookEvent: "PreToolUse",
		ConfigFormat:    ConfigFormatJSON,
	},
	{
		ID: "gemini", Name: "Gemini CLI", HookType: HookTypeNative,
		Description:     "Uses Gemini CLI's native BeforeTool hook for high-performance interception",
		TargetCmd:       "gemini",
		NativeHookEvent: "BeforeTool",
		ConfigFormat:    ConfigFormatJSON,
	},
	{
		ID: "codex", Name: "Codex", HookType: HookTypeNative,
		Description:     "Uses Codex's native hooks.json and enables codex_hooks in config.toml for Bash command monitoring",
		TargetCmd:       "codex",
		NativeHookEvent: "PreToolUse",
		NativeMatcher:   "Bash",
		ConfigFormat:    ConfigFormatJSON,
	},
	{
		ID: "copilot", Name: "GitHub Copilot", HookType: HookTypeNative,
		Description:     "Uses GitHub Copilot CLI's preToolUse hook for security inspection",
		TargetCmd:       "gh",
		NativeHookEvent: "preToolUse",
		ConfigFormat:    ConfigFormatJSON,
	},
	{
		ID: "kiro", Name: "Kiro CLI", HookType: HookTypeNative,
		Description:     "Creates a managed Kiro agent derived from kiro_default and installs a native preToolUse hook for execute_bash",
		TargetCmd:       "kiro-cli",
		NativeHookEvent: "preToolUse",
		NativeMatcher:   "execute_bash",
		ConfigFormat:    ConfigFormatJSON,
	},
	{
		ID: "cursor", Name: "Cursor", HookType: HookTypeWrapper,
		Description: "Intercepts cursor execution via shell alias wrapper",
		TargetCmd:   "cursor",
	},
}

func getRealHomeDir() string {
	// 1. Check for our own environment variable (passed across sudo/pkexec)
	if h := os.Getenv("AGENT_REAL_HOME"); h != "" {
		return h
	}
	// 2. If we are root, try to find the real user who started us via standard envs
	if os.Getuid() == 0 {
		// Try sudo user
		if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
			if u, err := user.Lookup(sudoUser); err == nil {
				return u.HomeDir
			}
		}
		// Try pkexec user (PolicyKit)
		if pkexecUid := os.Getenv("PKEXEC_UID"); pkexecUid != "" {
			if u, err := user.LookupId(pkexecUid); err == nil {
				return u.HomeDir
			}
		}
		// Try preserved HOME if it's not /root
		if home := os.Getenv("HOME"); home != "" && home != "/root" {
			return home
		}
		// Try to find the first non-root user in /home
		if entries, err := os.ReadDir("/home"); err == nil {
			for _, entry := range entries {
				if entry.IsDir() && entry.Name() != "lost+found" {
					return filepath.Join("/home", entry.Name())
				}
			}
		}
	}
	// Default to standard lookup
	h, _ := os.UserHomeDir()
	if h == "" || h == "/root" {
		// Final fallback: check for any /home/xxx
		if entries, err := os.ReadDir("/home"); err == nil && len(entries) > 0 {
			for _, entry := range entries {
				if entry.IsDir() && entry.Name() != "lost+found" {
					return filepath.Join("/home", entry.Name())
				}
			}
		}
	}
	return h
}

func getShellConfigPath() string {
	home := getRealHomeDir()
	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "zsh") {
		return filepath.Join(home, ".zshrc")
	}
	return filepath.Join(home, ".bashrc")
}

func resolveBackendPort() int {
	if raw := strings.TrimSpace(os.Getenv("AGENT_BACKEND_PORT")); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 {
			return port
		}
	}

	candidates := []string{".port"}
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(sourceFile), ".port"))
	}

	for _, candidate := range candidates {
		b, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		if port, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && port > 0 {
			return port
		}
	}

	return 8080
}

func resolveHookCallbackURL() string {
	if raw := strings.TrimSpace(os.Getenv("AGENT_HOOK_ENDPOINT")); raw != "" {
		return raw
	}
	return fmt.Sprintf("http://127.0.0.1:%d/hooks/event", resolveBackendPort())
}

const hookMarker = "agent-ebpf-hook-active"
const kiroManagedAgent = "agent-ebpf-hook"
const (
	textPreviewLimitBytes   = 64 * 1024
	binaryPreviewLimitBytes = 4 * 1024
	imagePreviewLimitBytes  = 2 * 1024 * 1024
)

func isTextLikeMime(mimeType string) bool {
	if mimeType == "" {
		return false
	}
	return strings.HasPrefix(mimeType, "text/") ||
		strings.Contains(mimeType, "json") ||
		strings.Contains(mimeType, "xml") ||
		strings.Contains(mimeType, "javascript") ||
		strings.Contains(mimeType, "yaml") ||
		strings.Contains(mimeType, "toml") ||
		strings.Contains(mimeType, "x-sh")
}

func buildFilePreview(path string) (*FilePreviewResponse, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	res := &FilePreviewResponse{
		Path:      absPath,
		Name:      info.Name(),
		ParentDir: filepath.Dir(absPath),
		IsDir:     info.IsDir(),
		Size:      info.Size(),
		Mode:      info.Mode().String(),
		ModTime:   info.ModTime(),
	}
	if absPath == "/" {
		res.ParentDir = "/"
	}

	if info.IsDir() {
		res.PreviewType = "directory"
		return res, nil
	}

	file, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	head := make([]byte, 512)
	n, readErr := file.Read(head)
	if readErr != nil && readErr != io.EOF {
		return nil, readErr
	}
	head = head[:n]

	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(absPath)))
	if mimeType == "" && len(head) > 0 {
		mimeType = http.DetectContentType(head)
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	res.MimeType = mimeType

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	if strings.HasPrefix(mimeType, "image/") {
		res.PreviewType = "image"
		if info.Size() > imagePreviewLimitBytes {
			res.Content = fmt.Sprintf("Image is too large to preview inline (limit: %d MiB).", imagePreviewLimitBytes/(1024*1024))
			res.Truncated = true
			return res, nil
		}

		data, err := io.ReadAll(io.LimitReader(file, imagePreviewLimitBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > imagePreviewLimitBytes {
			data = data[:imagePreviewLimitBytes]
			res.Truncated = true
		}
		res.DataURL = fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
		return res, nil
	}

	previewLimit := int64(binaryPreviewLimitBytes)
	if isTextLikeMime(mimeType) {
		previewLimit = textPreviewLimitBytes
	}

	data, err := io.ReadAll(io.LimitReader(file, previewLimit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > previewLimit {
		data = data[:previewLimit]
		res.Truncated = true
	}
	if info.Size() > int64(len(data)) {
		res.Truncated = true
	}

	if isTextLikeMime(mimeType) || utf8.Valid(data) {
		res.PreviewType = "text"
		res.Content = string(data)
		return res, nil
	}

	res.PreviewType = "binary"
	res.Content = hex.Dump(data)
	return res, nil
}

func hasNativeHookMarker(cfgPath string) bool {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), hookMarker)
}

func isCodexHooksFeatureEnabled(cfgPath string) bool {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return false
	}

	var cfg map[string]interface{}
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return false
	}

	features, _ := cfg["features"].(map[string]interface{})
	if features == nil {
		return false
	}

	enabled, _ := features["codex_hooks"].(bool)
	return enabled
}

// isNativeHookInstalled checks whether the agent-ebpf hook is present in the config
// and whether any required feature flags are enabled.
func isNativeHookInstalled(h HookDef) bool {
	if h.ID == "kiro" {
		return hasNativeHookMarker(h.NativeConfigPath) && isKiroManagedAgentSelected()
	}
	if !hasNativeHookMarker(h.NativeConfigPath) {
		return false
	}
	if h.NativeFeatureConfigPath != "" && !isCodexHooksFeatureEnabled(h.NativeFeatureConfigPath) {
		return false
	}
	return true
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
		return isNativeHookInstalled(h)
	}
	return isWrapperHookInstalled(h.TargetCmd)
}

func ensureCodexHooksFeatureEnabled(cfgPath string) error {
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return err
	}

	var cfg map[string]interface{}
	if b, err := os.ReadFile(cfgPath); err == nil {
		_ = toml.Unmarshal(b, &cfg)
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	features, _ := cfg["features"].(map[string]interface{})
	if features == nil {
		features = make(map[string]interface{})
	}
	features["codex_hooks"] = true
	cfg["features"] = features

	out, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0644)
}

func hookRelayScriptPath(h HookDef) string {
	return filepath.Join(filepath.Dir(h.NativeConfigPath), "hooks", hookMarker+"-"+h.ID+".sh")
}

func readJSONObjectFile(path string) (map[string]interface{}, error) {
	var cfg map[string]interface{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return make(map[string]interface{}), nil
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}
	return cfg, nil
}

func writeJSONObjectFile(path string, cfg map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func kiroSettingsPath() string {
	return filepath.Join(getRealHomeDir(), ".kiro", "settings", "cli.json")
}

func kiroHookStatePath() string {
	return filepath.Join(getRealHomeDir(), ".kiro", "settings", "agent-ebpf-hook-state.json")
}

func kiroManagedAgentPath() string {
	return filepath.Join(getRealHomeDir(), ".kiro", "agents", kiroManagedAgent+".json")
}

func ensureKiroManagedAgentExists() error {
	agentPath := kiroManagedAgentPath()
	if _, err := os.Stat(agentPath); err == nil {
		return nil
	}

	agentsDir := filepath.Dir(agentPath)
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return err
	}

	cmd := exec.Command("kiro-cli", "agent", "create", kiroManagedAgent, "--from", "kiro_default", "-d", agentsDir)
	configureCommandForRealUser(cmd)
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = setEnvValue(cmd.Env, "HOME", getRealHomeDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create managed Kiro agent from kiro_default: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func readKiroHookState() (kiroHookState, error) {
	state := kiroHookState{}
	b, err := os.ReadFile(kiroHookStatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if err := json.Unmarshal(b, &state); err != nil {
		return state, err
	}
	return state, nil
}

func writeKiroHookState(state kiroHookState) error {
	if err := os.MkdirAll(filepath.Dir(kiroHookStatePath()), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(kiroHookStatePath(), b, 0644)
}

func isKiroManagedAgentSelected() bool {
	settings, err := readJSONObjectFile(kiroSettingsPath())
	if err != nil {
		return false
	}
	agentName, _ := settings["chat.defaultAgent"].(string)
	return agentName == kiroManagedAgent
}

func ensureHookRelayScript(h HookDef) (string, error) {
	scriptDir := filepath.Join(filepath.Dir(h.NativeConfigPath), "hooks")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		return "", err
	}

	scriptPath := hookRelayScriptPath(h)
	scriptContent := fmt.Sprintf(`#!/usr/bin/env bash
tmp_file="$(mktemp "${TMPDIR:-/tmp}/agent-ebpf-hook.XXXXXX")" || exit 0
trap 'rm -f "$tmp_file"' EXIT
cat >"$tmp_file"
curl -fsS -X POST '%s' \
  -H 'Content-Type: application/json' \
  -H 'X-Agent-CLI: %s' \
  --data-binary "@$tmp_file" \
  >/dev/null 2>&1 || true
`, resolveHookCallbackURL(), h.ID)

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		return "", err
	}
	return scriptPath, nil
}

func installKiroDefaultAgentSelection() error {
	settings, err := readJSONObjectFile(kiroSettingsPath())
	if err != nil {
		return err
	}

	currentDefault, _ := settings["chat.defaultAgent"].(string)
	if currentDefault != kiroManagedAgent {
		if err := writeKiroHookState(kiroHookState{PreviousDefaultAgent: currentDefault}); err != nil {
			return err
		}
	}

	settings["chat.defaultAgent"] = kiroManagedAgent
	return writeJSONObjectFile(kiroSettingsPath(), settings)
}

func restoreKiroDefaultAgentSelection() error {
	settings, err := readJSONObjectFile(kiroSettingsPath())
	if err != nil {
		return err
	}

	state, err := readKiroHookState()
	if err != nil {
		return err
	}

	currentDefault, _ := settings["chat.defaultAgent"].(string)
	if currentDefault == kiroManagedAgent {
		if state.PreviousDefaultAgent != "" {
			settings["chat.defaultAgent"] = state.PreviousDefaultAgent
		} else {
			delete(settings, "chat.defaultAgent")
		}
		if err := writeJSONObjectFile(kiroSettingsPath(), settings); err != nil {
			return err
		}
	}

	_ = os.Remove(kiroHookStatePath())
	return nil
}

func installKiroNativeHook(h HookDef) error {
	if err := ensureKiroManagedAgentExists(); err != nil {
		return err
	}

	cfg, err := readJSONObjectFile(h.NativeConfigPath)
	if err != nil {
		return err
	}

	scriptPath, err := ensureHookRelayScript(h)
	if err != nil {
		return err
	}
	hookCommand := fmt.Sprintf(`'%s'`, scriptPath)

	hooks, _ := cfg["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = make(map[string]interface{})
	}
	eventHooks, _ := hooks[h.NativeHookEvent].([]interface{})

	filtered := make([]interface{}, 0, len(eventHooks))
	for _, entry := range eventHooks {
		em, ok := entry.(map[string]interface{})
		if !ok {
			filtered = append(filtered, entry)
			continue
		}
		cmd, _ := em["command"].(string)
		if strings.Contains(cmd, hookMarker) {
			continue
		}
		filtered = append(filtered, entry)
	}

	hookEntry := map[string]interface{}{
		"command": hookCommand,
	}
	if h.NativeMatcher != "" {
		hookEntry["matcher"] = h.NativeMatcher
	}

	hooks[h.NativeHookEvent] = append(filtered, hookEntry)
	cfg["hooks"] = hooks
	if err := writeJSONObjectFile(h.NativeConfigPath, cfg); err != nil {
		return err
	}
	return installKiroDefaultAgentSelection()
}

func uninstallKiroNativeHook(h HookDef) error {
	cfg, err := readJSONObjectFile(h.NativeConfigPath)
	if err != nil {
		return err
	}

	hooks, _ := cfg["hooks"].(map[string]interface{})
	if hooks != nil {
		eventHooks, _ := hooks[h.NativeHookEvent].([]interface{})
		filtered := make([]interface{}, 0, len(eventHooks))
		for _, entry := range eventHooks {
			em, ok := entry.(map[string]interface{})
			if !ok {
				filtered = append(filtered, entry)
				continue
			}
			cmd, _ := em["command"].(string)
			if strings.Contains(cmd, hookMarker) {
				continue
			}
			filtered = append(filtered, entry)
		}

		if len(filtered) == 0 {
			delete(hooks, h.NativeHookEvent)
		} else {
			hooks[h.NativeHookEvent] = filtered
		}
		if len(hooks) == 0 {
			delete(cfg, "hooks")
		} else {
			cfg["hooks"] = hooks
		}
		if err := writeJSONObjectFile(h.NativeConfigPath, cfg); err != nil {
			return err
		}
	}

	_ = os.Remove(hookRelayScriptPath(h))
	return restoreKiroDefaultAgentSelection()
}

func cleanupLegacyCodexHookConfig(h HookDef) {
	if h.ID != "codex" || h.NativeFeatureConfigPath == "" {
		return
	}

	legacyHookConfig := HookDef{
		NativeConfigPath: h.NativeFeatureConfigPath,
		NativeHookEvent:  h.NativeHookEvent,
		ConfigFormat:     ConfigFormatTOML,
	}
	if err := uninstallNativeHook(legacyHookConfig); err != nil {
		log.Printf("[WARN] failed to clean up legacy Codex hook config from %s: %v", h.NativeFeatureConfigPath, err)
	}
}

// installNativeHook injects a hook into the agent CLI's settings (JSON or TOML)
// that POSTs every tool call to our backend for inspection.
func installNativeHook(h HookDef) error {
	if h.ID == "kiro" {
		return installKiroNativeHook(h)
	}

	cleanupLegacyCodexHookConfig(h)

	if h.NativeFeatureConfigPath != "" {
		if err := ensureCodexHooksFeatureEnabled(h.NativeFeatureConfigPath); err != nil {
			return err
		}
	}

	cfgPath := h.NativeConfigPath
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return err
	}

	// Read existing config (may not exist yet).
	var cfg map[string]interface{}
	if b, err := os.ReadFile(cfgPath); err == nil {
		if h.ConfigFormat == ConfigFormatTOML {
			_ = toml.Unmarshal(b, &cfg)
		} else {
			_ = json.Unmarshal(b, &cfg)
		}
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	// Build the hook entry.
	scriptPath, err := ensureHookRelayScript(h)
	if err != nil {
		return err
	}
	hookCommand := fmt.Sprintf(`'%s'`, scriptPath)

	hookEntry := map[string]interface{}{
		"type":          "command",
		"command":       hookCommand,
		"statusMessage": "agent-ebpf-hook-active: inspecting...",
	}
	if h.ID != "codex" {
		hookEntry["async"] = true
	}
	matcher := map[string]interface{}{"hooks": []interface{}{hookEntry}}
	if h.NativeMatcher != "" {
		matcher["matcher"] = h.NativeMatcher
	} else {
		matcher["matcher"] = ""
	}

	// Merge into existing hooks[eventName] array.
	hooks, _ := cfg["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = make(map[string]interface{})
	}
	eventHooks, _ := hooks[h.NativeHookEvent].([]interface{})

	// Remove any existing agent-ebpf-hook entry to avoid duplicates.
	filtered := []interface{}{}
	for _, m := range eventHooks {
		if mm, ok := m.(map[string]interface{}); ok {
			hs, _ := mm["hooks"].([]interface{})
			isOurs := false
			for _, h := range hs {
				if hm, ok := h.(map[string]interface{}); ok {
					if cmd, _ := hm["command"].(string); strings.Contains(cmd, hookMarker) {
						isOurs = true
					}
				}
			}
			if !isOurs {
				filtered = append(filtered, m)
			}
		}
	}
	hooks[h.NativeHookEvent] = append(filtered, matcher)
	cfg["hooks"] = hooks

	var b []byte
	if h.ConfigFormat == ConfigFormatTOML {
		b, err = toml.Marshal(cfg)
	} else {
		b, err = json.MarshalIndent(cfg, "", "  ")
	}

	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, b, 0644)
}

// uninstallNativeHook removes the agent-ebpf hook from settings.
func uninstallNativeHook(h HookDef) error {
	if h.ID == "kiro" {
		return uninstallKiroNativeHook(h)
	}

	b, err := os.ReadFile(h.NativeConfigPath)
	if err != nil {
		_ = os.Remove(hookRelayScriptPath(h))
		cleanupLegacyCodexHookConfig(h)
		return nil // nothing to do
	}
	var cfg map[string]interface{}
	if h.ConfigFormat == ConfigFormatTOML {
		if err := toml.Unmarshal(b, &cfg); err != nil {
			return err
		}
	} else {
		if err := json.Unmarshal(b, &cfg); err != nil {
			return err
		}
	}

	hooks, _ := cfg["hooks"].(map[string]interface{})
	if hooks == nil {
		_ = os.Remove(hookRelayScriptPath(h))
		cleanupLegacyCodexHookConfig(h)
		return nil
	}
	eventHooks, _ := hooks[h.NativeHookEvent].([]interface{})
	filtered := []interface{}{}
	for _, m := range eventHooks {
		if mm, ok := m.(map[string]interface{}); ok {
			hs, _ := mm["hooks"].([]interface{})
			isOurs := false
			for _, h := range hs {
				if hm, ok := h.(map[string]interface{}); ok {
					if cmd, _ := hm["command"].(string); strings.Contains(cmd, hookMarker) {
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
		delete(hooks, h.NativeHookEvent)
	} else {
		hooks[h.NativeHookEvent] = filtered
	}
	if len(hooks) == 0 {
		delete(cfg, "hooks")
	}

	var out []byte
	if h.ConfigFormat == ConfigFormatTOML {
		out, err = toml.Marshal(cfg)
	} else {
		out, err = json.MarshalIndent(cfg, "", "  ")
	}

	if err != nil {
		return err
	}
	if err := os.WriteFile(h.NativeConfigPath, out, 0644); err != nil {
		return err
	}
	_ = os.Remove(hookRelayScriptPath(h))

	cleanupLegacyCodexHookConfig(h)
	return nil
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
		if clusterRequestAuthAllowed(c) {
			c.Next()
			return
		}
		token := requestAuthToken(c)
		expectedKey := runtimeSettingsStore.ExpectedToken()
		if token == "" || token != expectedKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		c.Next()
	}
}

func requestAuthToken(c *gin.Context) string {
	if token := strings.TrimSpace(c.Query("key")); token != "" {
		return token
	}
	if token := strings.TrimSpace(c.GetHeader("X-API-KEY")); token != "" {
		return token
	}
	if authHeader := strings.TrimSpace(c.GetHeader("Authorization")); authHeader != "" {
		lower := strings.ToLower(authHeader)
		if strings.HasPrefix(lower, "bearer ") {
			return strings.TrimSpace(authHeader[len("Bearer "):])
		}
	}
	return ""
}

func buildRuntimeConfigResponseFromSettings(settings RuntimeSettings) RuntimeConfigResponse {
	logPath := strings.TrimSpace(settings.LogFilePath)
	logAlive := false
	if settings.LogPersistenceEnabled && logPath != "" {
		if info, err := os.Stat(logPath); err == nil && !info.IsDir() {
			logAlive = true
		}
	}
	return RuntimeConfigResponse{
		Runtime:                settings,
		MCPEndpoint:            fmt.Sprintf("http://127.0.0.1:%d/mcp", resolveBackendPort()),
		AuthHeaderName:         "X-API-KEY",
		BearerAuthHeaderName:   "Authorization: Bearer",
		PersistedEventLogPath:  logPath,
		PersistedEventLogAlive: logAlive,
	}
}

func buildRuntimeConfigResponse() RuntimeConfigResponse {
	return buildRuntimeConfigResponseFromSettings(runtimeSettingsStore.Snapshot())
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
				select {
				case broadcast <- &pb.Event{Pid: req.Pid, Comm: req.Comm, Type: "wrapper_intercept", Tag: "Wrapper", Path: strings.Join(append([]string{req.Comm}, req.Args...), " ")}:
				default:
				}
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

func refreshHooksPaths() {
	home := getRealHomeDir()
	log.Printf("[DEBUG] Resolving agent config paths for home: %s", home)
	for i := range availableHooks {
		if availableHooks[i].HookType == HookTypeNative {
			switch availableHooks[i].ID {
			case "claude":
				availableHooks[i].NativeConfigPath = filepath.Join(home, ".claude", "settings.json")
			case "gemini":
				availableHooks[i].NativeConfigPath = filepath.Join(home, ".gemini", "settings.json")
			case "codex":
				availableHooks[i].NativeConfigPath = filepath.Join(home, ".codex", "hooks.json")
				availableHooks[i].NativeFeatureConfigPath = filepath.Join(home, ".codex", "config.toml")
			case "kiro":
				availableHooks[i].NativeConfigPath = filepath.Join(home, ".kiro", "agents", "agent-ebpf-hook.json")
			case "copilot":
				availableHooks[i].NativeConfigPath = filepath.Join(home, ".copilot", "config.json")
			}
		}
	}
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

	refreshHooksPaths()
	if _, err := runtimeSettingsStore.LoadOrCreate(); err != nil {
		log.Printf("[WARN] failed to load runtime settings: %v", err)
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
		selfPid := uint32(os.Getpid())
		for {
			record, err := rd.Read()
			if err != nil {
				return
			}
			_ = binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event)
			if event.PID == selfPid {
				continue
			}
			broadcast <- buildKernelEvent(event)
		}
	}()

	go startUDSServer(broadcast)

	r := gin.Default()
	r.Use(clusterGatewayMiddleware())
	clients := make(map[*websocket.Conn]bool)
	var clientsMu sync.Mutex
	go func() {
		for event := range broadcast {
			recordCapturedEvent(event)
			data, _ := proto.Marshal(event)
			clientsMu.Lock()
			for c := range clients {
				if c == nil {
					delete(clients, c)
					continue
				}
				if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
					c.Close()
					delete(clients, c)
				}
			}
			clientsMu.Unlock()
		}
	}()

	r.GET("/ws", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		clientsMu.Lock()
		clients[conn] = true
		clientsMu.Unlock()

		go func(conn *websocket.Conn) {
			defer func() {
				clientsMu.Lock()
				delete(clients, conn)
				clientsMu.Unlock()
				_ = conn.Close()
			}()

			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}(conn)
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
	})

	r.POST("/shell-sessions", handleCreateShellSession)
	r.GET("/shell-sessions", handleListShellSessions)
	r.DELETE("/shell-sessions/:id", handleDeleteShellSession)
	r.POST("/shell-sessions/:id/input", handleSendShellSessionInput)
	r.GET("/ws/shell", serveShellWS)

	r.GET("/events/recent", func(c *gin.Context) {
		limit := 50
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
				limit = parsed
			}
		}
		typeFilter := c.Query("type")
		records, source, err := runtimeSettingsStore.RecentEvents(limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if typeFilter != "" {
			filtered := make([]CapturedEventRecord, 0, len(records))
			for _, r := range records {
				if r.Event != nil && r.Event.Type == typeFilter {
					filtered = append(filtered, r)
				}
			}
			records = filtered
		}
		c.JSON(http.StatusOK, gin.H{"source": source, "events": records})
	})

	r.POST("/hooks/event", func(c *gin.Context) {
		// Receives events from various AI CLI native hook mechanisms.
		var payload map[string]interface{}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
			return
		}

		// Try to identify the tool and event from common AI CLI hook payload schemas
		toolName, _ := payload["tool_name"].(string)
		hookEvent, _ := payload["hook_event_name"].(string)
		toolInput, _ := payload["tool_input"].(map[string]interface{})

		// Fallbacks for Gemini CLI / Codex if they use slightly different fields
		if toolName == "" {
			toolName, _ = payload["tool"].(string)
		}
		if hookEvent == "" {
			hookEvent, _ = payload["event"].(string)
		}

		path := ""
		if toolInput != nil {
			if cmd, ok := toolInput["command"].(string); ok {
				path = cmd
			} else if fp, ok := toolInput["file_path"].(string); ok {
				path = fp
			} else if args, ok := toolInput["arguments"].([]interface{}); ok && len(args) > 0 {
				path = fmt.Sprintf("%v", args)
			}
		}

		// Try to determine which CLI sent this based on User-Agent or known metadata
		tag := "Native Hook"
		sourceCLI := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Agent-CLI")))
		ua := strings.ToLower(c.GetHeader("User-Agent"))
		if sourceCLI == "claude" || strings.Contains(ua, "claude") {
			tag = "Claude Code"
		} else if sourceCLI == "gemini" || strings.Contains(ua, "gemini") {
			tag = "Gemini CLI"
		} else if sourceCLI == "codex" || strings.Contains(ua, "codex") {
			tag = "Codex"
		} else if sourceCLI == "copilot" || strings.Contains(ua, "copilot") || strings.Contains(ua, "gh-copilot") {
			tag = "GitHub Copilot"
		} else if sourceCLI == "kiro" || strings.Contains(ua, "kiro") {
			tag = "Kiro CLI"
		} else {
			// Fallback: check hook event names or other characteristics
			if hookEvent == "BeforeTool" {
				tag = "Gemini CLI"
			} else if hookEvent == "preToolUse" {
				tag = "GitHub Copilot"
			} else if hookEvent == "agentSpawn" || hookEvent == "userPromptSubmit" || hookEvent == "stop" {
				tag = "Kiro CLI"
			} else if hookEvent == "PreToolUse" {
				// Ambiguous between Claude and Codex, but we can guess or leave as Native Hook
			}
		}

		broadcast <- &pb.Event{
			Type: "native_hook",
			Tag:  tag,
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

	r.POST("/cluster/heartbeat", clusterHeartbeatHandler)
	r.POST("/cluster/register", clusterHeartbeatHandler)

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
			config.GET("/runtime", func(c *gin.Context) {
				c.JSON(http.StatusOK, buildRuntimeConfigResponse())
			})
			config.PUT("/runtime", func(c *gin.Context) {
				var req RuntimeSettings
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid runtime settings"})
					return
				}
				settings, err := runtimeSettingsStore.UpdateLogging(req.LogPersistenceEnabled, req.LogFilePath)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, buildRuntimeConfigResponseFromSettings(settings))
			})
			config.POST("/access-token", func(c *gin.Context) {
				settings, err := runtimeSettingsStore.RotateAccessToken()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, buildRuntimeConfigResponseFromSettings(settings))
			})
			config.GET("/export", func(c *gin.Context) {
				runtimeSnapshot := runtimeSettingsStore.Snapshot()
				cfg := ExportConfig{Comms: make(map[string]string), Paths: make(map[string]string), Rules: make(map[string]WrapperRule), Runtime: &runtimeSnapshot}
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
				rulesMu.RLock()
				for comm, rule := range wrapperRules {
					cfg.Rules[comm] = rule
				}
				rulesMu.RUnlock()
				c.JSON(200, cfg)
			})
			config.POST("/import", func(c *gin.Context) {
				var cfg ExportConfig
				_ = c.ShouldBindJSON(&cfg)
				if cfg.Runtime != nil {
					if _, err := runtimeSettingsStore.Replace(*cfg.Runtime); err != nil {
						c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
						return
					}
				}
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
				rulesMu.Lock()
				wrapperRules = make(map[string]WrapperRule, len(cfg.Rules))
				for comm, rule := range cfg.Rules {
					wrapperRules[comm] = rule
				}
				rulesMu.Unlock()
				c.JSON(200, gin.H{"status": "ok"})
			})
			api.Any("/mcp", gin.WrapH(buildMCPHandler()))
			cluster := api.Group("/cluster")
			{
				cluster.GET("/state", clusterStateHandler)
				cluster.GET("/nodes", clusterNodesHandler)
			}
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
							if err := installNativeHook(target); err != nil {
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
							_ = uninstallNativeHook(target)
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
				hooks.GET("/:id/raw", func(c *gin.Context) {
					id := c.Param("id")
					var target HookDef
					found := false
					for _, h := range availableHooks {
						if h.ID == id {
							target = h
							found = true
							break
						}
					}
					if !found || target.HookType != HookTypeNative {
						c.JSON(404, gin.H{"error": "native hook not found"})
						return
					}
					if target.ID == "kiro" {
						if err := ensureKiroManagedAgentExists(); err != nil {
							c.JSON(500, gin.H{"error": err.Error()})
							return
						}
					}
					b, err := os.ReadFile(target.NativeConfigPath)
					if err != nil {
						if os.IsNotExist(err) {
							c.JSON(200, gin.H{"content": "{}", "path": target.NativeConfigPath, "format": target.ConfigFormat})
							return
						}
						c.JSON(500, gin.H{"error": err.Error()})
						return
					}
					c.JSON(200, gin.H{"content": string(b), "path": target.NativeConfigPath, "format": target.ConfigFormat})
				})
				hooks.POST("/:id/raw", func(c *gin.Context) {
					id := c.Param("id")
					var req struct {
						Content string `json:"content"`
					}
					if err := c.ShouldBindJSON(&req); err != nil {
						c.JSON(400, gin.H{"error": "invalid request"})
						return
					}
					var target HookDef
					found := false
					for _, h := range availableHooks {
						if h.ID == id {
							target = h
							found = true
							break
						}
					}
					if !found || target.HookType != HookTypeNative {
						c.JSON(404, gin.H{"error": "native hook not found"})
						return
					}
					// Basic validation: ensure it's valid format
					var js map[string]interface{}
					if target.ConfigFormat == ConfigFormatTOML {
						if err := toml.Unmarshal([]byte(req.Content), &js); err != nil {
							c.JSON(400, gin.H{"error": "invalid TOML: " + err.Error()})
							return
						}
					} else {
						if err := json.Unmarshal([]byte(req.Content), &js); err != nil {
							c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
							return
						}
					}

					if err := os.MkdirAll(filepath.Dir(target.NativeConfigPath), 0755); err != nil {
						c.JSON(500, gin.H{"error": err.Error()})
						return
					}
					if err := os.WriteFile(target.NativeConfigPath, []byte(req.Content), 0644); err != nil {
						c.JSON(500, gin.H{"error": err.Error()})
						return
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
			system.GET("/file-preview", func(c *gin.Context) {
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
	clusterManagerStore.ConfigurePort(actualPort)
	writePortFile(actualPort)
	startClusterHeartbeatLoop()
	_ = r.Run(fmt.Sprintf(":%d", actualPort))
}
