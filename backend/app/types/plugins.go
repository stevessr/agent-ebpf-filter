package types

import "time"

// PluginKind enumerates the plugin types supported by the registry.
type PluginKind string

const (
	PluginKindEBPF    PluginKind = "ebpf"    // user-authored eBPF program
	PluginKindWebhook PluginKind = "webhook" // forwards selected events to an HTTP endpoint
	PluginKindCommand PluginKind = "command" // wrapper rewrite rule expressed as a plugin
)

// PluginAttachKind describes how an eBPF plugin attaches to the kernel.
type PluginAttachKind string

const (
	PluginAttachTracepoint PluginAttachKind = "tracepoint"
	PluginAttachKprobe     PluginAttachKind = "kprobe"
	PluginAttachKretprobe  PluginAttachKind = "kretprobe"
	PluginAttachLSM        PluginAttachKind = "lsm"
	PluginAttachNone       PluginAttachKind = "none"
)

// PluginManifest is the on-disk descriptor for a registered plugin.
type PluginManifest struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Author      string     `json:"author,omitempty"`
	Version     string     `json:"version,omitempty"`
	Kind        PluginKind `json:"kind"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`

	// eBPF specific
	SourceSHA256 string           `json:"sourceSha256,omitempty"`
	ObjectSHA256 string           `json:"objectSha256,omitempty"`
	AttachKind   PluginAttachKind `json:"attachKind,omitempty"`
	AttachTarget string           `json:"attachTarget,omitempty"`
	ProgramName  string           `json:"programName,omitempty"`

	// Webhook specific
	WebhookURL    string   `json:"webhookUrl,omitempty"`
	WebhookEvents []string `json:"webhookEvents,omitempty"`

	// Command specific
	CommandComm    string   `json:"commandComm,omitempty"`
	CommandArgs    []string `json:"commandArgs,omitempty"`
	CommandRule    string   `json:"commandRule,omitempty"`
	CommandRewrite []string `json:"commandRewrite,omitempty"`

	// Runtime state
	Loaded    bool   `json:"loaded,omitempty"`
	LoadError string `json:"loadError,omitempty"`
}
