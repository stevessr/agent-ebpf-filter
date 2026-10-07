package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type monitoringModule struct {
	Key         string
	Title       string
	Description string
	Cost        string
	EventTypes  []uint32
}

type monitoringProfile struct {
	Key                string
	Title              string
	Description        string
	Modules            []string
	StatsIntervalMS    int
	LoopDetection      bool
	SignalProcessing   bool
	ResearchProcessing bool
}

var nativeMonitoringModules = []monitoringModule{
	{
		Key: "process", Title: "进程行为", Cost: "低",
		Description: "程序启动、派生、退出与进程生命周期；适合长期常开。",
		EventTypes:  []uint32{0, 18, 19, 26, 27, 28, 29},
	},
	{
		Key: "file-changes", Title: "文件改动", Cost: "低",
		Description: "创建、写入、删除、改名、权限与链接变化。",
		EventTypes:  []uint32{3, 4, 10, 12, 13, 14, 15, 16, 17},
	},
	{
		Key: "network", Title: "基础联网", Cost: "低",
		Description: "连接、监听与 DNS 行为，用于观察 Agent 外联目标。",
		EventTypes:  []uint32{2, 6, 31, 32, 34},
	},
	{
		Key: "file-access", Title: "文件访问", Cost: "高",
		Description: "open/read/ioctl 等高频访问，排查敏感文件读取时再开启。",
		EventTypes:  []uint32{1, 5, 9, 11},
	},
	{
		Key: "network-detail", Title: "网络细节", Cost: "高",
		Description: "send/recv/socket/accept 与 TCP 状态细节，适合短时调查。",
		EventTypes:  []uint32{7, 8, 20, 21, 22, 33},
	},
	{
		Key: "deep-syscall", Title: "深度系统调用", Cost: "高",
		Description: "generic syscall 轨迹，只建议专业诊断短时开启。",
		EventTypes:  []uint32{25},
	},
}

var nativeMonitoringProfiles = []monitoringProfile{
	{
		Key: "lite", Title: "轻量",
		Description: "进程 + 文件改动，10 秒系统采样，适合长期后台常驻。",
		Modules: []string{"process", "file-changes"}, StatsIntervalMS: 10_000,
	},
	{
		Key: "daily", Title: "日常推荐",
		Description: "进程 + 文件改动 + 基础联网，保留轻量行为检测。",
		Modules: []string{"process", "file-changes", "network"}, StatsIntervalMS: 5_000,
		LoopDetection: true, SignalProcessing: true,
	},
	{
		Key: "deep", Title: "深度",
		Description: "全部内核事件组 + 研究处理，适合短时排障。",
		Modules: []string{"process", "file-changes", "network", "file-access", "network-detail", "deep-syscall"},
		StatsIntervalMS: 2_000, LoopDetection: true, SignalProcessing: true, ResearchProcessing: true,
	},
}

type monitoringState struct {
	Ready                   bool
	Busy                    bool
	Error                   string
	Notice                  string
	Runtime                 map[string]any
	DisabledEventTypes      map[uint32]bool
	PersistedEventLogPath   string
	PersistedEventLogAlive  bool
	StatsIntervalMS         int
}

type rulesState struct {
	Ready  bool
	Busy   bool
	Error  string
	Notice string
	Items  []wrapperRule
}

func monitoringModuleByKey(key string) (monitoringModule, bool) {
	for _, module := range nativeMonitoringModules {
		if module.Key == key {
			return module, true
		}
	}
	return monitoringModule{}, false
}

func monitoringProfileByKey(key string) (monitoringProfile, bool) {
	for _, profile := range nativeMonitoringProfiles {
		if profile.Key == key {
			return profile, true
		}
	}
	return monitoringProfile{}, false
}

func allNativeMonitoringEventTypes() map[uint32]bool {
	out := make(map[uint32]bool)
	for _, module := range nativeMonitoringModules {
		for _, eventType := range module.EventTypes {
			out[eventType] = true
		}
	}
	return out
}

