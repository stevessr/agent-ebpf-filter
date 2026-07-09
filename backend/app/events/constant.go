package events

import (
	"time"

	"agent-ebpf-filter/internal/network"
	"agent-ebpf-filter/internal/protocoldetect"
)

const EventSchemaVersion = "event.v3"

const (
	AppProtoDNS  = protocoldetect.AppProtoDNS
	AppProtomDNS = protocoldetect.AppProtomDNS
)

const (
	TCPStateSynSent     = network.TCPStateSynSent
	TCPStateEstablished = network.TCPStateEstablished
)

const (
	SemanticSecretCorrelationTTL = 30 * time.Second
	SemanticExecCorrelationTTL   = 30 * time.Second
	SemanticForkWindow           = 2 * time.Second
	SemanticForkStormThreshold   = 8
	SemanticAgenticLoopWindow    = 10 * time.Second
	SemanticPromptLoopThreshold  = 3
	SemanticAPILoopThreshold     = 3
	SemanticFileIOLoopThreshold  = 3
	SemanticFileContentionTTL    = 15 * time.Second
)

const (
	kernelRiskFeedbackKindNetworkIP   = "cgroup_ip"
	kernelRiskFeedbackKindNetworkPort = "cgroup_port"
	kernelRiskFeedbackKindLSMFileName = "lsm_file_name"
	kernelRiskFeedbackKindLSMExecPath = "lsm_exec_path"
	kernelRiskFeedbackKindLSMExecName = "lsm_exec_name"

	KernelRiskFeedbackKindNetworkIP   = kernelRiskFeedbackKindNetworkIP
	KernelRiskFeedbackKindNetworkPort = kernelRiskFeedbackKindNetworkPort
	KernelRiskFeedbackKindLSMFileName = kernelRiskFeedbackKindLSMFileName
	KernelRiskFeedbackKindLSMExecPath = kernelRiskFeedbackKindLSMExecPath
	KernelRiskFeedbackKindLSMExecName = kernelRiskFeedbackKindLSMExecName
)