package network

import (
	netcore "agent-ebpf-filter/internal/network"
)

// Re-exported IPScope constants from internal/network.
const (
	ScopePublic        = netcore.ScopePublic
	ScopeLoopback      = netcore.ScopeLoopback
	ScopePrivate       = netcore.ScopePrivate
	ScopeLinkLocal     = netcore.ScopeLinkLocal
	ScopeCGNAT         = netcore.ScopeCGNAT
	ScopeMulticast     = netcore.ScopeMulticast
	ScopeBroadcast     = netcore.ScopeBroadcast
	ScopeDocumentation = netcore.ScopeDocumentation
	ScopeBenchmarking  = netcore.ScopeBenchmarking
	ScopeUnspecified   = netcore.ScopeUnspecified
	ScopeReserved      = netcore.ScopeReserved
	ScopeUniqueLocal   = netcore.ScopeUniqueLocal
	ScopeDiscard       = netcore.ScopeDiscard
	ScopeIPv4Mapped    = netcore.ScopeIPv4Mapped
	ScopeUnknown       = netcore.ScopeUnknown
)

// Re-exported TCPState constants from internal/network.
const (
	TCPStateUnknown     = netcore.TCPStateUnknown
	TCPStateEstablished = netcore.TCPStateEstablished
	TCPStateSynSent     = netcore.TCPStateSynSent
	TCPStateSynRecv     = netcore.TCPStateSynRecv
	TCPStateFinWait1    = netcore.TCPStateFinWait1
	TCPStateFinWait2    = netcore.TCPStateFinWait2
	TCPStateTimeWait    = netcore.TCPStateTimeWait
	TCPStateClose       = netcore.TCPStateClose
	TCPStateCloseWait   = netcore.TCPStateCloseWait
	TCPStateLastAck     = netcore.TCPStateLastAck
	TCPStateListen      = netcore.TCPStateListen
	TCPStateClosing     = netcore.TCPStateClosing
	TCPStateClosed      = netcore.TCPStateClosed
)