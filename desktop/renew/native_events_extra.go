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
	if limit <= 0 {
		limit = 1200
	}
	byID := make(map[string]eventSummary, len(existing)+len(incoming))
	for _, event := range existing {
		if strings.TrimSpace(event.EventID) != "" {
			byID[event.EventID] = event
		}
	}
	for _, event := range incoming {
		if strings.TrimSpace(event.EventID) != "" {
			byID[event.EventID] = event
		}
	}
	out := make([]eventSummary, 0, len(byID))
	for _, event := range byID {
		out = append(out, event)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ReceivedAtMS != out[j].ReceivedAtMS {
			return out[i].ReceivedAtMS > out[j].ReceivedAtMS
		}
		return out[i].EventID > out[j].EventID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func eventSessionKey(event eventSummary) string {
	contextID := strings.Trim(strings.Join([]string{event.AgentRunID, event.ConversationID}, ":"), ":")
	root := event.RootAgentPID
	if root <= 0 {
		root = event.PID
	}
	if contextID != "" {
		return contextID + " · PID " + strconv.Itoa(root)
	}
	return "PID " + strconv.Itoa(root)
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
			a.events = mergeEventSummaries(a.events, response.Events, 1200)
			a.historyCursor = response.NextCursor
			a.eventVisibleLimit += 50
		})
	}()
}

