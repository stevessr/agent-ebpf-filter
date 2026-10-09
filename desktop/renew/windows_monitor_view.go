package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Windows monitoring has a *current* TCP table in addition to the event
// history. The first inventory is intentionally not treated as connection
// start events, but it must still be visible in the Network page.
func windowsFilteredConnections(connections []windowsTCPSample, processes []systemProcess, query string) []windowsTCPSample {
	names := make(map[int]string, len(processes))
	for _, process := range processes { names[process.PID] = process.Name }
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" { return connections }
	rows := make([]windowsTCPSample, 0)
	for _, conn := range connections {
		if strings.Contains(strings.ToLower(strings.Join([]string{
			names[conn.PID], conn.Local, conn.Remote, strconv.Itoa(conn.PID),
		}, " ")), query) { rows = append(rows, conn) }
	}
	return rows
}

func (a *renewApp) windowsNetworkView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "网络").FontSize(28).Bold()
	ui.Text(c, "Windows IP Helper IPv4/IPv6 已建立 TCP 连接的本机快照（约 2 秒刷新）；无法证明哪个进程发起连接，也不包含 UDP、短连接或真实流量字节。").TextColor(t.Warning)
	rows := windowsFilteredConnections(a.windowsConnections, a.system.Processes, a.search)
	processNames := make(map[int]string, len(a.system.Processes))
	for _, p := range a.system.Processes { processNames[p.PID] = p.Name }
	card(c, fmt.Sprintf("当前 TCP 连接 · %d / 总计 %d", len(rows), len(a.windowsConnections)), func() {
		if !a.systemConnected {
			ui.Text(c, "Windows TCP 表暂不可用；正在进行下次采样。").TextColor(t.TextMuted)
			return
		}
		if len(rows) == 0 {
			ui.Text(c, "本次采样没有匹配的已建立 TCP 连接。").TextColor(t.TextMuted)
			return
		}
		cols := []ui.TableColumn{
			{Title: "PID", Width: 82, Fixed: true},
			{Title: "进程", Width: 156},
			{Title: "本地地址", MinWidth: 180},
			{Title: "对端地址", MinWidth: 220},
			{Title: "状态", Width: 92},
		}
		a.networkTable.Key = func(row int) any {
			conn := rows[row]
			return fmt.Sprintf("%d|%s|%s", conn.PID, conn.Local, conn.Remote)
		}
		ui.Table(c, &a.networkTable, cols, len(rows), func(row, col int) {
			conn := rows[row]
			switch col {
			case 0: ui.Text(c, strconv.Itoa(conn.PID)).Font("monospace")
			case 1: harnessIdentity(c, displayOr(processNames[conn.PID], "未知进程"), processNames[conn.PID])
			case 2: ui.Text(c, conn.Local).Font("monospace").SingleLine()
			case 3: ui.Text(c, conn.Remote).Font("monospace").SingleLine()
			case 4: ui.Text(c, "ESTABLISHED").FontSize(10).SingleLine()
			}
		}).Height(460).Label("Windows 已建立 TCP 连接")
		if a.networkSelected >= 0 && a.networkSelected < len(rows) {
			conn := rows[a.networkSelected]
			ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
				ui.Text(c, conn.Remote).Font("monospace").FontSize(11).TextColor(t.TextMuted).Grow(1)
				if ui.Button(c, "查看此 PID 的采样事件").Clicked() {
					a.clearEventFilters()
					a.eventPIDFilter = conn.PID
					a.page = "事件"
				}
			})
		}
	})
}
