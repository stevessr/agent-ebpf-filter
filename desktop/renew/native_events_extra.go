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
	a.enforcementErr = ""
	a.eventDetailTab = 0
	a.eventProcessTab = 0
	a.eventProcessSelectedPID = 0
	a.eventProcessExpanded = nil
	a.eventDetailFieldSearch = ""
	a.eventDetailFieldOffset = 0
	a.eventDetailSearchSnapshot = ""
	a.eventDetailExpandedField = ""
	a.eventDetailEnforcementReady = false
	a.eventDetailPendingAction = ""
	a.eventDetailGeneration++
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

// An event ID is not a request identity: the same event can be closed and
// reopened before its first fetch completes. Keep a generation lease.
func (a *renewApp) acceptsEventDetailResponse(id string, generation uint64) bool {
	return a.eventDetailOpen && a.eventDetailID == id && a.eventDetailGeneration == generation
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
	generation := a.eventDetailGeneration
	client := a.client
	policyEnabled := a.runtimeCfg.Runtime.PolicyManagementEnabled
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		detail, err := client.eventDetail(ctx, id)
		var enforcement enforcementSnapshot
		var enforcementErr error
		enforcementQueried := false
		// Never use a cached status to enable a privileged action.
		if err == nil && policyEnabled {
			targets := enforcementTargets(detail)
			if targets.IP != "" || targets.ExecPath != "" {
				enforcementQueried = true
				enforcement, enforcementErr = client.enforcementStatus(ctx)
			}
		}
		a.update(func() {
			if !a.acceptsEventDetailResponse(id, generation) {
				return
			}
			a.eventDetailLoading = false
			if err != nil {
				a.eventDetailErr = err.Error()
				return
			}
			a.eventDetail = detail
			a.eventDetailEnforcementReady = enforcementQueried && enforcementErr == nil
			if a.eventDetailEnforcementReady {
				a.enforcement = enforcement
			}
			if enforcementErr != nil {
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
		value, _, _ := eventDetailLookupPreferred(layers, keys...)
		return value
	}
	typed := eventDetailTypedPayload(detail)
	var target eventEnforcementTargets
	if kind == "network" {
		endpoint := get("netEndpoint", "endpoint")
		if typed.Kind == "network" {
			if value := mapText(typed.Data, "endpoint"); value != "" {
				// Policy actions may have system-wide impact. If the typed
				// endpoint conflicts with the legacy endpoint, refuse to
				// guess which address is safe to block.
				legacyEndpoint := mapText(record, "netEndpoint")
				if legacyEndpoint != "" && legacyEndpoint != value {
					return target
				}
				endpoint = value
			}
		}
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
			if typed.Kind == "network" {
				if typedIP := mapText(typed.Data, "dstIp"); typedIP != "" {
					if legacyIP := mapText(record, "dstIp"); legacyIP != "" && legacyIP != typedIP {
						return target
					}
					ip = typedIP
				}
			}
			if net.ParseIP(ip) != nil {
				target.IP = ip
				port, _ := strconv.Atoi(get("dstPort"))
				if typed.Kind == "network" {
					if typedPort := mapText(typed.Data, "dstPort"); typedPort != "" {
						port, _ = strconv.Atoi(typedPort)
					}
				}
				if port >= 1 && port <= 65535 {
					target.Port = port
				}
			}
		}
	}

	// Never derive an execution block from a file operation. If both
	// protobuf exec and legacy Event paths exist, they must agree.
	typ := strings.ToLower(mapText(record, "type", "Type"))
	if kind == "process" {
		path := ""
		if typed.Kind == "process" && strings.HasSuffix(typed.Origin, ".execEvent") {
			path = mapText(typed.Data, "path")
			if legacy := mapText(record, "path"); legacy != "" && legacy != path {
				return target
			}
		} else if strings.HasPrefix(typ, "exec") || typ == "process_exec" {
			path = mapText(record, "path")
		}
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
	if a.client == nil || a.enforcementBusy || !a.eventDetailOpen ||
		!a.eventDetailEnforcementReady || !a.runtimeCfg.Runtime.PolicyManagementEnabled {
		return
	}
	a.enforcementBusy = true
	a.eventDetailEnforcementReady = false
	a.enforcementErr = ""
	generation := a.eventDetailGeneration
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		err := a.client.enforcementAction(ctx, path, payload)
		snapshot, statusErr := a.client.enforcementStatus(ctx)
		a.update(func() {
			a.enforcementBusy = false
			// Never apply a late policy snapshot to a newer investigation.
			if generation != a.eventDetailGeneration || !a.eventDetailOpen {
				a.eventDetailEnforcementReady = false
				return
			}
			if err != nil {
				a.enforcementErr = err.Error()
				return
			}
			if statusErr != nil {
				a.enforcementErr = statusErr.Error()
				return
			}
			a.enforcement = snapshot
			a.eventDetailEnforcementReady = true
		})
	}()
}

func (a *renewApp) eventDetailModal(c *ui.Context) {
	if !a.eventDetailOpen {
		return
	}
	t := c.Theme()
	viewportWidth, viewportHeight := c.Size()
	panelWidth := min(float32(980), max(float32(260), viewportWidth-48))
	scrollHeight := min(float32(680), max(float32(140), viewportHeight-182))
	ui.Modal(c, &a.eventDetailOpen, func() {
		ui.Column(c).Width(panelWidth).Gap(12).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, "事件取证详情").FontSize(20).Bold()
					ui.Text(c, a.eventDetailID).Font("monospace").FontSize(10).TextColor(t.TextMuted)
				})
				if ui.Button(c, "复制事件 ID").Clicked() {
					c.WriteClipboard(a.eventDetailID)
					c.Toast("已复制事件 ID")
				}
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
				if ui.Button(c, "重新加载").Clicked() {
					a.openEventDetail(a.eventDetailID)
				}
				return
			}
			record := detailRecord(a.eventDetail)
			if record == nil {
				ui.Text(c, "后端未返回可显示的事件内容").TextColor(t.TextMuted)
				return
			}

			ui.Tabs(c, &a.eventDetailTab, "可视化详情", "原始 JSON", "字段浏览", "进程关联")
			switch a.eventDetailTab {
			case 3:
				a.eventProcessInvestigation(c, a.eventDetail, scrollHeight)
			case 1:
				a.ensureEventDetailText()
				if ui.Button(c, "复制完整 JSON").Tooltip("原始事件可能包含敏感信息；仅主动复制时写入剪贴板").Clicked() {
					c.WriteClipboard(a.eventDetailText)
					c.Toast("已复制原始 JSON，请注意敏感信息")
				}
				ui.Scroll(c).Height(scrollHeight-40).Padding(10).Children(func() {
					ui.Text(c, a.eventDetailText).Font("monospace").FontSize(10)
				})
			case 2:
				ui.TextInput(c, &a.eventDetailFieldSearch).
					Placeholder("搜索字段名、路径或值（仅当前事件）").
					Width(panelWidth - 12)
				if a.eventDetailSearchSnapshot != a.eventDetailFieldSearch {
					a.eventDetailSearchSnapshot = a.eventDetailFieldSearch
					a.eventDetailFieldOffset = 0
					a.eventDetailExpandedField = ""
				}
				page := eventDetailTreePage(a.eventDetail, a.eventDetailFieldSearch, a.eventDetailFieldOffset, eventFieldPageSize)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					if len(page.Items) == 0 {
						ui.Text(c, "没有匹配字段").FontSize(10).TextColor(t.TextMuted)
					} else {
						ui.Text(c, fmt.Sprintf("第 %d–%d 条字段", a.eventDetailFieldOffset+1, a.eventDetailFieldOffset+len(page.Items))).FontSize(10).TextColor(t.TextMuted)
					}
					if page.ScanLimited {
						ui.Text(c, "检索达到安全扫描上限，请缩小范围或查看原始 JSON").FontSize(10).TextColor(t.Warning)
					}
					if a.eventDetailFieldOffset > 0 && ui.Button(c, "上一页").Clicked() {
						a.eventDetailFieldOffset = max(0, a.eventDetailFieldOffset-eventFieldPageSize)
						a.eventDetailExpandedField = ""
					}
					if page.HasMore && !page.ScanLimited && ui.Button(c, "下一页").Clicked() {
						a.eventDetailFieldOffset += eventFieldPageSize
						a.eventDetailExpandedField = ""
					}
				})
				ui.Scroll(c).Key(fmt.Sprintf("event-fields-page-%d", a.eventDetailFieldOffset)).
					Height(max(float32(70), scrollHeight-82)).Gap(7).Children(func() {
					for _, field := range page.Items {
						field := field
						ui.Column(c).Gap(3).Padding(8).Radius(6).Background(t.Surface).Children(func() {
							ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
								ui.Text(c, field.Label).Font("monospace").FontSize(10).TextColor(t.TextMuted).Grow(1).MinWidth(0)
								if len([]rune(field.Value)) > 300 {
									key := "tree:" + field.Label
									label := "展开"
									if a.eventDetailExpandedField == key { label = "收起" }
									if ui.Button(c, label).Clicked() {
										if a.eventDetailExpandedField == key { a.eventDetailExpandedField = "" } else { a.eventDetailExpandedField = key }
									}
								}
								if ui.Button(c, "复制").Tooltip("复制该字段的完整值").Clicked() {
									c.WriteClipboard(field.Value)
									c.Toast("字段值已复制")
								}
							})
							if a.eventDetailExpandedField == "tree:"+field.Label {
								ui.Text(c, field.Value).Font("monospace").FontSize(11)
							} else {
								ui.Text(c, eventDetailPreview(field.Value, 300)).Font("monospace").FontSize(11).MaxLines(4)
							}
						})
					}
				})
			default:
				model := eventDetailModel(a.eventDetail)
				ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
					ref := processReferenceFromEvent(a.eventDetail)
					if ref.PID > 0 {
						if ui.Button(c, "查看进程树").Clicked() {
							a.eventProcessSelectedPID = ref.PID
							a.eventProcessTab = 0
							a.eventDetailTab = 3
						}
						if ui.Button(c, "进程详细信息").Clicked() {
							a.eventProcessSelectedPID = ref.PID
							a.eventProcessTab = 1
							a.eventDetailTab = 3
						}
					}
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
// All privileged policy mutations require a fresh status and a separate
// explicit confirmation. An IP or port rule can affect traffic beyond this
// single event, and must never be a one-click operation in a detail panel.
func (a *renewApp) eventDetailEnforcementChoice(
	c *ui.Context, label, path, key string, value any, scope string,
) {
	actionID := path + "|" + fmt.Sprint(value)
	if a.eventDetailPendingAction != actionID {
		if ui.Button(c, "选择 · "+label).Clicked() {
			a.eventDetailPendingAction = actionID
		}
		return
	}
	ui.Column(c).Gap(6).Padding(9).Radius(6).Background(c.Theme().Surface).Children(func() {
		ui.Text(c, "待确认："+label).FontSize(12).Bold()
		ui.Text(c, scope).FontSize(11).TextColor(c.Theme().Warning)
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "确认修改策略").Clicked() {
				a.eventDetailPendingAction = ""
				a.runEnforcement(path, map[string]any{key: value})
			}
			if ui.Button(c, "取消").Clicked() {
				a.eventDetailPendingAction = ""
			}
		})
	})
}

