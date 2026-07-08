package tls

import "time"

const (
	tlsAgentLoopDefaultWindow = 30 * time.Second
	tlsAgentLoopRepeatLimit   = 5
	tlsAgentLoopAlertMinRisk  = 0.97
	tlsPromptDigestBytes      = 8
)