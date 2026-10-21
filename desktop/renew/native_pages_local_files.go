package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func (a *renewApp) ensureLocalFiles() {
	if a.files.dir != "" || a.files.listLoading {
		return
	}
	home := terminalHomeDir()
	a.browseLocalDirectory(home)
}

func (a *renewApp) browseLocalDirectory(path string) {
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		a.files.errorText = err.Error()
		return
	}
	a.files.listGeneration++
	generation := a.files.listGeneration
	a.files.listLoading = true
	a.files.errorText = ""
	go func() {
		rows, truncated, err := readLocalDirectory(absolute)
		a.update(func() {
			if generation != a.files.listGeneration {
				return
			}
			a.files.listLoading = false
			if err != nil {
				a.files.errorText = fmt.Sprintf("无法打开目录：%v", err)
				return
			}
			a.files.dir = absolute
			a.files.address = absolute
			a.files.items = rows
			a.files.listTruncated = truncated
			a.files.listLoaded = true
			a.files.selected = ""
			a.files.pendingDelete = ""
			a.files.errorText = ""
		})
	}()
}

func (a *renewApp) selectedLocalPath() string {
	if a.files.selected == "" {
		return ""
	}
	path, err := localTarget(a.files.dir, a.files.selected)
	if err != nil {
		return ""
	}
	return path
}

func (a *renewApp) localDocument(path string) (*localFileDocument, int) {
	for i, doc := range a.files.docs {
		if doc.path == path {
			return doc, i
		}
	}
	return nil, -1
}

func (a *renewApp) openLocalFile(path string) {
	if doc, index := a.localDocument(path); doc != nil {
		a.files.active = index
		a.page = "编辑器"
		return
	}
	a.files.errorText = ""
	go func() {
		text, info, err := readLocalDocument(path)
		a.update(func() {
			if err != nil {
				a.files.errorText = fmt.Sprintf("无法打开文件：%v", err)
				return
			}
			if _, index := a.localDocument(path); index >= 0 {
				a.files.active = index
			} else {
				buf := editor.NewBuffer(text)
				buf.SetLanguage(editor.LanguageFor(path))
				doc := &localFileDocument{
					path: path,
					state: &editor.State{Buffer: buf, WrapGuides: []int{100}, Label: filepath.Base(path)},
					modTime: info.ModTime(), size: info.Size(), mode: info.Mode(),
				}
				a.files.docs = append(a.files.docs, doc)
				a.files.active = len(a.files.docs) - 1
			}
			a.page = "编辑器"
		})
	}()
}

func (a *renewApp) currentLocalDocument() *localFileDocument {
	if a.files.active < 0 || a.files.active >= len(a.files.docs) {
		return nil
	}
	return a.files.docs[a.files.active]
}

func (a *renewApp) closeLocalDocument(index int, discard bool) {
	if index < 0 || index >= len(a.files.docs) {
		return
	}
	doc := a.files.docs[index]
	if doc.saving {
		a.files.errorText = "正在保存，无法关闭标签"
		return
	}
	if doc.state.Buffer.Modified() && !discard {
		a.files.closePending = doc.path
		return
	}
	a.files.docs = append(a.files.docs[:index], a.files.docs[index+1:]...)
	if index < a.files.active {
		a.files.active--
	}
	if a.files.active >= len(a.files.docs) {
		a.files.active = len(a.files.docs)-1
	}
	a.files.closePending = ""
}

func (a *renewApp) saveActiveLocalDocument() {
	doc := a.currentLocalDocument()
	if doc == nil || doc.saving || !doc.state.Buffer.Modified() {
		return
	}
	snapshot := doc.state.Buffer.Snapshot()
	doc.saving = true
	a.files.errorText = ""
	go func() {
		info, err := saveLocalDocument(doc.path, doc.modTime, doc.size, doc.mode, snapshot)
		a.update(func() {
			doc.saving = false
			if err != nil {
				a.files.errorText = fmt.Sprintf("保存失败：%v", err)
				return
			}
			doc.modTime, doc.size, doc.mode = info.ModTime(), info.Size(), info.Mode()
			if doc.state.Buffer.Version() == snapshot.Version() {
				doc.state.Buffer.MarkSaved()
			}
			a.files.notice = "已保存 " + filepath.Base(doc.path)
			// Save may affect the active folder listing, but does not
			// modify the collector's configured path access policies.
		})
	}()
}

