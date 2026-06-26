package tls

// ---- moved from backend/zz_merged_backend.go section analyzers_agentsight.go ----

type AgentSightAnalyzer interface {
	Name() string
}

type AgentSightTLSAnalyzer interface {
	AgentSightAnalyzer
	Analyze(fragment CompletedTLSFragment) TLSPlaintextEvent
}

type AgentSightTLSFilter interface {
	AgentSightAnalyzer
	Filter(events []TLSPlaintextEvent, expression string) []TLSPlaintextEvent
}

type AgentSightHTTPAnalyzer struct{}

type AgentSightHTTPFilter struct{}

func (AgentSightHTTPAnalyzer) Name() string { return "agentsight.http_parser" }
func (AgentSightHTTPFilter) Name() string   { return "agentsight.http_filter" }

func (AgentSightHTTPAnalyzer) Analyze(fragment CompletedTLSFragment) TLSPlaintextEvent {
	return parseTLSPlaintext(fragment)
}

func (AgentSightHTTPFilter) Filter(events []TLSPlaintextEvent, expression string) []TLSPlaintextEvent {
	return filterTLSCaptureEvents(events, expression)
}

var agentSightHTTPAnalyzer AgentSightTLSAnalyzer = AgentSightHTTPAnalyzer{}
var agentSightHTTPFilter AgentSightTLSFilter = AgentSightHTTPFilter{}
