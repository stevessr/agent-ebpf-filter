package main

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Domains are observed clues, not reconstructed HTTP requests. Never reverse
// resolve bare IPs: shared hosting and ECH make this attribution unreliable.
type agentDomainRow struct {
	Agent    string
	Target   string
	Kind     string // 域名 or IP
	Events   int
	Alerts   int
	Bytes    int64
	LastMS   int64
	Risk     string
	Sessions int
}

func normalizedDestinationHost(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n") || (strings.Contains(raw, "[") && !strings.HasPrefix(raw, "[")) {
		return "", ""
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || (!strings.EqualFold(u.Scheme, "https") && !strings.EqualFold(u.Scheme, "http")) || u.User != nil {
			return "", ""
		}
		raw = u.Hostname()
	} else if host, port, err := net.SplitHostPort(raw); err == nil {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", ""
		}
		raw = host
	} else if idx := strings.LastIndex(raw, ":"); idx > 0 && !strings.Contains(raw[:idx], ":") {
		return "", "" // malformed host:port must not be silently normalized
	}
	raw = strings.ToLower(strings.TrimSuffix(strings.Trim(raw, "[]"), "."))
	if ip := net.ParseIP(raw); ip != nil {
		return ip.String(), "IP"
	}
	if len(raw) > 253 || !strings.Contains(raw, ".") || strings.ContainsAny(raw, "/?#@:[]\\") {
		return "", ""
	}
	for _, part := range strings.Split(raw, ".") {
		if len(part) == 0 || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
			return "", ""
		}
		for _, c := range part {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", ""
			}
		}
	}
	return raw, "域名"
}

func agentEventDestination(e eventSummary) (host, kind string) {
	if !isNetworkEvent(e) {
		return "", ""
	}
	// Prefer explicit hostname evidence; keep IP-only events distinct.
	for _, candidate := range []string{e.Domain, e.Target, e.NetEndpoint} {
		if host, kind = normalizedDestinationHost(candidate); kind == "域名" {
			return host, kind
		}
	}
	for _, candidate := range []string{e.Domain, e.Target, e.NetEndpoint} {
		if host, kind = normalizedDestinationHost(candidate); kind == "IP" {
			return host, kind
		}
	}
	return "", ""
}

func matchesAgentDomainEvent(event eventSummary, owners agentOwnershipIndex, agent, target, kind string) bool {
	owner := owners.attribution(event)
	if !owner.IsAgent || owner.OwnerLabel != agent {
		return false
	}
	host, hostKind := agentEventDestination(event)
	return host == target && hostKind == kind
}

func agentDomainRiskRank(risk string) int {
	switch risk {
	case "高风险":
		return 2
	case "需关注":
		return 1
	default:
		return 0
	}
}

func aggregateAgentDomains(events []eventSummary) []agentDomainRow {
	owners := buildAgentOwnershipIndex(events, nil)
	type entry struct {
		agentDomainRow
		sessions map[string]struct{}
	}
	byKey := make(map[string]*entry)
	for _, event := range events {
		owner := owners.attribution(event)
		if !owner.IsAgent {
			continue
		}
		host, kind := agentEventDestination(event)
		if host == "" {
			continue
		}
		key := owner.OwnerLabel + "\x00" + kind + "\x00" + host
		row := byKey[key]
		if row == nil {
			row = &entry{agentDomainRow: agentDomainRow{Agent: owner.OwnerLabel, Target: host, Kind: kind}, sessions: make(map[string]struct{})}
			byKey[key] = row
		}
		row.Events++
		row.Bytes += max(event.NetBytes, 0)
		row.sessions[eventSessionKey(event)] = struct{}{}
		risk := eventRisk(event)
		if risk != "正常" {
			row.Alerts++
		}
		if agentDomainRiskRank(risk) > agentDomainRiskRank(row.Risk) {
			row.Risk = risk
		}
		if event.ReceivedAtMS > row.LastMS {
			row.LastMS = event.ReceivedAtMS
		}
	}
	rows := make([]agentDomainRow, 0, len(byKey))
	for _, value := range byKey {
		value.Sessions = len(value.sessions)
		if value.Risk == "" {
			value.Risk = "正常"
		}
		rows = append(rows, value.agentDomainRow)
	}
	sort.Slice(rows, func(i, j int) bool {
		if agentDomainRiskRank(rows[i].Risk) != agentDomainRiskRank(rows[j].Risk) {
			return agentDomainRiskRank(rows[i].Risk) > agentDomainRiskRank(rows[j].Risk)
		}
		if rows[i].LastMS != rows[j].LastMS {
			return rows[i].LastMS > rows[j].LastMS
		}
		if rows[i].Agent != rows[j].Agent {
			return rows[i].Agent < rows[j].Agent
		}
		return rows[i].Target < rows[j].Target
	})
	return rows
}