func (a *renewApp) runLocalFileAction(action func() error, success string) {
	if a.files.listLoading {
		return
	}
	a.files.listLoading = true
	a.files.errorText = ""
	dir := a.files.dir
	go func() {
		err := action()
		a.update(func() {
			a.files.listLoading = false
			if err != nil {
				a.files.errorText = fmt.Sprintf("文件操作失败：%v", err)
				return
			}
			a.files.notice = success
			a.files.pendingDelete = ""
			a.files.newName = ""
			a.browseLocalDirectory(dir)
		})
	}()
}

func (a *renewApp) localFileManagerView(c *ui.Context) {
	a.ensureLocalFiles()
	t := c.Theme()
	ui.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(14).Gap(12).Children(func() {
		ui.Row(c).Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "本机文件管理").FontSize(21).Bold()
			statusPill(c, "当前用户权限", t.Success)
			ui.Spacer(c)
			if ui.Button(c, "在编辑器打开").Clicked() {
				if path := a.selectedLocalPath(); path != "" {
					a.openSelectedLocalItem()
				}
			}
			if ui.Button(c, "转到编辑器").Clicked() {
				a.page = "编辑器"
			}
		})
		ui.Text(c, "浏览、创建、重命名与删除本地文件；不经特权后端。删除为二次确认且不递归。").FontSize(11).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "⌂").Tooltip("主目录").Clicked() {
				a.browseLocalDirectory(terminalHomeDir())
			}
			if ui.Button(c, "↑").Tooltip("上级目录").Clicked() && a.files.dir != "" {
				a.browseLocalDirectory(filepath.Dir(a.files.dir))
			}
			ui.TextInput(c, &a.files.address).Label("本地绝对路径").Grow(1).MinWidth(160)
			if ui.Button(c, "前往").Clicked() {
				a.browseLocalDirectory(a.files.address)
			}
			if ui.Button(c, "刷新").Clicked() {
				a.browseLocalDirectory(a.files.dir)
			}
		})
		ui.Row(c).Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.SearchField(c, &a.files.filter).Label("过滤当前目录文件名").Grow(1).MinWidth(180)
			ui.TextInput(c, &a.files.newName).Label("新建文件或文件夹名称").Width(220)
			if ui.Button(c, "新建文件").Clicked() {
				name, dir := strings.TrimSpace(a.files.newName), a.files.dir
				a.runLocalFileAction(func() error { return createLocalItem(dir, name, false) }, "文件已创建")
			}
			if ui.Button(c, "新建文件夹").Clicked() {
				name, dir := strings.TrimSpace(a.files.newName), a.files.dir
				a.runLocalFileAction(func() error { return createLocalItem(dir, name, true) }, "文件夹已创建")
			}
		})
		if a.files.selected != "" {
			ui.Row(c).Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "已选择 "+a.files.selected).FontSize(12).TextColor(t.TextMuted)
				ui.TextInput(c, &a.files.renameName).Label("新名称").Width(220)
				if ui.Button(c, "重命名").Clicked() {
					from, to, dir := a.files.selected, strings.TrimSpace(a.files.renameName), a.files.dir
					if doc, _ := a.localDocument(filepath.Join(dir, from)); doc != nil {
						a.files.errorText = "请先关闭对应编辑器标签再重命名"
					} else {
						a.runLocalFileAction(func() error { return renameLocalItem(dir, from, to) }, "已重命名")
					}
				}
				if ui.Button(c, "删除…").Clicked() {
					a.files.pendingDelete = a.files.selected
				}
			})
		}
		if a.files.pendingDelete != "" {
			ui.Row(c).Wrap().Gap(10).Padding(10).Radius(8).Background(t.Danger.Alpha(0.08)).Children(func() {
				ui.Text(c, "确认删除 "+a.files.pendingDelete+"？目录必须为空。").TextColor(t.Danger)
				if ui.Button(c, "确认删除").Clicked() {
					name, dir := a.files.pendingDelete, a.files.dir
					if doc, _ := a.localDocument(filepath.Join(dir, name)); doc != nil {
						a.files.errorText = "请先关闭正在编辑的文件"
						a.files.pendingDelete = ""
					} else {
						a.runLocalFileAction(func() error { return deleteLocalItem(dir, name) }, "已删除")
					}
				}
				if ui.Button(c, "取消").Clicked() {
					a.files.pendingDelete = ""
				}
			})
		}
		if a.files.quitPending {
			ui.Row(c).Wrap().Gap(9).Padding(10).Radius(8).Background(t.Danger.Alpha(0.12)).Children(func() {
				ui.Text(c, "仍有未保存的文件。退出 Renew 将丢失这些修改。").TextColor(t.Danger)
				if ui.Button(c, "继续编辑").Clicked() { a.files.quitPending = false }
				if ui.Button(c, "放弃修改并退出").Clicked() {
					a.files.allowQuit = true
					if a.win != nil { a.win.Close() }
				}
			})
		}
		if a.files.errorText != "" {
			ui.Text(c, a.files.errorText).TextColor(t.Danger).FontSize(12)
		} else if a.files.notice != "" {
			ui.Text(c, a.files.notice).TextColor(t.Success).FontSize(12)
		}
		if a.files.listTruncated {
			ui.Text(c, "当前目录项目较多，只显示首 5000 项。可以转到更具体的子目录。").TextColor(t.Warning).FontSize(12)
		}
		if a.files.listLoading {
			ui.Row(c).Gap(8).Children(func() {
				renewLoadingIndicator(c, "正在读取本地目录")
				ui.Text(c, "正在读取本地目录…")
			})
		}
		filtered := make([]localFileEntry, 0, len(a.files.items))
		needle := strings.ToLower(strings.TrimSpace(a.files.filter))
		for _, e := range a.files.items {
			if needle == "" || strings.Contains(strings.ToLower(e.name), needle) {
				filtered = append(filtered, e)
			}
		}
		ui.Row(c).Gap(10).Children(func() {
			ui.Text(c, fmt.Sprintf("%d 项", len(filtered))).FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, "双击打开 · 单击选择").FontSize(11).TextColor(t.TextMuted)
		})
		ui.List(c, nil, len(filtered), func(i int) {
			e := filtered[i]
			row := ui.Row(c).Height(37).Padding(5, 10).Gap(12).AlignItems(ui.Center).Radius(7)
			if e.name == a.files.selected {
				row.Background(t.Accent.Alpha(0.13))
			}
			row.Children(func() {
				glyph := "▤"
				if e.dir {
					glyph = "▣"
				} else if e.link {
					glyph = "↗"
				}
				ui.Text(c, glyph).Width(22).TextColor(t.Accent)
				ui.Text(c, e.name).Grow(1).SingleLine()
				kind := "文件"
				if e.dir { kind = "目录" }
				if e.link { kind = "链接" }
				ui.Text(c, kind).Width(50).FontSize(11).TextColor(t.TextMuted)
			})
			if row.Clicked() {
				a.files.selected = e.name
				a.files.renameName = e.name
				a.files.pendingDelete = ""
			}
			if row.DoubleClicked() {
				a.files.selected = e.name
				a.openSelectedLocalItem()
			}
		}).Grow(1).MinHeight(120).MinWidth(0)
	})
}

