package main

import (
    "fmt"
    "strconv"
    "strings"

    "github.com/egoist/mygo/ui"
)

// The first sample is a live inventory, not an artificial burst of startup
// events. Do not make Network dependent on the event-delta retention window.
func windowsFilteredConnections(connections []windowsTCPSample, processes []systemProcess, query string) []windowsTCPSample {
    names := windowsProcessNames(processes)
    query = strings.ToLower(strings.TrimSpace(query))
    if query == "" { return connections }
    out := make([]windowsTCPSample, 0)
    for _, conn := range connections {
        if windowsMatchesNetworkQuery(query, names[conn.PID], conn.PID, conn.Local, conn.Remote) {
            out = append(out, conn)
        }
    }
    return out
}

func windowsFilteredUDPBindings(bindings []windowsUDPSample, processes []systemProcess, query string) []windowsUDPSample {
    names := windowsProcessNames(processes)
    query = strings.ToLower(strings.TrimSpace(query))
    if query == "" { return bindings }
    out := make([]windowsUDPSample, 0)
    for _, binding := range bindings {
        if windowsMatchesNetworkQuery(query, names[binding.PID], binding.PID, binding.Local) {
            out = append(out, binding)
        }
    }
    return out
}

func windowsProcessNames(processes []systemProcess) map[int]string {
    names := make(map[int]string,len(processes))
    for _, p := range processes { names[p.PID] = p.Name }
    return names
}

func windowsMatchesNetworkQuery(query, comm string, pid int, endpoints ...string) bool {
    fields := append([]string{comm,strconv.Itoa(pid)},endpoints...)
    return strings.Contains(strings.ToLower(strings.Join(fields," ")),query)
}

func (a *renewApp) windowsNetworkView(c *ui.Context) {
    t := c.Theme()
    ui.Text(c, "网络").FontSize(28).Bold()
    ui.Text(c, "Windows IP Helper · IPv4 / IPv6 本机状态采样（约 2 秒）。TCP 仅显示 ESTABLISHED，UDP 仅显示本机绑定地址；无法证明谁发起请求，也不代表流量记录。").TextColor(t.Warning)
    ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
        ui.Badge(c, fmt.Sprintf("TCP %d",len(a.windowsConnections)))
        ui.Badge(c, fmt.Sprintf("UDP %d",len(a.windowsUDPBindings)))
        ui.Text(c,"只读 · 非 ETW / WFP").FontSize(11).TextColor(t.TextMuted)
    })
    ui.Tabs(c,&a.windowsNetworkTab,"TCP 已建立连接","UDP 本地绑定")
    if a.windowsNetworkTab != a.windowsNetworkLastTab {
        a.networkSelected = -1
        a.windowsNetworkLastTab = a.windowsNetworkTab
    }
    names := windowsProcessNames(a.system.Processes)
    if a.windowsNetworkTab == 1 {
        rows := windowsFilteredUDPBindings(a.windowsUDPBindings,a.system.Processes,a.search)
        card(c,fmt.Sprintf("UDP 本地端点 · %d / %d",len(rows),len(a.windowsUDPBindings)),func(){
            if !a.systemConnected {
                ui.Text(c,"UDP 本机采集暂不可用。").TextColor(t.Warning)
                return
            }
            if len(rows) == 0 {
                ui.Text(c,"当前没有匹配的本机 UDP 绑定。").TextColor(t.TextMuted)
                return
            }
            columns := []ui.TableColumn{
                {Title:"PID",Width:86,Fixed:true},
                {Title:"进程",MinWidth:150},
                {Title:"本地监听 / 绑定端点",MinWidth:260},
                {Title:"采集方式",Width:132},
            }
            a.networkTable.Key = func(row int) any {
                b := rows[row]
                return fmt.Sprintf("udp|%d|%s",b.PID,b.Local)
            }
            ui.Table(c,&a.networkTable,columns,len(rows),func(row,col int){
                b := rows[row]
                switch col {
                case 0: ui.Text(c,strconv.Itoa(b.PID)).Font("monospace")
                case 1: harnessIdentity(c,displayOr(names[b.PID],"未知进程"),names[b.PID])
                case 2: ui.Text(c,b.Local).Font("monospace").SingleLine()
                case 3: ui.Text(c,"绑定快照").FontSize(11).SingleLine()
                }
            }).Height(450).Label("Windows UDP 本地绑定")
            if a.networkSelected >= 0 && a.networkSelected < len(rows) {
                b := rows[a.networkSelected]
                if ui.Button(c,fmt.Sprintf("查看 PID %d 的观察记录",b.PID)).Clicked() {
                    a.clearEventFilters()
                    a.eventPIDFilter = b.PID
                    a.page = "事件"
                }
            }
        })
        ui.Text(c,"UDP 无连接状态：此列表包含发送用途的临时绑定，但不提供远端 IP、域名或收发字节。").FontSize(11).TextColor(t.TextMuted)
        return
    }
    rows := windowsFilteredConnections(a.windowsConnections,a.system.Processes,a.search)
    card(c,fmt.Sprintf("TCP 已建立连接 · %d / %d",len(rows),len(a.windowsConnections)),func(){
        if !a.systemConnected {
            ui.Text(c,"Windows TCP 表暂不可用。").TextColor(t.Warning)
            return
        }
        if len(rows) == 0 {
            ui.Text(c,"当前没有匹配的已建立 TCP 连接。").TextColor(t.TextMuted)
            return
        }
        columns := []ui.TableColumn{
            {Title:"PID",Width:82,Fixed:true},
            {Title:"进程",Width:156},
            {Title:"本地地址",MinWidth:180},
            {Title:"对端地址",MinWidth:220},
            {Title:"状态",Width:100},
        }
        a.networkTable.Key = func(row int) any {
            conn := rows[row]
            return fmt.Sprintf("tcp|%d|%s|%s",conn.PID,conn.Local,conn.Remote)
        }
        ui.Table(c,&a.networkTable,columns,len(rows),func(row,col int){
            conn := rows[row]
            switch col {
            case 0: ui.Text(c,strconv.Itoa(conn.PID)).Font("monospace")
            case 1: harnessIdentity(c,displayOr(names[conn.PID],"未知进程"),names[conn.PID])
            case 2: ui.Text(c,conn.Local).Font("monospace").SingleLine()
            case 3: ui.Text(c,conn.Remote).Font("monospace").SingleLine()
            case 4: ui.Text(c,"ESTABLISHED").FontSize(10).SingleLine()
            }
        }).Height(450).Label("Windows TCP 已建立连接")
        if a.networkSelected >= 0 && a.networkSelected < len(rows) {
            conn := rows[a.networkSelected]
            ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func(){
                ui.Text(c,conn.Remote).Font("monospace").FontSize(11).TextColor(t.TextMuted).Grow(1)
                if ui.Button(c,"查看此 PID 的采样事件").Clicked(){
                    a.clearEventFilters()
                    a.eventPIDFilter = conn.PID
                    a.page = "事件"
                }
            })
        }
    })
    ui.Text(c,"同一连接跨越两个采样周期不代表这段期间没有关闭或重新建立；短连接也可能完全被遗漏。").FontSize(11).TextColor(t.TextMuted)
}
