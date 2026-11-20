package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (m *model) seedFromProcessEnv() {
	for _, key := range allKeys() {
		if value, ok := os.LookupEnv(key); ok && value != "" {
			m.values[key] = value
		}
	}
}

func (m *model) applyRecommendedDefaults() {
	defaults := map[string]string{
		"DISABLE_AUTH":                    "true",
		"GIN_MODE":                        "debug",
		"AGENT_EBPF_DEV_SESSION":          "agent-ebpf-dev",
		"AGENT_BUILD_FEATURES":            "all",
		"AGENT_FRONTEND_BUILD_FEATURES":   "all",
		"DEV_CONTAINER":                   "agent-ebpf-filiter-dev",
		"DEV_WORKSPACE":                   "/workspaces/agent-ebpf-filiter",
		"DEVCONTAINER_POSTCREATE_INSTALL": "0",
	}
	if cudaDefault() != "" {
		defaults["CUDA_GO_TAGS"] = cudaDefault()
	}
	for key, value := range defaults {
		if strings.TrimSpace(m.values[key]) == "" {
			m.values[key] = value
		}
	}
	if strings.TrimSpace(m.values["CONTAINER_CLI"]) == "" {
		m.values["CONTAINER_CLI"] = detectContainerCLI()
	}
	m.applyDetectedDevcontainerDefaults()
}

func (m *model) applyDetectedDevcontainerDefaults() {
	branch := strings.TrimSpace(runGit(m.root, "branch", "--show-current"))
	ownerRepo := detectOwnerRepo(m.root)
	if strings.TrimSpace(m.values["DEV_BRANCH"]) == "" && branch != "" {
		m.values["DEV_BRANCH"] = branch
	}
	if strings.TrimSpace(m.values["DEV_IMAGE_REPOSITORY"]) == "" && ownerRepo != "" {
		m.values["DEV_IMAGE_REPOSITORY"] = "ghcr.io/" + ownerRepo + "/devcontainer"
	}
	if strings.TrimSpace(m.values["DEV_IMAGE_TAG"]) == "" && branch != "" {
		m.values["DEV_IMAGE_TAG"] = branchTag(branch)
	}
	if strings.TrimSpace(m.values["DEV_IMAGE"]) == "" && m.values["DEV_IMAGE_REPOSITORY"] != "" && m.values["DEV_IMAGE_TAG"] != "" {
		m.values["DEV_IMAGE"] = m.values["DEV_IMAGE_REPOSITORY"] + ":" + m.values["DEV_IMAGE_TAG"]
	}
}

func (m *model) loadEnvFile() error {
	if !fileExists(m.shellEnv) {
		return nil
	}
	var script strings.Builder
	script.WriteString("set -a\n. \"$DEV_ENV_TUI_FILE\"\nset +a\n")
	script.WriteString("for key in")
	for _, key := range allKeys() {
		script.WriteByte(' ')
		script.WriteString(key)
	}
	script.WriteString("; do printf '%s=%s\\000' \"$key\" \"${!key-}\"; done\n")
	cmd := exec.Command("bash", "-lc", script.String())
	cmd.Env = append(os.Environ(), "DEV_ENV_TUI_FILE="+m.shellEnv)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	for _, part := range bytes.Split(out, []byte{0}) {
		if len(part) == 0 {
			continue
		}
		key, value, ok := strings.Cut(string(part), "=")
		if ok && value != "" {
			m.values[key] = value
		}
	}
	return nil
}