func (a *renewApp) openSelectedLocalItem() {
	path := a.selectedLocalPath()
	if path == "" {
		return
	}
	// Do not follow symlinks for the editor. Symlink directories can be
	// visited by entering their full path explicitly.
	for _, e := range a.files.items {
		if e.name != a.files.selected {
			continue
		}
		if e.link {
			a.files.errorText = "链接项不可直接打开，请输入明确的目标路径"
		} else if e.dir {
			a.browseLocalDirectory(path)
		} else {
			a.openLocalFile(path)
		}
		return
	}
}

func (a *renewApp) localEditorView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(10).Gap(8).Children(func() {
		ui.Row(c).Wrap().Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "原生代码编辑器").FontSize(19).Bold()
			statusPill(c, "MyGo ui/editor", t.Accent)
			ui.Spacer(c)
			if ui.Button(c, "文件管理").Clicked() {
				a.page = "文件"
			}
			if ui.Button(c, "保存").Clicked() {
				a.saveActiveLocalDocument()
			}
			if ui.Button(c, "关闭标签").Clicked() {
				a.closeLocalDocument(a.files.active, false)
			}
		})
		if a.files.errorText != "" {
			ui.Text(c, a.files.errorText).TextColor(t.Danger).FontSize(12)
		} else if a.files.notice != "" {
			ui.Text(c, a.files.notice).TextColor(t.Success).FontSize(12)
		}
		if len(a.files.docs) == 0 {
			ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
				ui.Text(c, "尚未打开文本文件").FontSize(18)
				ui.Text(c, "在文件管理中双击文件。编辑器支持语法高亮、查找、折叠、多光标以及撤销。").TextColor(t.TextMuted)
				if ui.PrimaryButton(c, "浏览本机文件").Clicked() {
					a.page = "文件"
				}
			})
			return
		}
		labels := make([]string, len(a.files.docs))
		for i, doc := range a.files.docs {
			labels[i] = filepath.Base(doc.path)
			if doc.state.Buffer.Modified() {
				labels[i] += " •"
			}
		}
		ui.Row(c).MinWidth(0).Children(func() {
			ui.Tabs(c, &a.files.active, labels...).Grow(1).MinWidth(0)
		})
		if a.files.closePending != "" {
			ui.Row(c).Wrap().Gap(9).Padding(9).Radius(8).Background(t.Warning.Alpha(0.12)).Children(func() {
				ui.Text(c, "文件尚未保存，丢弃修改并关闭？").TextColor(t.Warning)
				if ui.Button(c, "丢弃修改并关闭").Clicked() {
					if _, idx := a.localDocument(a.files.closePending); idx >= 0 {
						a.closeLocalDocument(idx, true)
					}
					a.files.closePending = ""
				}
				if ui.Button(c, "取消").Clicked() {
					a.files.closePending = ""
				}
			})
		}
		doc := a.currentLocalDocument()
		if doc == nil {
			return
		}
		ui.Text(c, doc.path).Font("monospace").FontSize(11).TextColor(t.TextMuted).SingleLine()
		editor.View(c, doc.state).Grow(1).MinHeight(0).FillWidth()
		ui.Row(c).Gap(14).Padding(4, 8).Children(func() {
			pos := doc.state.CursorPos()
			ui.Text(c, fmt.Sprintf("行 %d · 列 %d", pos.Line+1, pos.Col+1)).FontSize(11).TextColor(t.TextMuted)
			if lang := doc.state.Buffer.Language(); lang != nil {
				ui.Text(c, lang.Name).FontSize(11).TextColor(t.TextMuted)
			}
			if doc.state.Buffer.Modified() { statusPill(c, "未保存", t.Warning) }
			if doc.saving { statusPill(c, "保存中", t.Accent) }
			ui.Spacer(c)
			ui.Text(c, fmt.Sprintf("%d 行 · UTF-8", doc.state.Buffer.LineCount())).FontSize(11).TextColor(t.TextMuted)
		})
	})
}
