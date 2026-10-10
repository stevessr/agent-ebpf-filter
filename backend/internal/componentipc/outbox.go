package componentipc

import (
	"context"
	"sync"
	"sync/atomic"
)

// MaxPendingMessages is a hard upper bound on buffered frames per connection.
// Combined with MaxMessageBytes, it bounds a malicious producer's memory use.
const MaxPendingMessages = 64

type EnqueueResult byte

const (
	Enqueued EnqueueResult = iota
	QueueClosed
	QueueSaturated
	InvalidMessage
)

type queuedFrame struct {
	kind    Kind
	payload []byte
}

// Outbox implements a non-blocking, bounded ingress for one authenticated
// session. Accepted payloads are copied so callers may immediately reuse their
// backing buffers. Run owns the only sending worker, and never retries errors
// indefinitely. Outbox never creates additional privileged services.
type Outbox struct {
	session   *Session
	queue     chan queuedFrame
	mu        sync.Mutex
	accepting bool
	ran       bool
	accepted  atomic.Uint64
	dropped   atomic.Uint64
	sent      atomic.Uint64
	failed    atomic.Uint64
}

type OutboxStats struct {
	Accepted  uint64
	Dropped   uint64
	Sent      uint64
	Failed    uint64
	Queued    int
	Capacity  int
	Accepting bool
}

// NewOutbox never creates a buffer above the protocol maximum.
func NewOutbox(session *Session, capacity int) (*Outbox, error) {
	if session == nil || session.conn == nil || capacity <= 0 || capacity > MaxPendingMessages {
		return nil, ErrInvalidConfig
	}
	return &Outbox{session: session, queue: make(chan queuedFrame, capacity), accepting: true}, nil
}

func (o *Outbox) TryEnqueue(kind Kind, payload []byte) EnqueueResult {
	if o == nil {
		return QueueClosed
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.accepting {
		o.dropped.Add(1)
		return QueueClosed
	}
	if !CanSend(o.session.local, kind) || len(payload) == 0 || len(payload) > MaxMessageBytes {
		o.dropped.Add(1)
		return InvalidMessage
	}
	// Avoid the payload copy when an already full queue will reject the item.
	if len(o.queue) == cap(o.queue) {
		o.dropped.Add(1)
		return QueueSaturated
	}
	frame := queuedFrame{kind: kind, payload: append([]byte(nil), payload...)}
	select {
	case o.queue <- frame:
		o.accepted.Add(1)
		return Enqueued
	default:
		o.dropped.Add(1)
		return QueueSaturated
	}
}

func (o *Outbox) Stats() OutboxStats {
	if o == nil {
		return OutboxStats{}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return OutboxStats{
		Accepted:  o.accepted.Load(),
		Dropped:   o.dropped.Load(),
		Sent:      o.sent.Load(),
		Failed:    o.failed.Load(),
		Queued:    len(o.queue),
		Capacity:  cap(o.queue),
		Accepting: o.accepting,
	}
}

// Run is a one-shot synchronous drain. Cancellation halts admission; queued
// items are not silently assumed delivered. In-flight writes have a timeout.
func (o *Outbox) Run(ctx context.Context) error {
	if o == nil || ctx == nil {
		return ErrInvalidConfig
	}
	o.mu.Lock()
	if o.ran {
		o.mu.Unlock()
		return ErrInvalidConfig
	}
	o.ran = true
	o.mu.Unlock()
	defer o.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case item := <-o.queue:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := o.session.Send(item.kind, item.payload); err != nil {
				o.failed.Add(1)
				return err
			}
			o.sent.Add(1)
		}
	}
}

// Stop prevents new admissions and accounts for queued-but-undelivered
// frames as drops. It never closes the channel while producers may use it.
// An in-flight Send remains separately tracked as sent or failed by Run.
func (o *Outbox) Stop() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.accepting = false
	for {
		select {
		case <-o.queue:
			o.dropped.Add(1)
		default:
			return
		}
	}
}
