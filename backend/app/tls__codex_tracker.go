package app

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	bpf "agent-ebpf-filter/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

type CodexSyscallTracker struct {
	objs   *bpf.CodexSyscallTrackerObjects
	links  []link.Link
	store  *TLSCaptureStore
	mu     sync.Mutex
	closed bool
}

func NewCodexSyscallTracker(store *TLSCaptureStore) (*CodexSyscallTracker, error) {
	objs := &bpf.CodexSyscallTrackerObjects{}
	if err := bpf.LoadCodexSyscallTrackerObjects(objs, nil); err != nil {
		return nil, err
	}
	return &CodexSyscallTracker{
		objs:  objs,
		store: store,
	}, nil
}

func (t *CodexSyscallTracker) Attach() error {
	if t == nil || t.objs == nil {
		return errors.New("tracker not initialized")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	tp, err := link.Tracepoint("syscalls", "sys_enter_write", t.objs.TraceCodexWrite, nil)
	if err != nil {
		return err
	}
	t.links = append(t.links, tp)

	tp, err = link.Tracepoint("syscalls", "sys_exit_read", t.objs.TraceCodexRead, nil)
	if err != nil {
		return err
	}
	t.links = append(t.links, tp)

	return nil
}

func (t *CodexSyscallTracker) ReadLoop() error {
	if t == nil || t.objs == nil || t.objs.CodexEvents == nil {
		return nil
	}

	reader, err := ringbuf.NewReader(t.objs.CodexEvents)
	if err != nil {
		return err
	}
	defer reader.Close()

	for {
		rec, err := reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return nil
			}
			return err
		}

		var evt bpf.CodexSyscallTrackerCodexSyscallEvent
		if err := binary.Read(bytes.NewReader(rec.RawSample), binary.LittleEndian, &evt); err != nil {
			continue
		}

		t.processEvent(evt)
	}
}

func (t *CodexSyscallTracker) processEvent(evt bpf.CodexSyscallTrackerCodexSyscallEvent) {
	if t.store == nil {
		return
	}

	commBytes := make([]byte, len(evt.Comm))
	for i, v := range evt.Comm {
		commBytes[i] = byte(v)
	}
	comm := string(bytes.TrimRight(commBytes, "\x00"))

	dataBytes := make([]byte, evt.DataLen)
	for i := uint32(0); i < evt.DataLen && i < uint32(len(evt.Data)); i++ {
		dataBytes[i] = byte(evt.Data[i])
	}

	direction := "send"
	if evt.Direction == 0 {
		direction = "recv"
	}

	method, url := "", ""
	if bytes.HasPrefix(dataBytes, []byte("GET ")) || bytes.HasPrefix(dataBytes, []byte("POST ")) ||
		bytes.HasPrefix(dataBytes, []byte("PUT ")) || bytes.HasPrefix(dataBytes, []byte("PATCH ")) {
		parts := bytes.SplitN(dataBytes, []byte(" "), 3)
		if len(parts) >= 2 {
			method = string(parts[0])
			url = string(parts[1])
		}
	}

	event := TLSPlaintextEvent{
		Type:      "syscall_" + direction,
		Timestamp: time.Unix(0, int64(evt.TimestampNs)),
		PID:       evt.Pid,
		Comm:      comm,
		Method:    method,
		URL:       url,
		BodySize:  int(evt.DataLen),
	}

	t.store.Add(event)
}

func (t *CodexSyscallTracker) Close() error {
	if t == nil {
		return nil
	}

	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	links := t.links
	t.links = nil
	objs := t.objs
	t.objs = nil
	t.mu.Unlock()

	var errs []error
	for _, l := range links {
		if l != nil {
			if err := l.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if objs != nil {
		if err := objs.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
