package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTerminalSplitAndDetach(t *testing.T) {
	first := &renewTerminalPane{id: 1}
	second := &renewTerminalPane{id: 2}
	third := &renewTerminalPane{id: 3}
	root := &renewTerminalNode{id: 1, pane: first}

	if splitTerminalNode(root, 99, second, false, 10) {
		t.Fatal("split accepted a nonexistent focused pane")
	}
	if !splitTerminalNode(root, 1, second, false, 10) {
		t.Fatal("could not split the initial leaf")
	}
	if root.pane != nil || root.vertical || root.ratio != 0.5 {
		t.Fatalf("unexpected horizontal split: %+v", root)
	}
	if root.findPane(1) != first || root.findPane(2) != second {
		t.Fatal("split did not retain both shells")
	}
	if !splitTerminalNode(root, 2, third, true, 11) {
		t.Fatal("could not nest a vertical split")
	}
	if root.second == nil || !root.second.vertical {
		t.Fatal("nested split orientation was lost")
	}

	var removed *renewTerminalPane
	root, removed = detachTerminalPane(root, 2)
	if removed != second || root.findPane(2) != nil || root.findPane(3) != third {
		t.Fatal("closing nested pane removed the wrong shell")
	}
	root, removed = detachTerminalPane(root, 1)
	if removed != first || root.pane != third {
		t.Fatalf("failed to collapse split after closing the left pane: %+v", root)
	}
	root, removed = detachTerminalPane(root, 3)
	if removed != third || root != nil {
		t.Fatal("last pane did not remove the entire tree")
	}
}

func TestTerminalCloseMissingDoesNotAlterTree(t *testing.T) {
	pane := &renewTerminalPane{id: 42}
	root := &renewTerminalNode{pane: pane}
	next, removed := detachTerminalPane(root, 7)
	if next != root || removed != nil {
		t.Fatal("nonexistent pane changed layout")
	}
}

func TestTerminalShortDirectory(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	if got := terminalShortDir(home); got != "~" {
		t.Fatalf("home = %q, want ~", got)
	}
	path := filepath.Join(home, "projects", "agent-ebpf")
	if got := terminalShortDir(path); got != filepath.Join("~", "projects", "agent-ebpf") {
		t.Fatalf("project directory = %q", got)
	}
}
