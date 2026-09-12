package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"agent-ebpf-filter/pb"
)

// Palette matches tools/dev-env-tui so the two TUIs feel like one product.
var (
	themeBg       = tcell.NewRGBColor(0x07, 0x12, 0x1f)
	themePanel    = tcell.NewRGBColor(0x0f, 0x1b, 0x2d)
	themePanelAlt = tcell.NewRGBColor(0x1e, 0x29, 0x3b)
	themeBorder   = tcell.NewRGBColor(0x38, 0xbd, 0xf8)
	themeTitle    = tcell.NewRGBColor(0x7d, 0xd3, 0xfc)
	themeText     = tcell.NewRGBColor(0xe2, 0xe8, 0xf0)
	themeMuted    = tcell.NewRGBColor(0x94, 0xa3, 0xb8)
	themeAccent   = tcell.NewRGBColor(0x22, 0xd3, 0xee)
	themeWarning  = tcell.NewRGBColor(0xfb, 0xbf, 0x24)
	themeSuccess  = tcell.NewRGBColor(0x22, 0xc5, 0x5e)
	themeError    = tcell.NewRGBColor(0xf8, 0x71, 0x71)
	themeProcess  = tcell.NewRGBColor(0x86, 0xef, 0xac)
	themeNetwork  = tcell.NewRGBColor(0x67, 0xe8, 0xf9)
	themeFile     = tcell.NewRGBColor(0xc4, 0xb5, 0xfd)
)

func applyTheme() {
	tview.Styles.PrimitiveBackgroundColor = themeBg
	tview.Styles.ContrastBackgroundColor = themePanelAlt
	tview.Styles.MoreContrastBackgroundColor = themeAccent
	tview.Styles.BorderColor = themeBorder
	tview.Styles.TitleColor = themeTitle
	tview.Styles.GraphicsColor = themeBorder
	tview.Styles.PrimaryTextColor = themeText
	tview.Styles.SecondaryTextColor = themeMuted
	tview.Styles.TertiaryTextColor = themeAccent
	tview.Styles.InverseTextColor = themeBg
	tview.Styles.ContrastSecondaryTextColor = themeText
}

func colorTag(color tcell.Color) string {
	r, g, b := color.RGB()
	return fmt.Sprintf("[#%02x%02x%02x]", r, g, b)
}

func boldTag(color tcell.Color) string {
	r, g, b := color.RGB()
	return fmt.Sprintf("[#%02x%02x%02x::b]", r, g, b)
}

const resetTag = "[-:-:-]"

// safeText makes untrusted event data inert for tview: style tags are
// escaped and control characters (which could otherwise reach the terminal)
// are replaced.
func safeText(s string) string {
	if s == "" {
		return ""
	}
	clean := true
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c == 0x7f || c == '[' {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7f:
			b.WriteRune('·')
		default:
			b.WriteRune(r)
		}
	}
	return tview.Escape(b.String())
}

// ── Table columns ────────────────────────────────────────────────────────

type column struct {
	title    string
	maxWidth int
	align    int
	expand   int
}

var columns = []column{
	{title: "TIME", maxWidth: 12},
	{title: "TYPE", maxWidth: 18},
	{title: "PID", maxWidth: 7, align: tview.AlignRight},
	{title: "COMM", maxWidth: 16},
	{title: "TAG", maxWidth: 14},
	{title: "RISK", maxWidth: 6, align: tview.AlignRight},
	{title: "TARGET", maxWidth: 120, expand: 1},
}

// eventContent is a virtual tview.TableContent over the model snapshot. Only
// the cells tview asks for (the visible window) are materialised, so a
// 5000-row history costs nothing until it scrolls into view.
type eventContent struct {
	tview.TableContentReadOnly
	rows []entry
}

func (c *eventContent) GetRowCount() int    { return len(c.rows) + 1 }
func (c *eventContent) GetColumnCount() int { return len(columns) }

