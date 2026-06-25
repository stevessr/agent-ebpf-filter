package app

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
)

// ---- moved from backend/zz_merged_backend.go section context_event.go ----

const eventSchemaVersion = "event.v3"

type processContext struct {
	RootAgentPid   uint32
	AgentRunID     string
	TaskID         string
	ConversationID string
	TurnID         string
	ToolCallID     string
	ToolName       string
	TraceID        string
	SpanID         string
	Decision       string
	ContainerID    string
	ArgvDigest     string
	Cwd            string
	RiskScore      float64
}

type processContextStore struct {
	mu    sync.RWMutex
	items map[uint32]processContext
}

func newProcessContextStore() *processContextStore {
	return &processContextStore{items: make(map[uint32]processContext)}
}

func (s *processContextStore) Set(pid uint32, ctx processContext) {
	if s == nil || pid == 0 {
		return
	}
	ctx = normalizeProcessContext(ctx, pid)
	s.mu.Lock()
	s.items[pid] = ctx
	s.mu.Unlock()
}

func (s *processContextStore) Get(pid uint32) (processContext, bool) {
	if s == nil || pid == 0 {
		return processContext{}, false
	}
	s.mu.RLock()
	ctx, ok := s.items[pid]
	s.mu.RUnlock()
	return ctx, ok
}

func (s *processContextStore) Delete(pid uint32) {
	if s == nil || pid == 0 {
		return
	}
	s.mu.Lock()
	delete(s.items, pid)
	s.mu.Unlock()
}

func (s *processContextStore) Move(oldPID, newPID uint32) bool {
	if s == nil || oldPID == 0 || newPID == 0 || oldPID == newPID {
		return false
	}
	s.mu.Lock()
	ctx, ok := s.items[oldPID]
	if ok {
		delete(s.items, oldPID)
		s.items[newPID] = normalizeProcessContext(ctx, newPID)
	}
	s.mu.Unlock()
	return ok
}

func normalizeProcessContext(ctx processContext, pid uint32) processContext {
	ctx.AgentRunID = strings.TrimSpace(ctx.AgentRunID)
	ctx.TaskID = strings.TrimSpace(ctx.TaskID)
	ctx.ConversationID = strings.TrimSpace(ctx.ConversationID)
	ctx.TurnID = strings.TrimSpace(ctx.TurnID)
	ctx.ToolCallID = strings.TrimSpace(ctx.ToolCallID)
	ctx.ToolName = strings.TrimSpace(ctx.ToolName)
	ctx.TraceID = strings.TrimSpace(ctx.TraceID)
	ctx.SpanID = strings.TrimSpace(ctx.SpanID)
	ctx.Decision = strings.TrimSpace(strings.ToUpper(ctx.Decision))
	ctx.ContainerID = strings.TrimSpace(ctx.ContainerID)
	ctx.ArgvDigest = strings.TrimSpace(ctx.ArgvDigest)
	ctx.Cwd = strings.TrimSpace(ctx.Cwd)
	if ctx.RootAgentPid == 0 {
		ctx.RootAgentPid = pid
	}
	if ctx.RiskScore < 0 {
		ctx.RiskScore = 0
	}
	return ctx
}

func buildArgvDigest(parts ...string) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	if len(values) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return hex.EncodeToString(sum[:])
}

func buildArgvDigestFromCommand(comm string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, comm)
	parts = append(parts, args...)
	return buildArgvDigest(parts...)
}

func buildProcessContextFromRegister(req registerPayload) processContext {
	ctx := processContext{
		RootAgentPid:   req.RootAgentPID,
		AgentRunID:     req.AgentRunID,
		TaskID:         req.TaskID,
		ConversationID: req.ConversationID,
		TurnID:         req.TurnID,
		ToolCallID:     req.ToolCallID,
		ToolName:       req.ToolName,
		TraceID:        req.TraceID,
		SpanID:         req.SpanID,
		Decision:       req.Decision,
		ContainerID:    req.ContainerID,
		ArgvDigest:     req.ArgvDigest,
		Cwd:            req.Cwd,
		RiskScore:      req.RiskScore,
	}
	if ctx.ArgvDigest == "" {
		ctx.ArgvDigest = buildArgvDigest(req.ToolName, req.ToolCallID, req.AgentRunID, req.TaskID)
	}
	return normalizeProcessContext(ctx, req.PID)
}

