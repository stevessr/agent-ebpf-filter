package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"agent-ebpf-filter/app/jsplugins"
)

const (
	maxConcurrentJSInvocations = 4
	maxCachedJSPrograms       = 32
)

var (
	ErrJSPluginBusy   = jsplugins.ErrExecutorBusy
	jsInvocationSlots = make(chan struct{}, maxConcurrentJSInvocations)

	jsProgramCache = struct {
		mu       sync.Mutex
		programs map[string]*jsplugins.Program
		order    []string
	}{
		programs: make(map[string]*jsplugins.Program),
	}
)

// compiledJSProgram reuses immutable bytecode by verified source digest.
// The cache is globally bounded and cannot preserve runtime state between calls.
func compiledJSProgram(digest, source string) (*jsplugins.Program, error) {
	jsProgramCache.mu.Lock()
	defer jsProgramCache.mu.Unlock()

	if program, ok := jsProgramCache.programs[digest]; ok {
		return program, nil
	}
	program, err := jsplugins.Compile(source)
	if err != nil {
		return nil, err
	}
	if len(jsProgramCache.order) >= maxCachedJSPrograms {
		delete(jsProgramCache.programs, jsProgramCache.order[0])
		jsProgramCache.order = jsProgramCache.order[1:]
	}
	jsProgramCache.order = append(jsProgramCache.order, digest)
	jsProgramCache.programs[digest] = program
	return program, nil
}

// RunJavaScriptPlugin executes a bounded, opt-in JSON metadata hook.
// The artifact lock prevents source swaps, disable and deletion from racing
// an invocation. No kernel policy or BPF maps are changed by the returned data.
func RunJavaScriptPlugin(ctx context.Context, id, hook string, payload json.RawMessage) (json.RawMessage, error) {
	if err := validatePluginID(id); err != nil {
		return nil, err
	}
	if hook != "onEvent" && hook != "onAlert" {
		return nil, jsplugins.ErrHookUnsupported
	}
	if len(payload) == 0 || len(payload) > jsplugins.MaxPayloadBytes || !json.Valid(payload) {
		return nil, errors.New("invalid or oversized JSON plugin payload")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, jsplugins.DefaultTimeout)
	defer cancel()

	// Reject excess load rather than spawning unbounded JS runtimes or
	// placing work in a potentially unbounded HTTP request queue.
	select {
	case jsInvocationSlots <- struct{}{}:
		defer func() { <-jsInvocationSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, ErrJSPluginBusy
	}

	releaseArtifacts, err := acquirePluginArtifactLock(ctx, id)
	if err != nil {
		return nil, err
	}
	defer releaseArtifacts()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	manifest, ok := pluginRegistry.Get(id)
	if !ok {
		return nil, fmt.Errorf("plugin %q not found", id)
	}
	if manifest.Kind != PluginKindJavaScript {
		return nil, errors.New("not a JavaScript plugin")
	}
	if !manifest.Enabled {
		return nil, errors.New("JavaScript plugin is disabled")
	}
	source, err := readPluginFile(id, "plugin.mjs", jsplugins.MaxSourceBytes)
	if err != nil {
		return nil, err
	}
	if manifest.SourceSHA256 == "" || sha256Hex(source) != manifest.SourceSHA256 {
		return nil, errors.New("JavaScript plugin source checksum mismatch")
	}
	program, err := compiledJSProgram(manifest.SourceSHA256, string(source))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Program.Run creates a new runtime per call with its own interrupt,
	// allocation limit and deadline. Compiled bytecode is safe to share.
	return program.Run(ctx, hook, payload)
}