func (c *eventContent) GetCell(row, col int) *tview.TableCell {
	if col < 0 || col >= len(columns) {
		return nil
	}
	spec := columns[col]
	if row == 0 {
		return tview.NewTableCell(spec.title).
			SetTextColor(themeTitle).
			SetAttributes(tcell.AttrBold).
			SetAlign(spec.align).
			SetMaxWidth(spec.maxWidth).
			SetExpansion(spec.expand).
			SetSelectable(false)
	}
	row--
	if row < 0 || row >= len(c.rows) {
		return nil
	}
	e := c.rows[row]
	text, color := cellText(e, col)
	return tview.NewTableCell(text).
		SetTextColor(color).
		SetAlign(spec.align).
		SetMaxWidth(spec.maxWidth).
		SetExpansion(spec.expand).
		SetReference(e.ev)
}

func cellText(e entry, col int) (string, tcell.Color) {
	ev := e.ev
	switch col {
	case 0:
		return e.at.Format("15:04:05.000"), themeMuted
	case 1:
		return safeText(ev.GetType()), typeColor(ev.GetType())
	case 2:
		return fmt.Sprint(ev.GetPid()), themeText
	case 3:
		return safeText(ev.GetComm()), themeText
	case 4:
		return safeText(ev.GetTag()), themeMuted
	case 5:
		return riskLabel(ev)
	default:
		return safeText(targetText(ev)), themeText
	}
}

func targetText(ev *pb.Event) string {
	if endpoint := ev.GetNetEndpoint(); endpoint != "" {
		if domain := ev.GetDomain(); domain != "" && !strings.Contains(endpoint, domain) {
			return endpoint + "  " + domain
		}
		return endpoint
	}
	if path := ev.GetPath(); path != "" {
		if extra := ev.GetExtraPath(); extra != "" && (ev.GetType() == "rename" || ev.GetType() == "link" || ev.GetType() == "symlink") {
			return path + " → " + extra
		}
		return path
	}
	return ev.GetExtraInfo()
}

func riskLabel(ev *pb.Event) (string, tcell.Color) {
	switch decisionClass(ev.GetDecision()) {
	case decisionBlock:
		return "BLOCK", themeError
	case decisionAlert:
		return "ALERT", themeWarning
	}
	score := ev.GetRiskScore()
	switch {
	case score >= 70:
		return fmt.Sprintf("%.0f", score), themeError
	case score >= riskyThreshold:
		return fmt.Sprintf("%.0f", score), themeWarning
	case score > 0:
		return fmt.Sprintf("%.0f", score), themeMuted
	default:
		return "", themeMuted
	}
}

func typeColor(eventType string) tcell.Color {
	switch eventType {
	case "execve", "process_exec", "process_fork", "process_exit", "clone", "exit", "wait4":
		return themeProcess
	case "semantic_alert":
		return themeWarning
	}
	if strings.HasPrefix(eventType, "network_") || strings.HasPrefix(eventType, "tcp_") ||
		eventType == "dns_query" || eventType == "socket" || eventType == "accept" || eventType == "accept4" {
		return themeNetwork
	}
	return themeFile
}

// ── UI ───────────────────────────────────────────────────────────────────

// viewKind selects which stream the main table shows.
type viewKind int

const (
	viewEvents viewKind = iota
	viewTLS
)

func (v viewKind) String() string {
	if v == viewTLS {
		return "tls"
	}
	return "events"
}

// tableView is one stream's table plus its follow/pause state, so both the
// kernel event view and the TLS capture view share the same key handling.
type tableView struct {
	table  *tview.Table
	follow bool
	// stale forces one re-snapshot on the next refresh even while paused,
	// so a filter change or clear is reflected immediately.
	stale bool
	// programmatic suppresses the "user moved the selection" pause trigger
	// while the code itself repositions the cursor.
	programmatic bool
}

func (v *tableView) selectRow(row int) {
	v.programmatic = true
	v.table.Select(row, 0)
	v.programmatic = false
}

func (v *tableView) trackEnd(rows int) {
	if v.follow && rows > 0 {
		v.selectRow(rows)
		v.table.ScrollToEnd()
	}
}

