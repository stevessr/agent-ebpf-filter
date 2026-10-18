package main

// summaryOwnershipIndex is shared across UI consumers within the current
// compact event snapshot. High-rate event rows must not rebuild an O(events)
// attribution index on every frame/row.
//
// A zero version is treated as uncached: tests and initial setup may construct
// or replace snapshots directly without using mergeEventWindow.
func (a *renewApp) summaryOwnershipIndex() agentOwnershipIndex {
	if a.ownerCacheValid && a.eventsVersion != 0 &&
		a.ownerCacheVersion == a.eventsVersion {
		return a.ownerCache
	}
	a.ownerCache = buildAgentOwnershipIndex(a.events, nil)
	a.ownerCacheVersion = a.eventsVersion
	a.ownerCacheValid = true
	return a.ownerCache
}