func disabledEventTypeSet(runtime map[string]any) map[uint32]bool {
	out := make(map[uint32]bool)
	values, ok := runtime["disabledEventTypes"]
	if !ok {
		return out
	}
	switch list := values.(type) {
	case []any:
		for _, value := range list {
			switch n := value.(type) {
			case float64:
				if n >= 0 {
					out[uint32(n)] = true
				}
			case json.Number:
				if v, err := strconv.ParseUint(n.String(), 10, 32); err == nil {
					out[uint32(v)] = true
				}
			}
		}
	case []uint32:
		for _, value := range list {
			out[value] = true
		}
	case []int:
		for _, value := range list {
			if value >= 0 {
				out[uint32(value)] = true
			}
		}
	}
	return out
}

func sortedDisabledEventTypes(values map[uint32]bool) []uint32 {
	out := make([]uint32, 0, len(values))
	for value, disabled := range values {
		if disabled {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func moduleEnabled(disabled map[uint32]bool, module monitoringModule) bool {
	for _, eventType := range module.EventTypes {
		if disabled[eventType] {
			return false
		}
	}
	return true
}

func runtimeBool(runtime map[string]any, key string) bool {
	value, _ := runtime[key].(bool)
	return value
}

func runtimeNestedEnabled(runtime map[string]any, key string) bool {
	value, _ := runtime[key].(map[string]any)
	enabled, _ := value["enabled"].(bool)
	return enabled
}

func cloneAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func runtimeNestedPatch(runtime map[string]any, key string, enabled bool) map[string]any {
	nested, _ := runtime[key].(map[string]any)
	next := cloneAnyMap(nested)
	next["enabled"] = enabled
	return next
}

func monitoringProfileDisabled(current map[uint32]bool, profile monitoringProfile) map[uint32]bool {
	owned := allNativeMonitoringEventTypes()
	next := make(map[uint32]bool, len(current)+len(owned))
	for eventType, disabled := range current {
		if disabled && !owned[eventType] {
			next[eventType] = true
		}
	}
	enabledModules := make(map[string]bool, len(profile.Modules))
	for _, key := range profile.Modules {
		enabledModules[key] = true
	}
	for _, module := range nativeMonitoringModules {
		if enabledModules[module.Key] {
			continue
		}
		for _, eventType := range module.EventTypes {
			next[eventType] = true
		}
	}
	return next
}

func (a *nativeApp) refreshConfig() {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()
	if client == nil {
		return
	}
	go a.loadMonitoring(client)
	go a.loadRules(client)
}

func (a *nativeApp) loadMonitoring(client *apiClient) {
	a.update(func(data *appData) {
		data.Monitoring.Busy = true
		data.Monitoring.Error = ""
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	response, err := client.runtimeConfig(ctx)
	if err != nil {
		a.update(func(data *appData) {
			data.Monitoring.Busy = false
			data.Monitoring.Ready = false
			data.Monitoring.Error = "无法加载监控配置: " + err.Error()
		})
		return
	}
	a.update(func(data *appData) {
		applyRuntimeConfigResponse(&data.Monitoring, response)
		data.Monitoring.Busy = false
		data.Monitoring.Ready = true
		data.Monitoring.Error = ""
	})
}

func applyRuntimeConfigResponse(state *monitoringState, response runtimeConfigResponse) {
	state.Runtime = response.Runtime
	state.DisabledEventTypes = disabledEventTypeSet(response.Runtime)
	state.PersistedEventLogPath = response.PersistedEventLogPath
	state.PersistedEventLogAlive = response.PersistedEventLogAlive
	if state.StatsIntervalMS <= 0 {
		state.StatsIntervalMS = 5_000
	}
}

func (a *nativeApp) setMonitoringModule(key string, enabled bool) {
	module, ok := monitoringModuleByKey(key)
	if !ok {
		return
	}
	a.mu.Lock()
	client := a.client
	state := a.data.Monitoring
	if client == nil || !state.Ready || state.Busy {
		a.mu.Unlock()
		return
	}
	a.data.Monitoring.Busy = true
	a.data.Monitoring.Error = ""
	a.data.Monitoring.Notice = ""
	a.mu.Unlock()
	a.invalidate()

	go func() {
		disabled := make(map[uint32]bool, len(state.DisabledEventTypes))
		for eventType, value := range state.DisabledEventTypes {
			disabled[eventType] = value
		}
		for _, eventType := range module.EventTypes {
			if enabled {
				delete(disabled, eventType)
			} else {
				disabled[eventType] = true
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		response, err := client.patchRuntime(ctx, map[string]any{
			"disabledEventTypes": sortedDisabledEventTypes(disabled),
		})
		a.update(func(data *appData) {
			data.Monitoring.Busy = false
			if err != nil {
				data.Monitoring.Error = "更新监控模块失败: " + err.Error()
				return
			}
			applyRuntimeConfigResponse(&data.Monitoring, response)
			data.Monitoring.Ready = true
			data.Monitoring.Error = ""
			if enabled {
				data.Monitoring.Notice = "已启用：" + module.Title
			} else {
				data.Monitoring.Notice = "已停用：" + module.Title
			}
		})
	}()
}

func (a *nativeApp) setRuntimeMonitoringToggle(key string, enabled bool) {
	a.mu.Lock()
	client := a.client
	state := a.data.Monitoring
	if client == nil || !state.Ready || state.Busy {
		a.mu.Unlock()
		return
	}
	a.data.Monitoring.Busy = true
	a.data.Monitoring.Error = ""
	a.data.Monitoring.Notice = ""
	a.mu.Unlock()
	a.invalidate()

	var patch map[string]any
	switch key {
	case "loopDetection", "signalProcessing", "researchProcessing":
		patch = map[string]any{key: runtimeNestedPatch(state.Runtime, key, enabled)}
	case "tlsCaptureEnabled", "logPersistenceEnabled":
		patch = map[string]any{key: enabled}
	default:
		a.update(func(data *appData) { data.Monitoring.Busy = false })
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		response, err := client.patchRuntime(ctx, patch)
		a.update(func(data *appData) {
			data.Monitoring.Busy = false
			if err != nil {
				data.Monitoring.Error = "更新运行时监控配置失败: " + err.Error()
				return
			}
			applyRuntimeConfigResponse(&data.Monitoring, response)
			data.Monitoring.Ready = true
			data.Monitoring.Error = ""
			data.Monitoring.Notice = "运行时监控配置已更新"
		})
	}()
}

func (a *nativeApp) applyMonitoringProfile(key string) {
	profile, ok := monitoringProfileByKey(key)
	if !ok {
		return
	}
	a.mu.Lock()
	client := a.client
	state := a.data.Monitoring
	if client == nil || !state.Ready || state.Busy {
		a.mu.Unlock()
		return
	}
	a.data.Monitoring.Busy = true
	a.data.Monitoring.Error = ""
	a.data.Monitoring.Notice = ""
	a.mu.Unlock()
	a.invalidate()

	disabled := monitoringProfileDisabled(state.DisabledEventTypes, profile)
	patch := map[string]any{
		"disabledEventTypes":  sortedDisabledEventTypes(disabled),
		"loopDetection":       runtimeNestedPatch(state.Runtime, "loopDetection", profile.LoopDetection),
		"signalProcessing":    runtimeNestedPatch(state.Runtime, "signalProcessing", profile.SignalProcessing),
		"researchProcessing":  runtimeNestedPatch(state.Runtime, "researchProcessing", profile.ResearchProcessing),
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		response, err := client.patchRuntime(ctx, patch)
		if err != nil {
			a.update(func(data *appData) {
				data.Monitoring.Busy = false
				data.Monitoring.Error = "应用监控档位失败: " + err.Error()
			})
			return
		}
		a.update(func(data *appData) {
			applyRuntimeConfigResponse(&data.Monitoring, response)
			data.Monitoring.StatsIntervalMS = profile.StatsIntervalMS
			data.Monitoring.Busy = false
			data.Monitoring.Ready = true
			data.Monitoring.Error = ""
			data.Monitoring.Notice = "已应用监控档位：" + profile.Title
		})
		a.restartSystemStream()
	}()
}

func activeMonitoringProfile(state monitoringState) string {
	if !state.Ready {
		return ""
	}
	for _, profile := range nativeMonitoringProfiles {
		expected := monitoringProfileDisabled(map[uint32]bool{}, profile)
		matchOwned := true
		owned := allNativeMonitoringEventTypes()
		for eventType := range owned {
			if state.DisabledEventTypes[eventType] != expected[eventType] {
				matchOwned = false
				break
			}
		}
		if !matchOwned ||
			runtimeNestedEnabled(state.Runtime, "loopDetection") != profile.LoopDetection ||
			runtimeNestedEnabled(state.Runtime, "signalProcessing") != profile.SignalProcessing ||
			runtimeNestedEnabled(state.Runtime, "researchProcessing") != profile.ResearchProcessing ||
			state.StatsIntervalMS != profile.StatsIntervalMS {
			continue
		}
		return profile.Key
	}
	return "custom"
}

func (a *nativeApp) loadRules(client *apiClient) {
	a.update(func(data *appData) {
		data.Rules.Busy = true
		data.Rules.Error = ""
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	rules, err := client.rules(ctx)
	if err != nil {
		a.update(func(data *appData) {
			data.Rules.Busy = false
			data.Rules.Ready = false
			data.Rules.Error = "无法加载 Wrapper 规则: " + err.Error()
		})
		return
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority == rules[j].Priority {
			return rules[i].Comm < rules[j].Comm
		}
		return rules[i].Priority > rules[j].Priority
	})
	a.update(func(data *appData) {
		data.Rules.Items = rules
		data.Rules.Busy = false
		data.Rules.Ready = true
		data.Rules.Error = ""
	})
}

func (a *nativeApp) saveWrapperRule(rule wrapperRule) {
	a.mu.Lock()
	client := a.client
	if client == nil || a.data.Rules.Busy {
		a.mu.Unlock()
		return
	}
	a.data.Rules.Busy = true
	a.data.Rules.Error = ""
	a.data.Rules.Notice = ""
	a.mu.Unlock()
	a.invalidate()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		err := client.saveRule(ctx, rule)
		cancel()
		if err != nil {
			a.update(func(data *appData) {
				data.Rules.Busy = false
				data.Rules.Error = "保存规则失败: " + err.Error()
			})
			return
		}
		a.update(func(data *appData) {
			data.Rules.Busy = false
			data.Rules.Notice = "规则已保存：" + rule.Comm
		})
		a.loadRules(client)
	}()
}

func (a *nativeApp) deleteWrapperRule(comm string) {
	comm = strings.TrimSpace(comm)
	if comm == "" {
		return
	}
	a.mu.Lock()
	client := a.client
	if client == nil || a.data.Rules.Busy {
		a.mu.Unlock()
		return
	}
	a.data.Rules.Busy = true
	a.data.Rules.Error = ""
	a.data.Rules.Notice = ""
	a.mu.Unlock()
	a.invalidate()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		err := client.deleteRule(ctx, comm)
		cancel()
		if err != nil {
			a.update(func(data *appData) {
				data.Rules.Busy = false
				data.Rules.Error = "删除规则失败: " + err.Error()
			})
			return
		}
		a.update(func(data *appData) {
			data.Rules.Busy = false
			data.Rules.Notice = "规则已删除：" + comm
		})
		a.loadRules(client)
	}()
}

func parseRewriteArgs(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, fmt.Errorf("重写命令必须是 JSON 字符串数组，例如 [\"echo\",\"hello\"]: %w", err)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("固定 REWRITE 至少需要一个命令参数")
	}
	for _, value := range args {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("重写命令不能包含空参数")
		}
	}
	return args, nil
}