type UI struct {
	app      *tview.Application
	model    *Model
	tlsModel *TLSModel
	cfg      Config

	content    *eventContent
	tlsContent *tlsContent
	events     tableView
	tls        tableView
	active     viewKind

	body    *tview.Pages
	header  *tview.TextView
	sidebar *tview.TextView
	footer  *tview.TextView
	filter  *tview.InputField
	bottom  *tview.Pages
	pages   *tview.Pages
	detail  *tview.TextView

	sidebarMode int
}

func NewUI(cfg Config, model *Model, tlsModel *TLSModel) *UI {
	applyTheme()
	ui := &UI{
		app:        tview.NewApplication(),
		model:      model,
		tlsModel:   tlsModel,
		cfg:        cfg,
		content:    &eventContent{},
		tlsContent: &tlsContent{},
	}
	ui.build()
	return ui
}

// current returns the table view the user is looking at.
func (ui *UI) current() *tableView {
	if ui.active == viewTLS {
		return &ui.tls
	}
	return &ui.events
}

func styleBox(box *tview.Box, title string) {
	box.SetBorder(true).
		SetBackgroundColor(themePanel).
		SetBorderColor(themeBorder).
		SetTitleColor(themeTitle).
		SetTitle(" " + title + " ")
}

// newStreamTable builds a virtual table wired to view's follow state.
func (ui *UI) newStreamTable(view *tableView, content tview.TableContent, title string, onSelect func(row int)) {
	view.follow = true
	view.table = tview.NewTable().
		SetContent(content).
		SetFixed(1, 0).
		SetSelectable(true, false).
		SetSelectedStyle(tcell.StyleDefault.Background(themePanelAlt).Foreground(themeText).Bold(true)).
		SetSeparator(' ')
	styleBox(view.table.Box, title)
	view.table.SetSelectionChangedFunc(func(row, _ int) {
		if !view.programmatic && view.follow {
			view.follow = false
		}
	})
	view.table.SetSelectedFunc(func(row, _ int) { onSelect(row) })
	view.table.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseScrollUp || action == tview.MouseScrollDown {
			view.follow = false
		}
		return action, event
	})
}

func (ui *UI) build() {
	ui.header = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	ui.header.SetBackgroundColor(themePanel)

	ui.newStreamTable(&ui.events, ui.content, "Events  Tab switches view", ui.showDetail)
	ui.newStreamTable(&ui.tls, ui.tlsContent, "TLS capture  Tab switches view", ui.showTLSDetail)
	ui.body = tview.NewPages().
		AddPage(viewEvents.String(), ui.events.table, true, true).
		AddPage(viewTLS.String(), ui.tls.table, true, false)

	ui.sidebar = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	styleBox(ui.sidebar.Box, "Overview")

	ui.footer = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	ui.footer.SetBackgroundColor(themePanel)

	ui.filter = tview.NewInputField().
		SetLabel(" filter › ").
		SetLabelColor(themeAccent).
		SetFieldBackgroundColor(themePanelAlt).
		SetFieldTextColor(themeText).
		SetPlaceholder(`node  type:openat  comm:python  pid:1234  tag:"AI Agent"  risk:>=40  -type:read`).
		SetPlaceholderTextColor(themeMuted)
	ui.filter.SetBackgroundColor(themePanel)
	ui.filter.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			ui.setFilter(ParseFilter(ui.filter.GetText()))
		case tcell.KeyEscape:
			ui.filter.SetText(ui.activeFilter().Raw)
		}
		ui.bottom.SwitchToPage("keys")
		ui.app.SetFocus(ui.current().table)
		ui.refresh()
	})

	ui.bottom = tview.NewPages().
		AddPage("keys", ui.footer, true, true).
		AddPage("filter", ui.filter, true, false)

	body := tview.NewFlex().
		AddItem(ui.body, 0, 1, true).
		AddItem(ui.sidebar, 36, 0, false)

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.header, 1, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(ui.bottom, 1, 0, false)

	ui.detail = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	styleBox(ui.detail.Box, "Event detail  Esc closes")
	ui.detail.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyEnter || event.Rune() == 'q' {
			ui.closeOverlay("detail")
			return nil
		}
		return event
	})

	ui.pages = tview.NewPages().
		AddPage("main", root, true, true).
		AddPage("detail", centered(ui.detail, 100, 12), true, false).
		AddPage("help", centered(ui.helpView(), 76, 22), true, false)

	ui.app.SetRoot(ui.pages, true).EnableMouse(true).SetInputCapture(ui.handleKey)
	ui.app.SetFocus(ui.events.table)
	ui.refresh()
}

