package app

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"

	"agent-ebpf-filter/app/platform"
)

//go:embed hookassets/dsh.mjs
var dshPluginSource string

const dshPatchStart = "# BEGIN agent-ebpf-hook-active dsh\n"
const dshPatchEnd = "# END agent-ebpf-hook-active dsh\n"

func resolveDshHome(home string) string {
	p := strings.TrimSpace(os.Getenv("DSH_HOME"))
	if p == "" {
		return filepath.Join(home, ".dsh")
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	p, _ = filepath.Abs(p)
	return p
}

func buildDshPlugin(h HookDef) string {
	path, _ := json.Marshal(hookRelayScriptPath(h))
	return strings.ReplaceAll(dshPluginSource, "__RELAY_PATH__", string(path))
}

// Preserve the user's YAML and comments byte-for-byte outside our marked block.
// Reject formats we cannot safely append to, rather than rewriting user profiles.
func dshPatch(data string, h HookDef, install bool) (string, error) {
	start, end := strings.Index(data, dshPatchStart), strings.Index(data, dshPatchEnd)
	if start >= 0 || end >= 0 {
		if start < 0 || end < start || strings.Count(data, dshPatchStart) != 1 || strings.Count(data, dshPatchEnd) != 1 {
			return "", fmt.Errorf("invalid managed DeepSeek Harness patch markers")
		}
		data = data[:start] + data[end+len(dshPatchEnd):]
	}
	if !install {
		return data, nil
	}
	var rows []map[string]interface{}
	if err := yaml.Unmarshal([]byte(data), &rows); err != nil {
		return "", fmt.Errorf("read DeepSeek Harness patch: %w", err)
	}
	// A colliding unmanaged row must not be silently replaced by our insert.
	if strings.Contains(data, hookMarker) {
		return "", fmt.Errorf("unmanaged DeepSeek Harness patch references %s", hookMarker)
	}
	if strings.TrimSpace(data) == "[]" {
		data = ""
	}
	path, _ := json.Marshal(h.NativeConfigPath)
	block := dshPatchStart + "- insert:\n    - id: agent-ebpf-hook-active\n      name: " + string(path) + "\n" + dshPatchEnd
	if data != "" && !strings.HasSuffix(data, "\n") {
		data += "\n"
	}
	result := data + block
	var parsed []map[string]interface{}
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		return "", fmt.Errorf("cannot safely append plugin patch: %w", err)
	}
	if len(parsed) != len(rows)+1 {
		return "", fmt.Errorf("cannot safely append to this patch format; use a YAML block sequence")
	}
	return result, nil
}

func installDshNativeHook(h HookDef) error {
	data, err := os.ReadFile(h.NativeFeatureConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	patch, err := dshPatch(string(data), h, true)
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(h.NativeConfigPath); err == nil && !strings.Contains(string(existing), hookMarker) {
		return fmt.Errorf("refusing to overwrite unmanaged dsh plugin %s", h.NativeConfigPath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := platform.MkdirAllAsRealUser(filepath.Dir(h.NativeConfigPath), 0755); err != nil {
		return err
	}
	if err := platform.MkdirAllAsRealUser(hookRelayScriptDir(h), 0700); err != nil {
		return err
	}
	// Dedicated relay: bounded network time and owner-only authentication material.
	relay := strings.Replace(buildGenericHookRelayScript(h), "curl -fsS", "curl --connect-timeout 1 --max-time 2 -fsS", 1)
	if err := platform.WriteFileAsRealUser(hookRelayScriptPath(h), []byte(relay), 0700); err != nil {
		return err
	}
	if err := os.Chmod(hookRelayScriptPath(h), 0700); err != nil {
		return err
	}
	if err := platform.WriteFileAsRealUser(h.NativeConfigPath, []byte(buildDshPlugin(h)), 0644); err != nil {
		return err
	}
	return platform.WriteFileAsRealUser(h.NativeFeatureConfigPath, []byte(patch), 0644)
}

func uninstallDshNativeHook(h HookDef) error {
	data, err := os.ReadFile(h.NativeFeatureConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		patch, err := dshPatch(string(data), h, false)
		if err != nil {
			return err
		}
		if err := platform.WriteFileAsRealUser(h.NativeFeatureConfigPath, []byte(patch), 0644); err != nil {
			return err
		}
	}
	return uninstallTypeScriptNativeHook(h)
}
