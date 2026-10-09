package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// A static precedence PREVIEW of explicitly documented scalar keys. It never
// claims to reproduce a running Claude/Codex instance: CLI switches, project
// trust, environment injection, per-field merges, cloud and MDM requirements
// remain external inputs. Only safe scalar keys enter this overlay.
type previewScalar struct {
	Value any
	Scope string
}

func previewSet(dest map[string]previewScalar, source map[string]any, scope string, keys ...string) {
	for _, key := range keys {
		if value, ok := source[key]; ok {
			switch value.(type) {
			case string, bool:
				dest[key] = previewScalar{value, scope}
			}
		}
	}
}

func codexLocalTrust(doc map[string]any, project string) string {
	if project == "" || !filepath.IsAbs(project) {
		return ""
	}
	for cur, depth := filepath.Clean(project), 0; depth < 24; depth++ {
		if p := tomlTable(tomlTable(doc["projects"])[cur]); p != nil {
			switch tomlString(p, "trust_level") {
			case "trusted":
				return "trusted"
			case "untrusted":
				return "untrusted"
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return ""
}

// Most individual keys in config.toml overlay; provider and profile selectors
// are machine-local and must never be drawn from project layers.
func codexStaticConfigPreview(codexDir, project string) (agentConfigCandidate, []string, bool) {
	if codexDir == "" {
		return agentConfigCandidate{}, nil, false
	}
	user, _, err := safeTOMLConfig(filepath.Join(codexDir, "config.toml"))
	if err != nil {
		return agentConfigCandidate{}, []string{"Codex · 无法构建层级预览：用户配置格式错误"}, false
	}
	if user == nil {
		user = map[string]any{}
	}
	overlay := map[string]previewScalar{}
	// /etc and cloud defaults intentionally not included here. If user
	// config doesn't declare a scalar, never substitute a guessed value.
	previewSet(overlay, user, "用户", "model", "sandbox_mode", "approval_policy", "default_permissions")
	workspace := tomlTable(user["sandbox_workspace_write"])
	if b, ok := configBool(workspace, "network_access"); ok {
		overlay["network_access"] = previewScalar{b, "用户"}
	}

	// A top-level "profile" is an indication of a possible choice, not
	// proof of an active --profile flag. Inline [profiles.*] can also be
	// overridden by dedicated profile files, each treated as a preview.
	selected := strings.TrimSpace(tomlString(user, "profile"))
	profile := map[string]any{}
	if selected != "." && selected != ".." && safeCodexProfilePattern.MatchString(selected) {
		profile = tomlTable(tomlTable(user["profiles"])[selected])
		if cfg, exists, e := safeTOMLConfig(filepath.Join(codexDir, selected+".config.toml")); exists && e == nil {
			merged := make(map[string]any, len(profile)+len(cfg))
			for k, v := range profile { merged[k] = v }
			for k, v := range cfg { merged[k] = v }
			profile = merged
		}
		previewSet(overlay, profile, "Profile 候选", "model", "sandbox_mode", "approval_policy", "default_permissions")
		if b, ok := configBool(tomlTable(profile["sandbox_workspace_write"]), "network_access"); ok {
			overlay["network_access"] = previewScalar{b, "Profile 候选"}
		}
	}
	trust := codexLocalTrust(user, project)
	var notes []string
	if trust == "untrusted" {
		notes = append(notes, "Codex · 用户配置将该项目标记为 untrusted：层级预览跳过项目覆盖；实际 CLI/项目范围仍需核验")
	}
	if trust == "" && project != "" && filepath.IsAbs(project) {
		notes = append(notes, "Codex · 无法从用户配置确定项目信任状态；下面的合并结果仅为“假设受信任”预览")
	}
	if trust != "untrusted" {
		for _, path := range codexProjectLayers(project) {
			doc, exists, err := safeTOMLConfig(path)
			if !exists || err != nil { continue }
			previewSet(overlay, doc, "受信任项目候选", "model", "sandbox_mode", "approval_policy", "default_permissions")
			if b, ok := configBool(tomlTable(doc["sandbox_workspace_write"]), "network_access"); ok {
				overlay["network_access"] = previewScalar{b, "受信任项目候选"}
			}
		}
	}
	provider := configLabel(tomlString(user, "model_provider"))
	// Project model_provider and model_providers deliberately ignored.
	if value := configLabel(tomlString(profile, "model_provider")); value != "" {
		provider = value
	}
	if provider == "" && len(user) > 0 {
		provider = "openai（内建默认）"
	}
	host, kind := "", ""
	id := strings.TrimSuffix(provider, "（内建默认）")
	if id == "openai" {
		host, kind = safeConfiguredHost(tomlString(user, "openai_base_url"))
	}
	if value := tomlString(tomlTable(tomlTable(user["model_providers"])[id]), "base_url"); value != "" {
		host, kind = safeConfiguredHost(value)
	}
	keys := make(map[string]any)
	for _, name := range []string{"sandbox_mode", "approval_policy", "default_permissions"} {
		if v, ok := overlay[name]; ok { keys[name] = v.Value }
	}
	if v, ok := overlay["network_access"]; ok {
		keys["sandbox_workspace_write"] = map[string]any{"network_access": v.Value}
	}
	model := ""
	if v, ok := overlay["model"]; ok { model = configLabel(fmt.Sprint(v.Value)) }
	if len(overlay) == 0 && provider == "" { return agentConfigCandidate{}, notes, false }
	var origins []string
	for _, k := range []string{"model","sandbox_mode","approval_policy","default_permissions","network_access"} {
		if v, ok := overlay[k]; ok { origins = append(origins, k+" ← "+v.Scope) }
	}
	return agentConfigCandidate{
		Agent: "Codex", Scope: "静态合并预览（非运行时）",
		Provider: provider, Model: model, Host: host, HostKind: kind,
		Source: "按字段优先级：" + strings.Join(origins, "；"),
		Security: codexSafetySummary(keys, false),
	}, notes, true
}

func claudeStaticConfigPreview(claudeDir, project string) (agentConfigCandidate, bool) {
	if claudeDir == "" { return agentConfigCandidate{}, false }
	paths := []struct{Path,Scope string}{{filepath.Join(claudeDir,"settings.json"),"用户"}}
	if projectRoot := projectConfigRoot(project); projectRoot != "" {
		project = projectRoot
		paths = append(paths,
			struct{Path,Scope string}{filepath.Join(project,".claude","settings.json"),"项目"},
			struct{Path,Scope string}{filepath.Join(project,".claude","settings.local.json"),"本地"})
	}
	var model, mode, provider, host, kind string
	var origins []string
	var declared bool
	for _, layer := range paths {
		doc, exists, err := safeJSONConfig(layer.Path)
		if !exists || err != nil { continue }
		declared = true
		if v := configLabel(configText(doc,"model")); v!="" {
			model = v
			origins = append(origins, "model ← "+layer.Scope)
		}
		if v := configLabel(configText(jsonConfigTable(doc,"env"),"ANTHROPIC_MODEL")); v!="" {
			model=v
			origins = append(origins,"ANTHROPIC_MODEL ← "+layer.Scope)
		}
		if v := configText(jsonConfigTable(doc,"permissions"),"defaultMode"); v!="" {
			if v != "auto" || layer.Scope == "用户" {
				mode = v
				origins = append(origins,"defaultMode ← "+layer.Scope)
			}
		}
		env := jsonConfigTable(doc,"env")
		for _, flag := range []struct{Key,Label string}{{"CLAUDE_CODE_USE_BEDROCK","AWS Bedrock"},{"CLAUDE_CODE_USE_VERTEX","Google Vertex AI"},{"CLAUDE_CODE_USE_FOUNDRY","Microsoft Foundry"}} {
			if enabledClaudeFlag(configText(env,flag.Key)) {
				provider = flag.Label
			}
		}
		// Treat layer-specific URL and mode as independent candidate
		// fields, not a verified effective endpoint from a running process.
		for _, key := range []string{"ANTHROPIC_BASE_URL","ANTHROPIC_VERTEX_BASE_URL","ANTHROPIC_FOUNDRY_BASE_URL","ANTHROPIC_AWS_BASE_URL"} {
			if val := configText(env,key); val!="" {
				host,kind = safeConfiguredHost(val)
				if host!="" { origins=append(origins,"API Host ← "+layer.Scope) }
				break
			}
		}
	}
	if !declared { return agentConfigCandidate{},false }
	return agentConfigCandidate{
		Agent:"Claude Code",Scope:"静态合并预览（非运行时）",
		Model:model,Provider:displayOr(provider,"Anthropic / 未验证"),
		Host:host,HostKind:kind,
		Source:"按字段优先级："+strings.Join(origins,"；"),
		Security:claudeConfigSecurity(mode,"静态预览"),
	},true
}
