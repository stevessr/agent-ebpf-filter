package main

import (
	"fmt"
	"strings"

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
