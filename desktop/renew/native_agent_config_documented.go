package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// This file contains deliberately bounded, documentation-driven projections.
// It is an offline CONFIG AUDIT, not a runtime effective-settings emulator.
// Especially: project trust, CLI -c/--profile, managed/cloud policies and
// process environment cannot be inferred from desktop configuration files.

func configBool(table map[string]any, key string) (bool, bool) {
	value, exists := table[key]
	v, ok := value.(bool)
	return v, exists && ok
}

func jsonConfigTable(doc map[string]any, key string) map[string]any {
	value, _ := doc[key].(map[string]any)
	return value
}

func configText(table map[string]any, key string) string {
	value, _ := table[key].(string)
	return value
}

func configArrayCount(value any) int {
	items, _ := value.([]any)
	return len(items)
}

func safeJSONConfig(path string) (map[string]any, bool, error) {
	data, exists, err := safeConfigFile(path)
	if err != nil || !exists {
		return nil, exists, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil || doc == nil {
		return nil, true, errors.New("JSON 配置无效")
	}
	return doc, true, nil
}

func safeTOMLConfig(path string) (map[string]any, bool, error) {
	data, exists, err := safeConfigFile(path)
	if err != nil || !exists {
		return nil, exists, err
	}
	var doc map[string]any
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, true, errors.New("TOML 配置无效")
	}
	return doc, true, nil
}

func safeSourceHost(raw string) (string, string) {
	// Never interpolate env templates or derive endpoints from command lines.
	if strings.Contains(raw, "$") || strings.Contains(raw, "{") {
		return "", ""
	}
	return safeConfiguredHost(raw)
}

func configMCPRows(agent, scope, filename string, servers map[string]any) []agentConfigCandidate {
	names := make([]string, 0, len(servers))
	for name := range servers {
		if configLabel(name) != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) > 64 {
		names = names[:64]
	}
	var rows []agentConfigCandidate
	for _, name := range names {
		entry := tomlTable(servers[name])
		if entry == nil {
			continue
		}
		// For stdio servers, "command" and "args" are never interpreted
		// as DNS or URL evidence. Only explicit URL fields count.
		host, kind := safeSourceHost(configText(entry, "url"))
		status := "HTTP MCP · 声明，未验证连接"
		if disabled, ok := configBool(entry, "enabled"); ok && !disabled {
			status = "已禁用的 MCP 声明"
		}
		if disabled, ok := configBool(entry, "disabled"); ok && disabled {
			status = "已禁用的 MCP 声明"
		}
		if host == "" {
			if configText(entry, "command") != "" {
				status = "stdio MCP（无法推断外联域名）"
			} else {
				status = "MCP 未声明有效的 HTTP URL"
			}
		}
		rows = append(rows, agentConfigCandidate{
			Agent: agent, Scope: scope, Provider: "MCP · " + name,
			Host: host, HostKind: kind, Source: filename,
			Security: status,
		})
	}
	return rows
}

func readClaudeMCP(path, scope string) ([]agentConfigCandidate, bool, error) {
	doc, exists, err := safeJSONConfig(path)
	if err != nil || !exists {
		return nil, exists, err
	}
	servers := jsonConfigTable(doc, "mcpServers")
	rows := configMCPRows("Claude Code", scope, filepath.Base(path)+" · MCP", servers)
	return rows, true, nil
}

func codexAdditionalRows(doc map[string]any, scope, filename string, project bool) []agentConfigCandidate {
	// MCP is valid in trusted project configs. Never read bearer headers,
	// env vars, executable commands, or auth token references.
	var rows []agentConfigCandidate
	rows = append(rows, configMCPRows("Codex", scope, filename+" · MCP", tomlTable(doc["mcp_servers"]))...)
	if project {
		return rows
	}
	// Extra provider URL candidates are already covered in codexCandidate.
	// Preview HTTP proxy endpoints only from explicit documented URL keys.
	for _, key := range []string{"experimental_realtime_ws_base_url"} {
		host, kind := safeSourceHost(configText(doc, key))
		if host != "" {
			rows = append(rows, agentConfigCandidate{
				Agent: "Codex", Scope: scope, Provider: "其他 API 传输",
				Host: host, HostKind: kind, Source: filename+" · "+key,
				Security: "附加服务端点，仅为配置候选",
			})
		}
	}
	return rows
}