func (m *model) run() error {
	applyTheme()
	m.app = tview.NewApplication().EnableMouse(true)
	m.pages = tview.NewPages()
	m.list = tview.NewList().ShowSecondaryText(true)
	m.list.SetBorder(true)
	m.list.SetTitle(" Groups  Ctrl+G / ↑/↓ / wheel / click ")
	styleBox(m.list.Box)
	m.list.SetMainTextColor(themeText).
		SetSecondaryTextColor(themeMuted).
		SetShortcutColor(themeAccent).
		SetSelectedStyle(tcell.StyleDefault.Foreground(themeBg).Background(themeAccent).Bold(true)).
		SetSelectedTextColor(themeBg).
		SetSelectedBackgroundColor(themeAccent).
		SetHighlightFullLine(true)
	m.installListNavigation()
	m.form = tview.NewForm()
	m.form.SetBorder(true)
	styleBox(m.form.Box)
	styleForm(m.form)
	m.installFormNavigation()
	m.status = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetTextColor(themeText)
	m.status.SetBorder(true)
	m.status.SetTitle(" Status / Help ")
	styleBox(m.status.Box)

	for i, group := range groups {
		idx := i
		shortcut := rune(0)
		if i < 9 {
			shortcut = rune('1' + i)
		}
		m.list.AddItem(group.Title, group.Desc, shortcut, func() {
			m.selectGroup(idx, true)
		})
	}
	m.list.SetChangedFunc(func(index int, _ string, _ string, _ rune) {
		m.selectGroup(index, false)
	})
	m.rebuildForm()
	m.setStatus(successText("Ready.") + " ↑/↓ moves the focused panel; mouse wheel moves the panel under cursor; click groups/fields/buttons; Ctrl+G groups, Ctrl+F fields, Ctrl+S save, Ctrl+D doctor, Ctrl+P preview, Ctrl+Q quit.")

	header := tview.NewTextView().SetDynamicColors(true).SetTextColor(themeText)
	header.SetText(titleText("Agent eBPF Filter Dev Env TUI") + "\n" + mutedText("Edit .env.dev / .env.dev.mk for local development, LLM, and application behavior. Mouse enabled."))
	header.SetBorder(true)
	styleBox(header.Box)

	mainFlex := tview.NewFlex().
		AddItem(m.list, 32, 1, true).
		AddItem(m.form, 0, 3, false)
	rootFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 4, 0, false).
		AddItem(mainFlex, 0, 1, true).
		AddItem(m.status, 7, 0, false)

	m.pages.AddPage("main", rootFlex, true, true)
	m.app.SetRoot(m.pages, true)
	m.app.SetFocus(m.list)
	m.app.SetInputCapture(m.captureKey)
	return m.app.Run()
}

func (m *model) installListNavigation() {
	m.list.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if event == nil || !m.list.InRect(event.Position()) {
			return action, event
		}
		switch action {
		case tview.MouseScrollUp:
			m.moveGroupSelection(-1)
			return tview.MouseConsumed, nil
		case tview.MouseScrollDown:
			m.moveGroupSelection(1)
			return tview.MouseConsumed, nil
		}
		return action, event
	})
}

func (m *model) installFormNavigation() {
	m.form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyUp:
			m.moveFormFocus(-1)
			return nil
		case tcell.KeyDown:
			m.moveFormFocus(1)
			return nil
		}
		return event
	})
	m.form.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if event == nil || !m.form.InRect(event.Position()) {
			return action, event
		}
		switch action {
		case tview.MouseScrollUp:
			m.moveFormFocus(-1)
			return tview.MouseConsumed, nil
		case tview.MouseScrollDown:
			m.moveFormFocus(1)
			return tview.MouseConsumed, nil
		}
		return action, event
	})
}

func (m *model) selectGroup(index int, focusForm bool) {
	if index < 0 || index >= len(groups) {
		return
	}
	if m.selected != index || m.form.GetFormItemCount() == 0 {
		m.selected = index
		m.rebuildForm()
	}
	if focusForm && m.app != nil {
		m.app.SetFocus(m.form)
	}
}

func (m *model) moveGroupSelection(delta int) {
	if m.list == nil || len(groups) == 0 {
		return
	}
	index := m.list.GetCurrentItem()
	if index < 0 || index >= len(groups) {
		index = m.selected
	}
	index += delta
	if index < 0 {
		index = 0
	} else if index >= len(groups) {
		index = len(groups) - 1
	}
	m.list.SetCurrentItem(index)
	if m.selected != index {
		m.selected = index
		m.rebuildForm()
	}
	if m.app != nil {
		m.app.SetFocus(m.list)
	}
}

func (m *model) moveFormFocus(delta int) {
	if m.form == nil {
		return
	}
	total := m.form.GetFormItemCount() + m.form.GetButtonCount()
	if total == 0 {
		return
	}
	item, button := m.form.GetFocusedItemIndex()
	index := 0
	if item >= 0 {
		index = item
	} else if button >= 0 {
		index = m.form.GetFormItemCount() + button
	}
	index += delta
	if index < 0 {
		index = 0
	} else if index >= total {
		index = total - 1
	}
	m.form.SetFocus(index)
	if m.app != nil {
		m.app.SetFocus(m.form)
	}
}

