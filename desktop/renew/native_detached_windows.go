package main

import (
    "fmt"
    "math"
    "runtime"

    "github.com/egoist/mygo"
    "github.com/egoist/mygo/ui"
)

// reassignOwner ensures the exit callback always repaints the actual owner
// of a live PTY, even after repeated detachable-window transfers.
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

// takeTerminalTab moves a tab without closing any of its running PTYs.
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

// dockDetachedTerminals returns every live tab in the child window.
func (a *renewApp) dockDetachedTerminals(child *renewApp) {
    if child == nil || len(child.terminalTabs) == 0 {
        return
    }
    a.terminalInitialized = true
    for _, tab := range child.terminalTabs {
        tab.root.reassignOwner(a)
    }
    a.terminalTabs = append(a.terminalTabs, child.terminalTabs...)
    a.terminalActive = len(a.terminalTabs) - 1
    a.terminalTabScroll.X = math.MaxFloat32
    a.terminalFocusRequest = a.terminalTabs[a.terminalActive].focus
    child.terminalTabs = nil
    child.terminalFocusRequest = 0
    a.page = "终端"
    if a.win != nil {
        a.win.Restore()
        a.win.Show()
        a.win.Focus()
        a.win.Update(func() {})
    }
}

func (a *renewApp) openDetachedTerminalTab() {
    if a.detachedParent != nil || a.terminalClosing.Load() {
        return
    }
    tab := a.takeTerminalTab(a.terminalActive)
    if tab == nil {
        return
    }
    child := newRenewApp(a.backend)
    child.detachedParent = a
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
        Vibrancy:  windowMaterial(runtime.GOOS, a.materialEnabled),
        Content:   ui.View(child.detachedTerminalView),
    })
    child.win = win
    if a.detachedTerminals == nil {
        a.detachedTerminals = make(map[int]*renewApp)
    }
    a.detachedTerminals[tab.id] = child
    win.OnClose(func(_ *mygo.CloseEvent) {
        if !a.terminalClosing.Load() {
            a.dockDetachedTerminals(child)
        }
    })
    win.OnClosed(func() {
        delete(a.detachedTerminals, tab.id)
        child.win = nil
    })
    win.Show()
    win.Focus()
}

func (a *renewApp) detachedTerminalView(c *ui.Context) {
    styleRenewWorkspaceTheme(&a.workspaceTheme, c.Theme(), c.Preferences().HighContrast)
    c.SetTheme(&a.workspaceTheme)
    ui.Column(c).Fill().MinWidth(0).MinHeight(0).
        Background(c.Theme().Background).Children(func() {
            a.terminalView(c)
        })
}
