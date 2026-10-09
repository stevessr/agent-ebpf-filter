package main

// Terminal workspace for the unprivileged Renew desktop process.
//
// GoRex (github.com/egoist/gorex) demonstrates the MyGo terminal plugin and
// its split-pane layout. This integration uses that plugin directly: GoRex is
// an application (its session package is internal), not an importable SDK.
// No shell execution is routed through the privileged eBPF backend.
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

const terminalScrollbackBytes = 8 << 20

type renewTerminalPane struct {
	id       int
	term     *terminal.Terminal
	startDir string
	exited   bool
	exitCode int
}

type renewTerminalNode struct {
	id       int
	pane     *renewTerminalPane
	first    *renewTerminalNode
	second   *renewTerminalNode
	vertical bool
	ratio    float32
}

type renewTerminalTab struct {
	id    int
	label string
	root  *renewTerminalNode
	focus int
	zoom  int
}

func (n *renewTerminalNode) findPane(id int) *renewTerminalPane {
	if n == nil {
		return nil
	}
	if n.pane != nil {
		if n.pane.id == id {
			return n.pane
		}
		return nil
	}
	if p := n.first.findPane(id); p != nil {
		return p
	}
	return n.second.findPane(id)
}

func (n *renewTerminalNode) firstPane() *renewTerminalPane {
	if n == nil {
		return nil
	}
	if n.pane != nil {
		return n.pane
	}
	return n.first.firstPane()
}

// splitTerminalNode replaces a leaf with a split, preserving its existing
// terminal and adding the new one. vertical means panes stack top/bottom.
func splitTerminalNode(n *renewTerminalNode, focused int, next *renewTerminalPane, vertical bool, id int) bool {
	if n == nil {
		return false
	}
	if n.pane != nil {
		if n.pane.id != focused {
			return false
		}
		old := n.pane
		n.pane = nil
		n.vertical = vertical
		n.ratio = 0.5
		n.first = &renewTerminalNode{id: old.id, pane: old}
		n.second = &renewTerminalNode{id: id, pane: next}
		return true
	}
	return splitTerminalNode(n.first, focused, next, vertical, id) ||
		splitTerminalNode(n.second, focused, next, vertical, id)
}

// detachTerminalPane removes one leaf. The surviving sibling takes the split
// node's place; the caller closes only the removed terminal's PTY.
func detachTerminalPane(n *renewTerminalNode, id int) (*renewTerminalNode, *renewTerminalPane) {
	if n == nil {
		return nil, nil
	}
	if n.pane != nil {
		if n.pane.id == id {
			return nil, n.pane
		}
		return n, nil
	}
	if n.first != nil {
		child, pane := detachTerminalPane(n.first, id)
		if pane != nil {
			if child == nil {
				return n.second, pane
			}
			n.first = child
			return n, pane
		}
	}
	if n.second != nil {
		child, pane := detachTerminalPane(n.second, id)
		if pane != nil {
			if child == nil {
				return n.first, pane
			}
			n.second = child
			return n, pane
		}
	}
	return n, nil
}

func (n *renewTerminalNode) closeAll() {
	if n == nil {
		return
	}
	if n.pane != nil {
		if n.pane.term != nil {
			_ = n.pane.term.Close()
			n.pane.term = nil
		}
		return
	}
	n.first.closeAll()
	n.second.closeAll()
}

func (a *renewApp) nextTerminalID() int {
	a.terminalNextID++
	return a.terminalNextID
}

func terminalHomeDir() string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return home
	}
	return os.TempDir()
}

func (a *renewApp) newTerminalPane(dir string) (*renewTerminalPane, error) {
	if dir == "" {
		dir = terminalHomeDir()
	}
	pane := &renewTerminalPane{id: a.nextTerminalID(), startDir: dir}
	term, err := terminal.New(terminal.Options{
		Dir:        dir,
		Scrollback: terminalScrollbackBytes,
		Font:       terminal.Font{Family: "monospace", Size: 13},
		OnExit: func(code int) {
			if a.terminalClosing.Load() {
				return
			}
			a.update(func() {
				pane.exited = true
				pane.exitCode = code
			})
		},
	})
	if err != nil {
		return nil, err
	}
	pane.term = term
	return pane, nil
}

func (a *renewApp) currentTerminalTab() *renewTerminalTab {
	if a.terminalActive < 0 || a.terminalActive >= len(a.terminalTabs) {
		return nil
	}
	return a.terminalTabs[a.terminalActive]
}

