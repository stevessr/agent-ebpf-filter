package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	nativeIPCVersion = 1

	nativeFrameEventEnvelope byte = 1
	nativeFrameSystemStats   byte = 2

	nativeMaxFrameSize = 4 << 20
)

func (a *renewApp) runNativeIPC(ctx context.Context, session *backendSession) {
	err := a.consumeNativeIPC(ctx, session)
	a.update(func() {
		a.eventStreamConnected = false
		a.systemConnected = false
		if err != nil && ctx.Err() == nil {
			a.eventStreamErr = "native IPC: " + err.Error()
			a.systemErr = "native IPC: " + err.Error()
		}
	})
	if ctx.Err() != nil {
		return
	}

	// Native IPC is preferred for a bundled/local backend. If that private
	// channel is lost while the backend itself is still reachable, preserve
	// functionality through the existing remote-compatible transports.
	go a.runEventSummaryStream(ctx)
	go a.runSystemStats(ctx)
}

func (a *renewApp) consumeNativeIPC(ctx context.Context, session *backendSession) error {
	if session == nil || session.reader == nil || session.nativeIPCVersion < nativeIPCVersion {
		return fmt.Errorf("native IPC unavailable")
	}
	a.update(func() {
		a.eventStreamConnected = true
		a.systemConnected = true
		a.eventStreamErr = ""
		a.systemErr = ""
	})

	for ctx.Err() == nil {
		kind, payload, err := readNativeFrame(session.reader)
		if err != nil {
			return err
		}
		switch kind {
		case nativeFrameEventEnvelope:
			summary, err := decodeEventEnvelopeSummary(payload)
			if err != nil {
				continue
			}
			a.queueEventSummary(summary)
		case nativeFrameSystemStats:
			snapshot, err := decodeSystemStats(payload)
			if err != nil {
				continue
			}
			snapshot.FetchedAt = time.Now()
			a.update(func() {
				a.system = snapshot
				a.systemConnected = true
				a.systemErr = ""
			})
		}
	}
	return ctx.Err()
}

func readNativeFrame(r io.Reader) (byte, []byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	size := int(binary.BigEndian.Uint32(header[:]))
	if size < 2 || size > nativeMaxFrameSize {
		return 0, nil, fmt.Errorf("invalid native IPC frame size %d", size)
	}
	frame := make([]byte, size)
	if _, err := io.ReadFull(r, frame); err != nil {
		return 0, nil, err
	}
	return frame[0], frame[1:], nil
}

