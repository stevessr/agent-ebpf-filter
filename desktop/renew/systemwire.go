package main

import (
	"fmt"
	"math"

	"google.golang.org/protobuf/encoding/protowire"
)

// decodeSystemStats decodes only the fields the native Renew desktop renders.
// Keeping this decoder narrow avoids coupling the desktop module to backend/pb,
// which is generated during the full repository proto build and is intentionally
// not checked in. Field numbers and wire types mirror proto/tracker_system.proto.
func decodeSystemStats(payload []byte) (systemSnapshot, error) {
	var out systemSnapshot
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return out, protowire.ParseError(n)
		}
		payload = payload[n:]

		switch num {
		case 1: // repeated Process processes
			value, consumed, err := consumeMessage(payload, typ)
			if err != nil {
				return out, fieldError("SystemStats.processes", err)
			}
			process, err := decodeProcess(value)
			if err != nil {
				return out, err
			}
			out.Processes = append(out.Processes, process)
			payload = payload[consumed:]
			continue
		case 3: // CPUInfo cpu
			value, consumed, err := consumeMessage(payload, typ)
			if err != nil {
				return out, fieldError("SystemStats.cpu", err)
			}
			cpu, err := decodeCPUInfo(value)
			if err != nil {
				return out, err
			}
			out.CPUPercent = cpu
			payload = payload[consumed:]
			continue
		case 4: // MemoryInfo memory
			value, consumed, err := consumeMessage(payload, typ)
			if err != nil {
				return out, fieldError("SystemStats.memory", err)
			}
			if err := decodeMemoryInfo(value, &out); err != nil {
				return out, err
			}
			payload = payload[consumed:]
			continue
		case 5: // IOInfo io
			value, consumed, err := consumeMessage(payload, typ)
			if err != nil {
				return out, fieldError("SystemStats.io", err)
			}
			if err := decodeIOInfo(value, &out); err != nil {
				return out, err
			}
			payload = payload[consumed:]
			continue
		}

		consumed := protowire.ConsumeFieldValue(num, typ, payload)
		if consumed < 0 {
			return out, protowire.ParseError(consumed)
		}
		payload = payload[consumed:]
	}
	return out, nil
}

func decodeProcess(payload []byte) (processSnapshot, error) {
	var out processSnapshot
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return out, protowire.ParseError(n)
		}
		payload = payload[n:]

		switch num {
		case 1:
			value, consumed, err := consumeVarint(payload, typ)
			if err != nil {
				return out, fieldError("Process.pid", err)
			}
			out.PID = int32(value)
			payload = payload[consumed:]
			continue
		case 2:
			value, consumed, err := consumeVarint(payload, typ)
			if err != nil {
				return out, fieldError("Process.ppid", err)
			}
			out.PPID = int32(value)
			payload = payload[consumed:]
			continue
		case 3:
			value, consumed, err := consumeString(payload, typ)
			if err != nil {
				return out, fieldError("Process.name", err)
			}
			out.Name = value
			payload = payload[consumed:]
			continue
		case 4:
			value, consumed, err := consumeFixed64(payload, typ)
			if err != nil {
				return out, fieldError("Process.cpu", err)
			}
			out.CPU = math.Float64frombits(value)
			payload = payload[consumed:]
			continue
		case 5:
			value, consumed, err := consumeFixed32(payload, typ)
			if err != nil {
				return out, fieldError("Process.mem", err)
			}
			out.Memory = math.Float32frombits(value)
			payload = payload[consumed:]
			continue
		case 6:
			value, consumed, err := consumeString(payload, typ)
			if err != nil {
				return out, fieldError("Process.user", err)
			}
			out.User = value
			payload = payload[consumed:]
			continue
		case 10:
			value, consumed, err := consumeString(payload, typ)
			if err != nil {
				return out, fieldError("Process.cmdline", err)
			}
			out.Command = value
			payload = payload[consumed:]
			continue
		}

		consumed := protowire.ConsumeFieldValue(num, typ, payload)
		if consumed < 0 {
			return out, protowire.ParseError(consumed)
		}
		payload = payload[consumed:]
	}
	return out, nil
}

