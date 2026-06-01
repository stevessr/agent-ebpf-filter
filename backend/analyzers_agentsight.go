package main

type AgentSightAnalyzer interface {
	Name() string
}

type AgentSightTLSAnalyzer interface {
	AgentSightAnalyzer
	Analyze(fragment completedTLSFragment) TLSPlaintextEvent
}

type AgentSightTLSFilter interface {
	AgentSightAnalyzer
	Filter(events []TLSPlaintextEvent, expression string) []TLSPlaintextEvent
}

type AgentSightHTTPAnalyzer struct{}

type AgentSightHTTPFilter struct{}

func (AgentSightHTTPAnalyzer) Name() string { return "agentsight.http_parser" }
func (AgentSightHTTPFilter) Name() string   { return "agentsight.http_filter" }

func (AgentSightHTTPAnalyzer) Analyze(fragment completedTLSFragment) TLSPlaintextEvent {
	return parseTLSPlaintext(fragment)
}

func (AgentSightHTTPFilter) Filter(events []TLSPlaintextEvent, expression string) []TLSPlaintextEvent {
	return filterTLSCaptureEvents(events, expression)
}

var agentSightHTTPAnalyzer AgentSightTLSAnalyzer = AgentSightHTTPAnalyzer{}
var agentSightHTTPFilter AgentSightTLSFilter = AgentSightHTTPFilter{}
