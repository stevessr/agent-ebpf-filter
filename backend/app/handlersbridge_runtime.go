package app

import (
	"agent-ebpf-filter/app/shell"
	"agent-ebpf-filter/app/wsstream"
	"agent-ebpf-filter/internal/geoip"
	netcore "agent-ebpf-filter/internal/network"
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"
)

// ── AgentSight event store adapter ──────────────────────────────────

// agentSightStoreAdapter wraps *agentSightEventStore to implement the
// handlers.Deps.AgentSightUploadedEvents interface using any.
type agentSightStoreAdapter struct {
	store *agentSightEventStore
}

func (a *agentSightStoreAdapter) Clear() { a.store.Clear() }

func (a *agentSightStoreAdapter) Recent(limit int) []any {
	events := a.store.Recent(limit)
	out := make([]any, len(events))
	for i, e := range events {
		out[i] = e
	}
	return out
}

func (a *agentSightStoreAdapter) Add(events ...any) {
	batch := make([]agentSightExportEvent, 0, len(events))
	for _, e := range events {
		if ev, ok := e.(agentSightExportEvent); ok {
			batch = append(batch, ev)
		}
	}
	a.store.Add(batch...)
}

// ── Shell session adapter ──────────────────────────────────────────

// shellManagerAdapter wraps *shell.Manager to implement the handlers.Deps.ShellSessions interface.
type shellManagerAdapter struct {
	mgr *shell.Manager
}

type handlerNetworkFlowView struct{}

func (handlerNetworkFlowView) Query(query netcore.FlowQuery) netcore.FlowQueryResult {
	return currentNetworkFlowAggregator().Query(query)
}

func (handlerNetworkFlowView) Get(flowID string) (netcore.NetworkFlowSummary, bool) {
	return currentNetworkFlowAggregator().Get(flowID)
}

type handlerTCPTrackerView struct{}

func (handlerTCPTrackerView) Snapshot() []netcore.TCPConnectionState {
	return currentTCPConnections()
}

type handlerDNSCorrelationView struct{}

func (handlerDNSCorrelationView) LookupIP(ip string) (string, bool) {
	return currentDNSCorrelation().LookupIP(ip)
}

func (handlerDNSCorrelationView) LookupDomain(domain string) (string, bool) {
	return currentDNSCorrelation().LookupDomain(domain)
}

func (handlerDNSCorrelationView) Snapshot() []netcore.DNSCacheSnapshotEntry {
	return currentDNSCorrelation().Snapshot()
}

type handlerGeoIPView struct {
	fallback *geoip.Resolver
}

func (view handlerGeoIPView) Lookup(ip string) (geoip.Record, bool) {
	if manager := currentNetworkManager(); manager != nil {
		return manager.GeoIPLookup(ip)
	}
	return view.fallback.Lookup(ip)
}

func (a *shellManagerAdapter) Subscribe() chan struct{}     { return a.mgr.Subscribe() }
func (a *shellManagerAdapter) Unsubscribe(ch chan struct{}) { a.mgr.Unsubscribe(ch) }
func (a *shellManagerAdapter) List() []any {
	list := a.mgr.List()
	out := make([]any, len(list))
	for i, v := range list {
		out[i] = v
	}
	return out
}

func (a *shellManagerAdapter) NewSession(req any, deps any) (any, error) {
	// Convert through JSON to support the handler's anonymous request struct
	// which is structurally identical to shell.CreateRequest.
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal session request: %w", err)
	}
	var cr shell.CreateRequest
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, fmt.Errorf("unmarshal session request: %w", err)
	}
	return a.mgr.NewSession(cr, deps.(shell.Deps))
}

func (a *shellManagerAdapter) AttachWebSocket(id string, conn *websocket.Conn) error {
	session, ok := a.mgr.Get(id)
	if !ok {
		return fmt.Errorf("shell session not found")
	}
	if err := session.Attach(conn); err != nil {
		return err
	}
	go a.readWebSocket(session, conn)
	return nil
}

func (a *shellManagerAdapter) readWebSocket(session *shell.Session, conn *websocket.Conn) {
	defer session.Detach(conn)
	conn.SetReadLimit(wsstream.ControlReadLimit)
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType == websocket.TextMessage {
			var ctrl shell.ControlMessage
			if json.Unmarshal(data, &ctrl) == nil && ctrl.Type == "resize" {
				_ = session.Resize(ctrl.Cols, ctrl.Rows)
				continue
			}
		}
		_ = session.WriteInput(data)
	}
}
func (a *shellManagerAdapter) Delete(id string) error { return a.mgr.Delete(id) }
func (a *shellManagerAdapter) SendInput(id string, data []byte) error {
	return a.mgr.SendInput(id, data)
}
func (a *shellManagerAdapter) ClearClosed() { a.mgr.ClearClosed() }