func decodeEventEnvelopeSummary(data []byte) (eventSummary, error) {
	var out eventSummary
	var (
		eventID        string
		timestampNS    uint64
		agentRunID     string
		conversationID string
		turnID         string
		toolCallID     string
		toolName       string
		traceID        string
		spanID         string
		decision       string
		pid            uint64
		ppid           uint64
		uid            uint64
		comm           string
		eventType      uint64
		eventTypeSeen  bool
		riskScore      float64
		riskScoreSeen  bool
	)

	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return out, protowire.ParseError(n)
		}
		data = data[n:]

		switch num {
		case 2, 5, 7, 8, 9, 10, 11, 12, 18, 23:
			value, consumed := protowire.ConsumeBytes(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			text := string(value)
			switch num {
			case 2:
				eventID = text
			case 5:
				agentRunID = text
			case 7:
				conversationID = text
			case 8:
				turnID = text
			case 9:
				toolCallID = text
			case 10:
				toolName = text
			case 11:
				traceID = text
			case 12:
				spanID = text
			case 18:
				comm = text
			case 23:
				decision = text
			}
			data = data[consumed:]
		case 3, 13, 15, 16, 25:
			value, consumed := protowire.ConsumeVarint(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			switch num {
			case 3:
				timestampNS = value
			case 13:
				pid = value
			case 15:
				ppid = value
			case 16:
				uid = value
			case 25:
				eventType = value
				eventTypeSeen = true
			}
			data = data[consumed:]
		case 24:
			value, consumed := protowire.ConsumeFixed64(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			riskScore = math.Float64frombits(value)
			riskScoreSeen = true
			data = data[consumed:]
		case 26:
			value, consumed := protowire.ConsumeBytes(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			if err := decodeLegacyEventSummary(value, &out); err != nil {
				return out, err
			}
			data = data[consumed:]
		default:
			consumed := protowire.ConsumeFieldValue(num, typ, data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			data = data[consumed:]
		}
	}

	if eventID == "" {
		return out, fmt.Errorf("event envelope missing event id")
	}
	out.EventID = eventID
	if timestampNS != 0 {
		out.ReceivedAtMS = int64(timestampNS / uint64(time.Millisecond))
	}
	if out.ReceivedAtMS == 0 {
		out.ReceivedAtMS = time.Now().UnixMilli()
	}
	if pid != 0 {
		out.PID = int(pid)
	}
	if ppid != 0 {
		out.PPID = int(ppid)
	}
	if uid != 0 {
		out.UID = int(uid)
	}
	if comm != "" {
		out.Comm = comm
	}
	if eventTypeSeen {
		out.EventType = int(eventType)
	}
	if decision != "" {
		out.Decision = decision
	}
	if riskScoreSeen {
		out.RiskScore = riskScore
	}
	if agentRunID != "" {
		out.AgentRunID = agentRunID
	}
	if conversationID != "" {
		out.ConversationID = conversationID
	}
	if turnID != "" {
		out.TurnID = turnID
	}
	if toolCallID != "" {
		out.ToolCallID = toolCallID
	}
	if toolName != "" {
		out.ToolName = toolName
	}
	if traceID != "" {
		out.TraceID = traceID
	}
	if spanID != "" {
		out.SpanID = spanID
	}
	return out, nil
}

func decodeLegacyEventSummary(data []byte, out *eventSummary) error {
	if out == nil {
		return fmt.Errorf("nil event summary")
	}
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return protowire.ParseError(n)
		}
		data = data[n:]

		switch num {
		case 1, 2, 3, 10, 22, 28:
			value, consumed := protowire.ConsumeVarint(data)
			if consumed < 0 {
				return protowire.ParseError(consumed)
			}
			switch num {
			case 1:
				out.PID = int(value)
			case 2:
				out.PPID = int(value)
			case 3:
				out.UID = int(value)
			case 10:
				out.NetBytes = int64(value)
			case 22:
				out.EventType = int(value)
			case 28:
				out.RootAgentPID = int(value)
			}
			data = data[consumed:]
		case 4, 5, 6, 7, 8, 9, 14, 17, 29, 30, 31, 32, 33, 34, 35, 36:
			value, consumed := protowire.ConsumeBytes(data)
			if consumed < 0 {
				return protowire.ParseError(consumed)
			}
			text := string(value)
			switch num {
			case 4:
				out.Type = text
			case 5:
				out.Tag = text
			case 6:
				out.Comm = text
			case 7:
				out.Path = text
			case 8:
				out.NetDirection = text
			case 9:
				out.NetEndpoint = text
			case 14:
				out.ExtraPath = text
			case 17:
				out.Domain = text
			case 29:
				out.AgentRunID = text
			case 30:
				out.ConversationID = text
			case 31:
				out.TurnID = text
			case 32:
				out.ToolCallID = text
			case 33:
				out.ToolName = text
			case 34:
				out.TraceID = text
			case 35:
				out.SpanID = text
			case 36:
				out.Decision = text
			}
			data = data[consumed:]
		case 37:
			value, consumed := protowire.ConsumeFixed64(data)
			if consumed < 0 {
				return protowire.ParseError(consumed)
			}
			out.RiskScore = math.Float64frombits(value)
			data = data[consumed:]
		default:
			consumed := protowire.ConsumeFieldValue(num, typ, data)
			if consumed < 0 {
				return protowire.ParseError(consumed)
			}
			data = data[consumed:]
		}
	}
	return nil
}
