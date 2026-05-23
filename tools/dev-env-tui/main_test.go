package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rivo/tview"
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
	got := stripTviewTags("[green]ok[-] [#22c55e]good[-] [#7dd3fc::b]title[::-] keep [literal value]")
	want := "ok good title keep [literal value]"
	if got != want {
		t.Fatalf("stripTviewTags() = %q, want %q", got, want)
	}
}

func TestMoveFormFocusUsesArrowAndWheelSemantics(t *testing.T) {
	m := &model{form: tview.NewForm()}
	m.form.AddInputField("one", "", 20, nil, nil)
	m.form.AddInputField("two", "", 20, nil, nil)
	m.form.AddButton("save", nil)
	focusFormFirstItem(m.form)

	m.moveFormFocus(1)
	item, button := m.form.GetFocusedItemIndex()
	if item != 1 || button != -1 {
		t.Fatalf("after moving down, focused item=%d button=%d, want item=1 button=-1", item, button)
	}

	m.moveFormFocus(1)
	item, button = m.form.GetFocusedItemIndex()
	if item != -1 || button != 0 {
		t.Fatalf("after moving to button, focused item=%d button=%d, want item=-1 button=0", item, button)
	}

	m.moveFormFocus(1)
	item, button = m.form.GetFocusedItemIndex()
	if item != -1 || button != 0 {
		t.Fatalf("focus should clamp at last button, got item=%d button=%d", item, button)
	}
}

func TestMoveGroupSelectionUpdatesSelectedGroup(t *testing.T) {
	m := &model{
		values: make(map[string]string),
		list:   tview.NewList(),
		form:   tview.NewForm(),
		status: tview.NewTextView(),
	}
	for _, group := range groups {
		m.list.AddItem(group.Title, group.Desc, 0, nil)
	}
	m.list.SetChangedFunc(func(index int, _ string, _ string, _ rune) {
		m.selectGroup(index, false)
	})
	m.rebuildForm()

	m.moveGroupSelection(1)
	if got := m.selected; got != 1 {
		t.Fatalf("selected group = %d, want 1", got)
	}
	if got := m.list.GetCurrentItem(); got != 1 {
		t.Fatalf("current list item = %d, want 1", got)
	}

	m.moveGroupSelection(-10)
	if got := m.selected; got != 0 {
		t.Fatalf("selected group after clamp = %d, want 0", got)
	}
}

func focusFormFirstItem(form *tview.Form) {
	var focus func(tview.Primitive)
	focus = func(p tview.Primitive) {
		p.Focus(focus)
	}
	form.Focus(focus)
}
