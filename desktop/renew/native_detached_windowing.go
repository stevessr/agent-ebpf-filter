package main

import (
	"fmt"
	"math"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// reassignOwner redirects PTY exit callbacks to the window that currently
// owns the pane, including both leaves of an existing split tree.
func (n *renewTerminalNode) reassignOwner(owner *renewApp) {
	if n == nil {
		return
	}
	if n.pane != nil {
		n.pane.owner.Store(owner)
		return
	}
	n.first.reassignOwner(owner)
	n.second.reassignOwner(owner)
}

// takeTerminalTab changes the owning list only: it never terminates a PTY.
func (a *renewApp) takeTerminalTab(index int) *renewTerminalTab {
	if index < 0 || index >= len(a.terminalTabs) {
		return nil
	}
	tab := a.terminalTabs[index]
	a.terminalTabs = append(a.terminalTabs[:index], a.terminalTabs[index+1:]...)
	if index < a.terminalActive {
		a.terminalActive--
	}
	if a.terminalActive >= len(a.terminalTabs) {
		a.terminalActive = len(a.terminalTabs) - 1
	}
	if a.terminalActive < 0 {
		a.terminalActive = 0
	}
	if next := a.currentTerminalTab(); next != nil {
		a.terminalFocusRequest = next.focus
	} else {
		a.terminalFocusRequest = 0
		a.terminalTabScroll.X = 0
	}
	return tab
}

// dockTerminalTab transfers precisely one live tab. Closing an entire window
// uses this same ownership path for each of its tabs.
func (a *renewApp) dockTerminalTab(child *renewApp, index int) bool {
	if child == nil || child.detachedParent != a || a.terminalClosing.Load() {
		return false
	}
	tab := child.takeTerminalTab(index)
	if tab == nil {
		return false
	}
	tab.root.reassignOwner(a)
	a.terminalInitialized = true
	a.terminalTabs = append(a.terminalTabs, tab)
	a.terminalActive = len(a.terminalTabs) - 1
	a.terminalTabScroll.X = math.MaxFloat32
	a.terminalFocusRequest = tab.focus
	return true
}

func (a *renewApp) dockDetachedTerminals(child *renewApp) {
	if child == nil || child.detachedParent != a {
		return
	}
	for len(child.terminalTabs) > 0 {
		if !a.dockTerminalTab(child, 0) {
			return
		}
	}
}

func (a *renewApp) revealDockedTerminal() {
	a.page = "终端"
	if a.win != nil {
		a.hiddenForDetached = false
		a.win.Restore()
		a.win.Show()
		a.win.Focus()
		a.win.Update(func() {})
	}
}

// dockFocusedDetachedTab is an explicit user action. Closing a detached
// window using its title bar instead docks in the background without
// stealing focus from an already-visible main workspace.
func (a *renewApp) dockFocusedDetachedTab() {
	parent := a.detachedParent
	if parent == nil {
		return
	}
	if !parent.dockTerminalTab(a, a.terminalActive) {
		return
	}
	parent.revealDockedTerminal()
	if len(a.terminalTabs) == 0 && a.win != nil {
		a.win.Close()
	}
}

// restoreMainAfterLastDetached is only for the case where the user tried to
// close the owner while its children were open. It must not undo an explicit
// minimize-to-tray request.
func (a *renewApp) restoreMainAfterLastDetached() {
	if !a.hiddenForDetached || len(a.detachedTerminals) != 0 || len(a.detachedPages) != 0 {
		return
	}
	a.hiddenForDetached = false
	if a.win != nil {
		a.win.Show()
		a.win.Focus()
	}
}

func (a *renewApp) openDetachedTerminalTab() {
	parent := a
	if a.detachedParent != nil {
		parent = a.detachedParent
	}
	if a.terminalClosing.Load() || parent.terminalClosing.Load() {
		return
	}
	tab := a.takeTerminalTab(a.terminalActive)
	if tab == nil {
		return
	}
	child := newRenewApp(parent.backend)
	child.detachedParent = parent
	tab.root.reassignOwner(child)
	child.terminalInitialized = true
	child.terminalTabs = []*renewTerminalTab{tab}
	child.terminalActive = 0
	child.terminalFocusRequest = tab.focus
	child.page = "终端"

	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     desktopBrandName + " · " + tab.label,
		Width:     960,
		Height:    680,
		MinWidth:  560,
		MinHeight: 360,
		StateKey:  fmt.Sprintf("terminal-%d", tab.id),
		Vibrancy:  windowMaterial(runtime.GOOS, parent.materialEnabled),
		Content:   ui.View(child.detachedTerminalView),
	})
	child.win = win
	if parent.detachedTerminals == nil {
		parent.detachedTerminals = make(map[int]*renewApp)
	}
	parent.detachedTerminals[tab.id] = child
	win.OnClose(func(_ *mygo.CloseEvent) {
		if !parent.terminalClosing.Load() {
			parent.dockDetachedTerminals(child)
		}
	})
	win.OnClosed(func() {
		delete(parent.detachedTerminals, tab.id)
		child.win = nil
		parent.restoreMainAfterLastDetached()
	})
	win.Show()
	win.Focus()
	// A floating window can itself split out another tab; if it is now
	// empty, retire the old shell window without creating a fresh PTY.
	if a != parent && len(a.terminalTabs) == 0 && a.win != nil {
		a.win.Close()
	}
}

func (a *renewApp) detachedTerminalView(c *ui.Context) {
	styleRenewWorkspaceTheme(&a.workspaceTheme, c.Theme(), c.Preferences().HighContrast)
	c.SetTheme(&a.workspaceTheme)
	ui.Column(c).Fill().MinWidth(0).MinHeight(0).
		Background(c.Theme().Background).Children(func() {
			a.terminalView(c)
		})
}