func (a *renewApp) newTerminalTab(dir string) {
	pane, err := a.newTerminalPane(dir)
	if err != nil {
		a.terminalError = fmt.Sprintf("无法打开终端：%v", err)
		return
	}
	a.terminalError = ""
	id := a.nextTerminalID()
	tab := &renewTerminalTab{
		id:    id,
		label: fmt.Sprintf("终端 %d", len(a.terminalTabs)+1),
		root:  &renewTerminalNode{id: pane.id, pane: pane},
		focus: pane.id,
	}
	a.terminalTabs = append(a.terminalTabs, tab)
	a.terminalActive = len(a.terminalTabs) - 1
	a.terminalFocusRequest = pane.id
}

func (a *renewApp) focusedTerminal() *renewTerminalPane {
	tab := a.currentTerminalTab()
	if tab == nil {
		return nil
	}
	return tab.root.findPane(tab.focus)
}

func (a *renewApp) terminalWorkingDir() string {
	if pane := a.focusedTerminal(); pane != nil {
		if pane.term != nil {
			if dir := pane.term.Dir(); dir != "" {
				if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
					return dir
				}
			}
		}
		return pane.startDir
	}
	return terminalHomeDir()
}

func (a *renewApp) splitFocusedTerminal(vertical bool) {
	tab := a.currentTerminalTab()
	if tab == nil || tab.root == nil {
		return
	}
	pane, err := a.newTerminalPane(a.terminalWorkingDir())
	if err != nil {
		a.terminalError = fmt.Sprintf("无法拆分终端：%v", err)
		return
	}
	if !splitTerminalNode(tab.root, tab.focus, pane, vertical, a.nextTerminalID()) {
		_ = pane.term.Close()
		return
	}
	tab.focus = pane.id
	tab.zoom = 0
	a.terminalFocusRequest = pane.id
	a.terminalError = ""
}

func (a *renewApp) closeFocusedTerminal() {
	tab := a.currentTerminalTab()
	if tab == nil || tab.root == nil {
		return
	}
	var pane *renewTerminalPane
	tab.root, pane = detachTerminalPane(tab.root, tab.focus)
	if pane == nil {
		return
	}
	if pane.term != nil {
		_ = pane.term.Close()
		pane.term = nil
	}
	tab.zoom = 0
	if tab.root == nil {
		a.closeTerminalTab(a.terminalActive)
		return
	}
	tab.focus = tab.root.firstPane().id
	a.terminalFocusRequest = tab.focus
}

func (a *renewApp) closeTerminalTab(index int) {
	if index < 0 || index >= len(a.terminalTabs) {
		return
	}
	tab := a.terminalTabs[index]
	tab.root.closeAll()
	a.terminalTabs = append(a.terminalTabs[:index], a.terminalTabs[index+1:]...)
	if a.terminalActive >= len(a.terminalTabs) {
		a.terminalActive = len(a.terminalTabs) - 1
	}
	if a.terminalActive < 0 {
		a.terminalActive = 0
	}
	if current := a.currentTerminalTab(); current != nil {
		a.terminalFocusRequest = current.focus
	}
}

func (a *renewApp) closeTerminals() {
	a.terminalClosing.Store(true)
	for _, tab := range a.terminalTabs {
		tab.root.closeAll()
	}
	a.terminalTabs = nil
}

func terminalPaneLabel(p *renewTerminalPane) string {
	if p == nil || p.term == nil {
		return "终端"
	}
	if title := strings.TrimSpace(p.term.Title()); title != "" {
		return title
	}
	if p.exited {
		return fmt.Sprintf("已退出 (%d)", p.exitCode)
	}
	return "Shell"
}