// activeFilter returns the filter of the view being shown.
func (ui *UI) activeFilter() Filter {
	if ui.active == viewTLS {
		return ui.tlsModel.Summary(0).Filter
	}
	return ui.model.Summary(time.Now(), 0).Filter
}

func (ui *UI) setFilter(f Filter) {
	ui.current().stale = true
	if ui.active == viewTLS {
		ui.tlsModel.SetFilter(f)
		return
	}
	ui.model.SetFilter(f)
}

// switchView flips between the kernel event table and the TLS table.
func (ui *UI) switchView(kind viewKind) {
	ui.active = kind
	ui.body.SwitchToPage(kind.String())
	ui.filter.SetText(ui.activeFilter().Raw)
	if kind == viewTLS {
		ui.filter.SetPlaceholder(`api.anthropic.com  host:openai  method:POST  status:4  comm:node  vendor:anthropic  -type:sse_message`)
	} else {
		ui.filter.SetPlaceholder(`node  type:openat  comm:python  pid:1234  tag:"AI Agent"  risk:>=40  -type:read`)
	}
	ui.app.SetFocus(ui.current().table)
	ui.refresh()
}

func centered(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 0, true).
			AddItem(nil, 0, 1, false), width, 0, true).
		AddItem(nil, 0, 1, false)
}

func (ui *UI) helpView() *tview.TextView {
	help := tview.NewTextView().SetDynamicColors(true)
	styleBox(help.Box, "Keys  Esc closes")
	accent := colorTag(themeAccent)
	lines := []string{
		"", accent + "  Tab  1  2[-]  switch events / TLS  " + accent + "Esc[-]     clear filter / close",
		accent + "  /[-]         edit filter          " + accent + "?[-]       this help",
		accent + "  Enter[-]     event detail          " + accent + "End  G[-]  jump to newest, follow",
		accent + "  Space[-]     pause / follow        " + accent + "Home[-]    jump to oldest",
		accent + "  ↑ ↓ PgUp[-]  browse (pauses)       " + accent + "c[-]       clear history",
		accent + "  s[-]         cycle sidebar         " + accent + "q  Ctrl+C[-] quit",
		"",
		colorTag(themeMuted) + "  Filters are per view and ANDed; prefix a term with - to negate.",
		"  Events: free text matches comm, type, path, endpoint, domain, tag,",
		"  extra info; keys type: comm: path: tag: net: decision: tool: run:",
		"  pid: ppid: uid: risk:>=N.  TLS: free text matches host, url, comm,",
		"  method, type, vendor; keys host: url: comm: method: status: type:",
		"  vendor: dir: tool: run: pid:.[-]",
	}
	help.SetText(strings.Join(lines, "\n"))
	help.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEscape, event.Key() == tcell.KeyEnter, event.Rune() == 'q', event.Rune() == '?':
			ui.closeOverlay("help")
		}
		return nil
	})
	return help
}