func claudeSafetySummary(doc map[string]any) string {
	var detail []string
	perms := jsonConfigTable(doc, "permissions")
	switch configText(perms, "defaultMode") {
	case "bypassPermissions":
		detail = append(detail, "权限绕过（需核查）")
	case "acceptEdits":
		detail = append(detail, "编辑自动批准")
	case "dontAsk":
		detail = append(detail, "拒绝未经允许的授权请求")
	case "plan":
		detail = append(detail, "计划模式")
	case "default":
		detail = append(detail, "默认审批")
	}
	if configText(perms, "disableBypassPermissionsMode") == "disable" {
		detail = append(detail, "禁止权限绕过")
	}
	for _, rule := range []string{"deny", "ask", "allow"} {
		if n := configArrayCount(perms[rule]); n > 0 {
			detail = append(detail, fmt.Sprintf("%s 规则 %d", rule, n))
		}
	}
	sandbox := jsonConfigTable(doc, "sandbox")
	if enabled, ok := configBool(sandbox, "enabled"); ok {
		if enabled {
			detail = append(detail, "Bash 沙箱已声明开启")
		} else {
			detail = append(detail, "Bash 沙箱已声明关闭")
		}
	}
	if strict, ok := configBool(sandbox, "failIfUnavailable"); ok && strict {
		detail = append(detail, "沙箱不可用则拒绝启动")
	}
	if disabled, ok := configBool(jsonConfigTable(sandbox, "filesystem"), "disabled"); ok && disabled {
		detail = append(detail, "文件系统隔离关闭（需核查）")
	}
	if escape, ok := configBool(sandbox, "allowUnsandboxedCommands"); ok && !escape {
		detail = append(detail, "禁止命令逃逸沙箱")
	}
	if n := configArrayCount(sandbox["excludedCommands"]); n > 0 {
		detail = append(detail, fmt.Sprintf("沙箱排除命令 %d", n))
	}
	if len(detail) == 0 {
		return "未声明可审计安全模式"
	}
	return strings.Join(detail, " · ")
}

func safePermissionProfileLabel(raw string) string {
	switch raw {
	case ":read-only", ":workspace", ":danger-full-access":
		return raw
	}
	return configLabel(raw)
}

func codexSafetySummary(doc map[string]any, project bool) string {
	detail := []string{}
	if v := safePermissionProfileLabel(tomlString(doc, "default_permissions")); v != "" {
		detail = append(detail, "权限配置 "+v)
	}
	if mode := tomlString(doc, "sandbox_mode"); mode != "" {
		switch mode {
		case "danger-full-access":
			detail = append(detail, "无沙箱（需核查）")
		case "workspace-write":
			detail = append(detail, "工作区写入")
		case "read-only":
			detail = append(detail, "只读沙箱")
		default:
			detail = append(detail, "未识别的沙箱模式")
		}
	}
	switch tomlString(doc, "approval_policy") {
	case "never":
		detail = append(detail, "不请求审批（需核查）")
	case "on-request":
		detail = append(detail, "按需审批")
	case "untrusted":
		detail = append(detail, "untrusted 已弃用（需迁移）")
	}
	if workspace := tomlTable(doc["sandbox_workspace_write"]); workspace != nil {
		if enabled, ok := configBool(workspace, "network_access"); ok {
			if enabled {
				detail = append(detail, "工作区沙箱网络访问声明允许")
			} else {
				detail = append(detail, "工作区沙箱网络访问声明禁止")
			}
		}
	}
	if profiles := tomlTable(doc["permissions"]); len(profiles) > 0 {
		detail = append(detail, fmt.Sprintf("命名权限模板 %d", len(profiles)))
	}
	if project && (doc["model_provider"] != nil || doc["model_providers"] != nil ||
		doc["openai_base_url"] != nil || doc["profile"] != nil || doc["profiles"] != nil ||
		doc["chatgpt_base_url"] != nil || doc["otel"] != nil || doc["notify"] != nil) {
		detail = append(detail, "项目中含无效的机器级覆盖键（已忽略）")
	}
	if len(detail) == 0 {
		return "未声明可审计安全模式"
	}
	return strings.Join(detail, " · ")
}

