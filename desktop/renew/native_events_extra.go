package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func mergeEventSummaries(existing, incoming []eventSummary, limit int) []eventSummary {
	return mergeEventSummariesInto(nil, existing, incoming, limit)
}

func mergeEventSummariesInto(dst, existing, incoming []eventSummary, limit int) []eventSummary {
	if limit <= 0 {
		limit = 1200
	}
	if len(incoming) == 0 {
		if len(existing) > limit {
			return existing[:limit]
		}
		return existing
	}

	// Incoming batches are normally tiny compared with the retained window.
	// Deduplicate and sort only that batch, then linearly merge it with the
	// already-sorted window instead of rebuilding and sorting all ~1200 rows.
	replacements := make(map[string]eventSummary, len(incoming))
	for _, event := range incoming {
		id := strings.TrimSpace(event.EventID)
		if id == "" {
			continue
		}
		event.EventID = id
		event.SearchText = buildEventSearchText(event)
		replacements[id] = event
	}
	if len(replacements) == 0 {
		return existing
	}
	fresh := make([]eventSummary, 0, len(replacements))
	for _, event := range replacements {
		fresh = append(fresh, event)
	}
	sort.Slice(fresh, func(i, j int) bool { return eventSummaryBefore(fresh[i], fresh[j]) })

	capacity := len(existing) + len(fresh)
	if capacity > limit {
		capacity = limit
	}
	var out []eventSummary
	if cap(dst) >= capacity {
		out = dst[:0]
	} else {
		out = make([]eventSummary, 0, limit)
	}
	i, j := 0, 0
	for len(out) < limit && (i < len(fresh) || j < len(existing)) {
		for j < len(existing) {
			if _, replaced := replacements[strings.TrimSpace(existing[j].EventID)]; replaced {
				j++
				continue
			}
			break
		}
		if i >= len(fresh) {
			if j >= len(existing) {
				break
			}
			event := existing[j]
			if event.SearchText == "" {
				event.SearchText = buildEventSearchText(event)
			}
			out = append(out, event)
			j++
			continue
		}
		if j >= len(existing) {
			out = append(out, fresh[i])
			i++
			continue
		}

		existingEvent := existing[j]
		if existingEvent.SearchText == "" {
			existingEvent.SearchText = buildEventSearchText(existingEvent)
		}
		if eventSummaryBefore(fresh[i], existingEvent) {
			out = append(out, fresh[i])
			i++
		} else {
			out = append(out, existingEvent)
			j++
		}
	}
	return out
}

func eventSummaryBefore(a, b eventSummary) bool {
	if a.ReceivedAtMS != b.ReceivedAtMS {
		return a.ReceivedAtMS > b.ReceivedAtMS
	}
	return a.EventID > b.EventID
}

func matchesEventDecision(event eventSummary, filter string) bool {
	decision := strings.ToUpper(strings.TrimSpace(event.Decision))
	switch filter {
	case "已阻断":
		return decision == "BLOCK" || decision == "DENY"
	case "告警":
		return decision == "ALERT"
	case "已允许":
		return decision == "ALLOW"
	default:
		return true
	}
}

func uniqueEventTypes(events []eventSummary) []string {
	set := make(map[string]struct{})
	for _, event := range events {
		if strings.TrimSpace(event.Type) != "" {
			set[event.Type] = struct{}{}
		}
	}
	out := make([]string, 0, len(set)+1)
	out = append(out, "")
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out[1:])
	return out
}

func uniqueEventSessions(events []eventSummary) []string {
	set := make(map[string]struct{})
	for _, event := range events {
		set[eventSessionKey(event)] = struct{}{}
	}
	out := make([]string, 0, len(set)+1)
	out = append(out, "")
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out[1:])
	return out
}

func (a *renewApp) loadOlderEvents() {
	if a.client == nil || a.historyLoading || strings.TrimSpace(a.historyCursor) == "" {
		return
	}
	cursor := a.historyCursor
	a.historyLoading = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		response, err := a.client.eventSummaries(ctx, 240, cursor)
		a.update(func() {
			a.historyLoading = false
			if err != nil {
				a.lastErr = err.Error()
				return
			}
			a.mergeEventWindow(response.Events, 1200)
			a.historyCursor = response.NextCursor
			a.eventVisibleLimit += 50
		})
	}()
}

