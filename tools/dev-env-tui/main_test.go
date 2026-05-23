package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFilesQuotesSecretsAndExportsMakeVars(t *testing.T) {
	tmp := t.TempDir()
	m := &model{
		root:     tmp,
		shellEnv: filepath.Join(tmp, ".env.dev"),
		makeEnv:  filepath.Join(tmp, ".env.dev.mk"),
		values: map[string]string{
			"DISABLE_AUTH":      "true",
			"AGENT_LLM_ENABLED": "true",
			"AGENT_LLM_API_KEY": "sk-test'abc",
			"AGENT_LLM_MODEL":   "qwen2.5-coder",
		},
	}
	if err := m.writeFiles(); err != nil {
		t.Fatalf("writeFiles() error = %v", err)
	}
	shellData, err := os.ReadFile(m.shellEnv)
	if err != nil {
		t.Fatal(err)
	}
	shellText := string(shellData)
	if !strings.Contains(shellText, "export AGENT_LLM_API_KEY='sk-test'\\''abc'") {
		t.Fatalf("shell env did not quote API key correctly:\n%s", shellText)
	}
	info, err := os.Stat(m.shellEnv)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("shell env mode = %v, want 0600", got)
	}

	makeData, err := os.ReadFile(m.makeEnv)
	if err != nil {
		t.Fatal(err)
	}
	makeText := string(makeData)
	if !strings.Contains(makeText, "AGENT_LLM_MODEL := qwen2.5-coder\nexport AGENT_LLM_MODEL") {
		t.Fatalf("make env did not export model var:\n%s", makeText)
	}
}

func TestStripTviewTagsKeepsNormalBrackets(t *testing.T) {
	got := stripTviewTags("[green]ok[-] keep [literal value]")
	want := "ok keep [literal value]"
	if got != want {
		t.Fatalf("stripTviewTags() = %q, want %q", got, want)
	}
}
