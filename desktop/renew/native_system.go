package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protowire"
)

type systemProcess struct {
	PID        int
	PPID       int
	Name       string
	CPU        float64
	MemPercent float64
	User       string
	Cmdline    string
	ImagePath string // Windows only: executable image path, never a fake command line
	CreateTime int64
}

type systemSnapshot struct {
	Processes  []systemProcess
	CPUTotal   float64
	MemTotal   uint64
	MemUsed    uint64
	MemPercent float64
	DiskRead   uint64
	DiskWrite  uint64
	NetRecv    uint64
	NetSent    uint64
	FetchedAt  time.Time
}

func (a *renewApp) runSystemStats(ctx context.Context) {
	for ctx.Err() == nil {
		err := a.consumeSystemStats(ctx)
		a.update(func() {
			a.systemConnected = false
			if err != nil && ctx.Err() == nil {
				a.systemErr = err.Error()
			}
		})
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (a *renewApp) consumeSystemStats(ctx context.Context) error {
	if a.client == nil {
		return fmt.Errorf("system stats: API client unavailable")
	}
	endpoint, err := systemWebSocketURL(a.client.origin)
	if err != nil {
		return err
	}
	header := http.Header{}
	if a.client.token != "" {
		header.Set("X-API-KEY", a.client.token)
		header.Set("Authorization", "Bearer "+a.client.token)
	}
	dialer := a.client.websocketDialer()
	conn, _, err := dialer.DialContext(ctx, endpoint, header)
	if err != nil {
		return fmt.Errorf("system stats websocket: %w", err)
	}
	defer conn.Close()

	a.update(func() {
		a.systemConnected = true
		a.systemErr = ""
	})

	for ctx.Err() == nil {
		_ = conn.SetReadDeadline(time.Now().Add(45 * time.Second))
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("system stats read: %w", err)
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		snapshot, err := decodeSystemStats(data)
		if err != nil {
			return fmt.Errorf("system stats protobuf: %w", err)
		}
		snapshot.FetchedAt = time.Now()
		a.update(func() {
			a.system = snapshot
			a.systemConnected = true
			a.systemErr = ""
		})
	}
	return ctx.Err()
}

func systemWebSocketURL(origin string) (string, error) {
	parsed, err := url.Parse(origin)
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported backend scheme %q", parsed.Scheme)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/ws/system"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func decodeSystemStats(data []byte) (systemSnapshot, error) {
	var out systemSnapshot
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return out, protowire.ParseError(n)
		}
		data = data[n:]
		switch num {
		case 1:
			value, consumed := protowire.ConsumeBytes(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			process, err := decodeSystemProcess(value)
			if err != nil {
				return out, err
			}
			out.Processes = append(out.Processes, process)
			data = data[consumed:]
		case 3:
			value, consumed := protowire.ConsumeBytes(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			if err := decodeCPUInfo(value, &out); err != nil {
				return out, err
			}
			data = data[consumed:]
		case 4:
			value, consumed := protowire.ConsumeBytes(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			if err := decodeMemoryInfo(value, &out); err != nil {
				return out, err
			}
			data = data[consumed:]
		case 5:
			value, consumed := protowire.ConsumeBytes(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			if err := decodeIOInfo(value, &out); err != nil {
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
	return out, nil
}

func decodeSystemProcess(data []byte) (systemProcess, error) {
	var out systemProcess
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return out, protowire.ParseError(n)
		}
		data = data[n:]
		switch num {
		case 1, 2, 11:
			value, consumed := protowire.ConsumeVarint(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			switch num {
			case 1:
				out.PID = int(int32(value))
			case 2:
				out.PPID = int(int32(value))
			case 11:
				out.CreateTime = int64(value)
			}
			data = data[consumed:]
		case 3, 6, 10:
			value, consumed := protowire.ConsumeBytes(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			switch num {
			case 3:
				out.Name = string(value)
			case 6:
				out.User = string(value)
			case 10:
				out.Cmdline = string(value)
			}
			data = data[consumed:]
		case 4:
			value, consumed := protowire.ConsumeFixed64(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			out.CPU = math.Float64frombits(value)
			data = data[consumed:]
		case 5:
			value, consumed := protowire.ConsumeFixed32(data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			out.MemPercent = float64(math.Float32frombits(value))
			data = data[consumed:]
		default:
			consumed := protowire.ConsumeFieldValue(num, typ, data)
			if consumed < 0 {
				return out, protowire.ParseError(consumed)
			}
			data = data[consumed:]
		}
	}
	return out, nil
}

func decodeCPUInfo(data []byte, out *systemSnapshot) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return protowire.ParseError(n)
		}
		data = data[n:]
		if num == 1 && typ == protowire.Fixed64Type {
			value, consumed := protowire.ConsumeFixed64(data)
			if consumed < 0 {
				return protowire.ParseError(consumed)
			}
			out.CPUTotal = math.Float64frombits(value)
			data = data[consumed:]
			continue
		}
		consumed := protowire.ConsumeFieldValue(num, typ, data)
		if consumed < 0 {
			return protowire.ParseError(consumed)
		}
		data = data[consumed:]
	}
	return nil
}

func decodeMemoryInfo(data []byte, out *systemSnapshot) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return protowire.ParseError(n)
		}
		data = data[n:]
		switch num {
		case 1, 2:
			value, consumed := protowire.ConsumeVarint(data)
			if consumed < 0 {
				return protowire.ParseError(consumed)
			}
			if num == 1 {
				out.MemTotal = value
			} else {
				out.MemUsed = value
			}
			data = data[consumed:]
		case 3:
			value, consumed := protowire.ConsumeFixed32(data)
			if consumed < 0 {
				return protowire.ParseError(consumed)
			}
			out.MemPercent = float64(math.Float32frombits(value))
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

func decodeIOInfo(data []byte, out *systemSnapshot) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return protowire.ParseError(n)
		}
		data = data[n:]
		if num >= 1 && num <= 4 && typ == protowire.VarintType {
			value, consumed := protowire.ConsumeVarint(data)
			if consumed < 0 {
				return protowire.ParseError(consumed)
			}
			switch num {
			case 1:
				out.DiskRead = value
			case 2:
				out.DiskWrite = value
			case 3:
				out.NetRecv = value
			case 4:
				out.NetSent = value
			}
			data = data[consumed:]
			continue
		}
		consumed := protowire.ConsumeFieldValue(num, typ, data)
		if consumed < 0 {
			return protowire.ParseError(consumed)
		}
		data = data[consumed:]
	}
	return nil
}

func (a *renewApp) filteredSystemProcesses() []systemProcess {
	q := strings.ToLower(strings.TrimSpace(a.search))
	rows := make([]systemProcess, 0, len(a.system.Processes))
	for _, process := range a.system.Processes {
		if q != "" && !strings.Contains(strings.ToLower(strings.Join([]string{
			process.Name, process.User, process.Cmdline, process.ImagePath,
			fmt.Sprint(process.PID), fmt.Sprint(process.PPID),
		}, " ")), q) {
			continue
		}
		rows = append(rows, process)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CPU != rows[j].CPU {
			return rows[i].CPU > rows[j].CPU
		}
		return rows[i].PID < rows[j].PID
	})
	return rows
}
