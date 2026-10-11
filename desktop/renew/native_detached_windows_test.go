package main

import "testing"

func makeDetachedTestTab(id int) *renewTerminalTab {
    return &renewTerminalTab{
        id: id, label: "shell", focus: id,
        root: &renewTerminalNode{id: id, pane: &renewTerminalPane{id: id}},
    }
}

func TestTakeTerminalTabKeepsPTYOwnership(t *testing.T) {
    a := newRenewApp("http://127.0.0.1:8080")
    x, y, z := makeDetachedTestTab(1), makeDetachedTestTab(2), makeDetachedTestTab(3)
    a.terminalTabs = []*renewTerminalTab{x, y, z}
    a.terminalActive = 1
    if got := a.takeTerminalTab(1); got != y {
        t.Fatal("detaching must return the original tab and split tree, not a copy")
    }
    if len(a.terminalTabs) != 2 || a.terminalTabs[0] != x || a.terminalTabs[1] != z || a.terminalActive != 1 {
        t.Fatalf("wrong remaining tabs: %+v active=%d", a.terminalTabs, a.terminalActive)
    }
    if a.takeTerminalTab(-1) != nil || a.takeTerminalTab(99) != nil {
        t.Fatal("invalid index must leave the tabs unchanged")
    }
}

func TestDockDetachedTabsIsIdempotent(t *testing.T) {
    parent := newRenewApp("http://127.0.0.1:8080")
    child := newRenewApp(parent.backend)
    child.detachedParent = parent
    tab1, tab2 := makeDetachedTestTab(11), makeDetachedTestTab(12)
    child.terminalTabs = []*renewTerminalTab{tab1, tab2}
    parent.terminalTabs = []*renewTerminalTab{makeDetachedTestTab(4)}
    parent.dockDetachedTerminals(child)
    if len(parent.terminalTabs) != 3 || parent.terminalTabs[1] != tab1 || parent.terminalTabs[2] != tab2 {
        t.Fatal("all floating tabs must reattach by identity")
    }
    if len(child.terminalTabs) != 0 || parent.terminalActive != 2 || parent.page != "终端" {
        t.Fatal("the floating window must relinquish tab ownership")
    }
    parent.dockDetachedTerminals(child)
    if len(parent.terminalTabs) != 3 {
        t.Fatal("repeated close must not duplicate tabs")
    }
}

func TestFloatingTerminalSharesIDAllocator(t *testing.T) {
    parent := newRenewApp("http://127.0.0.1:8080")
    child := newRenewApp(parent.backend)
    child.detachedParent = parent
    if id := parent.nextTerminalID(); id != 1 {
        t.Fatalf("first ID = %d", id)
    }
    if id := child.nextTerminalID(); id != 2 {
        t.Fatalf("child should use parent's allocator, got %d", id)
    }
    if id := parent.nextTerminalID(); id != 3 {
        t.Fatalf("terminal IDs may not collide across windows: %d", id)
    }
}
