package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/gorilla/websocket"
)

const (
	initialSummaryLimit = 240
	maxNativeEvents     = 1200
	reconnectDelay      = 3 * time.Second
)

type processSnapshot struct {
	PID     int32
	PPID    int32
	Name    string
	CPU     float64
	Memory  float32
	User    string
	Command string
}

type systemSnapshot struct {
	CPUPercent float64
	MemPercent float32
	MemUsed    uint64
	MemTotal   uint64
	NetRecv    uint64
	NetSent    uint64
	Processes  []processSnapshot
}

type appData struct {
	StartupStatus   string
	StartupError    string
	EventsConnected bool
	SystemConnected bool
	LastStreamError string
	Events          []eventSummary
	System          systemSnapshot
	SelectedEventID string
	Detail          string
	DetailLoading   bool
	Monitoring      monitoringState
	Rules           rulesState
}

type nativeApp struct {
	backend string

	mu     sync.RWMutex
	data   appData
	client     *apiClient
	window     *mygo.Window
	systemConn *websocket.Conn

	// View-only state is read and written on MyGo's main thread.
	page              string
	query             string
	onlyAttention     bool
	onlyAgents        bool
	ruleComm          string
	ruleAction        string
	ruleRegex         string
	ruleReplacement   string
	rulePriority      string
	ruleRewrite       string
	ruleEditing       string
	pendingRuleDelete string
}

func newNativeApp(backend string) *nativeApp {
	return &nativeApp{
		backend: backend,
		page:    "overview",
		ruleAction:  "ALERT",
		rulePriority: "0",
		ruleRewrite: "[]",
		data: appData{
			StartupStatus: "准备启动…",
			Monitoring: monitoringState{
				Runtime:            map[string]any{},
				DisabledEventTypes: map[uint32]bool{},
				StatsIntervalMS:    5_000,
			},
		},
	}
}

func (a *nativeApp) setWindow(window *mygo.Window) {
	a.mu.Lock()
	a.window = window
	a.mu.Unlock()
}

func (a *nativeApp) invalidate() {
	a.mu.RLock()
	window := a.window
	a.mu.RUnlock()
	if window != nil {
		window.Invalidate()
	}
}

func (a *nativeApp) update(fn func(*appData)) {
	a.mu.Lock()
	fn(&a.data)
	a.mu.Unlock()
	a.invalidate()
}

func (a *nativeApp) snapshot() appData {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := a.data
	out.Events = append([]eventSummary(nil), a.data.Events...)
	out.System.Processes = append([]processSnapshot(nil), a.data.System.Processes...)
	out.Rules.Items = append([]wrapperRule(nil), a.data.Rules.Items...)
	return out
}

func (a *nativeApp) setStartup(status, errText string) {
	a.update(func(data *appData) {
		data.StartupStatus = status
		data.StartupError = errText
	})
}

func (a *nativeApp) setClient(client *apiClient) {
	a.mu.Lock()
	a.client = client
	a.mu.Unlock()
}

func (a *nativeApp) run(ctx context.Context, client *apiClient) {
	a.refreshSummaries(ctx, client)
	a.refreshConfig()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		a.runSummaryStream(ctx, client)
	}()
	go func() {
		defer wg.Done()
		a.runSystemStream(ctx, client)
	}()
	<-ctx.Done()
	wg.Wait()
}

func (a *nativeApp) refresh() {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()
	if client == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		a.refreshSummaries(ctx, client)
	}()
	a.refreshConfig()
}

func (a *nativeApp) refreshSummaries(ctx context.Context, client *apiClient) {
	events, err := client.summaries(ctx, initialSummaryLimit)
	if err != nil {
		a.update(func(data *appData) { data.LastStreamError = "加载事件摘要失败: " + err.Error() })
		return
	}
	a.update(func(data *appData) {
		data.Events = mergeEventSummaries(data.Events, events, maxNativeEvents)
		data.LastStreamError = ""
	})
}

func (a *nativeApp) runSummaryStream(ctx context.Context, client *apiClient) {
	for ctx.Err() == nil {
		conn, _, err := client.dialWebSocket(ctx, "/ws/event-summaries", nil)
		if err != nil {
			a.streamFailure("事件流", err, true)
			if !sleepContext(ctx, reconnectDelay) {
				return
			}
			continue
		}
		a.update(func(data *appData) {
			data.EventsConnected = true
			data.LastStreamError = ""
		})
		stopClose := closeWebSocketOnContext(ctx, conn)
		err = a.consumeSummaryStream(ctx, conn)
		close(stopClose)
		_ = conn.Close()
		if ctx.Err() != nil {
			return
		}
		a.streamFailure("事件流", err, true)
		if !sleepContext(ctx, reconnectDelay) {
			return
		}
	}
}

func (a *nativeApp) consumeSummaryStream(ctx context.Context, conn *websocket.Conn) error {
	for ctx.Err() == nil {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		var batch summaryBatch
		if err := json.Unmarshal(payload, &batch); err != nil {
			return fmt.Errorf("decode event summaries: %w", err)
		}
		if len(batch.Events) == 0 {
			continue
		}
		a.update(func(data *appData) {
			data.Events = mergeEventSummaries(data.Events, batch.Events, maxNativeEvents)
		})
	}
	return ctx.Err()
}