func (a *renewApp) releaseEventDetailPayload() {
	a.eventDetailLoading = false
	a.eventDetailID = ""
	a.eventDetail = nil
	a.eventDetailText = ""
	a.eventDetailErr = ""
	a.eventDetailTab = 0
	a.eventDetailFieldSearch = ""
}

func (a *renewApp) closeEventDetail() {
	a.eventDetailOpen = false
	a.releaseEventDetailPayload()
}

func (a *renewApp) ensureEventDetailText() {
	if a.eventDetailText != "" || a.eventDetail == nil {
		return
	}
	if data, err := json.MarshalIndent(a.eventDetail, "", "  "); err == nil {
		a.eventDetailText = string(data)
	}
}

func (a *renewApp) openEventDetail(eventID string) {
	if a.client == nil || strings.TrimSpace(eventID) == "" {
		return
	}
	id := strings.TrimSpace(eventID)
	a.releaseEventDetailPayload()
	a.eventDetailID = id
	a.eventDetailOpen = true
	a.eventDetailLoading = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		detail, err := a.client.eventDetail(ctx, id)
		var enforcement enforcementSnapshot
		var enforcementErr error
		// An unrelated file/CPU event needs only the detail request.
		// Avoid a second backend round-trip unless there is a valid action.
		targets := enforcementTargets(detail)
		if err == nil && a.runtimeCfg.Runtime.PolicyManagementEnabled &&
			(targets.IP != "" || targets.ExecPath != "") {
			enforcement, enforcementErr = a.client.enforcementStatus(ctx)
		}
		a.update(func() {
			if !a.eventDetailOpen || a.eventDetailID != id {
				return
			}
			a.eventDetailLoading = false
			if err != nil {
				a.eventDetailErr = err.Error()
				return
			}
			a.eventDetail = detail
			if enforcementErr == nil {
				a.enforcement = enforcement
			} else {
				a.enforcementErr = enforcementErr.Error()
			}
		})
	}()
}

func detailRecord(detail map[string]any) map[string]any {
	if detail == nil {
		return nil
	}
	for _, key := range []string{"event", "Event"} {
		if record, ok := detail[key].(map[string]any); ok {
			return record
		}
	}
	return detail
}

func mapValue(record map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := record[key]; ok {
			return value
		}
	}
	for existing, value := range record {
		for _, key := range keys {
			if strings.EqualFold(existing, key) {
				return value
			}
		}
	}
	return nil
}

func mapText(record map[string]any, keys ...string) string {
	value := mapValue(record, keys...)
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func mapNumber(record map[string]any, keys ...string) float64 {
	value := mapValue(record, keys...)
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case json.Number:
		n, _ := typed.Float64()
		return n
	default:
		n, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(value)), 64)
		return n
	}
}

type eventEnforcementTargets struct {
	IP       string
	Port     int
	ExecPath string
}

