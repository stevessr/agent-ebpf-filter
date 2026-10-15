// Package jsplugins runs narrowly scoped, data-only JavaScript hooks using moejs.
// It grants no filesystem, networking, shell, kernel, or privileged host APIs.
package jsplugins

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "strings"
    "time"

    "github.com/Calcium-Ion/moejs"
)

const (
    MaxSourceBytes = 128 << 10
    MaxPayloadBytes = 32 << 10
    MaxResultBytes = 32 << 10
    DefaultTimeout = 100 * time.Millisecond
    MemoryLimitBytes int64 = 8 << 20
)

var (
	ErrHookUnsupported = errors.New("javascript plugin hook must be onEvent or onAlert")
	ErrExecutorBusy = errors.New("javascript plugin executor is busy")
)

// Program is a compiled immutable module. Each call uses a fresh runtime.
// The current API deliberately has no plugin-to-host function imports.
type Program struct {
    module *moejs.Module
    hooks map[string]moejs.Hook
}

// Compile validates source and its declared hooks without evaluating module code.
func Compile(source string) (*Program, error) {
    if strings.TrimSpace(source) == "" || len(source) > MaxSourceBytes {
        return nil, fmt.Errorf("javascript plugin source must be 1..%d bytes", MaxSourceBytes)
    }
    mod, err := moejs.Compile("plugin.mjs", source)
    if err != nil {
        return nil, err
    }
    if len(mod.Requests()) != 0 {
        return nil, errors.New("static module imports are not supported")
    }
    hooks := make(map[string]moejs.Hook, 2)
    for _, name := range []string{"onEvent", "onAlert"} {
        if hook, err := mod.Hook(name); err == nil {
            hooks[name] = hook
        }
    }
    if len(hooks) == 0 {
        return nil, errors.New("javascript plugin must export onEvent or onAlert")
    }
    return &Program{module: mod, hooks: hooks}, nil
}

// Run executes an explicit hook on JSON metadata only. It never enforces
// kernel policy or writes to any event stream.
func (p *Program) Run(ctx context.Context, name string, payload json.RawMessage) (json.RawMessage, error) {
    if name != "onEvent" && name != "onAlert" {
        return nil, ErrHookUnsupported
    }
    hook, ok := p.hooks[name]
    if !ok {
        return nil, fmt.Errorf("plugin does not export %s", name)
    }
    if len(payload) == 0 || len(payload) > MaxPayloadBytes || !json.Valid(payload) {
        return nil, errors.New("invalid or oversized JSON plugin payload")
    }
    if ctx == nil {
        ctx = context.Background()
    }
    ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
    defer cancel()
    if err := ctx.Err(); err != nil {
        return nil, err
    }
    rt := moejs.NewRuntime(moejs.Options{
        DisableDynamicCode: true,
        MemoryLimit: MemoryLimitBytes,
    })
    // Interrupt is the only runtime method allowed on a different goroutine.
    // Wait for a racing cancellation before releasing the runtime.
    interrupted := make(chan struct{})
    stop := context.AfterFunc(ctx, func() {
        rt.Interrupt(context.Cause(ctx))
        close(interrupted)
    })
    defer func() {
        if !stop() {
            <-interrupted
        }
    }()
    if err := rt.Load(p.module); err != nil {
        if ctx.Err() != nil {
            return nil, ctx.Err()
        }
        return nil, fmt.Errorf("load javascript plugin: %w", err)
    }
    callable, err := rt.Has(hook)
    if err != nil {
        return nil, err
    }
    if !callable {
        return nil, errors.New("plugin export is not callable")
    }
    arg, err := rt.ParseJSONString(string(payload))
    if err != nil {
        return nil, err
    }
    value, err := rt.Call(hook, arg)
    if err != nil {
        if ctx.Err() != nil {
            return nil, ctx.Err()
        }
        return nil, err
    }
    // moejs flushes queued Promise reactions before Call returns. A Promise
    // still pending has no host event loop to advance it, so fail explicitly
    // instead of serializing it as the misleading JSON object "{}".
    if state, settled, isPromise := moejs.PromiseResult(value); isPromise {
        switch state {
        case moejs.PromiseFulfilled:
            value = settled
        case moejs.PromiseRejected:
            return nil, errors.New("javascript plugin returned a rejected Promise")
        default:
            return nil, errors.New("javascript plugin returned a pending Promise")
        }
    }
    out, err := rt.AppendJSON(nil, value)
    if err != nil {
        if ctx.Err() != nil {
            return nil, ctx.Err()
        }
        return nil, err
    }
    if err := ctx.Err(); err != nil {
        return nil, err
    }
    if len(out) > MaxResultBytes || !json.Valid(out) {
        return nil, errors.New("invalid or oversized plugin result")
    }
    return json.RawMessage(out), nil
}
