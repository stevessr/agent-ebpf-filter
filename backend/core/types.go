// Package core provides shared types and constants used across all
// sub-packages of agent-ebpf-filter.
package core

import (
	"log"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/cilium/ebpf"
)

// ── Constants ────────────────────────────────────────────────────────────────



// ── eBPF event ───────────────────────────────────────────────────────────────

// BpfEvent is the raw event struct read from the eBPF ring buffer.
type BpfEvent struct {
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

// ── Wrapper rules ────────────────────────────────────────────────────────────

// WrapperRule defines a policy rule for the agent-wrapper CLI interceptor.
type WrapperRule struct {
	Comm         string   `json:"comm"`
	Action       string   `json:"action"`
	RewrittenCmd []string `json:"rewritten_cmd,omitempty"`
	Regex        string   `json:"regex,omitempty"`
	Replacement  string   `json:"replacement,omitempty"`
	Priority     int      `json:"priority,omitempty"`
	Behavior     string   `json:"behavior,omitempty"`
}

// ── Hook definitions ─────────────────────────────────────────────────────────

// HookType distinguishes how the hook intercepts the agent CLI.
type HookType string

// ConfigFormat defines if the config is JSON or TOML.
type ConfigFormat string

// HookDef describes one supported AI-CLI hook integration.
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

// AvailableHooks is the list of all supported AI-CLI hook integrations.
var AvailableHooks = []HookDef{
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
		ID: "antigravity", Name: "Antigravity CLI", HookType: HookTypeNative,
		Description:     "Installs an Antigravity CLI plugin with hooks.json and a JSON-stdout aware relay for all tool calls",
		TargetCmd:       "agy",
		NativeHookEvent: "PreToolUse",
		NativeMatcher:   "*",
		ConfigFormat:    ConfigFormatJSON,
	},
	{
		ID: "cursor", Name: "Cursor", HookType: HookTypeWrapper,
		Description: "Intercepts cursor execution via shell alias wrapper",
		TargetCmd:   "cursor",
	},
}

// ── Hardware info ────────────────────────────────────────────────────────────

// GpuInfo holds per-process GPU utilization metrics.
type GpuInfo struct{ Mem, GPU, Util uint32 }

// VmFaultCounters holds page-fault / swap counters for a process.
type VmFaultCounters struct {
	PageFaults  uint64
	MajorFaults uint64
	SwapIn      uint64
	SwapOut     uint64
}

// ── Misc types ───────────────────────────────────────────────────────────────

// KiroHookState tracks the previous default agent before Kiro hook installation.
type KiroHookState struct {
	PreviousDefaultAgent string `json:"previous_default_agent,omitempty"`
}

// FilePreviewResponse is returned by the filesystem explorer preview endpoint.
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
	Encoding    string    `json:"encoding,omitempty"`
	Content     string    `json:"content,omitempty"`
	DataURL     string    `json:"dataUrl,omitempty"`
	Truncated   bool      `json:"truncated,omitempty"`
	Streamable  bool      `json:"streamable,omitempty"`
	Hexable     bool      `json:"hexable,omitempty"`
}

// TrackerMapSet holds references to the pinned eBPF maps.
type TrackerMapSet struct {
	AgentPids       *ebpf.Map
	TrackedComms    *ebpf.Map
	TrackedPaths    *ebpf.Map
	TrackedPrefixes *ebpf.Map
	Events          *ebpf.Map
	CollectorStats  *ebpf.Map
}

// ShellControlMessage is sent over the WebSocket to resize the PTY.
type ShellControlMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

// ── NVML initialization ──────────────────────────────────────────────────────

var NvmlInitialized bool

func init() {
	if ret := nvml.Init(); ret == nvml.SUCCESS {
		NvmlInitialized = true
	} else {
		log.Printf("NVML Init failed: %v", nvml.ErrorString(ret))
	}
}