func enforcementTargets(detail map[string]any) eventEnforcementTargets {
	record := detailRecord(detail)
	if record == nil {
		return eventEnforcementTargets{}
	}
	kind := eventDetailCategory(detail, mapText(record, "type", "Type"))
	layers := eventDetailLayers(detail)
	get := func(keys ...string) string {
		value, _, _ := eventDetailLookup(layers, keys...)
		return value
	}
	var target eventEnforcementTargets
	if kind == "network" {
		// Only actual network evidence may suggest a cgroup IP/port policy.
		endpoint := get("netEndpoint", "endpoint")
		if endpoint != "" {
			host := endpoint
			port := 0
			if parsedHost, parsedPort, err := net.SplitHostPort(endpoint); err == nil {
				host = strings.Trim(parsedHost, "[]")
				port, _ = strconv.Atoi(parsedPort)
			}
			if net.ParseIP(strings.Trim(host, "[]")) != nil {
				target.IP = strings.Trim(host, "[]")
				if port >= 1 && port <= 65535 {
					target.Port = port
				}
			}
		}
		if target.IP == "" {
			ip := get("dstIp")
			if net.ParseIP(ip) != nil {
				target.IP = ip
				port, _ := strconv.Atoi(get("dstPort"))
				if port >= 1 && port <= 65535 {
					target.Port = port
				}
			}
		}
	}
	// File writes are not process executions. Prefer the typed exec payload,
	// or a legacy executable path only when the syscall is clearly exec.
	typ := strings.ToLower(mapText(record, "type", "Type"))
	envelope, _ := mapValue(detail, "Envelope", "envelope").(map[string]any)
	_, typedExec := mapValue(envelope, "execEvent").(map[string]any)
	if kind == "process" && (typedExec || strings.HasPrefix(typ, "exec") || typ == "process_exec") {
		path := get("path")
		if strings.HasPrefix(path, "/") {
			target.ExecPath = path
		}
	}
	return target
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (a *renewApp) runEnforcement(path string, payload map[string]any) {
	if a.client == nil || a.enforcementBusy || !a.runtimeCfg.Runtime.PolicyManagementEnabled {
		return
	}
	a.enforcementBusy = true
	a.enforcementErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		err := a.client.enforcementAction(ctx, path, payload)
		snapshot, statusErr := a.client.enforcementStatus(ctx)
		a.update(func() {
			a.enforcementBusy = false
			if err != nil {
				a.enforcementErr = err.Error()
				return
			}
			if statusErr != nil {
				a.enforcementErr = statusErr.Error()
				return
			}
			a.enforcement = snapshot
		})
	}()
}

func (a *renewApp) eventDetailModal(c *ui.Context) {
	if !a.eventDetailOpen {
		return
	}
	t := c.Theme()
	viewportWidth, viewportHeight := c.Size()
	panelWidth := min(float32(980), max(float32(300), viewportWidth-48))
	scrollHeight := min(float32(680), max(float32(240), viewportHeight-182))
	ui.Modal(c, &a.eventDetailOpen, func() {
		ui.Column(c).Width(panelWidth).Gap(12).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, "事件取证详情").FontSize(20).Bold()
					ui.Text(c, a.eventDetailID).Font("monospace").FontSize(10).TextColor(t.TextMuted)
				})
				if ui.Button(c, "关闭").Clicked() {
					a.closeEventDetail()
				}
			})
			if a.eventDetailLoading {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c)
					ui.Text(c, "正在按需读取完整事件…").TextColor(t.TextMuted)
				})
				return
			}
			if a.eventDetailErr != "" {
				ui.Text(c, "完整记录读取失败：" + a.eventDetailErr).TextColor(t.Danger)
				return
			}
			record := detailRecord(a.eventDetail)
			if record == nil {
				ui.Text(c, "后端未返回可显示的事件内容").TextColor(t.TextMuted)
				return
			}

			ui.Tabs(c, &a.eventDetailTab, "可视化详情", "原始 JSON", "字段浏览")
			switch a.eventDetailTab {
			case 1:
				a.ensureEventDetailText()
				ui.Scroll(c).Height(scrollHeight).Padding(10).Children(func() {
					ui.Text(c, a.eventDetailText).Font("monospace").FontSize(10)
				})
			case 2:
				ui.TextInput(c, &a.eventDetailFieldSearch).Placeholder("筛选字段名、路径或值（仅当前事件）").Width(panelWidth - 12)
				fields := eventDetailTreeItems(a.eventDetail, a.eventDetailFieldSearch)
				ui.Text(c, fmt.Sprintf("匹配 %d 条叶子字段，最多展示 300 条；原始 JSON 保留完整结构。", len(fields))).FontSize(10).TextColor(t.TextMuted)
				ui.Scroll(c).Height(scrollHeight - 56).Gap(7).Children(func() {
					if len(fields) == 0 {
						ui.Text(c, "没有匹配的字段。").TextColor(t.TextMuted)
					}
					for _, field := range fields {
						ui.Column(c).Gap(2).Padding(8).Radius(6).Background(t.Surface).Children(func() {
							ui.Text(c, field.Label).Font("monospace").FontSize(10).TextColor(t.TextMuted)
							ui.Text(c, field.Value).Font("monospace").FontSize(11)
						})
					}
				})
			default:
				model := eventDetailModel(a.eventDetail)
				ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
					pidText, _, ok := eventDetailLookup(eventDetailLayers(a.eventDetail), "pid")
					if ok {
						if pid, err := strconv.Atoi(pidText); err == nil && pid > 0 {
							if ui.Button(c, fmt.Sprintf("查看 PID %d 的事件", pid)).Clicked() {
								a.openEventFilter(eventSummary{PID: pid}, "pid")
								a.closeEventDetail()
							}
						}
					}
					if model.Type != "" && ui.Button(c, "筛选同类操作").Clicked() {
						a.openEventFilter(eventSummary{Type: model.Type}, "type")
						a.closeEventDetail()
					}
					ui.Text(c, "详情来自当前记录，不会自动加载其它事件的完整负载。").FontSize(10).TextColor(t.TextMuted)
				})
				ui.Scroll(c).Height(scrollHeight).Gap(12).Children(func() {
					a.richEventDetail(c, a.eventDetail, panelWidth)
					a.eventDetailEnforcement(c)
					ui.Text(c, "完整负载只在详情打开期间保存在桌面内存中；关闭时立即释放。").FontSize(10).TextColor(t.TextMuted)
				})
			}
		})
	})
}

