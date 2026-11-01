// This test is copied into yetone/magpie/internal/middleware by our CI to
// run against the REAL upstream moejs plugin loader and request hooks.
// Keep the test in package middleware, not in the Agent eBPF Filter backend.
package middleware

import (
    "bytes"
    "context"
    "net/http/httptest"
    "os"
    "strings"
    "testing"

    "github.com/yetone/magpie/internal/plugin"
)

func TestAgentEBPFExternalMiddleware(t *testing.T) {
    dir := os.Getenv("AGENT_EBPF_MAGPIE_PLUGIN")
    if dir == "" {
        t.Skip("set AGENT_EBPF_MAGPIE_PLUGIN to test a local middleware directory")
    }
    if _, err := os.Stat(dir + "/package.json"); err != nil {
        t.Fatal(err)
    }

    // Exercise the same plugin.Add path as 'magpie plugin add <folder>',
    // including its package.json manifest and middleware entry detection.
    registered, err := plugin.Add(context.Background(), dir)
    if err != nil {
        t.Fatalf("Magpie rejected behavior plugin installation: %v", err)
    }
    if registered.Spec != dir {
        t.Fatalf("registered unexpected plugin source: %s", registered.Spec)
    }
    if entry, _ := plugin.Middleware(plugin.Target(registered.Spec)); entry == "" {
        t.Fatal("installed plugin is not recognized as Magpie middleware")
    }

    use(t, plugin.Entry{Spec: dir, Options: map[string]any{
        "mode": "enforce",
        "blocked_tools": []string{"Bash", "shell*"},
        "audit": false,
    }})
    policy := Begin(Info{Protocol: "chat", Model: "test-model", Agent: "codex", Stream: true, Path: "/v1/chat/completions"})
    if policy == nil {
        t.Fatal("Magpie did not load the external middleware")
    }
    _, denial := policy.Request([]byte(`{"model":"test-model","tools":[{"type":"function","function":{"name":"Bash"}}]}`))
    policy.End()
    if denial == nil || denial.Status != 403 || !strings.Contains(denial.Message, "unapproved_tool") {
        t.Fatalf("tool policy bypassed: %+v", denial)
    }

    permit := Begin(Info{Protocol: "chat", Model: "test-model", Agent: "codex", Stream: true, Path: "/v1/chat/completions"})
    if permit == nil {
        t.Fatal("middleware vanished")
    }
    payload := []byte(`{"model":"test-model","tools":[{"type":"function","function":{"name":"Read"}}]}`)
    got, denied := permit.Request(payload)
    if denied != nil || !bytes.Equal(got, payload) {
        t.Fatalf("non-blocked request changed: body=%s deny=%+v", got, denied)
    }

    // The plugin observes streaming tool calls, but must never rewrite or
    // drop the agent's stream, even if the model proposes Bash.
    recorder := httptest.NewRecorder()
    recorder.Header().Set("Content-Type", "text/event-stream")
    stream, finish := permit.Wrap(recorder)
    event := []byte("event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"name\":\"Bash\"}}\n\n")
    if _, err := stream.Write(event); err != nil {
        t.Fatal(err)
    }
    finish()
    permit.End()
    if !bytes.Equal(recorder.Body.Bytes(), event) {
        t.Fatalf("stream changed by audit plugin: got %s", recorder.Body.String())
    }
}