func terminalShortDir(dir string) string {
	home, _ := os.UserHomeDir()
	if home != "" && (dir == home || strings.HasPrefix(dir, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(dir, home)
	}
	return dir
}

// terminalView does not use ui.Scroll: a PTY must see its actual pixel bounds
// to resize full-screen terminal applications correctly.
func (a *renewApp) terminalView(c *ui.Context) {
	t := c.Theme()
	if !a.terminalInitialized {
		a.terminalInitialized = true
		a.newTerminalTab(terminalHomeDir())
	}
	ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Padding(14).Gap(10).Children(func() {
		ui.Row(c).Height(38).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "本地终端").FontSize(15).Bold()
			statusPill(c, "普通用户权限", t.Success)
			ui.Spacer(c)
			if ui.Button(c, "新建标签").Clicked() {
				a.newTerminalTab(a.terminalWorkingDir())
			}
			tab := a.currentTerminalTab()
			if tab != nil {
				if ui.Button(c, "左右拆分").Clicked() {
					a.splitFocusedTerminal(false)
				}
				if ui.Button(c, "上下拆分").Clicked() {
					a.splitFocusedTerminal(true)
				}
				label := "最大化窗格"
				if tab.zoom != 0 {
					label = "还原窗格"
				}
				if ui.Button(c, label).Clicked() {
					if tab.zoom != 0 {
						tab.zoom = 0
					} else {
						tab.zoom = tab.focus
					}
				}
				if ui.Button(c, "关闭窗格").Clicked() {
					a.closeFocusedTerminal()
				}
			}
		})
		ui.Row(c).Height(34).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
			for i, tab := range a.terminalTabs {
				index := i
				name := tab.label
				if tab.root != nil && tab.root.first == nil {
					if title := terminalPaneLabel(tab.root.pane); title != "Shell" && title != "" {
						name = title
					}
				}
				if ui.Button(c, name).Clicked() {
					a.terminalActive = index
					a.terminalFocusRequest = tab.focus
				}
			}
			if len(a.terminalTabs) > 0 {
				if ui.Button(c, "关闭标签").Clicked() {
					a.closeTerminalTab(a.terminalActive)
				}
			}
		})
		if a.terminalError != "" {
			ui.Text(c, a.terminalError).TextColor(t.Danger).FontSize(12)
		}
		tab := a.currentTerminalTab()
		if tab == nil || tab.root == nil {
			ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
				ui.Text(c, "没有打开的终端").TextColor(t.TextMuted)
				if ui.PrimaryButton(c, "启动 Shell").Clicked() {
					a.newTerminalTab(terminalHomeDir())
				}
			})
			return
		}
		if tab.zoom != 0 {
			if pane := tab.root.findPane(tab.zoom); pane != nil {
				a.terminalPaneView(c, tab, pane).Grow(1).MinHeight(0)
				return
			}
			tab.zoom = 0
		}
		a.terminalNodeView(c, tab, tab.root).Grow(1).MinHeight(0).MinWidth(0)
	})
}

func (a *renewApp) terminalNodeView(c *ui.Context, tab *renewTerminalTab, n *renewTerminalNode) ui.Element {
	if n.pane != nil {
		return a.terminalPaneView(c, tab, n.pane)
	}
	var box ui.Element
	if n.vertical {
		box = ui.Column(c)
	} else {
		box = ui.Row(c)
	}
	box.MinWidth(0).MinHeight(0).AlignItems(ui.Stretch)
	bounds := box.Bounds()
	box.Children(func() {
		a.terminalNodeView(c, tab, n.first).Grow(n.ratio).Basis(0).MinWidth(0).MinHeight(0)
		divider := ui.Box(c).Role(ui.RoleSplitter).Label("终端分隔条")
		if n.vertical {
			divider.Height(8).Cursor(ui.CursorResizeRow)
		} else {
			divider.Width(8).Cursor(ui.CursorResizeColumn)
		}
		if dx, dy, dragged := divider.Dragged(); dragged {
			total, delta := bounds.W-8, dx
			if n.vertical {
				total, delta = bounds.H-8, dy
			}
			if total > 0 {
				n.ratio = min(max(n.ratio+delta/total, 0.1), 0.9)
			}
		}
		if divider.DoubleClicked() {
			n.ratio = 0.5
		}
		a.terminalNodeView(c, tab, n.second).Grow(1-n.ratio).Basis(0).MinWidth(0).MinHeight(0)
	})
	return box
}

func (a *renewApp) terminalPaneView(c *ui.Context, tab *renewTerminalTab, pane *renewTerminalPane) ui.Element {
	t := c.Theme()
	col := ui.Column(c).MinWidth(0).MinHeight(0).Radius(10).Clip().
		Background(t.Surface).Border(1, t.Border)
	col.Children(func() {
		header := ui.Row(c).Height(34).Padding(0, 10).Gap(8).AlignItems(ui.Center)
		header.Children(func() {
			if tab.focus == pane.id {
				statusPill(c, "当前", t.Accent)
			}
			ui.Text(c, terminalPaneLabel(pane)).FontSize(12).Bold().SingleLine()
			dir := pane.startDir
			if pane.term != nil && pane.term.Dir() != "" {
				dir = pane.term.Dir()
			}
			ui.Text(c, terminalShortDir(dir)).FontSize(11).TextColor(t.TextMuted).SingleLine()
			ui.Spacer(c)
			if pane.exited {
				statusPill(c, fmt.Sprintf("退出码 %d", pane.exitCode), t.Warning)
			}
		})
		if header.Clicked() {
			tab.focus = pane.id
			a.terminalFocusRequest = pane.id
		}
		ui.Box(c).Grow(1).MinHeight(0).MinWidth(0).Padding(4).Children(func() {
			if pane.term == nil {
				return
			}
			view := terminal.View(c, pane.term).Fill()
			if a.terminalFocusRequest == pane.id && a.currentTerminalTab() == tab {
				view.Focus()
				a.terminalFocusRequest = 0
			}
			if view.Focused() {
				tab.focus = pane.id
			}
		})
	})
	return col
}
