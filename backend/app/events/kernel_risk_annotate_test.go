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
		{newKernelRiskDecision("ALERT", 96, "sensitive_path", "secret_material_path"), ""},
		{newKernelRiskDecision("", 8, "agent_context"), "fd=3 count=512"},
		{newKernelRiskDecision("OBSERVE", 42.5, "a"), ""},
		{newKernelRiskDecision("OBSERVE", 43.5, "a"), "x"},
		{newKernelRiskDecision("ALERT", 100, "r1", "r2", "r3", "r4", "r5", "r6"), "existing=1"},
		{newKernelRiskDecision("ALERT", 0.4, "tiny"), ""},
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
	decision := newKernelRiskDecision("ALERT", 72, "agent_context", "destructive_file_mutation")
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

func TestKernelRiskDecisionReasonsDedupeAndCap(t *testing.T) {
	d := newKernelRiskDecision("ALERT", 10, " a ", "b", "a", "", "c", "d", "e", "f", "g", "b")
	if got := d.Reasons(); len(got) != kernelRiskMaxReasons || got[0] != "a" || got[5] != "f" {
		t.Fatalf("Reasons() = %v", got)
	}
	if d.reasonText() != "a,b,c,d,e,f" {
		t.Fatalf("reasonText = %q", d.reasonText())
	}
	if allocs := testing.AllocsPerRun(200, func() {
		var d kernelRiskDecision
		d.addReason("agent_context")
		d.addReason("destructive_file_mutation")
		d.addReason("agent_context")
	}); allocs != 0 {
		t.Fatalf("addReason allocated %.1f per run", allocs)
	}
}
