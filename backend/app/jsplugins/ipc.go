package jsplugins

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	// One request/response per Unix socket connection; no long-lived state crosses IPC.
	ProtocolVersion = 1
	// JSON encodes some valid source bytes (e.g. '<') as six-byte
	// escapes. Count worst-case wire encoding, not unescaped source length.
	MaxWireBytes = 6*(MaxSourceBytes+MaxPayloadBytes) + 8<<10
	MaxWorkerConnections = 4
	MaxWorkerCacheEntries = 32
	WorkerIOTimeout = 250 * time.Millisecond
)

type workerRequest struct {
	Operation string `json:"operation"`
	Version int             `json:"version"`
	Source  string          `json:"source"`
	Hook    string          `json:"hook"`
	Payload json.RawMessage `json:"payload"`
}

type workerResponse struct {
	Version int             `json:"version"`
	Result  json.RawMessage `json:"result,omitempty"`
	Code    string          `json:"code,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// InvokeRemote sends only verified plugin source and JSON data to a separate
// process. This transport itself does not enforce process privileges: deploy
// the worker under a separate restricted OS identity.
func InvokeRemote(ctx context.Context, socketPath, source, hook string, payload json.RawMessage) (json.RawMessage, error) {
	return invokeRemote(ctx, socketPath, source, hook, "invoke", payload)
}

// ValidateRemote ensures the worker, not the privileged backend, compiles
// plugin source before enabling it. Disabled drafts can remain unvalidated.
func ValidateRemote(ctx context.Context, socketPath, source string) error {
	_, err := invokeRemote(ctx, socketPath, source, "", "validate", json.RawMessage("null"))
	return err
}

func invokeRemote(ctx context.Context, socketPath, source, hook, operation string, payload json.RawMessage) (json.RawMessage, error) {
	if socketPath == "" {
		return nil, errors.New("JavaScript worker socket is not configured; in-process execution is disabled")
	}
	if len(source) == 0 || len(source) > MaxSourceBytes ||
		len(payload) == 0 || len(payload) > MaxPayloadBytes || !json.Valid(payload) {
		return nil, errors.New("invalid JavaScript worker request")
	}
	if operation == "invoke" && hook != "onEvent" && hook != "onAlert" {
		return nil, ErrHookUnsupported
	}
	if operation != "invoke" && operation != "validate" {
		return nil, errors.New("unsupported worker operation")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	if err != nil {
		if ctx.Err() != nil { return nil, ctx.Err() }
		return nil, fmt.Errorf("connect JavaScript worker: %w", err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil { return nil, err }
	} else {
		_ = conn.SetDeadline(time.Now().Add(WorkerIOTimeout))
	}
	stop := context.AfterFunc(ctx, func(){ _ = conn.Close() })
	defer stop()
	req := workerRequest{Version: ProtocolVersion, Source: source, Hook: hook, Operation: operation, Payload: payload}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		if ctx.Err() != nil { return nil, ctx.Err() }
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() { return nil, context.DeadlineExceeded }
		return nil, fmt.Errorf("write JavaScript worker request: %w", err)
	}
	var response workerResponse
	if err := json.NewDecoder(io.LimitReader(conn, MaxResultBytes+4096)).Decode(&response); err != nil {
		if ctx.Err() != nil { return nil, ctx.Err() }
		// A socket deadline can fire just before ctx.Err becomes visible.
		// Never surface a raw i/o timeout as an unrelated VM failure.
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() { return nil, context.DeadlineExceeded }
		return nil, fmt.Errorf("read JavaScript worker response: %w", err)
	}
	if response.Version != ProtocolVersion {
		return nil, errors.New("JavaScript worker protocol version mismatch")
	}
	if response.Error != "" {
		if response.Code == "busy" {
			return nil, ErrExecutorBusy
		}
		return nil, fmt.Errorf("JavaScript worker: %s", response.Error)
	}
	if len(response.Result) == 0 || len(response.Result) > MaxResultBytes || !json.Valid(response.Result) {
		return nil, errors.New("invalid or oversized JavaScript worker result")
	}
	if err := ctx.Err(); err != nil { return nil, err }
	return response.Result, nil
}

// Worker owns compilation and execution, not the privileged backend. Cache
// entries hold only immutable bytecode; every call creates a fresh JS runtime.
type Worker struct {
	slots chan struct{}
	mu sync.Mutex
	programs map[[32]byte]*Program
	order [][32]byte
}

func NewWorker() *Worker {
	return &Worker{slots: make(chan struct{}, MaxWorkerConnections), programs: make(map[[32]byte]*Program)}
}

func (w *Worker) compiled(source string) (*Program, error) {
	key := sha256.Sum256([]byte(source))
	w.mu.Lock()
	defer w.mu.Unlock()
	if p, ok := w.programs[key]; ok { return p, nil }
	p, err := Compile(source)
	if err != nil { return nil, err }
	if len(w.order) >= MaxWorkerCacheEntries {
		delete(w.programs, w.order[0])
		w.order = w.order[1:]
	}
	w.order = append(w.order, key)
	w.programs[key] = p
	return p, nil
}

// Serve accepts bounded single-request connections until ctx is canceled.
// It is intended for a dedicated non-root process with systemd sandboxing.
// The listener must have been bound with secure filesystem permissions.
func (w *Worker) Serve(ctx context.Context, listener net.Listener) error {
	if ctx == nil { ctx = context.Background() }
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stop()
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil { return nil }
			return err
		}
		select {
		case w.slots <- struct{}{}:
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func(){ <-w.slots }()
				w.handleConn(conn)
			}()
		default:
			_ = conn.SetWriteDeadline(time.Now().Add(WorkerIOTimeout))
			_ = json.NewEncoder(conn).Encode(workerResponse{Version: ProtocolVersion, Code:"busy", Error:"JavaScript worker capacity exceeded"})
			_ = conn.Close()
		}
	}
}

func (w *Worker) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(WorkerIOTimeout))
	var resp workerResponse
	resp.Version = ProtocolVersion
	// The newline terminator is part of the protocol. Restrict bytes BEFORE
	// decoding: an attacker must not be able to stream unbounded JSON.
	line, err := bufio.NewReader(io.LimitReader(conn, MaxWireBytes+1)).ReadBytes('\n')
	if err != nil || len(line) > MaxWireBytes {
		resp.Code, resp.Error = "validation", "invalid or oversized JavaScript worker frame"
		_ = json.NewEncoder(conn).Encode(resp)
		return
	}
	var req workerRequest
	if err := json.Unmarshal(bytes.TrimSpace(line), &req); err != nil {
		resp.Code, resp.Error = "validation", "malformed JavaScript worker request"
		_ = json.NewEncoder(conn).Encode(resp)
		return
	}
	if req.Version != ProtocolVersion ||
		(req.Operation != "invoke" && req.Operation != "validate") ||
		(req.Operation == "invoke" && req.Hook != "onEvent" && req.Hook != "onAlert") ||
		(req.Operation == "validate" && req.Hook != "") ||
		len(req.Source) == 0 || len(req.Source) > MaxSourceBytes ||
		len(req.Payload) == 0 || len(req.Payload) > MaxPayloadBytes || !json.Valid(req.Payload) {
		resp.Code, resp.Error = "validation", "invalid JavaScript worker version, hook, source or payload"
		_ = json.NewEncoder(conn).Encode(resp)
		return
	}
	program, err := w.compiled(req.Source)
	if err == nil {
		if req.Operation == "validate" {
			resp.Result = json.RawMessage("true")
		} else {
			runCtx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
			// The client may time out or disconnect before the worker's own
			// deadline. Detect EOF to interrupt the VM and release its slot,
			// rather than allowing execution after the caller has returned.
			peerDone := make(chan struct{})
			go func() {
				defer close(peerDone)
				var extra [1]byte
				if _, readErr := conn.Read(extra[:]); readErr != nil {
					cancel()
				}
			}()
			resp.Result, err = program.Run(runCtx, req.Hook, req.Payload)
			// Wake the reader on normal completion too. A deadline also
			// bounds it if the peer leaves the connection open.
			_ = conn.SetReadDeadline(time.Now())
			<-peerDone
			cancel()
		}
	}
	if err != nil {
		resp.Code, resp.Error = "runtime", err.Error()
	}
	_ = json.NewEncoder(conn).Encode(resp)
}
