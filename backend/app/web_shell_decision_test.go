package app

import (
	"testing"

	"agent-ebpf-filter/internal/heuristics"
	"agent-ebpf-filter/pb"
)

func TestApplyWebShellFindingPolicyModes(t *testing.T) {
	finding := heuristics.ShellFinding{
		Code: "PYTHON_WEB_SHELL_SPAWN", Runtime: "Python",
		Shell: "sh", ParentPID: 123,
	}
	tests := []struct {
		name   string
		mode   string
		action pb.WrapperResponse_Action
		want   pb.WrapperResponse_Action
	}{
		{"default audit", "", pb.WrapperResponse_ALLOW, pb.WrapperResponse_ALERT},
		{"explicit audit", "audit", pb.WrapperResponse_ALLOW, pb.WrapperResponse_ALERT},
		{"opted out", "off", pb.WrapperResponse_ALLOW, pb.WrapperResponse_ALLOW},
		{"explicit enforce", "block", pb.WrapperResponse_ALLOW, pb.WrapperResponse_BLOCK},
		{"preserve existing block", "audit", pb.WrapperResponse_BLOCK, pb.WrapperResponse_BLOCK},
		{"preserve alert", "audit", pb.WrapperResponse_ALERT, pb.WrapperResponse_ALERT},
		{"preserve rewrite", "block", pb.WrapperResponse_REWRITE, pb.WrapperResponse_REWRITE},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := applyWebShellFinding(tc.action, "original", tc.mode, finding)
			if got != tc.want {
				t.Fatalf("mode=%q action=%v: got %v, want %v", tc.mode, tc.action, got, tc.want)
			}
			if got == tc.action && tc.action != pb.WrapperResponse_ALLOW && reason != "original" {
				t.Fatalf("should preserve stronger prior decision reason: %q", reason)
			}
		})
	}
}
