package events

import "agent-ebpf-filter/pb"

// NewUnifiedRiskAlert preserves the legacy semantic-alert wire schema and
// its 0..100 scoring convention for findings from the unified message stage.
func NewUnifiedRiskAlert(source *pb.Event, code, target, reason string, riskScore float64) *pb.Event {
	if source == nil {
		return nil
	}
	if riskScore < 0 {
		riskScore = 0
	}
	if riskScore > 100 {
		riskScore = 100
	}
	return newSemanticAlertEvent(source, code, target, reason, riskScore/100)
}
