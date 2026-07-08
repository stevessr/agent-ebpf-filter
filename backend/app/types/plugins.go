package types

import "time"

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