func decodeCPUInfo(payload []byte) (float64, error) {
	var total float64
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return 0, protowire.ParseError(n)
		}
		payload = payload[n:]
		if num == 1 {
			value, consumed, err := consumeFixed64(payload, typ)
			if err != nil {
				return 0, fieldError("CPUInfo.total", err)
			}
			total = math.Float64frombits(value)
			payload = payload[consumed:]
			continue
		}
		consumed := protowire.ConsumeFieldValue(num, typ, payload)
		if consumed < 0 {
			return 0, protowire.ParseError(consumed)
		}
		payload = payload[consumed:]
	}
	return total, nil
}

func decodeMemoryInfo(payload []byte, out *systemSnapshot) error {
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return protowire.ParseError(n)
		}
		payload = payload[n:]
		switch num {
		case 1:
			value, consumed, err := consumeVarint(payload, typ)
			if err != nil {
				return fieldError("MemoryInfo.total", err)
			}
			out.MemTotal = value
			payload = payload[consumed:]
			continue
		case 2:
			value, consumed, err := consumeVarint(payload, typ)
			if err != nil {
				return fieldError("MemoryInfo.used", err)
			}
			out.MemUsed = value
			payload = payload[consumed:]
			continue
		case 3:
			value, consumed, err := consumeFixed32(payload, typ)
			if err != nil {
				return fieldError("MemoryInfo.percent", err)
			}
			out.MemPercent = math.Float32frombits(value)
			payload = payload[consumed:]
			continue
		}
		consumed := protowire.ConsumeFieldValue(num, typ, payload)
		if consumed < 0 {
			return protowire.ParseError(consumed)
		}
		payload = payload[consumed:]
	}
	return nil
}

func decodeIOInfo(payload []byte, out *systemSnapshot) error {
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return protowire.ParseError(n)
		}
		payload = payload[n:]
		if num == 3 || num == 4 {
			value, consumed, err := consumeVarint(payload, typ)
			if err != nil {
				return fieldError("IOInfo totals", err)
			}
			if num == 3 {
				out.NetRecv = value
			} else {
				out.NetSent = value
			}
			payload = payload[consumed:]
			continue
		}
		consumed := protowire.ConsumeFieldValue(num, typ, payload)
		if consumed < 0 {
			return protowire.ParseError(consumed)
		}
		payload = payload[consumed:]
	}
	return nil
}

func consumeMessage(payload []byte, typ protowire.Type) ([]byte, int, error) {
	if typ != protowire.BytesType {
		return nil, 0, fmt.Errorf("wire type %d, want bytes", typ)
	}
	value, n := protowire.ConsumeBytes(payload)
	if n < 0 {
		return nil, 0, protowire.ParseError(n)
	}
	return value, n, nil
}

func consumeString(payload []byte, typ protowire.Type) (string, int, error) {
	value, n, err := consumeMessage(payload, typ)
	return string(value), n, err
}

func consumeVarint(payload []byte, typ protowire.Type) (uint64, int, error) {
	if typ != protowire.VarintType {
		return 0, 0, fmt.Errorf("wire type %d, want varint", typ)
	}
	value, n := protowire.ConsumeVarint(payload)
	if n < 0 {
		return 0, 0, protowire.ParseError(n)
	}
	return value, n, nil
}

func consumeFixed32(payload []byte, typ protowire.Type) (uint32, int, error) {
	if typ != protowire.Fixed32Type {
		return 0, 0, fmt.Errorf("wire type %d, want fixed32", typ)
	}
	value, n := protowire.ConsumeFixed32(payload)
	if n < 0 {
		return 0, 0, protowire.ParseError(n)
	}
	return value, n, nil
}

func consumeFixed64(payload []byte, typ protowire.Type) (uint64, int, error) {
	if typ != protowire.Fixed64Type {
		return 0, 0, fmt.Errorf("wire type %d, want fixed64", typ)
	}
	value, n := protowire.ConsumeFixed64(payload)
	if n < 0 {
		return 0, 0, protowire.ParseError(n)
	}
	return value, n, nil
}

func fieldError(field string, err error) error {
	return fmt.Errorf("decode %s: %w", field, err)
}