func (a *renewApp) openEventDetail(eventID string) {
	if a.client == nil || strings.TrimSpace(eventID) == "" {
		return
	}
	id := strings.TrimSpace(eventID)
	a.eventDetailID = id
	a.eventDetailOpen = true
	a.eventDetailLoading = true
	a.eventDetailErr = ""
	a.eventDetail = nil
	a.eventDetailText = ""
	a.eventDetailTab = 0
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		detail, err := a.client.eventDetail(ctx, id)
		var enforcement enforcementSnapshot
		var enforcementErr error
		if err == nil && a.runtimeCfg.Runtime.PolicyManagementEnabled {
			enforcement, enforcementErr = a.client.enforcementStatus(ctx)
		}
		a.update(func() {
			if a.eventDetailID != id {
				return
			}
			a.eventDetailLoading = false
			if err != nil {
				a.eventDetailErr = err.Error()
				return
			}
			a.eventDetail = detail
			if data, marshalErr := json.MarshalIndent(detail, "", "  "); marshalErr == nil {
				a.eventDetailText = string(data)
			}
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
	var target eventEnforcementTargets
	endpoint := mapText(record, "net_endpoint", "netEndpoint", "NetEndpoint", "endpoint")
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
		ip := mapText(record, "dst_ip", "dstIp", "DstIp")
		if net.ParseIP(ip) != nil {
			target.IP = ip
			port := int(mapNumber(record, "dst_port", "dstPort", "DstPort"))
			if port >= 1 && port <= 65535 {
				target.Port = port
			}
		}
	}
	path := mapText(record, "path", "Path")
	if strings.HasPrefix(path, "/") {
		target.ExecPath = path
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
	ui.Modal(c, &a.eventDetailOpen, func() {
		ui.Column(c).Width(760).Gap(12).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Children(func() {
					ui.Text(c, "事件详情").FontSize(20).Bold()
					ui.Text(c, a.eventDetailID).Font("monospace").FontSize(10).TextColor(t.TextMuted)
				})
				if ui.Button(c, "关闭").Clicked() {
					a.eventDetailOpen = false
				}
			})
			if a.eventDetailLoading {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c)
					ui.Text(c, "正在从后端读取完整事件…").TextColor(t.TextMuted)
				})
				return
			}
			if a.eventDetailErr != "" {
				ui.Text(c, a.eventDetailErr).TextColor(t.TextMuted)
				return
			}
			ui.Tabs(c, &a.eventDetailTab, "可视化详情", "原始 JSON")
			if a.eventDetailTab == 1 {
				ui.Scroll(c).Height(520).Children(func() {
					ui.Text(c, a.eventDetailText).Font("monospace").FontSize(10)
				})
				return
			}

			record := detailRecord(a.eventDetail)
			if record == nil {
				ui.Text(c, "后端未返回可显示的事件内容").TextColor(t.TextMuted)
				return
			}
			eventType := mapText(record, "type", "Type")
			comm := mapText(record, "comm", "Comm")
			pid := int(mapNumber(record, "pid", "Pid"))
			decision := strings.ToUpper(mapText(record, "decision", "Decision"))
			risk := mapNumber(record, "risk_score", "riskScore", "RiskScore")
			target := mapText(record, "path", "Path")
			if target == "" {
				target = mapText(record, "net_endpoint", "netEndpoint", "NetEndpoint")
			}
			if target == "" {
				target = mapText(record, "domain", "Domain")
			}
			ui.Scroll(c).Height(520).Gap(12).Children(func() {
				card(c, eventAction(eventSummary{Type: eventType}), func() {
					ui.Row(c).Gap(8).Wrap().Children(func() {
						ui.Badge(c, displayOr(eventType, "event"))
						if decision != "" {
							ui.Badge(c, decision)
						}
						if risk > 0 {
							ui.Badge(c, fmt.Sprintf("风险 %.0f", risk))
						}
					})
					ui.Text(c, displayOr(target, "未提供明确目标")).Font("monospace").TextColor(t.TextMuted)
				})
				card(c, "主体", func() {
					ui.Textf(c, "%s · PID %d", displayOr(comm, "未知进程"), pid)
					for _, item := range [][2]string{
						{"父 PID", mapText(record, "ppid", "Ppid")},
						{"UID", mapText(record, "uid", "Uid")},
						{"Tag", mapText(record, "tag", "Tag")},
						{"Agent Run", mapText(record, "agent_run_id", "agentRunId", "AgentRunId")},
						{"Conversation", mapText(record, "conversation_id", "conversationId", "ConversationId")},
						{"Tool", mapText(record, "tool_name", "toolName", "ToolName")},
						{"Trace", mapText(record, "trace_id", "traceId", "TraceId")},
					} {
						if item[1] != "" {
							ui.Text(c, item[0]+"："+item[1]).FontSize(11).TextColor(t.TextMuted)
						}
					}
				})

				targets := enforcementTargets(a.eventDetail)
				if targets.IP != "" || targets.ExecPath != "" {
					card(c, "处置动作", func() {
						if !a.runtimeCfg.Runtime.PolicyManagementEnabled {
							ui.Text(c, "策略管理未启用；先在“监控”页启用 policy_management 后才能执行阻断。").FontSize(11).TextColor(t.TextMuted)
							return
						}
						ui.Row(c).Gap(8).Wrap().Children(func() {
							if targets.IP != "" {
								blocked := containsString(a.enforcement.Cgroup.BlockedIPs, targets.IP)
								label := "阻断 IP " + targets.IP
								path := "/sandbox/cgroup/block-ip"
								if blocked {
									label = "解除 IP " + targets.IP
									path = "/sandbox/cgroup/unblock-ip"
								}
								if ui.Button(c, label).Clicked() {
									a.runEnforcement(path, map[string]any{"ip": targets.IP})
								}
							}
							if targets.Port > 0 {
								blocked := containsInt(a.enforcement.Cgroup.BlockedPorts, targets.Port)
								label := fmt.Sprintf("阻断端口 %d", targets.Port)
								path := "/sandbox/cgroup/block-port"
								if blocked {
									label = fmt.Sprintf("解除端口 %d", targets.Port)
									path = "/sandbox/cgroup/unblock-port"
								}
								if ui.Button(c, label).Clicked() {
									a.runEnforcement(path, map[string]any{"port": targets.Port})
								}
							}
							if targets.ExecPath != "" {
								blocked := containsString(a.enforcement.LSM.BlockedExecPaths, targets.ExecPath)
								label := "阻断执行 " + targets.ExecPath
								path := "/sandbox/lsm/block-exec-path"
								if blocked {
									label = "解除执行 " + targets.ExecPath
									path = "/sandbox/lsm/unblock-exec-path"
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
							ui.Text(c, a.enforcementErr).FontSize(11).TextColor(t.TextMuted)
						}
					})
				}
				ui.Text(c, "完整负载仅在详情打开期间保存在桌面内存中。").FontSize(10).TextColor(t.TextMuted)
			})
		})
	})
}
