package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var tlsColumns = []column{
	{title: "TIME", maxWidth: 12},
	{title: "DIR", maxWidth: 4},
	{title: "PID", maxWidth: 7, align: tview.AlignRight},
	{title: "COMM", maxWidth: 14},
	{title: "TYPE", maxWidth: 14},
	{title: "METHOD", maxWidth: 7},
	{title: "STATUS", maxWidth: 6, align: tview.AlignRight},
	{title: "SIZE", maxWidth: 8, align: tview.AlignRight},
	{title: "HOST", maxWidth: 32},
	{title: "URL", maxWidth: 120, expand: 1},
}

// tlsContent is the virtual table over the TLS snapshot.
type tlsContent struct {
	tview.TableContentReadOnly
	rows []*TLSEvent
}

func (c *tlsContent) GetRowCount() int    { return len(c.rows) + 1 }
func (c *tlsContent) GetColumnCount() int { return len(tlsColumns) }

func (c *tlsContent) GetCell(row, col int) *tview.TableCell {
	if col < 0 || col >= len(tlsColumns) {
		return nil
	}
	spec := tlsColumns[col]
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
	ev := c.rows[row]
	text, color := tlsCellText(ev, col)
	return tview.NewTableCell(text).
		SetTextColor(color).
		SetAlign(spec.align).
		SetMaxWidth(spec.maxWidth).
		SetExpansion(spec.expand).
		SetReference(ev)
}

func tlsCellText(ev *TLSEvent, col int) (string, tcell.Color) {
	switch col {
	case 0:
		return ev.Timestamp.Local().Format("15:04:05.000"), themeMuted
	case 1:
		if ev.Direction == "send" {
			return "→", themeNetwork
		}
		return "←", themeProcess
	case 2:
		return strconv.FormatUint(uint64(ev.PID), 10), themeText
	case 3:
		return safeText(ev.Comm), themeText
	case 4:
		return safeText(ev.Type), tlsTypeColor(ev.Type)
	case 5:
		return safeText(ev.Method), themeAccent
	case 6:
		return statusLabel(ev.StatusCode)
	case 7:
		return humanBytes(max(ev.BodySize, ev.CapturedLen)), themeMuted
	case 8:
		return safeText(ev.Host), themeText
	default:
		text := ev.URL
		if text == "" && ev.SSEEvent != "" {
			text = "sse:" + ev.SSEEvent
		}
		if ev.Vendor != "" {
			text = "[" + ev.Vendor + "] " + text
		}
		return safeText(text), themeText
	}
}

func tlsTypeColor(eventType string) tcell.Color {
	switch eventType {
	case "http_request":
		return themeNetwork
	case "http_response":
		return themeProcess
	case "sse_message":
		return themeAccent
	case "semantic_alert":
		return themeWarning
	default:
		return themeFile
	}
}

func statusLabel(code int) (string, tcell.Color) {
	switch {
	case code == 0:
		return "", themeMuted
	case code >= 500:
		return strconv.Itoa(code), themeError
	case code >= 400:
		return strconv.Itoa(code), themeWarning
	case code >= 300:
		return strconv.Itoa(code), themeMuted
	default:
		return strconv.Itoa(code), themeSuccess
	}
}

