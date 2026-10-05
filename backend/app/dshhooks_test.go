package app

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDshNativeHookLifecycle(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node required")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl required")
	}
	old := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{HookSecrets: map[string]string{"dsh": "test-secret"}}}
	t.Cleanup(func() { runtimeSettingsStore = old })
	observed := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Agent-CLI") != "dsh" || r.Header.Get("X-Agent-Hook-Secret") != "test-secret" {
			t.Error("missing hook authentication")
		}
		body, _ := io.ReadAll(r.Body)
		observed <- string(body)
		w.WriteHeader(204)
	}))
	defer server.Close()
	t.Setenv("AGENT_HOOK_ENDPOINT", server.URL)
	root := t.TempDir()
	h := HookDef{ID: "dsh", HookType: HookTypeNative, ConfigFormat: ConfigFormatTypeScript, NativeConfigPath: filepath.Join(root, "plugins", "hook.mjs"), NativeFeatureConfigPath: filepath.Join(root, "cordis.patch.yml")}
	original := "# user comment\n- insert:\n    - id: user-plugin\n      name: example\n"
	if err := os.WriteFile(h.NativeFeatureConfigPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := installNativeHook(h); err != nil {
			t.Fatal(err)
		}
	}
	if !isNativeHookInstalled(h) {
		t.Fatal("plugin not detected")
	}
	patch, _ := os.ReadFile(h.NativeFeatureConfigPath)
	if strings.Count(string(patch), dshPatchStart) != 1 || !strings.HasPrefix(string(patch), original) {
		t.Fatal("user patch changed or duplicate plugin")
	}
	info, err := os.Stat(hookRelayScriptPath(h))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("relay permissions: %v", info.Mode())
	}
	script := fmt.Sprintf(`import { apply } from %q;
const callbacks = new Map();
apply({ on: (name, fn) => callbacks.set(name, fn), effect: (fn) => callbacks.set("dispose", fn()) });
const session = { id: 'session-test', header: { cwd: '/tmp' } };
callbacks.get('session/created')(session);
callbacks.get('session/event')(session, { type: 'tool/call', data: {name:'bash',callId:'call-test',arguments:'SECRET_INPUT'} });
callbacks.get('session/event')(session, { type: 'tool/result', data: {message:{content:[{type:'tool-result',toolCallId:'call-test',isError:true,content:'SECRET_OUTPUT'}]}} });
await new Promise(r=>setTimeout(r,500));
callbacks.get('dispose')();
`, "file://"+h.NativeConfigPath)
	cmd := exec.Command("node", "--input-type=module", "-e", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin run: %v %s", err, output)
	}
	for i := 0; i < 3; i++ {
		select {
		case body := <-observed:
			if strings.Contains(body, "SECRET_") || !strings.Contains(body, "session-test") {
				t.Fatalf("unsafe or missing metadata: %s", body)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("missing relayed event")
		}
	}
	if err := uninstallNativeHook(h); err != nil {
		t.Fatal(err)
	}
	patch, _ = os.ReadFile(h.NativeFeatureConfigPath)
	if string(patch) != original || isNativeHookInstalled(h) {
		t.Fatal("uninstall did not preserve user configuration")
	}
	if _, err := os.Stat(h.NativeConfigPath); !os.IsNotExist(err) {
		t.Fatal("plugin remains")
	}
}

func TestDshPatchRejectsUnsafeDocuments(t *testing.T) {
	h := HookDef{NativeConfigPath: "/tmp/plugin.mjs"}
	for _, input := range []string{"a: b\n", dshPatchStart, "[invalid", "[]\n---\n[]\n"} {
		if _, err := dshPatch(input, h, true); err == nil {
			t.Errorf("accepted unsafe patch %q", input)
		}
	}
	for _, input := range []string{"", "[]", "# comment\n"} {
		if _, err := dshPatch(input, h, true); err != nil {
			t.Errorf("empty document %q: %v", input, err)
		}
	}
}

func TestResolveDshHome(t *testing.T) {
	for _, test := range []struct{ env, want string }{{"", "/home/test/.dsh"}, {"~/.custom", "/home/test/.custom"}, {"/tmp/custom", "/tmp/custom"}} {
		t.Setenv("DSH_HOME", test.env)
		if got := resolveDshHome("/home/test"); got != test.want {
			t.Errorf("got %s want %s", got, test.want)
		}
	}
}
