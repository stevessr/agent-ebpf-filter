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
	if tomlString(doc, "default_permissions") != "" &&
		(tomlString(doc, "sandbox_mode") != "" || doc["sandbox_workspace_write"] != nil) {
		detail = append(detail, "权限模板与旧沙箱键同时声明（需核查）")
	}
	if profiles := tomlTable(doc["permissions"]); len(profiles) > 0 {
		detail = append(detail, fmt.Sprintf("命名权限模板 %d", len(profiles)))
	}
	if project && (doc["model_provider"] != nil || doc["model_providers"] != nil ||
		doc["openai_base_url"] != nil || doc["profile"] != nil || doc["profiles"] != nil ||
		doc["chatgpt_base_url"] != nil || doc["otel"] != nil || doc["notify"] != nil ||
		doc["apps_mcp_product_sku"] != nil || doc["experimental_realtime_ws_base_url"] != nil) {
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

func codexRequirementSummary(doc map[string]any) string {
	var parts []string
	for _, item := range []struct{key,label string}{
		{"allowed_approval_policies","允许审批策略"},
		{"allowed_sandbox_modes","允许沙箱模式"},
	} {
		if _, declared := doc[item.key]; declared {
			parts = append(parts, fmt.Sprintf("%s %d 项",item.label,configArrayCount(doc[item.key])))
		}
	}
	if n := len(tomlTable(doc["allowed_permission_profiles"])); n > 0 {
		parts = append(parts,fmt.Sprintf("权限模板约束 %d 项",n))
	}
	if _,ok := doc["default_permissions"].(string);ok {
		parts = append(parts,"强制默认权限模板")
	}
	if v,ok := configBool(doc,"allow_managed_hooks_only");ok && v {
		parts = append(parts,"仅允许受管 Hooks")
	}
	experimental := tomlTable(doc["experimental_network"])
	if experimental != nil {
		if n := configArrayCount(experimental["allowed_domains"]); n > 0 {
			parts = append(parts,fmt.Sprintf("网络允许域名规则 %d 项",n))
		}
		if n := configArrayCount(experimental["denied_domains"]); n > 0 {
			parts = append(parts,fmt.Sprintf("网络拒绝域名规则 %d 项",n))
		}
		if n := len(tomlTable(experimental["domains"])); n > 0 {
			parts = append(parts,fmt.Sprintf("网络域名决策规则 %d 项",n))
		}
	}
	if len(parts)==0 { return "检测到 requirements.toml，未识别可展示的本地约束" }
	return strings.Join(parts," · ")
}

func claudeManagedSummary(doc map[string]any) string {
	parts:=[]string{claudeSafetySummary(doc)}
	for _,key:=range []string{"allowManagedHooksOnly","allowManagedMcpServersOnly","allowManagedPermissionRulesOnly"} {
		if v,ok:=configBool(doc,key);ok&&v{
			switch key{
			case "allowManagedHooksOnly":parts=append(parts,"仅受管 Hooks")
			case "allowManagedMcpServersOnly":parts=append(parts,"仅受管 MCP")
			case "allowManagedPermissionRulesOnly":parts=append(parts,"仅受管权限规则")
			}
		}
	}
	return strings.Join(parts," · ")
}

// Explain on-disk policy without falsely claiming the merged policy is the
// runtime effective policy (the cloud or MDM may enforce higher requirements).
func inspectSystemAgentPolicy(claudeDir, codexDir string) agentConfigInspection {
	var out agentConfigInspection
	// A requirements file in CODEX_HOME is not an administrator-enforced
	// requirements layer. Never call it authoritative.
	if codexDir != "" {
		if _, exists, err := safeTOMLConfig(filepath.Join(codexDir, "requirements.toml")); exists && err == nil {
			out.Notes = append(out.Notes, "Codex · 用户目录 requirements.toml 并非官方系统强制层，未将其作为安全约束")
		}
	}
	if os.PathSeparator != '/' { return out }

	if doc, exists, err := safeTOMLConfig("/etc/codex/config.toml"); exists {
		if err != nil {
			out.Notes=append(out.Notes,"Codex · 系统默认配置无法安全读取")
		} else {
			out.Candidates=append(out.Candidates,agentConfigCandidate{
				Agent:"Codex",Scope:"系统默认（非强制）",Provider:"安全策略",
				Model:configLabel(tomlString(doc,"model")),Source:"/etc/codex/config.toml",
				Security:codexSafetySummary(doc,false),
			})
		}
	}
	if doc,exists,err:=safeTOMLConfig("/etc/codex/requirements.toml");exists {
		if err!=nil {
			out.Notes=append(out.Notes,"Codex · 系统强制配置无法安全读取")
		} else {
			out.Candidates=append(out.Candidates,agentConfigCandidate{
				Agent:"Codex",Scope:"系统受管候选",Provider:"管理员约束",Source:"/etc/codex/requirements.toml",
				Security:codexRequirementSummary(doc),
			})
		}
	}
	// The managed-settings.json file is the local Linux source. The desktop
	// cannot observe managed policy downloaded from an authenticated session.
	addManaged := func(path,label string) {
		doc,exists,err:=safeJSONConfig(path)
		if !exists { return }
		if err!=nil {out.Notes=append(out.Notes,"Claude Code · "+label+"无法安全读取");return}
		out.Candidates=append(out.Candidates,agentConfigCandidate{
			Agent:"Claude Code",Scope:"系统受管候选",Provider:"管理员设置",
			Source:label,Security:claudeManagedSummary(doc),
		})
	}
	addManaged("/etc/claude-code/managed-settings.json","managed-settings.json")
	// Official Linux managed drop-ins are merged alphabetically. Show only a
	// bounded inventory of sanitized rules, not a guessed effective merge.
	files,_:=filepath.Glob("/etc/claude-code/managed-settings.d/*.json")
	if len(files)>32 { files=files[:32];out.Notes=append(out.Notes,"Claude Code · 受管配置分片超过审计上限 32")}
	for _,path:=range files {addManaged(path,"managed-settings.d / "+filepath.Base(path))}
	if len(out.Candidates)!=0 {
		out.Notes=append(out.Notes,"本机受管设置仅是已发现的磁盘来源；云端下发、MDM 与客户端运行版本仍可能改变强制规则")
	}
	return out
}
