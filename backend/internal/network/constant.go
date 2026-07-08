package network

// ── IP Address Classification ─────────────────────────────────────────────────

const (
	ScopePublic        IPScope = "Public"
	ScopeLoopback      IPScope = "Loopback"
	ScopePrivate       IPScope = "Private"
	ScopeLinkLocal     IPScope = "Link-Local"
	ScopeCGNAT         IPScope = "CGNAT"
	ScopeMulticast     IPScope = "Multicast"
	ScopeBroadcast     IPScope = "Broadcast"
	ScopeDocumentation IPScope = "Documentation"
	ScopeBenchmarking  IPScope = "Benchmarking"
	ScopeUnspecified   IPScope = "Unspecified"
	ScopeReserved      IPScope = "Reserved"
	ScopeUniqueLocal   IPScope = "Unique-Local"
	ScopeDiscard       IPScope = "Discard"
	ScopeIPv4Mapped    IPScope = "IPv4-Mapped"
	ScopeUnknown       IPScope = "Unknown"
)

// ── TCP State Machine (RFC 793) ──────────────────────────────────────────────

const (
	TCPStateUnknown     TCPState = 0
	TCPStateEstablished TCPState = 1
	TCPStateSynSent     TCPState = 2
	TCPStateSynRecv     TCPState = 3
	TCPStateFinWait1    TCPState = 4
	TCPStateFinWait2    TCPState = 5
	TCPStateTimeWait    TCPState = 6
	TCPStateClose       TCPState = 7
	TCPStateCloseWait   TCPState = 8
	TCPStateLastAck     TCPState = 9
	TCPStateListen      TCPState = 10
	TCPStateClosing     TCPState = 11
	TCPStateClosed      TCPState = 12
)