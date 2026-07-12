package app

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"agent-ebpf-filter/pb"
)

const (
	udsMaxWrapperArgs          = 4096
	udsMaxWrapperArgumentBytes = 128 << 10
	udsMaxWrapperArgsBytes     = 256 << 10
	udsMaxWrapperCommandBytes  = 4096
	udsMaxWrapperIdentityBytes = 1024
	udsMaxWrapperPathBytes     = 4096
	udsMaxTrainingArgs         = 256
	udsMaxTrainingArgBytes     = 2048
	udsMaxTrainingArgsBytes    = 16 << 10
	udsMaxTrainingCommBytes    = 512
	udsMaxTrainingCommandBytes = 20 << 10
)

func validateWrapperRequest(req *pb.WrapperRequest) error {
	if req == nil {
		return fmt.Errorf("wrapper request is nil")
	}
	if req.Pid == 0 {
		return fmt.Errorf("wrapper pid is required")
	}
	if err := validateWrapperString("comm", req.Comm, 1, udsMaxWrapperCommandBytes); err != nil {
		return err
	}
	if len(req.Args) > udsMaxWrapperArgs {
		return fmt.Errorf("wrapper argument count %d exceeds %d", len(req.Args), udsMaxWrapperArgs)
	}
	argsBytes := 0
	for index, arg := range req.Args {
		if len(arg) > udsMaxWrapperArgumentBytes {
			return fmt.Errorf("wrapper argument %d exceeds %d bytes", index, udsMaxWrapperArgumentBytes)
		}
		argsBytes += len(arg)
		if argsBytes > udsMaxWrapperArgsBytes {
			return fmt.Errorf("wrapper arguments exceed %d bytes", udsMaxWrapperArgsBytes)
		}
	}
	for _, field := range []struct {
		name  string
		value string
		max   int
	}{
		{name: "user", value: req.User, max: udsMaxWrapperIdentityBytes},
		{name: "agent_run_id", value: req.AgentRunId, max: udsMaxWrapperIdentityBytes},
		{name: "conversation_id", value: req.ConversationId, max: udsMaxWrapperIdentityBytes},
		{name: "turn_id", value: req.TurnId, max: udsMaxWrapperIdentityBytes},
		{name: "tool_call_id", value: req.ToolCallId, max: udsMaxWrapperIdentityBytes},
		{name: "tool_name", value: req.ToolName, max: udsMaxWrapperIdentityBytes},
		{name: "trace_id", value: req.TraceId, max: udsMaxWrapperIdentityBytes},
		{name: "span_id", value: req.SpanId, max: udsMaxWrapperIdentityBytes},
		{name: "decision", value: req.Decision, max: 128},
		{name: "container_id", value: req.ContainerId, max: udsMaxWrapperIdentityBytes},
		{name: "argv_digest", value: req.ArgvDigest, max: 256},
		{name: "task_id", value: req.TaskId, max: udsMaxWrapperIdentityBytes},
		{name: "cwd", value: req.Cwd, max: udsMaxWrapperPathBytes},
		{name: "binary_path", value: req.BinaryPath, max: udsMaxWrapperPathBytes},
	} {
		if err := validateWrapperString(field.name, field.value, 0, field.max); err != nil {
			return err
		}
	}
	if math.IsNaN(req.RiskScore) || math.IsInf(req.RiskScore, 0) {
		return fmt.Errorf("wrapper risk score must be finite")
	}
	return nil
}

func validateWrapperString(name, value string, minBytes, maxBytes int) error {
	length := len(value)
	if length < minBytes {
		return fmt.Errorf("wrapper %s is required", name)
	}
	if length > maxBytes {
		return fmt.Errorf("wrapper %s exceeds %d bytes", name, maxBytes)
	}
	return nil
}

func boundedWrapperTrainingArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	limit := len(args)
	if limit > udsMaxTrainingArgs {
		limit = udsMaxTrainingArgs
	}
	out := make([]string, 0, limit)
	total := 0
	for _, arg := range args[:limit] {
		if len(arg) > udsMaxTrainingArgBytes {
			arg = truncateWrapperUTF8(arg, udsMaxTrainingArgBytes)
		}
		remaining := udsMaxTrainingArgsBytes - total
		if remaining <= 0 {
			break
		}
		if len(arg) > remaining {
			arg = truncateWrapperUTF8(arg, remaining)
		}
		out = append(out, strings.Clone(arg))
		total += len(arg)
	}
	return out
}

func boundedWrapperTrainingString(value string, maxBytes int) string {
	if maxBytes <= 0 || value == "" {
		return ""
	}
	if len(value) > maxBytes {
		value = truncateWrapperUTF8(value, maxBytes)
	}
	return strings.Clone(value)
}

func truncateWrapperUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
