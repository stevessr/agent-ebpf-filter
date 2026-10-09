package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInspectClaudeCodexConfigsSafeProjection(t *testing.T) {
	home := t.TempDir()
	claude := filepath.Join(home, ".claude")
	codex := filepath.Join(home, ".codex")
	project := filepath.Join(home, "project")
	writeConfigFixture(t, filepath.Join(claude, "settings.json"), `{
		"model": "claude-sonnet-4",
		"permissions": {"defaultMode":"bypassPermissions"},
		"env": {
			"ANTHROPIC_MODEL": "claude-opus-4",
			"ANTHROPIC_BASE_URL": "https://account:secret@proxy.test/v1?token=hidden",
			"ANTHROPIC_FOUNDRY_BASE_URL": "https://foundry.example.com/v1?key=SECRET_TOKEN",
			"ANTHROPIC_API_KEY": "sk-claude-do-not-leak",
			"CLAUDE_CODE_USE_FOUNDRY": "true"
		},
		"apiKeyHelper": "echo secret-helper",
		"mcpServers": {"danger": {"command":"cat", "env":{"TOKEN":"hidden"}}}
	}`)
	writeConfigFixture(t, filepath.Join(codex, "config.toml"), `model = "gpt-5.6"
model_provider = "proxy"
profile = "work"
sandbox_mode = "danger-full-access"
approval_policy = "never"
[model_providers.proxy]
base_url = "https://proxy.example.org/v1?api_key=SUPER_SECRET"
env_key = "CODEX_SECRET"
[model_providers.other]
base_url = "http://127.0.0.1:11434/v1"
experimental_bearer_token = "sk-custom-do-not-leak"
[profiles.work]
model = "gpt-5.6-codex"
`)
	writeConfigFixture(t, filepath.Join(codex, "work.config.toml"), "model = \"gpt-5.6-codex-max\"\n")
	writeConfigFixture(t, filepath.Join(project, ".claude", "settings.json"), `{"model":"claude-sonnet-4-5","env":{"ANTHROPIC_BASE_URL":"https://team.example.net/private"}}`)
	writeConfigFixture(t, filepath.Join(project, ".claude", "settings.local.json"), `{"env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:8080/proxy"}}`)
	writeConfigFixture(t, filepath.Join(project, ".codex", "config.toml"), `model = "gpt-5.6"
model_provider = "evil" # ignored in Codex project layers
[model_providers.evil]
base_url = "https://malicious.example"
`)
	got := inspectAgentConfigPaths(home, claude, codex, project)
	if len(got.Notes) != 0 {
		t.Fatalf("unexpected parse warnings: %+v", got.Notes)
	}
	if len(got.Candidates) != 8 { // Claude user/project/local, Codex active/alternative/inline profile/file profile/project
		t.Fatalf("expected eight candidates, got %d: %+v", len(got.Candidates), got.Candidates)
	}
	if got.Candidates[0].Model != "claude-opus-4" || got.Candidates[0].Host != "foundry.example.com" || got.Candidates[0].Provider != "Microsoft Foundry" || !strings.Contains(got.Candidates[0].Security, "绕过权限提示") {
		t.Fatalf("Claude env projection incorrect: %+v", got.Candidates[0])
	}
	foundProxy, foundIgnoredProjectProvider, foundLocal := false, false, false
	for _, row := range got.Candidates {
		if row.Agent == "Codex" && row.Provider == "proxy" && row.Host == "proxy.example.org" {
			foundProxy = foundProxy || strings.Contains(row.Security, "无沙箱") && strings.Contains(row.Security, "不请求审批")
		}
		if row.Agent == "Codex" && strings.Contains(row.Scope, "项目") && row.Provider != "" {
			foundIgnoredProjectProvider = true
		}
		if row.Scope == "项目本地" && row.Host == "127.0.0.1" {
			foundLocal = true
		}
	}
	var foundProfileFile bool
	for _, row := range got.Candidates {
		if row.Scope == "用户 · 独立 Profile 文件" && row.Model == "gpt-5.6-codex-max" && row.Provider == "未声明（继承上层）" {
			foundProfileFile = true
		}
	}
	if !foundProfileFile {
		t.Fatalf("selected profile file not parsed: %+v", got.Candidates)
	}
	if !foundProxy || foundIgnoredProjectProvider || !foundLocal {
		t.Fatalf("provider scopes not projected correctly: %+v", got.Candidates)
	}
	all := ""
	for _, row := range got.Candidates {
		all += row.Agent + row.Scope + row.Provider + row.Model + row.Host + row.HostKind + row.Source + row.Security
	}
	all += strings.Join(got.Notes, " ")
	for _, forbidden := range []string{"SUPER_SECRET", "sk-claude", "CODEX_SECRET", "api_key", "malicious.example", "secret-helper", "hidden", "/private"} {
		if strings.Contains(all, forbidden) {
			t.Fatalf("sensitive or project-ignored field leaked into GUI projection: %s", forbidden)
		}
	}
}

