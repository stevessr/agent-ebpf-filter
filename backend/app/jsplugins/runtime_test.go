package jsplugins

import (
    "context"
    "encoding/json"
    "errors"
    "strings"
    "testing"
    "time"
)

func TestCompileAndRun(t *testing.T) {
    p, err := Compile("export function onEvent(event) { return { alert: event.risk >= 80, name: event.comm }; }")
    if err != nil { t.Fatal(err) }
    got, err := p.Run(context.Background(), "onEvent", json.RawMessage(`{"risk":90,"comm":"agent"}`))
    if err != nil { t.Fatal(err) }
    var result struct { Alert bool `json:"alert"`; Name string `json:"name"` }
    if err := json.Unmarshal(got, &result); err != nil { t.Fatal(err) }
    if !result.Alert || result.Name != "agent" { t.Fatalf("result=%s",got) }
}

func TestInvalidSourcesAndHook(t *testing.T) {
    for _, source := range []string{
        "export const x = 1;",
        "import {a} from 'fs'; export function onEvent(e){return e}",
        "export function onEvent( { ",
        strings.Repeat("a", MaxSourceBytes + 1),
    } {
        if _, err := Compile(source); err == nil { t.Errorf("accepted source %q",source[:min(len(source),32)]) }
    }
    p, err := Compile("export function onEvent(e){ return e; }")
    if err != nil { t.Fatal(err) }
    if _, err := p.Run(context.Background(), "onPolicy", json.RawMessage("{}")); !errors.Is(err,ErrHookUnsupported) { t.Fatal(err) }
    if _, err := p.Run(context.Background(), "onAlert", json.RawMessage("{}")); err == nil { t.Fatal("missing hook accepted") }
    if _, err := p.Run(context.Background(), "onEvent", json.RawMessage("{bad")); err == nil { t.Fatal("bad JSON accepted") }
}

func TestLoopInterrupted(t *testing.T) {
    p, err := Compile("export function onEvent(e) { for (;;) {} }")
    if err != nil { t.Fatal(err) }
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
    defer cancel()
    started:=time.Now()
    _, err = p.Run(ctx, "onEvent", json.RawMessage("{}"))
    if err == nil || time.Since(started)>time.Second { t.Fatalf("interrupt err=%v elapsed=%s",err,time.Since(started)) }
}