// Enforcement actions remain explicitly gated and separate from observation.
// Never translate a file write path into an executable block rule.
func (a *renewApp) eventDetailEnforcement(c *ui.Context) {
	t := c.Theme()
	targets := enforcementTargets(a.eventDetail)
	if targets.IP == "" && targets.ExecPath == "" {
		return
	}
	card(c, "可选处置", func() {
		if !a.runtimeCfg.Runtime.PolicyManagementEnabled {
			ui.Text(c, "策略管理未启用。处置须在“监控”页手动启用 policy_management。").FontSize(11).TextColor(t.TextMuted)
			return
		}
		ui.Text(c, "以下操作会更改内核阻断策略，请核对目标。").FontSize(11).TextColor(t.Warning)
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if targets.IP != "" {
				blocked := containsString(a.enforcement.Cgroup.BlockedIPs, targets.IP)
				label, path := "阻断 IP "+targets.IP, "/sandbox/cgroup/block-ip"
				if blocked {
					label, path = "解除 IP "+targets.IP, "/sandbox/cgroup/unblock-ip"
				}
				if ui.Button(c, label).Clicked() {
					a.runEnforcement(path, map[string]any{"ip": targets.IP})
				}
			}
			if targets.Port > 0 {
				blocked := containsInt(a.enforcement.Cgroup.BlockedPorts, targets.Port)
				label, path := fmt.Sprintf("阻断端口 %d", targets.Port), "/sandbox/cgroup/block-port"
				if blocked {
					label, path = fmt.Sprintf("解除端口 %d", targets.Port), "/sandbox/cgroup/unblock-port"
				}
				if ui.Button(c, label).Clicked() {
					a.runEnforcement(path, map[string]any{"port": targets.Port})
				}
			}
			if targets.ExecPath != "" {
				blocked := containsString(a.enforcement.LSM.BlockedExecPaths, targets.ExecPath)
				label, path := "阻断执行 "+targets.ExecPath, "/sandbox/lsm/block-exec-path"
				if blocked {
					label, path = "解除执行 "+targets.ExecPath, "/sandbox/lsm/unblock-exec-path"
				}
				if ui.Button(c, label).Clicked() {
					a.runEnforcement(path, map[string]any{"path": targets.ExecPath})
				}
			}
		})
		if a.enforcementBusy {
			ui.Text(c, "正在等待内核状态确认…").FontSize(11).TextColor(t.TextMuted)
		}
		if a.enforcementErr != "" {
			ui.Text(c, a.enforcementErr).FontSize(11).TextColor(t.Danger)
		}
	})
}
