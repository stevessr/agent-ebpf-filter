package events

import (
	"math"
	"testing"
)

func TestKernelRiskFeedbackCurrentAuthorization(t *testing.T) {
	kinds := []string{
		kernelRiskFeedbackKindNetworkIP, kernelRiskFeedbackKindNetworkPort,
		kernelRiskFeedbackKindLSMFileName, kernelRiskFeedbackKindLSMExecPath,
		kernelRiskFeedbackKindLSMExecName, "unknown",
	}
	scores := []float64{0, 59, 60, 84, 85, 94, 95, 100, math.NaN(), math.Inf(1)}
	cases := 0
	for mask := 0; mask < 32; mask++ {
		for _, threshold := range []float64{0, 60, 95, math.NaN()} {
			settings := RuntimeSettings{
				PolicyManagementEnabled: mask&1 != 0,
				KernelRiskFeedback: KernelRiskFeedbackSettings{
					Enabled: mask&2 != 0, EnforceNetwork: mask&4 != 0,
					EnforceFileNames: mask&8 != 0, EnforceExec: mask&16 != 0,
					MinRiskScore: threshold,
				},
			}
			minimum := threshold
			if minimum == 0 {
				minimum = 85
			}
			for i, kind := range kinds {
				for _, score := range scores {
					gate := []bool{mask&4 != 0, mask&4 != 0, mask&8 != 0, mask&16 != 0, mask&16 != 0, false}[i]
					want := mask&3 == 3 && gate && !math.IsInf(score, 0) && score >= minimum && ((i != 1 && i != 4) || score >= 95)
					got := kernelRiskFeedbackActionAllowed(settings, kernelRiskFeedbackAction{Kind: kind, Target: "fixture", Score: score})
					if got != want {
						t.Fatalf("authorization mismatch mask=%d threshold=%v kind=%s score=%v got=%t want=%t", mask, threshold, kind, score, got, want)
					}
					cases++
				}
			}
		}
	}
	if cases != 7680 {
		t.Fatalf("unexpected coverage: %d", cases)
	}
	t.Logf("AD_FIXTURE feedback_authorization cases=%d", cases)
}
