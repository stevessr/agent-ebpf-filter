package main

import (
	"log"
	"os/user"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/cilium/ebpf"
)

const udsPath = "/tmp/agent-ebpf.sock"
const ebpfPinRoot = "/sys/fs/bpf/agent-ebpf"
const ebpfPinMapsDir = ebpfPinRoot + "/maps"
const ebpfPinLinksDir = ebpfPinRoot + "/links"

type bpfEvent struct {
	PID, TGID, PPID, UID, GID, Type, TagID uint32
	Comm                                   [16]byte
	Path                                   [256]byte
	NetFamily                              uint32
	NetDirection                           uint32
	NetBytes                               uint32
	NetPort                                uint32
	NetAddr                                [16]byte
	_                                      [4]byte // Padding for 8-byte alignment of Retval
	Retval                                 int64
	DurationNs                             uint64
	CgroupID                               uint64
	Extra1                                 uint32
	Extra2                                 uint32
	Extra3                                 uint64
	Extra4                                 [256]byte
}

type WrapperRule struct {
	Comm         string   `json:"comm"`
	Action       string   `json:"action"`
	RewrittenCmd []string `json:"rewritten_cmd,omitempty"`
	Regex        string   `json:"regex,omitempty"`
	Replacement  string   `json:"replacement,omitempty"`
	Priority     int      `json:"priority,omitempty"`
	Behavior     string   `json:"behavior,omitempty"`
}

type shellControlMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

type trackerMapSet struct {
	AgentPids       *ebpf.Map
	TrackedComms    *ebpf.Map
	TrackedPaths    *ebpf.Map
	TrackedPrefixes *ebpf.Map
	Events          *ebpf.Map
	CollectorStats  *ebpf.Map
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
	Language    string    `json:"language,omitempty"`
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
		ID: "augment", Name: "Augment (Auggie CLI)", HookType: HookTypeNative,
		Description:     "Uses Auggie's native PreToolUse hook in ~/.augment/settings.json to intercept tool calls",
		TargetCmd:       "auggie",
		NativeHookEvent: "PreToolUse",
		ConfigFormat:    ConfigFormatJSON,
	},
	{
		ID: "cursor", Name: "Cursor", HookType: HookTypeWrapper,
		Description: "Intercepts cursor execution via shell alias wrapper",
		TargetCmd:   "cursor",
	},
}

type gpuInfo struct{ mem, gpu, util uint32 }

type vmFaultCounters struct {
	pageFaults  uint64
	majorFaults uint64
	swapIn      uint64
	swapOut     uint64
}

var (
	trackerMaps trackerMapSet

	tagsMu      sync.RWMutex
	tagMap             = map[uint32]string{0: "Unknown", 1: "AI Agent", 2: "Git", 3: "Build Tool", 4: "System Pkg", 5: "Runtime", 6: "System Tool", 7: "Network Tool", 8: "Security", 9: "Shell", 10: "Language Pkg", 11: "Container CLI", 12: "Agent CLI"}
	tagNameToID        = map[string]uint32{"AI Agent": 1, "Git": 2, "Build Tool": 3, "System Pkg": 4, "Runtime": 5, "System Tool": 6, "Network Tool": 7, "Security": 8, "Shell": 9, "Language Pkg": 10, "Container CLI": 11, "Agent CLI": 12}
	nextTagID   uint32 = 13

	rulesMu      sync.RWMutex
	wrapperRules = make(map[string]WrapperRule)

	disabledCommsMu sync.RWMutex
	disabledComms   = make(map[string]struct{})

	disabledEventTypesMu sync.RWMutex
	disabledEventTypes   = make(map[uint32]struct{})

	nvmlInitialized bool

	// For non-NVIDIA GPU tracking (Intel/AMD via fdinfo)
	fdinfoHistory   = make(map[string]uint64) // pid:fd -> last_engine_ns
	fdinfoHistoryMu sync.RWMutex
	fdinfoTime      time.Time

	sudoUser          *user.User
	sudoUserHomeCache string
)

func init() {
	if ret := nvml.Init(); ret == nvml.SUCCESS {
		nvmlInitialized = true
	} else {
		log.Printf("NVML Init failed: %v", nvml.ErrorString(ret))
	}
}
