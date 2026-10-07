package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	nativeIPCVersion = 1

	nativeFrameEventEnvelope byte = 1 // v1 compatibility
	nativeFrameSystemStats   byte = 2
	nativeFrameEventSummary  byte = 3 // v2 compact desktop summary

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

	var frameBuf []byte
	for ctx.Err() == nil {
		kind, payload, nextBuf, err := readNativeFrameInto(session.reader, frameBuf)
		if err != nil {
			return err
		}
		frameBuf = nextBuf
		switch kind {
		case nativeFrameEventEnvelope:
			summary, err := decodeEventEnvelopeSummary(payload)
			if err != nil {
				continue
			}
			a.queueEventSummary(summary)
		case nativeFrameEventSummary:
			summary, err := decodeDesktopEventSummary(payload)
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
	kind, payload, _, err := readNativeFrameInto(r, nil)
	return kind, payload, err
}

func readNativeFrameInto(r io.Reader, frame []byte) (byte, []byte, []byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, frame, err
	}
	size := int(binary.BigEndian.Uint32(header[:]))
	if size < 2 || size > nativeMaxFrameSize {
		return 0, nil, frame, fmt.Errorf("invalid native IPC frame size %d", size)
	}
	if cap(frame) < size {
		frame = make([]byte, size)
	} else {
		frame = frame[:size]
	}
	if _, err := io.ReadFull(r, frame); err != nil {
		return 0, nil, frame, err
	}
	return frame[0], frame[1:], frame, nil
}

func decodeDesktopEventSummary(data []byte) (eventSummary, error) {
	var out eventSummary
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return out, protowire.ParseError(n)
		}
		data = data[n:]

		switch num {
		case 1, 7, 8, 9, 10, 13, 15, 16, 17:
			value, consumed := protowire.ConsumeBytes(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			text := string(value)
			switch num {
			case 1:
				out.EventID = text
			case 7:
				out.Type = text
			case 8:
				out.Tag = text
			case 9:
				out.Comm = text
			case 10:
				out.Target = text
			case 13:
				out.Decision = text
			case 15:
				out.AgentRunID = text
			case 16:
				out.ConversationID = text
			case 17:
				out.ToolName = text
			}
			data = data[consumed:]
		case 2, 3, 4, 5, 6, 11, 12, 18:
			value, consumed := protowire.ConsumeVarint(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			switch num {
			case 2:
				out.ReceivedAtMS = int64(value)
			case 3:
				out.PID = int(value)
			case 4:
				out.PPID = int(value)
			case 5:
				out.RootAgentPID = int(value)
			case 6:
				out.EventType = int(value)
			case 11:
				out.Network = value != 0
			case 12:
				out.NetBytes = int64(value)
			case 18:
				out.HasAgentContext = value != 0
			}
			data = data[consumed:]
		case 14:
			value, consumed := protowire.ConsumeFixed64(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			out.RiskScore = math.Float64frombits(value)
			data = data[consumed:]
		default:
			consumed := protowire.ConsumeFieldValue(num, typ, data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			data = data[consumed:]
		}
	}
	if strings.TrimSpace(out.EventID) == "" {
		return out, fmt.Errorf("desktop event summary missing event id")
	}
	if out.ReceivedAtMS == 0 {
		out.ReceivedAtMS = time.Now().UnixMilli()
	}
	return out, nil
}

func decodeEventEnvelopeSummary(data []byte) (eventSummary, error) {
	var out eventSummary
	var (
		eventID        string
		timestampNS    uint64
		agentRunID     string
		conversationID string
		toolCallID     string
		toolName       string
		decision       string
		pid            uint64
		ppid           uint64
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
		case 2, 5, 7, 9, 10, 18, 23:
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
			case 9:
				toolCallID = text
			case 10:
				toolName = text
			case 18:
				comm = text
			case 23:
				decision = text
			}
			data = data[consumed:]
		case 3, 13, 15, 25:
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
	if toolName != "" {
		out.ToolName = toolName
		if out.Target == "" {
			out.Target = toolName
		}
	}
	if agentRunID != "" || conversationID != "" || toolCallID != "" {
		out.HasAgentContext = true
	}
	return out, nil
}

func decodeLegacyEventSummary(data []byte, out *eventSummary) error {
	if out == nil {
		return fmt.Errorf("nil event summary")
	}
	var path, endpoint, domain, extraPath, toolCallID string
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return protowire.ParseError(n)
		}
		data = data[n:]

		switch num {
		case 1, 2, 10, 22, 28:
			value, consumed := protowire.ConsumeVarint(data)
			if consumed < 0 {
				return protowire.ParseError(consumed)
			}
			switch num {
			case 1:
				out.PID = int(value)
			case 2:
				out.PPID = int(value)
			case 10:
				out.NetBytes = int64(value)
			case 22:
				out.EventType = int(value)
			case 28:
				out.RootAgentPID = int(value)
				if value != 0 {
					out.HasAgentContext = true
				}
			}
			data = data[consumed:]
		case 4, 5, 6, 7, 9, 14, 17, 29, 30, 32, 33, 36:
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
				if strings.TrimSpace(text) != "" && !strings.EqualFold(text, "Unknown") {
					out.HasAgentContext = true
				}
			case 6:
				out.Comm = text
			case 7:
				path = text
			case 9:
				endpoint = text
			case 14:
				extraPath = text
			case 17:
				domain = text
			case 29:
				out.AgentRunID = text
				if text != "" {
					out.HasAgentContext = true
				}
			case 30:
				out.ConversationID = text
				if text != "" {
					out.HasAgentContext = true
				}
			case 32:
				toolCallID = text
			case 33:
				out.ToolName = text
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
	for _, value := range []string{path, endpoint, domain, extraPath, out.ToolName} {
		if strings.TrimSpace(value) != "" {
			out.Target = value
			break
		}
	}
	out.Network = strings.TrimSpace(endpoint) != "" || strings.TrimSpace(domain) != ""
	if !out.Network {
		lowerType := strings.ToLower(out.Type)
		out.Network = strings.Contains(lowerType, "network") ||
			strings.Contains(lowerType, "connect") ||
			strings.Contains(lowerType, "socket") ||
			strings.Contains(lowerType, "tcp") ||
			strings.Contains(lowerType, "dns")
	}
	if toolCallID != "" {
		out.HasAgentContext = true
	}
	return nil
}

