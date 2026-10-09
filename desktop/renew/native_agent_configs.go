package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// This is a deliberately small, read-only projection of local Agent configs.
// Never keep the raw settings maps, authentication values, command arguments,
// custom headers, MCP server env, or full URLs in desktop app state.
type agentConfigCandidate struct {
	Agent    string
	Scope    string
	Provider string
	Model    string
	Host     string
	HostKind string
	Source   string
}

type agentConfigInspection struct {
	Candidates []agentConfigCandidate
	Notes      []string
}

const maxAgentConfigBytes = 512 * 1024

var safeAgentLabelPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:/+-]{0,99}$`)

func configLabel(value string) string {
	value = strings.TrimSpace(value)
	if safeAgentLabelPattern.MatchString(value) {
		return value
	}
	return "" // Unknown/custom strings can contain tokens; do not render them.
}

func safeConfiguredHost(raw string) (host, kind string) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil ||
		!strings.EqualFold(u.Scheme, "https") && !strings.EqualFold(u.Scheme, "http") {
		return "", ""
	}
	host = u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return "localhost", "本地"
	}
	host, kind = normalizedDestinationHost(host)
	if kind == "域名" || kind == "IP" {
		return host, kind
	}
	return "", ""
}

func safeConfigFile(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.New("无法访问配置文件")
	}
	// Refuse symlinks/special files; never block on a FIFO or cross into a
	// sensitive redirected target. The UI intentionally hides raw paths.
	if !info.Mode().IsRegular() || info.Size() > maxAgentConfigBytes {
		return nil, true, errors.New("配置不是普通文件或超过 512 KiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, true, errors.New("配置不可读取")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, true, errors.New("配置文件在检查过程中发生变化")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxAgentConfigBytes+1))
	if err != nil || len(data) > maxAgentConfigBytes {
		return nil, true, errors.New("配置读取失败或超过大小限制")
	}
	return data, true, nil
}

func readClaudeCandidate(path, scope string) (agentConfigCandidate, bool, error) {
	data, exists, err := safeConfigFile(path)
	if err != nil || !exists {
		return agentConfigCandidate{}, exists, err
	}
	// Decode strictly and avoid logging parse failures (they may contain secrets).
	var settings struct {
		Model string            `json:"model"`
		Env   map[string]string `json:"env"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return agentConfigCandidate{}, true, errors.New("JSON 格式不正确或字段类型不匹配")
	}
	c := agentConfigCandidate{
		Agent: "Claude Code", Scope: scope, Provider: "Anthropic / 未明确指定",
		Model: configLabel(settings.Model), Source: "settings.json",
	}
	if model := configLabel(settings.Env["ANTHROPIC_MODEL"]); model != "" {
		c.Model = model
		c.Source = "settings.json · env"
	}
	switch {
	case enabledClaudeFlag(settings.Env["CLAUDE_CODE_USE_BEDROCK"]):
		c.Provider = "AWS Bedrock"
	case enabledClaudeFlag(settings.Env["CLAUDE_CODE_USE_VERTEX"]):
		c.Provider = "Google Vertex AI"
	case enabledClaudeFlag(settings.Env["CLAUDE_CODE_USE_FOUNDRY"]):
		c.Provider = "Microsoft Foundry"
	}
	for _, variable := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_VERTEX_BASE_URL", "ANTHROPIC_FOUNDRY_BASE_URL"} {
		if value := settings.Env[variable]; value != "" {
			c.Host, c.HostKind = safeConfiguredHost(value)
			if c.Host != "" {
				c.Source = "settings.json · " + variable
				break
			}
		}
	}
	return c, true, nil
}

func enabledClaudeFlag(value string) bool {
	return value == "1" || strings.EqualFold(value, "true")
}

func tomlTable(value any) map[string]any {
	table, _ := value.(map[string]any)
	return table
}

func tomlString(table map[string]any, key string) string {
	value, _ := table[key].(string)
	return value
}

func codexCandidate(doc map[string]any, scope string, allowProvider bool) []agentConfigCandidate {
	model := configLabel(tomlString(doc, "model"))
	provider := configLabel(tomlString(doc, "model_provider"))
	if !allowProvider {
		// Codex rejects machine-local provider configuration from project files.
		return []agentConfigCandidate{{Agent: "Codex", Scope: scope, Model: model, Source: "config.toml · 项目模型候选"}}
	}
	if provider == "" {
		provider = "openai"
	}
	base := ""
	if provider == "openai" {
		base = tomlString(doc, "openai_base_url")
	}
	providers := tomlTable(doc["model_providers"])
	if detail := tomlTable(providers[provider]); len(detail) > 0 {
		base = tomlString(detail, "base_url")
	}
	host, kind := safeConfiguredHost(base)
	result := []agentConfigCandidate{{
		Agent: "Codex", Scope: scope, Provider: provider, Model: model,
		Host: host, HostKind: kind, Source: "config.toml · 当前声明",
	}}
	// Other configured providers are alternatives, not active endpoints.
	names := make([]string, 0, len(providers))
	for name := range providers {
		if name != provider && configLabel(name) != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		url := tomlString(tomlTable(providers[name]), "base_url")
		host, kind := safeConfiguredHost(url)
		result = append(result, agentConfigCandidate{
			Agent: "Codex", Scope: scope, Provider: name, Host: host,
			HostKind: kind, Source: "config.toml · 未选用的 Provider",
		})
	}
	// A configured [profiles.name] profile is a candidate, not an active
	// CLI selection. The active --profile and -c switches are not observable.
	profiles := tomlTable(doc["profiles"])
	selected := configLabel(tomlString(doc, "profile"))
	if selected != "" {
		profile := tomlTable(profiles[selected])
		if len(profile) > 0 {
			name := configLabel(tomlString(profile, "model_provider"))
			if name == "" {
				name = provider
			}
			alternateModel := configLabel(tomlString(profile, "model"))
			if alternateModel == "" {
				alternateModel = model
			}
			host, kind := "", ""
			if name == "openai" {
				host, kind = safeConfiguredHost(tomlString(doc, "openai_base_url"))
			}
			if cfg := tomlTable(providers[name]); len(cfg) > 0 {
				host, kind = safeConfiguredHost(tomlString(cfg, "base_url"))
			}
			result = append(result, agentConfigCandidate{
				Agent: "Codex", Scope: scope + " · profile " + selected,
				Provider: name, Model: alternateModel, Host: host, HostKind: kind,
				Source: "config.toml · 选中 Profile（可能被 CLI 覆盖）",
			})
		}
	}
	return result
}

