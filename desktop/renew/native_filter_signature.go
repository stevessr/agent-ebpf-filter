package main

import "strings"

// eventFilterSignature replaces a concatenated string key with a comparable,
// allocation-free value. Even arbitrary text containing NUL separators cannot
// collide with another filter combination.
type eventFilterSignature struct {
	Search            string
	Type              string
	Session           string
	Decision          string
	Risk              string
	Target            string
	DomainAgent       string
	DomainTarget      string
	DomainKind        string
	Manager           string
	PID               int
	RootPID           int
	MinRisk           float64
	FileEditsOnly     bool
	DelegatedOnly     bool
	AttentionOnly     bool
}

func (a *renewApp) eventFilterCacheKey() eventFilterSignature {
	// Most frames are repaints of an unchanged query. Normalize only on
	// actual user edits to avoid allocating for a mixed-case search every frame.
	if !a.searchKeyReady || a.searchKeySource != a.search {
		a.searchKeySource = a.search
		a.searchKeyNormalized = strings.ToLower(strings.TrimSpace(a.search))
		a.searchKeyReady = true
	}
	return eventFilterSignature{
		Search: a.searchKeyNormalized,
		Type: a.eventTypeFilter,
		Session: a.eventSessionFilter,
		Decision: a.eventDecisionFilter,
		Risk: a.eventRiskFilter,
		Target: a.eventTargetFilter,
		DomainAgent: a.eventDomainAgent,
		DomainTarget: a.eventDomainTarget,
		DomainKind: a.eventDomainKind,
		Manager: a.managementEventFilter,
		PID: a.eventPIDFilter,
		RootPID: a.eventRootPIDFilter,
		MinRisk: a.eventMinRiskScore,
		FileEditsOnly: a.eventFileEditsOnly,
		DelegatedOnly: a.eventDelegatedOnly,
		AttentionOnly: a.eventAttentionOnly,
	}
}