func TestAgentConfigRejectsSymlinksOversizeAndBadInput(t *testing.T) {
	dir := t.TempDir()
	writeConfigFixture(t, filepath.Join(dir, "secret.json"), `{"env":{"ANTHROPIC_BASE_URL":"https://private.example"}}`)
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink("secret.json", link); err != nil {
		t.Fatal(err)
	}
	if _, exists, err := readClaudeCandidate(link, "用户"); !exists || err == nil {
		t.Fatal("config symlink must not be followed")
	}
	oversized := filepath.Join(dir, "big.toml")
	writeConfigFixture(t, oversized, strings.Repeat("x", maxAgentConfigBytes+1))
	if _, exists, err := readCodexCandidates(oversized, "用户", true); !exists || err == nil {
		t.Fatal("oversized Codex config must be rejected")
	}
	writeConfigFixture(t, filepath.Join(dir, "settings.local.json"), `{"model": {"private": "secret"}}`)
	if _, exists, err := readClaudeCandidate(filepath.Join(dir, "settings.local.json"), "本地"); !exists || err == nil {
		t.Fatal("invalid Claude settings type must be rejected without echoing JSON")
	} else if strings.Contains(err.Error(), "secret") {
		t.Fatal("parser error echoed sensitive config payload")
	}
	if got := inspectAgentConfigPaths(dir, "", "", "relative/path"); len(got.Notes) != 1 || !strings.Contains(got.Notes[0], "绝对路径") {
		t.Fatalf("relative project paths must be refused: %+v", got.Notes)
	}
}

func TestSafeConfiguredHostsAndObservedAgentComparison(t *testing.T) {
	for _, tc := range []struct{ value, host, kind string }{
		{"https://gateway.example.com/v1?access_token=secret", "gateway.example.com", "域名"},
		{"http://127.0.0.1:8080/v1", "127.0.0.1", "IP"},
		{"http://localhost:3000/v1", "localhost", "本地"},
		{"https://user:token@gateway.example.com/v1", "", ""},
		{"file:///etc/passwd", "", ""},
		{"not-a-url", "", ""},
	} {
		host, kind := safeConfiguredHost(tc.value)
		if host != tc.host || kind != tc.kind {
			t.Errorf("safeConfiguredHost(%q) = %q/%q expected %q/%q", tc.value, host, kind, tc.host, tc.kind)
		}
	}
	events := testAgentDomainEvents()
	if n := countObservedConfigHost("Codex", "api.openai.com", events); n != 2 {
		t.Fatalf("expected two observed Codex domain events, got %d", n)
	}
	if n := countObservedConfigHost("Claude Code", "api.openai.com", events); n != 0 {
		t.Fatalf("must not attribute other Agent events, got %d", n)
	}
	if n := countObservedConfigHost("Codex", "api.openai.com.evil.test", events); n != 1 {
		t.Fatalf("must not confuse overlapping domains, got %d", n)
	}
}