func buildProcessContextFromWrapperRequest(req *pb.WrapperRequest, decision string, riskScore float64) processContext {
	if req == nil {
		return processContext{}
	}
	ctx := processContext{
		RootAgentPid:   req.RootAgentPid,
		AgentRunID:     req.AgentRunId,
		TaskID:         req.TaskId,
		ConversationID: req.ConversationId,
		TurnID:         req.TurnId,
		ToolCallID:     req.ToolCallId,
		ToolName:       platform.FirstNonEmpty(req.ToolName, req.Comm),
		TraceID:        req.TraceId,
		SpanID:         req.SpanId,
		Decision:       decision,
		ContainerID:    req.ContainerId,
		ArgvDigest:     req.ArgvDigest,
		Cwd:            req.Cwd,
		RiskScore:      riskScore,
	}
	if ctx.ArgvDigest == "" {
		ctx.ArgvDigest = buildArgvDigestFromCommand(req.Comm, req.Args)
	}
	return normalizeProcessContext(ctx, req.Pid)
}

func buildProcessContextFromHookPayload(payload map[string]interface{}, toolName, path string) (uint32, processContext) {
	if toolName == "" {
		if toolCall, _ := payload["toolCall"].(map[string]interface{}); toolCall != nil {
			toolName, _ = toolCall["name"].(string)
		}
	}
	toolCallID := events.PayloadString(payload, "tool_call_id", "toolCallId")
	if toolCallID == "" {
		if toolCall, _ := payload["toolCall"].(map[string]interface{}); toolCall != nil {
			toolCallID = events.PayloadString(toolCall, "id", "callId", "toolCallId")
		}
	}
	cwd := events.PayloadString(payload, "cwd", "working_directory", "workingDirectory")
	if cwd == "" {
		if toolCall, _ := payload["toolCall"].(map[string]interface{}); toolCall != nil {
			if args, _ := toolCall["args"].(map[string]interface{}); args != nil {
				cwd = events.PayloadString(args, "cwd", "Cwd", "working_directory", "workingDirectory")
			}
		}
	}
	pid := events.PayloadUint32(payload, "pid", "process_id", "processId", "agent_pid", "agentPid")
	ctx := processContext{
		RootAgentPid:   events.PayloadUint32(payload, "root_agent_pid", "rootAgentPid"),
		AgentRunID:     events.PayloadString(payload, "agent_run_id", "agentRunId"),
		TaskID:         events.PayloadString(payload, "task_id", "taskId"),
		ConversationID: events.PayloadString(payload, "conversation_id", "conversationId"),
		TurnID:         events.PayloadString(payload, "turn_id", "turnId"),
		ToolCallID:     toolCallID,
		ToolName:       platform.FirstNonEmpty(events.PayloadString(payload, "tool_name", "toolName"), toolName),
		TraceID:        events.PayloadString(payload, "trace_id", "traceId"),
		SpanID:         events.PayloadString(payload, "span_id", "spanId"),
		Decision:       events.PayloadString(payload, "decision"),
		ContainerID:    events.PayloadString(payload, "container_id", "containerId"),
		ArgvDigest:     events.PayloadString(payload, "argv_digest", "argvDigest"),
		Cwd:            cwd,
		RiskScore:      events.PayloadFloat64(payload, "risk_score", "riskScore"),
	}
	if ctx.ArgvDigest == "" {
		ctx.ArgvDigest = buildArgvDigest(ctx.ToolName, path, ctx.TaskID)
	}
	return pid, normalizeProcessContext(ctx, pid)
}

func enrichEventContext(event *pb.Event) *pb.Event {
	if event == nil {
		return nil
	}
	if strings.TrimSpace(event.SchemaVersion) == "" {
		event.SchemaVersion = eventSchemaVersion
	}
	if event.Pid == 0 {
		return event
	}

	if event.Type == "process_exec" {
		if oldPID := platform.ParseUintField(event.ExtraInfo, "old_pid"); oldPID > 0 && oldPID != event.Pid {
			trackedProcessContexts.Move(oldPID, event.Pid)
		}
	}

	ctx, ok := trackedProcessContexts.Get(event.Pid)
	if !ok && event.Ppid != 0 {
		if parentCtx, parentOK := trackedProcessContexts.Get(event.Ppid); parentOK {
			trackedProcessContexts.Set(event.Pid, parentCtx)
			ctx, ok = trackedProcessContexts.Get(event.Pid)
		}
	}
	// Try cgroup-based attribution if no direct PID context
	if !ok && event.CgroupId != 0 {
		if agentRunID, taskID, toolCallID := enrichEventWithCgroupContext(event.CgroupId); agentRunID != "" {
			ctx = processContext{
				AgentRunID: agentRunID,
				TaskID:     taskID,
				ToolCallID: toolCallID,
			}
			ok = true
		}
	}
	if ok {
		applyProcessContextToEvent(event, ctx)
		// Lazily bind cgroup to agent context for future child attribution
		if event.CgroupId != 0 && ctx.AgentRunID != "" {
			cgroupAttribution.Set(event.CgroupId, cgroupAttributionEntry{
				AgentRunID:   ctx.AgentRunID,
				TaskID:       ctx.TaskID,
				ToolCallID:   ctx.ToolCallID,
				RootAgentPID: ctx.RootAgentPid,
			})
		}
	}

	// Record tool baseline for drift detection
	if event.ToolName != "" && event.Comm != "" {
		toolBaseline.Record(event.ToolName, event.Comm, event.Type, event.Path)
	}

	if event.Type == "process_exit" || event.Type == "exit" {
		trackedProcessContexts.Delete(event.Pid)
	}
	return event
}

