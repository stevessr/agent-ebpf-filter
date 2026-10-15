package app

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"

    "agent-ebpf-filter/app/jsplugins"
)

// RunJavaScriptPlugin executes a user-selected, data-only plugin hook.
// No BPF map writes, shell invocations, or policy mutation are possible.
func RunJavaScriptPlugin(ctx context.Context, id, hook string, payload json.RawMessage) (json.RawMessage, error) {
    if err := validatePluginID(id); err != nil {
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
    program, err := jsplugins.Compile(string(source))
    if err != nil {
        return nil, err
    }
    return program.Run(ctx, hook, payload)
}