func (ui *UI) handleKey(event *tcell.EventKey) *tcell.EventKey {
	if name, _ := ui.pages.GetFrontPage(); name != "main" {
		return event
	}
	if ui.app.GetFocus() == ui.filter {
		return event
	}
	view := ui.current()
	switch event.Key() {
	case tcell.KeyCtrlC:
		ui.app.Stop()
		return nil
	case tcell.KeyTab, tcell.KeyBacktab:
		if ui.active == viewEvents {
			ui.switchView(viewTLS)
		} else {
			ui.switchView(viewEvents)
		}
		return nil
	case tcell.KeyEscape:
		if !ui.activeFilter().IsZero() {
			ui.filter.SetText("")
			ui.setFilter(Filter{})
			ui.refresh()
		}
		return nil
	case tcell.KeyEnd:
		view.follow = true
		ui.refresh()
		return nil
	case tcell.KeyHome:
		view.follow = false
		view.selectRow(1)
		return nil
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyPgUp, tcell.KeyPgDn:
		view.follow = false
		return event
	}
	switch event.Rune() {
	case 'q':
		ui.app.Stop()
	case '1':
		ui.switchView(viewEvents)
	case '2':
		ui.switchView(viewTLS)
	case '/':
		ui.bottom.SwitchToPage("filter")
		ui.app.SetFocus(ui.filter)
	case ' ':
		view.follow = !view.follow
		ui.refresh()
	case 'G':
		view.follow = true
		ui.refresh()
	case 'c':
		if ui.active == viewTLS {
			ui.tlsModel.Clear()
			ui.tlsContent.rows = ui.tlsContent.rows[:0]
		} else {
			ui.model.Clear()
			ui.content.rows = ui.content.rows[:0]
		}
		view.follow = true
		ui.refresh()
	case 's':
		ui.sidebarMode = (ui.sidebarMode + 1) % 3
		ui.refresh()
	case '?':
		ui.pages.ShowPage("help")
		ui.app.SetFocus(ui.pages)
	default:
		return event
	}
	return nil
}

func (ui *UI) closeOverlay(name string) {
	ui.pages.HidePage(name)
	ui.app.SetFocus(ui.current().table)
}

// refresh re-reads the models. While a view follows, its table shows the
// live snapshot and tracks the newest row; while paused, the rows the user
// is browsing stay exactly where they are even though the ring keeps
// filling. Both tables are refreshed so switching views is instant.
func (ui *UI) refresh() {
	now := time.Now()
	summary := ui.model.Summary(now, 8)
	tlsSummary := ui.tlsModel.Summary(8)
	if ui.events.follow || ui.events.stale {
		ui.content.rows = ui.model.Snapshot(ui.content.rows)
		ui.events.trackEnd(len(ui.content.rows))
		ui.events.stale = false
	}
	if ui.tls.follow || ui.tls.stale {
		ui.tlsContent.rows = ui.tlsModel.Snapshot(ui.tlsContent.rows)
		ui.tls.trackEnd(len(ui.tlsContent.rows))
		ui.tls.stale = false
	}
	if ui.active == viewTLS {
		ui.header.SetText(ui.renderTLSHeader(tlsSummary, summary))
		ui.sidebar.SetText(ui.renderTLSSidebar(tlsSummary))
	} else {
		ui.header.SetText(ui.renderHeader(summary, now))
		ui.sidebar.SetText(ui.renderSidebar(summary))
	}
	ui.footer.SetText(ui.renderFooter())
}

func (ui *UI) renderTLSHeader(s TLSSummary, events Summary) string {
	var b strings.Builder
	switch s.State {
	case StateConnected:
		b.WriteString(boldTag(themeSuccess) + " ● " + resetTag)
	case StateConnecting:
		b.WriteString(boldTag(themeWarning) + " ◌ " + resetTag)
	default:
		b.WriteString(boldTag(themeError) + " ○ " + resetTag)
	}
	fmt.Fprintf(&b, "%s%s%s  %sTLS ", colorTag(themeText), safeText(ui.cfg.BackendURL), resetTag, colorTag(themeMuted))
	if s.State == StateConnected {
		b.WriteString("live")
	} else if s.StateDetail != "" {
		b.WriteString(safeText(s.StateDetail))
	} else {
		b.WriteString(s.State.String())
	}
	fmt.Fprintf(&b, "%s   %scaptured%s %s%d%s", resetTag, colorTag(themeMuted), resetTag, colorTag(themeText), s.Total, resetTag)
	fmt.Fprintf(&b, "   %sreq/resp%s %s%d/%d%s", colorTag(themeMuted), resetTag, colorTag(themeAccent), s.Requests, s.Responses, resetTag)
	fmt.Fprintf(&b, "   %skernel events%s %s%d%s", colorTag(themeMuted), resetTag, colorTag(themeText), events.Total, resetTag)
	if !s.Filter.IsZero() {
		fmt.Fprintf(&b, "   %sfilter%s %s%s%s", colorTag(themeMuted), resetTag, colorTag(themeAccent), safeText(s.Filter.Raw), resetTag)
	}
	if ui.tls.follow {
		fmt.Fprintf(&b, "   %s▶ follow%s", colorTag(themeSuccess), resetTag)
	} else {
		fmt.Fprintf(&b, "   %s‖ paused%s", colorTag(themeWarning), resetTag)
	}
	return b.String()
}