func applyBestEffortProcessContextToEvent(event *pb.Event) {
	if event == nil || event.Pid == 0 {
		return
	}
	ctx, ok := trackedProcessContexts.Get(event.Pid)
	if !ok && event.Ppid != 0 {
		if parentCtx, parentOK := trackedProcessContexts.Get(event.Ppid); parentOK {
			ctx = parentCtx
			ok = true
		}
	}
	if !ok && event.CgroupId != 0 {
		if agentRunID, taskID, toolCallID := enrichEventWithCgroupContext(event.CgroupId); agentRunID != "" {
			ctx = processContext{
				AgentRunID: agentRunID,
				TaskID:     taskID,
				ToolCallID: toolCallID,
			}
			ok = true
		}
	}
	if ok {
		applyProcessContextToEvent(event, ctx)
	}
}

func applyProcessContextToEvent(event *pb.Event, ctx processContext) {
	if event.RootAgentPid == 0 {
		event.RootAgentPid = ctx.RootAgentPid
	}
	if strings.TrimSpace(event.AgentRunId) == "" {
		event.AgentRunId = ctx.AgentRunID
	}
	if strings.TrimSpace(event.TaskId) == "" {
		event.TaskId = ctx.TaskID
	}
	if strings.TrimSpace(event.ConversationId) == "" {
		event.ConversationId = ctx.ConversationID
	}
	if strings.TrimSpace(event.TurnId) == "" {
		event.TurnId = ctx.TurnID
	}
	if strings.TrimSpace(event.ToolCallId) == "" {
		event.ToolCallId = ctx.ToolCallID
	}
	if strings.TrimSpace(event.ToolName) == "" {
		event.ToolName = ctx.ToolName
	}
	if strings.TrimSpace(event.TraceId) == "" {
		event.TraceId = ctx.TraceID
	}
	if strings.TrimSpace(event.SpanId) == "" {
		event.SpanId = ctx.SpanID
	}
	if strings.TrimSpace(event.Decision) == "" {
		event.Decision = ctx.Decision
	}
	if event.RiskScore == 0 && ctx.RiskScore > 0 {
		event.RiskScore = ctx.RiskScore
	}
	if strings.TrimSpace(event.ContainerId) == "" {
		event.ContainerId = ctx.ContainerID
	}
	if strings.TrimSpace(event.ArgvDigest) == "" {
		event.ArgvDigest = ctx.ArgvDigest
	}
	if strings.TrimSpace(event.Cwd) == "" {
		event.Cwd = ctx.Cwd
	}
}

var trackedProcessContexts = newProcessContextStore()

type registerPayload struct {
	PID            uint32  `json:"pid"`
	Tag            string  `json:"tag,omitempty"`
	AgentRunID     string  `json:"agent_run_id,omitempty"`
	TaskID         string  `json:"task_id,omitempty"`
	ConversationID string  `json:"conversation_id,omitempty"`
	TurnID         string  `json:"turn_id,omitempty"`
	ToolCallID     string  `json:"tool_call_id,omitempty"`
	ToolName       string  `json:"tool_name,omitempty"`
	TraceID        string  `json:"trace_id,omitempty"`
	SpanID         string  `json:"span_id,omitempty"`
	RootAgentPID   uint32  `json:"root_agent_pid,omitempty"`
	Decision       string  `json:"decision,omitempty"`
	RiskScore      float64 `json:"risk_score,omitempty"`
	ContainerID    string  `json:"container_id,omitempty"`
	ArgvDigest     string  `json:"argv_digest,omitempty"`
	Cwd            string  `json:"cwd,omitempty"`
}