func (a *renewApp) eventDetailEnforcement(c *ui.Context) {
	t := c.Theme()
	targets := enforcementTargets(a.eventDetail)
	if targets.IP == "" && targets.ExecPath == "" {
		return
	}
	card(c, "可选处置 · 手动确认", func() {
		if !a.runtimeCfg.Runtime.PolicyManagementEnabled {
			ui.Text(c, "策略管理未启用，可前往采集设置查看权限。").FontSize(11).TextColor(t.TextMuted)
			return
		}
		if a.enforcementBusy {
			ui.Text(c, "正在等待策略提交及内核状态确认…").FontSize(11).TextColor(t.TextMuted)
			return
		}
		if a.enforcementErr != "" {
			ui.Text(c, "无法确认策略状态："+a.enforcementErr).FontSize(11).TextColor(t.Danger)
			return
		}
		if !a.eventDetailEnforcementReady {
			ui.Text(c, "当前没有可验证的新鲜策略状态，已禁用修改操作。请重新加载事件详情。").FontSize(11).TextColor(t.Warning)
			return
		}
		ui.Text(c, "策略会持续影响后续进程或网络活动；与这条历史事件的风险评分无直接等价关系。").FontSize(11).TextColor(t.TextMuted)
		ui.Column(c).Gap(8).Children(func() {
			if targets.IP != "" {
				blocked := containsString(a.enforcement.Cgroup.BlockedIPs, targets.IP)
				label, path := "阻断 IP "+targets.IP, "/sandbox/cgroup/block-ip"
				if blocked {
					label, path = "解除 IP "+targets.IP, "/sandbox/cgroup/unblock-ip"
				}
				a.eventDetailEnforcementChoice(c, label, path, "ip", targets.IP,
					"此操作影响命中该 IP 的后续网络连接，不仅限于当前事件。")
			}
			if targets.Port > 0 {
				blocked := containsInt(a.enforcement.Cgroup.BlockedPorts, targets.Port)
				label, path := fmt.Sprintf("阻断端口 %d（所有目标 IP）", targets.Port), "/sandbox/cgroup/block-port"
				if blocked {
					label, path = fmt.Sprintf("解除端口 %d（所有目标 IP）", targets.Port), "/sandbox/cgroup/unblock-port"
				}
				a.eventDetailEnforcementChoice(c, label, path, "port", targets.Port,
					"端口策略按端口匹配，不绑定事件中的目标 IP，可能影响多个服务。")
			}
			if targets.ExecPath != "" {
				blocked := containsString(a.enforcement.LSM.BlockedExecPaths, targets.ExecPath)
				label, path := "阻止后续执行 "+targets.ExecPath, "/sandbox/lsm/block-exec-path"
				if blocked {
					label, path = "解除执行阻断 "+targets.ExecPath, "/sandbox/lsm/unblock-exec-path"
				}
				a.eventDetailEnforcementChoice(c, label, path, "path", targets.ExecPath,
					"此 LSM 路径规则作用于后续执行请求，不会撤销已有进程或当前记录。")
			}
		})
	})
}
