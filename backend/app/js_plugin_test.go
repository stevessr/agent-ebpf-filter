package app

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/app/jsplugins"
)

func isolatedJSRegistry(t *testing.T) {
	t.Helper()
	withPluginRoot(t)
	old := pluginRegistry
	pluginRegistry = &pluginStore{entries: make(map[string]*PluginManifest), loaded: true}
	t.Cleanup(func() { pluginRegistry = old })

	// Integration tests exercise the same private Unix IPC path as production.
	// This local in-process test server is not a production security sandbox.
	socket := filepath.Join(t.TempDir(), "test-worker.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil { t.Fatal(err) }
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func(){ done <- jsplugins.NewWorker().Serve(ctx, listener) }()
	t.Setenv("AGENT_EBPF_JS_WORKER_SOCKET", socket)
	t.Cleanup(func(){
		stop()
		select {
		case err := <-done:
			if err != nil { t.Errorf("test JS worker: %v", err) }
		case <-time.After(time.Second):
			t.Error("test JS worker did not shut down")
		}
	})
}

func jsManifest() *PluginManifest {
	return &PluginManifest{ID: "testing-js", Name: "Testing JS", Kind: PluginKindJavaScript}
}

const jsSource = `export function onEvent(event) { return { flagged: event.risk >= 80, comm: event.comm }; }`

func TestJavaScriptRegistryLifecycle(t *testing.T) {
	isolatedJSRegistry(t)
	if err := pluginRegistry.UpsertWithSource(jsManifest(), jsSource); err != nil {
		t.Fatal(err)
	}
	stored, ok := pluginRegistry.Get("testing-js")
	if !ok || stored.SourceSHA256 != sha256Hex([]byte(jsSource)) || stored.Enabled {
		t.Fatalf("invalid stored JS plugin: %+v", stored)
	}
	if source, err := PluginSource("testing-js"); err != nil || source != jsSource {
		t.Fatalf("source=%q error=%v", source, err)
	}
	if _, err := RunJavaScriptPlugin(context.Background(), "testing-js", "onEvent", json.RawMessage(`{}`)); err == nil {
		t.Fatal("disabled plugin was callable")
	}
	if _, err := pluginRegistry.SetEnabled(context.Background(), "testing-js", true); err != nil {
		t.Fatal(err)
	}
	out, err := RunJavaScriptPlugin(context.Background(), "testing-js", "onEvent", json.RawMessage(`{"comm":"agent","risk":90}`))
	if err != nil || !strings.Contains(string(out), `"flagged":true`) {
		t.Fatalf("invoke=%s err=%v", out, err)
	}
	changed := `export function onEvent(event) { return { flagged: false }; }`
	if err := writePluginFile("testing-js", "plugin.mjs", []byte(changed), jsplugins.MaxSourceBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := RunJavaScriptPlugin(context.Background(), "testing-js", "onEvent", json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("modified plugin ran: %v", err)
	}
	if _, err := pluginRegistry.SetEnabled(context.Background(), "testing-js", false); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginRegistry.SetEnabled(context.Background(), "testing-js", true); err == nil {
		t.Fatal("enabled tampered source")
	}
	if err := pluginRegistry.Delete("testing-js"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(pluginsRootPath(), "testing-js", "plugin.mjs")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plugin source was not deleted: %v", err)
	}
	if _, err := RunJavaScriptPlugin(context.Background(), "testing-js", "onEvent", json.RawMessage(`{}`)); err == nil {
		t.Fatal("deleted plugin remained callable")
	}
}

func TestJavaScriptSourceUpdateAndKindImmutability(t *testing.T) {
	isolatedJSRegistry(t)
	if err := pluginRegistry.UpsertWithSource(jsManifest(), jsSource); err != nil {
		t.Fatal(err)
	}
	replacement := `export function onAlert(event) { return { kind: "alert" }; }`
	if err := pluginRegistry.UpsertWithSource(jsManifest(), replacement); err != nil {
		t.Fatal(err)
	}
	manifest, _ := pluginRegistry.Get("testing-js")
	if manifest.SourceSHA256 != sha256Hex([]byte(replacement)) {
		t.Fatalf("digest was not updated: %+v", manifest)
	}
	modified := jsManifest()
	modified.Kind = PluginKindCommand
	if err := pluginRegistry.Upsert(modified); err == nil {
		t.Fatal("plugin kind changed in place")
	}
	if _, err := pluginRegistry.SetEnabled(context.Background(), "testing-js", true); err != nil {
		t.Fatal(err)
	}
	if _, err := RunJavaScriptPlugin(context.Background(), "testing-js", "onEvent", json.RawMessage(`{}`)); err == nil {
		t.Fatal("stale old hook remained callable")
	}
	out, err := RunJavaScriptPlugin(context.Background(), "testing-js", "onAlert", json.RawMessage(`{}`))
	if err != nil || !strings.Contains(string(out), "alert") {
		t.Fatalf("updated hook: %s / %v", out, err)
	}
}

func TestJavaScriptExecutorRejectsOverload(t *testing.T) {
	isolatedJSRegistry(t)
	if err := pluginRegistry.UpsertWithSource(jsManifest(), jsSource); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginRegistry.SetEnabled(context.Background(), "testing-js", true); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < cap(jsInvocationSlots); i++ {
		jsInvocationSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for i := 0; i < cap(jsInvocationSlots); i++ {
			<-jsInvocationSlots
		}
	})
	_, err := RunJavaScriptPlugin(context.Background(), "testing-js", "onEvent", json.RawMessage(`{}`))
	if !errors.Is(err, ErrJSPluginBusy) {
		t.Fatalf("expected load shedding, got %v", err)
	}
}

func TestJavaScriptDisableWaitsForInvocationLease(t *testing.T) {
	isolatedJSRegistry(t)
	if err := pluginRegistry.UpsertWithSource(jsManifest(), jsSource); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginRegistry.SetEnabled(context.Background(), "testing-js", true); err != nil {
		t.Fatal(err)
	}
	release, err := acquirePluginArtifactLock(context.Background(), "testing-js")
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() { if !released { release() } }()
	done := make(chan error, 1)
	go func() {
		_, err := pluginRegistry.SetEnabled(context.Background(), "testing-js", false)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("disable bypassed plugin lease: %v", err)
	case <-time.After(15 * time.Millisecond):
	}
	release()
	released = true
	select {
	case err := <-done:
		if err != nil { t.Fatal(err) }
	case <-time.After(time.Second):
		t.Fatal("disable did not complete after execution lock released")
	}
}

func TestJavaScriptRequiresIsolatedWorker(t *testing.T) {
	isolatedJSRegistry(t)
	if err := pluginRegistry.UpsertWithSource(jsManifest(), jsSource); err != nil { t.Fatal(err) }
	if _,err:=pluginRegistry.SetEnabled(context.Background(),"testing-js",true);err!=nil{t.Fatal(err)}
	t.Setenv("AGENT_EBPF_JS_WORKER_SOCKET","")
	_,err:=RunJavaScriptPlugin(context.Background(),"testing-js","onEvent",json.RawMessage(`{}`))
	if err==nil || !strings.Contains(err.Error(),"in-process execution is disabled"){
		t.Fatalf("missing worker must fail closed: %v",err)
	}
}
