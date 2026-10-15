package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"agent-ebpf-filter/app/jsplugins"
)

const maxConcurrentJSInvocations = 4

var (
	ErrJSPluginBusy = jsplugins.ErrExecutorBusy
	jsInvocationSlots = make(chan struct{}, maxConcurrentJSInvocations)
)

// RunJavaScriptPlugin validates the signed-on-disk source and invokes only a
// separate JS worker process over a private Unix socket. No backend VM is run.
// The artifact lease coordinates invocation with updates and disable/delete.
func RunJavaScriptPlugin(ctx context.Context, id, hook string, payload json.RawMessage) (json.RawMessage, error) {
	if err := validatePluginID(id); err != nil { return nil, err }
	if hook != "onEvent" && hook != "onAlert" { return nil, jsplugins.ErrHookUnsupported }
	if len(payload) == 0 || len(payload) > jsplugins.MaxPayloadBytes || !json.Valid(payload) {
		return nil, errors.New("invalid or oversized JSON plugin payload")
	}
	if ctx == nil { ctx = context.Background() }
	ctx, cancel := context.WithTimeout(ctx, jsplugins.DefaultTimeout)
	defer cancel()

	select {
	case jsInvocationSlots <- struct{}{}:
		defer func(){ <-jsInvocationSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, ErrJSPluginBusy
	}

	releaseArtifacts, err := acquirePluginArtifactLock(ctx, id)
	if err != nil { return nil, err }
	defer releaseArtifacts()

	if err := ctx.Err(); err != nil { return nil, err }
	manifest, ok := pluginRegistry.Get(id)
	if !ok { return nil, fmt.Errorf("plugin %q not found", id) }
	if manifest.Kind != PluginKindJavaScript { return nil, errors.New("not a JavaScript plugin") }
	if !manifest.Enabled { return nil, errors.New("JavaScript plugin is disabled") }
	source, err := readPluginFile(id, "plugin.mjs", jsplugins.MaxSourceBytes)
	if err != nil { return nil, err }
	if manifest.SourceSHA256 == "" || sha256Hex(source) != manifest.SourceSHA256 {
		return nil, errors.New("JavaScript plugin source checksum mismatch")
	}
	if err := ctx.Err(); err != nil { return nil, err }
	return jsplugins.InvokeRemote(ctx, os.Getenv("AGENT_EBPF_JS_WORKER_SOCKET"), string(source), hook, payload)
}