func readCodexCandidates(path, scope string, allowProvider bool) ([]agentConfigCandidate, bool, error) {
	data, exists, err := safeConfigFile(path)
	if err != nil || !exists {
		return nil, exists, err
	}
	var doc map[string]any
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, true, errors.New("TOML 格式不正确")
	}
	return codexCandidate(doc, scope, allowProvider), true, nil
}

var safeCodexProfilePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)

func selectedCodexProfileFile(configPath string) string {
	data, exists, err := safeConfigFile(configPath)
	if !exists || err != nil {
		return ""
	}
	var doc map[string]any
	if toml.Unmarshal(data, &doc) != nil {
		return ""
	}
	profile := strings.TrimSpace(tomlString(doc, "profile"))
	if profile == "." || profile == ".." || !safeCodexProfilePattern.MatchString(profile) {
		return ""
	}
	return profile + ".config.toml"
}

func configHomeOverride(home, envKey, fallback string) (string, string) {
	if path := strings.TrimSpace(os.Getenv(envKey)); path != "" {
		if !filepath.IsAbs(path) {
			return "", envKey + " 不是绝对路径，已跳过"
		}
		return filepath.Clean(path), ""
	}
	return filepath.Join(home, fallback), ""
}

// inspectAgentConfigPaths is explicit and synchronous. The GUI invokes it on
// demand on a worker goroutine. No arbitrary project directories are scanned.
func inspectAgentConfigPaths(home, claudeDir, codexDir, project string) agentConfigInspection {
	var out agentConfigInspection
	addClaude := func(path, scope string) {
		c, exists, err := readClaudeCandidate(path, scope)
		if err != nil {
			out.Notes = append(out.Notes, "Claude Code · "+scope+"："+err.Error())
		} else if exists {
			out.Candidates = append(out.Candidates, c)
		}
	}
	addCodex := func(path, scope string, provider bool) {
		records, exists, err := readCodexCandidates(path, scope, provider)
		if err != nil {
			out.Notes = append(out.Notes, "Codex · "+scope+"："+err.Error())
		} else if exists {
			out.Candidates = append(out.Candidates, records...)
		}
	}
	if claudeDir != "" {
		addClaude(filepath.Join(claudeDir, "settings.json"), "用户")
	}
	if codexDir != "" {
		userConfig := filepath.Join(codexDir, "config.toml")
		addCodex(userConfig, "用户", true)
		if profileFile := selectedCodexProfileFile(userConfig); profileFile != "" {
			addCodex(filepath.Join(codexDir, profileFile), "用户 · 独立 Profile 文件", true)
		}
	}
	if strings.TrimSpace(project) != "" {
		if !filepath.IsAbs(project) {
			out.Notes = append(out.Notes, "项目路径请使用绝对路径")
		} else {
			project = filepath.Clean(project)
			addClaude(filepath.Join(project, ".claude", "settings.json"), "项目")
			addClaude(filepath.Join(project, ".claude", "settings.local.json"), "项目本地")
			addCodex(filepath.Join(project, ".codex", "config.toml"), "项目（仅模型，信任状态未核实）", false)
		}
	}
	if len(out.Candidates) == 0 && len(out.Notes) == 0 {
		out.Notes = append(out.Notes, "未找到可读取的用户级配置；可指定项目绝对路径补充检查")
	}
	return out
}

func inspectLocalAgentConfigs(project string) agentConfigInspection {
	home, err := os.UserHomeDir()
	if err != nil {
		return agentConfigInspection{Notes: []string{"无法确定桌面用户目录"}}
	}
	claudeDir, claudeErr := configHomeOverride(home, "CLAUDE_CONFIG_DIR", ".claude")
	codexDir, codexErr := configHomeOverride(home, "CODEX_HOME", ".codex")
	out := inspectAgentConfigPaths(home, claudeDir, codexDir, project)
	for _, message := range []string{claudeErr, codexErr} {
		if message != "" {
			out.Notes = append(out.Notes, message)
		}
	}
	return out
}

func countObservedConfigHost(agent, target string, events []eventSummary) int {
	return countObservedConfigHostWithIndex(agent, target, events, buildAgentOwnershipIndex(events, nil))
}

func countObservedConfigHostWithIndex(agent, target string, events []eventSummary, index agentOwnershipIndex) int {
	if target == "" || target == "localhost" {
		return 0
	}
	count := 0
	for _, event := range events {
		owner := index.attribution(event)
		if !owner.IsAgent || owner.OwnerLabel != agent {
			continue
		}
		host, kind := agentEventDestination(event)
		if kind == "域名" && host == target || kind == "IP" && host == target {
			count++
		}
	}
	return count
}

func formatConfigCandidate(c agentConfigCandidate) string {
	if c.Host != "" {
		return fmt.Sprintf("%s · %s", c.Host, c.HostKind)
	}
	return "未显式配置 URL"
}