func (ui *UI) renderHeader(s Summary, now time.Time) string {
	var b strings.Builder
	switch s.State {
	case StateConnected:
		b.WriteString(boldTag(themeSuccess) + " ● " + resetTag)
	case StateConnecting:
		b.WriteString(boldTag(themeWarning) + " ◌ " + resetTag)
	default:
		b.WriteString(boldTag(themeError) + " ○ " + resetTag)
	}
	fmt.Fprintf(&b, "%s%s%s  %s", colorTag(themeText), safeText(ui.cfg.BackendURL), resetTag, colorTag(themeMuted))
	if s.State == StateConnected {
		b.WriteString("live")
	} else if s.StateDetail != "" {
		b.WriteString(safeText(s.StateDetail))
	} else {
		b.WriteString(s.State.String())
	}
	fmt.Fprintf(&b, "%s   %sevents%s %s%d%s", resetTag, colorTag(themeMuted), resetTag, colorTag(themeText), s.Total, resetTag)
	fmt.Fprintf(&b, "   %srate%s %s%.0f/s%s", colorTag(themeMuted), resetTag, colorTag(themeAccent), s.Rate10s, resetTag)
	if s.Risky > 0 {
		fmt.Fprintf(&b, "   %srisky%s %s%d%s", colorTag(themeMuted), resetTag, colorTag(themeWarning), s.Risky, resetTag)
	}
	if s.Blocked > 0 {
		fmt.Fprintf(&b, "   %sblocked%s %s%d%s", colorTag(themeMuted), resetTag, colorTag(themeError), s.Blocked, resetTag)
	}
	if !s.Filter.IsZero() {
		fmt.Fprintf(&b, "   %sfilter%s %s%s%s", colorTag(themeMuted), resetTag, colorTag(themeAccent), safeText(s.Filter.Raw), resetTag)
	}
	if ui.events.follow {
		fmt.Fprintf(&b, "   %s▶ follow%s", colorTag(themeSuccess), resetTag)
	} else {
		fmt.Fprintf(&b, "   %s‖ paused%s", colorTag(themeWarning), resetTag)
	}
	return b.String()
}

var sparkRunes = []rune("▁▂▃▄▅▆▇█")

func sparkline(values []uint32) string {
	var peak uint32
	for _, v := range values {
		peak = max(peak, v)
	}
	var b strings.Builder
	for _, v := range values {
		if v == 0 {
			b.WriteRune(' ')
			continue
		}
		idx := int(uint64(v) * uint64(len(sparkRunes)-1) / uint64(peak))
		b.WriteRune(sparkRunes[idx])
	}
	return b.String()
}

func (ui *UI) renderSidebar(s Summary) string {
	var b strings.Builder
	muted, text, accent := colorTag(themeMuted), colorTag(themeText), colorTag(themeAccent)
	fmt.Fprintf(&b, "%sthroughput%s\n", boldTag(themeTitle), resetTag)
	fmt.Fprintf(&b, "%s%s%s\n", accent, sparkline(s.Sparkline), resetTag)
	fmt.Fprintf(&b, "%snow %s%.0f/s%s  %s10s avg %s%.1f/s%s\n\n", muted, text, s.Rate1s, resetTag, muted, text, s.Rate10s, resetTag)

	fmt.Fprintf(&b, "%shistory%s\n", boldTag(themeTitle), resetTag)
	fmt.Fprintf(&b, "%sretained %s%d%s / %d", muted, text, s.Retained, resetTag, s.Capacity)
	if s.Evicted > 0 {
		fmt.Fprintf(&b, "  %sevicted %d%s", muted, s.Evicted, resetTag)
	}
	b.WriteString("\n")
	if !s.LastEventAt.IsZero() {
		fmt.Fprintf(&b, "%slast event %s%s%s\n", muted, text, s.LastEventAt.Format("15:04:05"), resetTag)
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "%srisk%s\n", boldTag(themeTitle), resetTag)
	fmt.Fprintf(&b, "%srisky %s%d%s   %sblocked %s%d%s\n\n", muted, colorTag(themeWarning), s.Risky, resetTag, muted, colorTag(themeError), s.Blocked, resetTag)

	title, items := "top event types", s.TopTypes
	switch ui.sidebarMode {
	case 1:
		title, items = "top processes", s.TopComms
	case 2:
		title, items = "top tags", s.TopTags
	}
	fmt.Fprintf(&b, "%s%s%s  %s(s cycles)%s\n", boldTag(themeTitle), title, resetTag, muted, resetTag)
	writeHistogram(&b, items, 32, 16)
	return b.String()
}

