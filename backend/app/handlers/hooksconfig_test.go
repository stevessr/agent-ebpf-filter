package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"agent-ebpf-filter/core"
	"github.com/gin-gonic/gin"
)

func TestHookConfigRawSupportsTypeScriptAndValidatesDocuments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	hooks := []core.HookDef{
		{
			ID:        "dsh",
			HookType:  core.HookTypeWrapper,
			TargetCmd: "dsh",
		},
		{
			ID:               "pi",
			HookType:         core.HookTypeNative,
			NativeConfigPath: filepath.Join(root, "pi.ts"),
			ConfigFormat:     core.ConfigFormatTypeScript,
		},
		{
			ID:               "json-native",
			HookType:         core.HookTypeNative,
			NativeConfigPath: filepath.Join(root, "native.json"),
			ConfigFormat:     core.ConfigFormatJSON,
		},
		{
			ID:               "toml-native",
			HookType:         core.HookTypeNative,
			NativeConfigPath: filepath.Join(root, "native.toml"),
			ConfigFormat:     core.ConfigFormatTOML,
		},
	}
	oldAvailableHooks := Deps.AvailableHooks
	oldIsHookInstalled := Deps.IsHookInstalled
	Deps.AvailableHooks = func() []core.HookDef { return hooks }
	Deps.IsHookInstalled = func(core.HookDef) bool { return false }
	t.Cleanup(func() {
		Deps.AvailableHooks = oldAvailableHooks
		Deps.IsHookInstalled = oldIsHookInstalled
	})
	listWriter := httptest.NewRecorder()
	listContext, _ := gin.CreateTestContext(listWriter)
	listContext.Request = httptest.NewRequest("GET", "/config/hooks", nil)
	HandleConfigHooksList(listContext)
	var listed []map[string]any
	if err := json.Unmarshal(listWriter.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode hook list: %v", err)
	}
	for _, item := range listed {
		switch item["id"] {
		case "dsh":
			if _, ok := item["config_format"]; ok {
				t.Fatalf("dsh wrapper unexpectedly reported config format: %#v", item)
			}
		case "pi":
			if item["config_format"] != string(core.ConfigFormatTypeScript) {
				t.Fatalf("Pi config format = %#v", item["config_format"])
			}
		}
	}

	type rawResponse struct {
		Content string `json:"content"`
		Format  string `json:"format"`
	}
	post := func(id, content string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]string{"content": content})
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: id}}
		c.Request = httptest.NewRequest("POST", "/config/hooks/"+id+"/raw", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		HandleConfigHooksRawPost(c)
		return w
	}
	get := func(id string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: id}}
		c.Request = httptest.NewRequest("GET", "/config/hooks/"+id+"/raw", nil)
		HandleConfigHooksRawGet(c)
		return w
	}

	typescript := "// agent-ebpf-hook-active\nexport default function register(pi: any) {}\n"
	if response := post("pi", typescript); response.Code != 200 {
		t.Fatalf("TypeScript raw POST status = %d, body = %s", response.Code, response.Body.String())
	}
	if content, err := os.ReadFile(filepath.Join(root, "pi.ts")); err != nil || string(content) != typescript {
		t.Fatalf("TypeScript source was not preserved: content=%q err=%v", content, err)
	}
	response := get("pi")
	if response.Code != 200 {
		t.Fatalf("TypeScript raw GET status = %d, body = %s", response.Code, response.Body.String())
	}
	var got rawResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode TypeScript raw GET: %v", err)
	}
	if got.Content != typescript || got.Format != core.ConfigFormatTypeScript {
		t.Fatalf("unexpected TypeScript raw GET: %#v", got)
	}

	if response := post("json-native", "{"); response.Code != 400 {
		t.Fatalf("invalid JSON status = %d, body = %s", response.Code, response.Body.String())
	}
	if response := post("json-native", `{"hooks":[]}`); response.Code != 200 {
		t.Fatalf("valid JSON status = %d, body = %s", response.Code, response.Body.String())
	}
	if response := post("toml-native", "x = ["); response.Code != 400 {
		t.Fatalf("invalid TOML status = %d, body = %s", response.Code, response.Body.String())
	}
	if response := post("toml-native", "x = \"y\"\n"); response.Code != 200 {
		t.Fatalf("valid TOML status = %d, body = %s", response.Code, response.Body.String())
	}

	body, err := io.ReadAll(get("json-native").Body)
	if err != nil || !bytes.Contains(body, []byte(`"format":"json"`)) {
		t.Fatalf("JSON raw GET did not report format: body=%s err=%v", body, err)
	}
}

func TestDshHookConfigurationUsesWrapperAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shellConfig := filepath.Join(t.TempDir(), ".bashrc")
	oldAvailableHooks := Deps.AvailableHooks
	oldGetShellConfigPath := Deps.GetShellConfigPath
	Deps.AvailableHooks = func() []core.HookDef {
		return []core.HookDef{{
			ID:        "dsh",
			Name:      "DeepSeek Harness",
			TargetCmd: "dsh",
			HookType:  core.HookTypeWrapper,
		}}
	}
	Deps.GetShellConfigPath = func() string { return shellConfig }
	t.Cleanup(func() {
		Deps.AvailableHooks = oldAvailableHooks
		Deps.GetShellConfigPath = oldGetShellConfigPath
	})

	request := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/config/hooks", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		HandleConfigHooksInstall(c)
		return w
	}

	if response := request(`{"id":"dsh","install":true}`); response.Code != 200 {
		t.Fatalf("dsh install status = %d, body = %s", response.Code, response.Body.String())
	}
	content, err := os.ReadFile(shellConfig)
	if err != nil {
		t.Fatalf("read dsh shell config: %v", err)
	}
	if !bytes.Contains(content, []byte("alias dsh='agent-wrapper dsh' # agent-ebpf-hook")) {
		t.Fatalf("dsh alias missing from shell config: %s", content)
	}
	if response := request(`{"id":"dsh","install":false}`); response.Code != 200 {
		t.Fatalf("dsh uninstall status = %d, body = %s", response.Code, response.Body.String())
	}
	content, err = os.ReadFile(shellConfig)
	if err != nil {
		t.Fatalf("read dsh shell config after uninstall: %v", err)
	}
	if bytes.Contains(content, []byte("alias dsh=")) {
		t.Fatalf("dsh alias remained after uninstall: %s", content)
	}
	rawWriter := httptest.NewRecorder()
	rawContext, _ := gin.CreateTestContext(rawWriter)
	rawContext.Params = gin.Params{{Key: "id", Value: "dsh"}}
	rawContext.Request = httptest.NewRequest("GET", "/config/hooks/dsh/raw", nil)
	HandleConfigHooksRawGet(rawContext)
	if rawWriter.Code != 404 {
		t.Fatalf("dsh raw config status = %d, body = %s", rawWriter.Code, rawWriter.Body.String())
	}
}