func humanBytes(n int) string {
	switch {
	case n <= 0:
		return ""
	case n < 1024:
		return strconv.Itoa(n) + " B"
	case n < 1024*1024:
		return fmt.Sprintf("%.1f K", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f M", float64(n)/(1024*1024))
	}
}

func (ui *UI) renderTLSSidebar(s TLSSummary) string {
	var b strings.Builder
	muted, text := colorTag(themeMuted), colorTag(themeText)
	fmt.Fprintf(&b, "%stls capture%s\n", boldTag(themeTitle), resetTag)
	switch s.State {
	case StateConnected:
		fmt.Fprintf(&b, "%s● subscribed%s\n", colorTag(themeSuccess), resetTag)
	case StateConnecting:
		fmt.Fprintf(&b, "%s◌ connecting%s\n", colorTag(themeWarning), resetTag)
	default:
		fmt.Fprintf(&b, "%s○ %s%s\n", colorTag(themeError), safeText(shorten(s.StateDetail, 30)), resetTag)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "%straffic%s\n", boldTag(themeTitle), resetTag)
	fmt.Fprintf(&b, "%srequests %s%d%s  %sresponses %s%d%s\n", muted, text, s.Requests, resetTag, muted, text, s.Responses, resetTag)
	fmt.Fprintf(&b, "%splaintext %s%s%s\n\n", muted, text, humanBytes(int(min(s.Bytes, 1<<62))), resetTag)

	fmt.Fprintf(&b, "%shistory%s\n", boldTag(themeTitle), resetTag)
	fmt.Fprintf(&b, "%sretained %s%d%s / %d", muted, text, s.Retained, resetTag, s.Capacity)
	if s.Evicted > 0 {
		fmt.Fprintf(&b, "  %sevicted %d%s", muted, s.Evicted, resetTag)
	}
	b.WriteString("\n")
	if !s.LastEventAt.IsZero() {
		fmt.Fprintf(&b, "%slast event %s%s%s\n", muted, text, s.LastEventAt.Local().Format("15:04:05"), resetTag)
	}
	b.WriteString("\n")

	title, items := "top hosts", s.TopHosts
	switch ui.sidebarMode {
	case 1:
		title, items = "top processes", s.TopComms
	case 2:
		title, items = "top vendors", s.TopVendors
	}
	fmt.Fprintf(&b, "%s%s%s  %s(s cycles)%s\n", boldTag(themeTitle), title, resetTag, muted, resetTag)
	// Hostnames are longer than event types; trade bar width for label width.
	writeHistogram(&b, items, 32, 22)
	return b.String()
}

func shorten(s string, n int) string {
	if runes := []rune(s); len(runes) > n {
		return string(runes[:n-1]) + "…"
	}
	return s
}

// describeTLSEvent renders one captured exchange: metadata, headers sorted
// by name, and the (already redacted by the backend) body.
func describeTLSEvent(ev *TLSEvent) string {
	var b strings.Builder
	muted, text := colorTag(themeMuted), colorTag(themeText)
	title := ev.Type
	if ev.Method != "" {
		title = ev.Method + " " + ev.URL
	} else if ev.StatusCode != 0 {
		title = "HTTP " + strconv.Itoa(ev.StatusCode)
	}
	fmt.Fprintf(&b, "%s%s%s  %s%s%s\n\n", boldTag(tlsTypeColor(ev.Type)), safeText(title), resetTag, muted, ev.Timestamp.Local().Format("2006-01-02 15:04:05.000"), resetTag)

	field := func(name, value string) {
		if value == "" {
			return
		}
		fmt.Fprintf(&b, "%s%-16s%s %s%s%s\n", muted, name, resetTag, text, safeText(value), resetTag)
	}
	field("host", ev.Host)
	field("process", fmt.Sprintf("%s (pid %d, tgid %d)", ev.Comm, ev.PID, ev.TGID))
	field("direction", ev.Direction)
	field("library", strings.TrimSpace(ev.Lib+" "+ev.Function))
	if ev.StatusCode != 0 {
		field("status", strconv.Itoa(ev.StatusCode))
	}
	field("content-type", ev.ContentType)
	if ev.BodySize > 0 || ev.CapturedLen > 0 {
		size := humanBytes(ev.BodySize)
		if ev.Truncated || ev.OriginalLen > ev.CapturedLen {
			size += fmt.Sprintf("  (captured %d of %d bytes)", ev.CapturedLen, ev.OriginalLen)
		}
		field("size", size)
	}
	field("vendor", ev.Vendor)
	field("role", ev.MessageRole)
	if ev.PromptLen > 0 {
		field("prompt length", strconv.Itoa(ev.PromptLen))
	}
	field("tool", ev.ToolName)
	field("agent run", ev.AgentRunID)
	field("sse event", ev.SSEEvent)
	if ev.HTTP2StreamID != 0 {
		field("http2", fmt.Sprintf("stream %d %s", ev.HTTP2StreamID, ev.HTTP2FrameType))
	}
	if ev.LatencyMs > 0 {
		field("latency", fmt.Sprintf("%.1f ms", ev.LatencyMs))
	}
	field("redaction", ev.RedactionState)
	if ev.LoopAlert {
		fmt.Fprintf(&b, "%s%-16s%s %sloop alert%s\n", muted, "alert", resetTag, colorTag(themeWarning), resetTag)
	}

	if len(ev.Headers) > 0 {
		names := make([]string, 0, len(ev.Headers))
		for name := range ev.Headers {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Fprintf(&b, "\n%sheaders%s\n", boldTag(themeTitle), resetTag)
		for _, name := range names {
			fmt.Fprintf(&b, "%s%s:%s %s\n", colorTag(themeAccent), safeText(name), resetTag, safeText(ev.Headers[name]))
		}
	}
	if ev.Body != "" {
		fmt.Fprintf(&b, "\n%sbody%s\n%s\n", boldTag(themeTitle), resetTag, safeText(shorten(ev.Body, 4000)))
	}
	return b.String()
}