// writeHistogram renders items as label/bar/count rows fitting width columns.
func writeHistogram(b *strings.Builder, items []countedKey, width, labelWidth int) {
	if len(items) == 0 {
		fmt.Fprintf(b, "%s—%s\n", colorTag(themeMuted), resetTag)
		return
	}
	peak := items[0].Count
	barWidth := max(width-labelWidth-8, 4)
	for _, item := range items {
		label := item.Key
		if runes := []rune(label); len(runes) > labelWidth {
			label = string(runes[:labelWidth-1]) + "…"
		}
		filled := int(item.Count * uint64(barWidth) / peak)
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		fmt.Fprintf(b, "%s%-*s%s %s%s%s %s%d%s\n",
			colorTag(themeText), labelWidth, safeText(label), resetTag,
			colorTag(themeAccent), bar, resetTag,
			colorTag(themeMuted), item.Count, resetTag)
	}
}

func (ui *UI) renderFooter() string {
	accent, muted := colorTag(themeAccent), colorTag(themeMuted)
	keys := []string{
		accent + "Tab" + muted + " view",
		accent + "/" + muted + " filter",
		accent + "Enter" + muted + " detail",
		accent + "Space" + muted + " pause",
		accent + "End" + muted + " newest",
		accent + "s" + muted + " sidebar",
		accent + "c" + muted + " clear",
		accent + "?" + muted + " help",
		accent + "q" + muted + " quit",
	}
	return " " + strings.Join(keys, "   ") + resetTag
}

func (ui *UI) showDetail(row int) {
	if row <= 0 || row-1 >= len(ui.content.rows) {
		return
	}
	ui.openDetail(describeEvent(ui.content.rows[row-1]))
}

func (ui *UI) showTLSDetail(row int) {
	if row <= 0 || row-1 >= len(ui.tlsContent.rows) {
		return
	}
	ui.openDetail(describeTLSEvent(ui.tlsContent.rows[row-1]))
}

func (ui *UI) openDetail(text string) {
	ui.detail.SetText(text)
	ui.detail.ScrollToBeginning()
	// Size the overlay to its content instead of a fixed box.
	height := min(strings.Count(text, "\n")+3, 40)
	ui.pages.RemovePage("detail")
	ui.pages.AddPage("detail", centered(ui.detail, 100, height), true, true)
	ui.app.SetFocus(ui.detail)
}

// Run drives the application until it stops. A ticker coalesces model changes
// into at most a few redraws per second regardless of the event rate.
func (ui *UI) Run(stop <-chan struct{}) error {
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		lastFull := time.Now()
		for {
			select {
			case <-stop:
				ui.app.QueueUpdateDraw(func() { ui.app.Stop() })
				return
			case now := <-ticker.C:
				// Rate figures decay on their own; repaint at least once a second.
				dirty := ui.model.ConsumeDirty()
				if ui.tlsModel.ConsumeDirty() {
					dirty = true
				}
				if dirty || now.Sub(lastFull) >= time.Second {
					lastFull = now
					ui.app.QueueUpdateDraw(ui.refresh)
				}
			}
		}
	}()
	return ui.app.Run()
}