// The supplied directory is the session's intended working directory.
// Traverse only within a detected VCS root (or the selected directory when
// no root marker is visible); never scan neighboring projects or descendants.
func codexProjectLayers(project string) []string {
	if !filepath.IsAbs(project) {
		return nil
	}
	project = filepath.Clean(project)
	info, err := os.Stat(project)
	if err != nil || !info.IsDir() {
		return nil
	}
	root := project
	for cur, depth := project, 0; depth < 24; depth++ {
		if _, err := os.Lstat(filepath.Join(cur, ".git")); err == nil {
			root = cur
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	var ascending []string
	for cur := project; ; cur = filepath.Dir(cur) {
		ascending = append(ascending, filepath.Join(cur, ".codex", "config.toml"))
		if cur == root {
			break
		}
	}
	for i, j := 0, len(ascending)-1; i < j; i, j = i+1, j-1 {
		ascending[i], ascending[j] = ascending[j], ascending[i]
	}
	return ascending
}

func inspectAgentConfigExtensions(claudeDir, codexDir, project string) agentConfigInspection {
	var out agentConfigInspection
	addClaude := func(path, scope string) {
		doc, exists, err := safeJSONConfig(path)
		if err != nil {
			out.Notes = append(out.Notes, "Claude Code · "+scope+"：文件读取或解析失败")
			return
		}
		if !exists {
			return
		}
		// The summary doesn't include credential-bearing permission rule text.
		out.Candidates = append(out.Candidates, agentConfigCandidate{
			Agent: "Claude Code", Scope: scope, Provider: "权限与沙箱",
			Source: filepath.Base(path), Security: claudeSafetySummary(doc),
		})
	}
	addCodex := func(path, scope string, projectLayer bool) {
		doc, exists, err := safeTOMLConfig(path)
		if err != nil {
			out.Notes = append(out.Notes, "Codex · "+scope+"：文件读取或解析失败")
			return
		}
		if !exists {
			return
		}
		out.Candidates = append(out.Candidates, agentConfigCandidate{
			Agent: "Codex", Scope: scope, Provider: "安全策略",
			Source: filepath.Base(path), Security: codexSafetySummary(doc, projectLayer),
		})
		out.Candidates = append(out.Candidates, codexAdditionalRows(doc, scope, filepath.Base(path), projectLayer)...)
	}
	if claudeDir != "" {
		addClaude(filepath.Join(claudeDir, "settings.json"), "用户")
	}
	if codexDir != "" {
		addCodex(filepath.Join(codexDir, "config.toml"), "用户", false)
	}
	if project != "" && filepath.IsAbs(project) {
		addClaude(filepath.Join(project, ".claude", "settings.json"), "项目")
		addClaude(filepath.Join(project, ".claude", "settings.local.json"), "项目本地")
		for _, path := range codexProjectLayers(project) {
			// The original inspector displays the selected project's main row.
			// Additional layers here show security and MCP configurations.
			label := "项目配置（信任未核实）"
			if filepath.Dir(filepath.Dir(path)) != filepath.Clean(project) {
				label = "父目录项目配置（信任未核实）"
			}
			addCodex(path, label, true)
		}
		// Only explicit project MCP configs; Claude's ~/.claude.json also
		// contains session credentials and must not be opened for this audit.
		rows, exists, err := readClaudeMCP(filepath.Join(project, ".mcp.json"), "项目")
		if err != nil {
			out.Notes = append(out.Notes, "Claude Code · 项目 MCP：读取或解析失败")
		} else if exists {
			out.Candidates = append(out.Candidates, rows...)
		}
	}
	return out
}

// Explain on-disk policy without falsely claiming the merged policy is the
// runtime effective policy (the cloud or MDM may enforce higher requirements).
func inspectSystemAgentPolicy(claudeDir, codexDir string) agentConfigInspection {
	var out agentConfigInspection
	if codexDir != "" {
		if doc, exists, err := safeTOMLConfig(filepath.Join(codexDir, "requirements.toml")); exists && err == nil {
			_ = doc // file presence is not equivalent to an enforced requirement
			out.Notes = append(out.Notes, "检测到用户目录 requirements.toml：非系统强制策略，不能据此确认沙箱或审批约束")
		}
	}
	// Limit direct system inspection to static, documented Unix locations.
	if os.PathSeparator != '/' {
		return out
	}
	for _, config := range []struct{ path, agent, label string }{
		{"/etc/codex/config.toml", "Codex", "系统默认配置（非强制）"},
		{"/etc/codex/requirements.toml", "Codex", "系统强制要求（本地候选）"},
		{"/etc/claude-code/managed-settings.json", "Claude Code", "系统受管设置（本地候选）"},
	} {
		_, exists, err := safeConfigFile(config.path)
		if err != nil {
			out.Notes = append(out.Notes, config.agent+" · "+config.label+"：无法安全读取")
		} else if exists {
			out.Notes = append(out.Notes, config.agent+" · 已检测到"+config.label+"；运行时还可能有云端或 MDM 层")
		}
	}
	return out
}