func (a *renewApp) agentDomainRows() []agentDomainRow {
	if a.domainCacheValid && a.domainCacheVersion == a.eventsVersion {
		return a.domainCacheRows
	}
	a.domainCacheRows = aggregateAgentDomains(a.events)
	a.domainCacheVersion = a.eventsVersion
	a.domainCacheValid = true
	return a.domainCacheRows
}

func (a *renewApp) matchingAgentDomainRows() []agentDomainRow {
	q := strings.ToLower(strings.TrimSpace(a.domainSearch))
	rows := make([]agentDomainRow, 0, len(a.agentDomainRows()))
	for _, row := range a.agentDomainRows() {
		if a.domainAgentFilter != "" && row.Agent != a.domainAgentFilter {
			continue
		}
		if !a.domainShowIPs && row.Kind == "IP" {
			continue
		}
		if a.domainRiskOnly && row.Risk == "正常" {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(row.Agent+" "+row.Target), q) {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func (a *renewApp) openAgentDomainEvents(row agentDomainRow) {
	a.beginEventDrilldown()
	a.eventDomainAgent = row.Agent
	a.eventDomainTarget = row.Target
	a.eventDomainKind = row.Kind
}

func agentDomainRowKey(row agentDomainRow) string {
	return row.Agent + "\x00" + row.Kind + "\x00" + row.Target
}

func agentDomainSelectedIndex(rows []agentDomainRow, key string) int {
	for i, row := range rows {
		if agentDomainRowKey(row) == key {
			return i
		}
	}
	return -1
}

func (a *renewApp) agentDomainsView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "Agent 域名监控").FontSize(28).Bold()
	ui.Text(c, "按 Agent 关联域名和 IP 外联，查看告警与相关会话。无需代理或导入 API Key。").TextColor(t.TextMuted)
	ui.Text(c, "仅统计当前已加载的有限事件摘要，不代表完整请求数、真实流量、Token 用量或所有出站连接；DoH、ECH、DNS/SNI 缺失均可能使域名不可见。域名是观测线索，并非已验证的 HTTP Host。").FontSize(11).TextColor(t.TextMuted)

	all := a.agentDomainRows()
	domainCount, ipCount, alertCount := 0, 0, 0
	agents := make(map[string]struct{})
	for _, row := range all {
		agents[row.Agent] = struct{}{}
		if row.Kind == "域名" {
			domainCount++
		} else {
			ipCount++
		}
		if row.Risk != "正常" {
			alertCount++
		}
	}
	ui.Row(c).Gap(12).Wrap().Children(func() {
		statCard(c, "域名线索", strconv.Itoa(domainCount), "按 Agent 分组去重")
		statCard(c, "IP-only 外联", strconv.Itoa(ipCount), "不猜测目标域名")
		statCard(c, "已关联 Agent", strconv.Itoa(len(agents)), "真实进程或会话上下文")
		statCard(c, "风险目标", strconv.Itoa(alertCount), "已有策略决策与风险评分")
	})

	agentNames := []string{""}
	for agent := range agents {
		agentNames = append(agentNames, agent)
	}
	sort.Strings(agentNames[1:])
	ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
		ui.SearchField(c, &a.domainSearch).Label("搜索域名、IP 或 Agent").Width(265)
		ui.Select(c, &a.domainAgentFilter, agentNames).Label("Agent（空为全部）").Width(190)
		ui.Checkbox(c, &a.domainRiskOnly, "只看风险")
		ui.Checkbox(c, &a.domainShowIPs, "显示仅 IP 连接")
		if ui.Button(c, "清除筛选").Clicked() {
			a.domainSearch, a.domainAgentFilter = "", ""
			a.domainRiskOnly = false
			a.domainShowIPs = true
		}
	})
	rows := a.matchingAgentDomainRows()
	filterKey := fmt.Sprintf("%s\x00%s\x00%t\x00%t", a.domainSearch, a.domainAgentFilter, a.domainRiskOnly, a.domainShowIPs)
	if a.domainLastFilter != filterKey {
		a.domainSelected = -1
		a.domainSelectedKey = ""
		a.domainLastFilter = filterKey
	}
	if a.domainLastVersion != a.eventsVersion {
		a.domainSelected = agentDomainSelectedIndex(rows, a.domainSelectedKey)
		a.domainLastVersion = a.eventsVersion
	}
	card(c, fmt.Sprintf("外联目的地 · %d 组", len(rows)), func() {
		if len(rows) == 0 {
			ui.Text(c, "当前没有可关联的 Agent 网络目标。请确认采集器在线、网络事件已启用，且存在 Agent 运行上下文。").TextColor(t.TextMuted)
			return
		}
		cols := []ui.TableColumn{
			{Title: "域名 / IP", MinWidth: 225, Fixed: true},
			{Title: "Agent", Width: 146},
			{Title: "证据", Width: 80},
			{Title: "事件", Width: 60, Align: ui.End},
			{Title: "摘要字节", Width: 100, Align: ui.End},
			{Title: "会话", Width: 60, Align: ui.End},
			{Title: "风险", Width: 85},
			{Title: "最近", Width: 80},
		}
		a.domainTable.Key = func(i int) any { return agentDomainRowKey(rows[i]) }
		table := ui.Table(c, &a.domainTable, cols, len(rows), func(i, col int) {
			row := rows[i]
			switch col {
			case 0:
				ui.Text(c, row.Target).SingleLine()
			case 1:
				ui.Text(c, row.Agent).SingleLine()
			case 2:
				ui.Text(c, row.Kind+"线索").FontSize(11).TextColor(t.TextMuted).SingleLine()
			case 3:
				ui.Text(c, strconv.Itoa(row.Events))
			case 4:
				ui.Text(c, formatBytes(row.Bytes))
			case 5:
				ui.Text(c, strconv.Itoa(row.Sessions))
			case 6:
				ui.Text(c, row.Risk).TextColor(riskTextColor(t, row.Risk))
			case 7:
				ui.Text(c, summaryTime(row.LastMS)).Font("monospace")
			}
		}).Height(470).Label("Agent 网络目的地")
		if table.Changed() && a.domainSelected >= 0 && a.domainSelected < len(rows) {
			a.domainSelectedKey = agentDomainRowKey(rows[a.domainSelected])
		}
		if a.domainSelected >= 0 && a.domainSelected < len(rows) {
			row := rows[a.domainSelected]
			ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
				ui.Text(c, fmt.Sprintf("%s · %s · %d 条摘要 / %d 条风险", row.Agent, row.Target, row.Events, row.Alerts)).Grow(1).MaxLines(2).FontSize(11).TextColor(t.TextMuted)
				if ui.PrimaryButton(c, "查看关联事件").Clicked() {
					a.openAgentDomainEvents(row)
				}
			})
		}
	})
}