func (a *nativeApp) runSystemStream(ctx context.Context, client *apiClient) {
	for ctx.Err() == nil {
		query := make(url.Values)
		query.Set("interval", fmt.Sprint(a.systemIntervalMS()))
		conn, _, err := client.dialWebSocket(ctx, "/ws/system", query)
		if err != nil {
			a.streamFailure("系统指标流", err, false)
			if !sleepContext(ctx, reconnectDelay) {
				return
			}
			continue
		}
		a.setSystemConn(conn)
		a.update(func(data *appData) {
			data.SystemConnected = true
			data.LastStreamError = ""
		})
		stopClose := closeWebSocketOnContext(ctx, conn)
		err = a.consumeSystemStream(ctx, conn)
		close(stopClose)
		a.clearSystemConn(conn)
		_ = conn.Close()
		if ctx.Err() != nil {
			return
		}
		a.streamFailure("系统指标流", err, false)
		if !sleepContext(ctx, reconnectDelay) {
			return
		}
	}
}

func (a *nativeApp) systemIntervalMS() int {
	a.mu.RLock()
	interval := a.data.Monitoring.StatsIntervalMS
	a.mu.RUnlock()
	if interval != 2_000 && interval != 5_000 && interval != 10_000 && interval != 30_000 {
		return 5_000
	}
	return interval
}

func (a *nativeApp) setSystemConn(conn *websocket.Conn) {
	a.mu.Lock()
	a.systemConn = conn
	a.mu.Unlock()
}

func (a *nativeApp) clearSystemConn(conn *websocket.Conn) {
	a.mu.Lock()
	if a.systemConn == conn {
		a.systemConn = nil
	}
	a.mu.Unlock()
}

func (a *nativeApp) restartSystemStream() {
	a.mu.RLock()
	conn := a.systemConn
	a.mu.RUnlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (a *nativeApp) consumeSystemStream(ctx context.Context, conn *websocket.Conn) error {
	for ctx.Err() == nil {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		snapshot, err := decodeSystemStats(payload)
		if err != nil {
			return fmt.Errorf("decode system stats: %w", err)
		}
		sort.Slice(snapshot.Processes, func(i, j int) bool {
			if snapshot.Processes[i].CPU == snapshot.Processes[j].CPU {
				return snapshot.Processes[i].Memory > snapshot.Processes[j].Memory
			}
			return snapshot.Processes[i].CPU > snapshot.Processes[j].CPU
		})
		a.update(func(data *appData) { data.System = snapshot })
	}
	return ctx.Err()
}

func (a *nativeApp) streamFailure(name string, err error, eventStream bool) {
	a.update(func(data *appData) {
		if eventStream {
			data.EventsConnected = false
		} else {
			data.SystemConnected = false
		}
		if err != nil {
			data.LastStreamError = name + "断开: " + err.Error()
		}
	})
}

func (a *nativeApp) loadDetail(eventID string) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return
	}
	a.mu.Lock()
	client := a.client
	a.data.SelectedEventID = eventID
	a.data.DetailLoading = true
	a.data.Detail = ""
	a.mu.Unlock()
	a.invalidate()
	if client == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		detail, err := client.eventDetail(ctx, eventID)
		a.update(func(data *appData) {
			if data.SelectedEventID != eventID {
				return
			}
			data.DetailLoading = false
			if err != nil {
				data.Detail = "读取完整事件失败: " + err.Error()
				return
			}
			data.Detail = detail
		})
	}()
}

func mergeEventSummaries(existing, incoming []eventSummary, limit int) []eventSummary {
	if limit <= 0 {
		limit = maxNativeEvents
	}
	byID := make(map[string]eventSummary, len(existing)+len(incoming))
	for _, event := range existing {
		id := eventIdentity(event)
		if id != "" {
			byID[id] = event
		}
	}
	for _, event := range incoming {
		id := eventIdentity(event)
		if id != "" {
			byID[id] = event
		}
	}
	merged := make([]eventSummary, 0, len(byID))
	for _, event := range byID {
		merged = append(merged, event)
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].ReceivedAtMS > merged[j].ReceivedAtMS })
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}

func eventIdentity(event eventSummary) string {
	if strings.TrimSpace(event.EventID) != "" {
		return strings.TrimSpace(event.EventID)
	}
	return strings.TrimSpace(event.Key)
}

func isAttentionEvent(event eventSummary) bool {
	decision := strings.ToUpper(strings.TrimSpace(event.Decision))
	if strings.Contains(decision, "BLOCK") || strings.Contains(decision, "DENY") || strings.Contains(decision, "ALERT") {
		return true
	}
	if event.RiskScore >= 60 {
		return true
	}
	combined := strings.ToLower(event.Type + " " + event.Tag)
	return strings.Contains(combined, "semantic_alert") || strings.Contains(combined, "agentsight_alert")
}

func isAgentEvent(event eventSummary) bool {
	return event.RootAgentPID != 0 || event.AgentRunID != "" || event.ConversationID != "" || event.ToolCallID != ""
}

func closeWebSocketOnContext(ctx context.Context, conn *websocket.Conn) chan struct{} {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	return done
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
