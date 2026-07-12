package app

import (
	"errors"
	"math"
	"net"
	"strings"
	"testing"
	"unicode/utf8"

	"agent-ebpf-filter/pb"
)

func TestUDSConnectionSetEnforcesLimitAndClosure(t *testing.T) {
	set := newUDSConnectionSet(1)
	serverOne, clientOne := net.Pipe()
	defer clientOne.Close()
	serverTwo, clientTwo := net.Pipe()
	defer clientTwo.Close()

	if err := set.Add(serverOne); err != nil {
		t.Fatalf("Add(first) error = %v", err)
	}
	if err := set.Add(serverTwo); !errors.Is(err, errUDSConnectionLimit) {
		t.Fatalf("Add(over limit) error = %v", err)
	}
	set.Remove(serverOne)
	_ = serverOne.Close()
	if err := set.Add(serverTwo); err != nil {
		t.Fatalf("Add(after remove) error = %v", err)
	}
	set.CloseAll()
	serverThree, clientThree := net.Pipe()
	defer serverThree.Close()
	defer clientThree.Close()
	if err := set.Add(serverThree); !errors.Is(err, errUDSConnectionSetClosed) {
		t.Fatalf("Add(after close) error = %v", err)
	}
}

func TestValidateWrapperRequestBounds(t *testing.T) {
	valid := func() *pb.WrapperRequest {
		return &pb.WrapperRequest{Pid: 42, Comm: "codex", Args: []string{"exec", "--help"}, RiskScore: 0.5}
	}
	tests := []struct {
		name   string
		mutate func(*pb.WrapperRequest)
	}{
		{name: "missing pid", mutate: func(req *pb.WrapperRequest) { req.Pid = 0 }},
		{name: "missing comm", mutate: func(req *pb.WrapperRequest) { req.Comm = "" }},
		{name: "too many args", mutate: func(req *pb.WrapperRequest) { req.Args = make([]string, udsMaxWrapperArgs+1) }},
		{name: "oversized arg", mutate: func(req *pb.WrapperRequest) { req.Args = []string{strings.Repeat("x", udsMaxWrapperArgumentBytes+1)} }},
		{name: "oversized args total", mutate: func(req *pb.WrapperRequest) {
			req.Args = []string{strings.Repeat("x", udsMaxWrapperArgsBytes/2+1), strings.Repeat("y", udsMaxWrapperArgsBytes/2+1)}
		}},
		{name: "oversized identity", mutate: func(req *pb.WrapperRequest) { req.TraceId = strings.Repeat("x", udsMaxWrapperIdentityBytes+1) }},
		{name: "oversized path", mutate: func(req *pb.WrapperRequest) { req.BinaryPath = strings.Repeat("x", udsMaxWrapperPathBytes+1) }},
		{name: "non finite risk", mutate: func(req *pb.WrapperRequest) { req.RiskScore = math.NaN() }},
	}
	if err := validateWrapperRequest(valid()); err != nil {
		t.Fatalf("validateWrapperRequest(valid) error = %v", err)
	}
	if err := validateWrapperRequest(nil); err == nil {
		t.Fatal("validateWrapperRequest(nil) accepted request")
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := valid()
			tc.mutate(req)
			if err := validateWrapperRequest(req); err == nil {
				t.Fatal("validateWrapperRequest() accepted invalid request")
			}
		})
	}
}

func TestBoundedWrapperTrainingArgsLimitsRetainedData(t *testing.T) {
	args := make([]string, udsMaxTrainingArgs+10)
	for index := range args {
		args[index] = strings.Repeat("x", udsMaxTrainingArgBytes+100)
	}
	bounded := boundedWrapperTrainingArgs(args)
	if len(bounded) == 0 || len(bounded) > udsMaxTrainingArgs {
		t.Fatalf("bounded arg count = %d", len(bounded))
	}
	total := 0
	for index, arg := range bounded {
		if len(arg) > udsMaxTrainingArgBytes {
			t.Fatalf("bounded arg %d length = %d", index, len(arg))
		}
		total += len(arg)
	}
	if total > udsMaxTrainingArgsBytes {
		t.Fatalf("bounded arg bytes = %d", total)
	}
	if got := len(boundedWrapperTrainingString(strings.Repeat("x", 100), 16)); got != 16 {
		t.Fatalf("bounded string length = %d", got)
	}
	if got := boundedWrapperTrainingString("命令参数", 5); !utf8.ValidString(got) || len(got) > 5 {
		t.Fatalf("bounded UTF-8 string = %q", got)
	}
}