func (m *model) captureKey(event *tcell.EventKey) *tcell.EventKey {
	if m.modalOpen && event.Key() == tcell.KeyEsc {
		m.closeModal()
		return nil
	}
	switch event.Key() {
	case tcell.KeyCtrlS:
		m.saveFromUI()
		return nil
	case tcell.KeyCtrlD:
		m.showModal("Doctor", m.doctorText())
		return nil
	case tcell.KeyCtrlP:
		m.showModal("Preview", m.previewText(true))
		return nil
	case tcell.KeyCtrlQ:
		m.app.Stop()
		return nil
	case tcell.KeyCtrlG:
		m.app.SetFocus(m.list)
		return nil
	case tcell.KeyCtrlF:
		m.app.SetFocus(m.form)
		return nil
	}
	return event
}

func (m *model) rebuildForm() {
	group := groups[m.selected]
	m.form.Clear(true)
	styleForm(m.form)
	m.form.SetTitle(" " + group.Title + "  ↑/↓ / wheel / click fields ")
	for _, item := range group.Vars {
		item := item
		label := fmt.Sprintf("%s (%s)", item.Label, item.Key)
		value := m.values[item.Key]
		if item.Secret {
			m.form.AddPasswordField(label, value, 48, '*', func(text string) {
				m.setValue(item.Key, text)
			})
		} else {
			m.form.AddInputField(label, value, 48, nil, func(text string) {
				m.setValue(item.Key, text)
			})
		}
	}
	m.form.AddButton("Save", m.saveFromUI)
	m.form.AddButton("Doctor", func() { m.showModal("Doctor", m.doctorText()) })
	m.form.AddButton("Preview", func() { m.showModal("Preview", m.previewText(true)) })
	m.form.AddButton("Quit", func() { m.app.Stop() })
	m.form.SetButtonsAlign(tview.AlignRight)
	m.setStatus(fmt.Sprintf("%s: %s\n%s", warnText(group.Title), group.Desc, mutedText("Tip: ↑/↓ or mouse wheel moves between fields. "+firstHint(group))))
}

func styleForm(form *tview.Form) {
	form.SetLabelColor(themeText)
	form.SetFieldStyle(tcell.StyleDefault.Foreground(themeText).Background(themePanelAlt))
	form.SetFieldTextColor(themeText)
	form.SetFieldBackgroundColor(themePanelAlt)
	form.SetButtonStyle(tcell.StyleDefault.Foreground(themeText).Background(themeButton).Bold(true))
	form.SetButtonActivatedStyle(tcell.StyleDefault.Foreground(themeBg).Background(themeAccent).Bold(true))
	form.SetButtonDisabledStyle(tcell.StyleDefault.Foreground(themeMuted).Background(themePanelAlt))
}

func (m *model) setValue(key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		delete(m.values, key)
		return
	}
	m.values[key] = value
}

func (m *model) saveFromUI() {
	if err := m.writeFiles(); err != nil {
		m.setStatus(errorText("Save failed:") + " " + err.Error())
		return
	}
	m.lastSavedAt = time.Now().Format("15:04:05")
	m.setStatus(fmt.Sprintf("%s %s and %s", successText("Saved at "+m.lastSavedAt+"."), rel(m.root, m.shellEnv), rel(m.root, m.makeEnv)))
}

func (m *model) setStatus(text string) {
	m.status.SetText(text)
}

func (m *model) showModal(title, content string) {
	view := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(false).SetTextColor(themeText)
	view.SetBorder(true)
	view.SetTitle(" " + title + "  Esc closes / mouse wheel scrolls ")
	styleBox(view.Box)
	view.SetText(content)
	view.SetDoneFunc(func(key tcell.Key) {
		m.closeModal()
	})
	m.modalOpen = true
	m.pages.AddPage("modal", centered(view, 90, 28), true, true)
	m.app.SetFocus(view)
}

func (m *model) closeModal() {
	m.pages.RemovePage("modal")
	m.modalOpen = false
	m.app.SetFocus(m.form)
}

func centered(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
}
