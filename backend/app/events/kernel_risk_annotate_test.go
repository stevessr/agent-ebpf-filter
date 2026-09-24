package events

import (
	"fmt"
	"strings"
	"testing"
)

func TestAnnotateExtraInfoMatchesSprintfFormat(t *testing.T) {
	cases := []struct {
		decision kernelRiskDecision
		extra    string
	}{
		{kernelRiskDecision{Decision: "ALERT", Score: 96, Reasons: []string{"sensitive_path", "secret_material_path"}}, ""},
		{kernelRiskDecision{Decision: "", Score: 8, Reasons: []string{"agent_context"}}, "fd=3 count=512"},
		{kernelRiskDecision{Decision: "OBSERVE", Score: 42.5, Reasons: []string{"a"}}, ""},
		{kernelRiskDecision{Decision: "OBSERVE", Score: 43.5, Reasons: []string{"a"}}, "x"},
		{kernelRiskDecision{Decision: "ALERT", Score: 100, Reasons: []string{"r1", "r2", "r3", "r4", "r5", "r6"}}, "existing=1"},
		{kernelRiskDecision{Decision: "ALERT", Score: 0.4, Reasons: []string{"tiny"}}, ""},
	}
	for _, tc := range cases {
		want := fmt.Sprintf("kernel_risk score=%.0f decision=%s reasons=%s", tc.decision.Score, trimDefault(tc.decision.Decision, "OBSERVE"), tc.decision.reasonText())
		if tc.extra != "" {
			want = tc.extra + " " + want
		}
		if got := tc.decision.annotateExtraInfo(tc.extra); got != want {
			t.Fatalf("annotateExtraInfo(%q) = %q, want %q", tc.extra, got, want)
		}
	}
}

func TestAnnotateExtraInfoAllocatesOnce(t *testing.T) {
	decision := kernelRiskDecision{Decision: "ALERT", Score: 72, Reasons: []string{"agent_context", "destructive_file_mutation"}}
	if allocs := testing.AllocsPerRun(200, func() { decision.annotateExtraInfo("fd=1 count=4096") }); allocs > 1 {
		t.Fatalf("annotateExtraInfo allocated %.1f times, want at most 1", allocs)
	}
}

func TestContainsFoldASCII(t *testing.T) {
	cases := []struct {
		s, sub string
		want   bool
	}{
		{"AI Agent", "agent", true},
		{"Wrapper CLI", "wrapper", true},
		{"Git", "agent", false},
		{"", "agent", false},
		{"agent", "", true},
		{"AGENT", "agent", true},
		{"agen", "agent", false},
		{"xagentx", "agent", true},
	}
	for _, tc := range cases {
		if got := containsFoldASCII(tc.s, tc.sub); got != tc.want {
			t.Fatalf("containsFoldASCII(%q, %q) = %v, want %v", tc.s, tc.sub, got, tc.want)
		}
		if got := strings.Contains(strings.ToLower(tc.s), tc.sub); got != tc.want {
			t.Fatalf("reference disagrees for (%q, %q)", tc.s, tc.sub)
		}
	}
}
