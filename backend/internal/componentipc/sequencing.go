package componentipc

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
)

// SequencedSchemaVersion versions the optional event/alert/status body envelope.
// It is distinct from the connection handshake version. Legacy raw Send and
// Receive frames are deliberately unchanged by this codec.
const (
	SequencedSchemaVersion uint16 = 1
	SequencedHeaderBytes          = 2 + 16 + 8
	MaxSequencedPayloadBytes      = MaxMessageBytes - SequencedHeaderBytes
)

var (
	ErrInvalidSequence = errors.New("invalid component IPC sequence envelope")
	ErrSequenceGap     = errors.New("component IPC sequence gap")
	ErrSequenceReplay  = errors.New("component IPC duplicate or stale sequence")
	ErrForeignStream   = errors.New("component IPC stream identity mismatch")
	ErrAckAhead        = errors.New("component IPC acknowledgement exceeds sent sequence")
)

// StreamID differentiates new publisher lifetimes. It is NOT a credential.
// An identifier must be freshly generated on each new stream.
type StreamID [16]byte

func (id StreamID) valid() bool {
	return id != (StreamID{})
}

// NewStreamID uses a cryptographically random identifier for each stream.
func NewStreamID() (StreamID, error) {
	var id StreamID
	if _, err := rand.Read(id[:]); err != nil {
		return StreamID{}, err
	}
	if !id.valid() {
		return StreamID{}, ErrInvalidSequence
	}
	return id, nil
}

// Sequenced carries opaque domain data with independently versioned transport
// metadata. It does not define a canonical BPF event or status schema.
// DecodeSequenced returns a borrowed Payload; copy before retaining it.
type Sequenced struct {
	Stream   StreamID
	Sequence uint64
	Payload  []byte
}

// Ack acknowledges application-level processing of every sequence up to and
// including Sequence, within one stream. Calling EncodeAck does NOT imply
// that the record was processed or persisted.
type Ack struct {
	Stream   StreamID
	Sequence uint64
}

func dataKind(kind Kind) bool {
	return kind == KindEvent || kind == KindAlert || kind == KindStatus
}

func validSequence(stream StreamID, sequence uint64) bool {
	return stream.valid() && sequence != 0
}

func putSequenceHeader(out []byte, stream StreamID, sequence uint64) {
	binary.BigEndian.PutUint16(out[:2], SequencedSchemaVersion)
	copy(out[2:18], stream[:])
	binary.BigEndian.PutUint64(out[18:SequencedHeaderBytes], sequence)
}

func parseSequenceHeader(raw []byte) (StreamID, uint64, error) {
	if len(raw) < SequencedHeaderBytes {
		return StreamID{}, 0, ErrInvalidSequence
	}
	if binary.BigEndian.Uint16(raw[:2]) != SequencedSchemaVersion {
		return StreamID{}, 0, ErrVersionMismatch
	}
	var id StreamID
	copy(id[:], raw[2:18])
	seq := binary.BigEndian.Uint64(raw[18:SequencedHeaderBytes])
	if !validSequence(id, seq) {
		return StreamID{}, 0, ErrInvalidSequence
	}
	return id, seq, nil
}

// EncodeSequenced encodes a bounded, versioned body for event, alert or status.
// It does not write to the socket; Session.Send still enforces sender roles.
func EncodeSequenced(kind Kind, frame Sequenced) ([]byte, error) {
	if !dataKind(kind) {
		return nil, ErrFrameDenied
	}
	if !validSequence(frame.Stream, frame.Sequence) || len(frame.Payload) == 0 {
		return nil, ErrInvalidSequence
	}
	if len(frame.Payload) > MaxSequencedPayloadBytes {
		return nil, fmt.Errorf("%w: %d", ErrFrameTooLarge, len(frame.Payload))
	}
	raw := make([]byte, SequencedHeaderBytes+len(frame.Payload))
	putSequenceHeader(raw, frame.Stream, frame.Sequence)
	copy(raw[SequencedHeaderBytes:], frame.Payload)
	return raw, nil
}

// DecodeSequenced borrows the decoded body and rejects unsupported versions,
// zero sequence IDs, empty bodies and oversized application data.
func DecodeSequenced(kind Kind, raw []byte) (Sequenced, error) {
	if !dataKind(kind) {
		return Sequenced{}, ErrFrameDenied
	}
	if len(raw) < SequencedHeaderBytes+1 {
		return Sequenced{}, ErrInvalidSequence
	}
	if len(raw) > MaxMessageBytes {
		return Sequenced{}, ErrFrameTooLarge
	}
	id, seq, err := parseSequenceHeader(raw)
	if err != nil {
		return Sequenced{}, err
	}
	return Sequenced{Stream: id, Sequence: seq, Payload: raw[SequencedHeaderBytes:]}, nil
}

// EncodeAck produces a fixed-size receipt for the KindAck message type.
func EncodeAck(receipt Ack) ([]byte, error) {
	if !validSequence(receipt.Stream, receipt.Sequence) {
		return nil, ErrInvalidSequence
	}
	raw := make([]byte, SequencedHeaderBytes)
	putSequenceHeader(raw, receipt.Stream, receipt.Sequence)
	return raw, nil
}

func DecodeAck(raw []byte) (Ack, error) {
	if len(raw) != SequencedHeaderBytes {
		return Ack{}, ErrInvalidSequence
	}
	id, seq, err := parseSequenceHeader(raw)
	if err != nil {
		return Ack{}, err
	}
	return Ack{Stream: id, Sequence: seq}, nil
}

// SendSequenced is an opt-in wrapper; it preserves the role authorization,
// framing and write deadline applied by Session.Send.
func (s *Session) SendSequenced(kind Kind, frame Sequenced) error {
	raw, err := EncodeSequenced(kind, frame)
	if err != nil {
		return err
	}
	return s.Send(kind, raw)
}

// SendAck writes an explicit receipt. Applications MUST call this only after
// successfully processing the referenced record; no automatic ACK is sent.
func (s *Session) SendAck(receipt Ack) error {
	raw, err := EncodeAck(receipt)
	if err != nil {
		return err
	}
	return s.Send(KindAck, raw)
}

// SequenceReceiver tracks a single stream of committed contiguous records.
// Check and Commit support an application that first validates a record,
// applies it, then Commits and sends the resulting ACK. The application must
// serialize its processing, since Check does not reserve a sequence number.
type SequenceReceiver struct {
	mu     sync.Mutex
	stream StreamID
	last   uint64
}

func NewSequenceReceiver(stream StreamID) (*SequenceReceiver, error) {
	if !stream.valid() {
		return nil, ErrInvalidSequence
	}
	return &SequenceReceiver{stream: stream}, nil
}

func checkNext(stream StreamID, last uint64, next Sequenced) error {
	if stream != next.Stream {
		return ErrForeignStream
	}
	if next.Sequence == 0 {
		return ErrInvalidSequence
	}
	if next.Sequence <= last {
		return ErrSequenceReplay
	}
	// last+1 does not overflow because next.Sequence > last.
	if next.Sequence != last+1 {
		return fmt.Errorf("%w: expected %d got %d", ErrSequenceGap, last+1, next.Sequence)
	}
	return nil
}

func (r *SequenceReceiver) Check(next Sequenced) error {
	if r == nil {
		return ErrInvalidConfig
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return checkNext(r.stream, r.last, next)
}

// Commit advances only on the exact next record. Caller must have processed
// the payload successfully before invoking Commit; it does not send an ACK.
func (r *SequenceReceiver) Commit(next Sequenced) (Ack, error) {
	if r == nil {
		return Ack{}, ErrInvalidConfig
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := checkNext(r.stream, r.last, next); err != nil {
		return Ack{}, err
	}
	r.last = next.Sequence
	return Ack{Stream: r.stream, Sequence: r.last}, nil
}

func (r *SequenceReceiver) LastCommitted() uint64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}

// AckTracker tracks writes completed in contiguous order, and the highest
// cumulative ACK validated for that same stream. It does not retain payloads,
// resubmit failed writes or persist checkpoints across process restarts.
type AckTracker struct {
	mu         sync.Mutex
	stream     StreamID
	lastSent   uint64
	lastAcked  uint64
}

func NewAckTracker(stream StreamID) (*AckTracker, error) {
	if !stream.valid() {
		return nil, ErrInvalidSequence
	}
	return &AckTracker{stream: stream}, nil
}

// MarkSent must be called only after successful write of the matching record.
// Gaps are rejected; any lost record requires an explicit higher-layer reset
// or replay strategy rather than silently treating later writes as contiguous.
func (t *AckTracker) MarkSent(sequence uint64) error {
	if t == nil {
		return ErrInvalidConfig
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if sequence == 0 || sequence <= t.lastSent {
		return ErrSequenceReplay
	}
	if sequence != t.lastSent+1 {
		return ErrSequenceGap
	}
	t.lastSent = sequence
	return nil
}

func (t *AckTracker) ApplyAck(receipt Ack) error {
	if t == nil {
		return ErrInvalidConfig
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if receipt.Stream != t.stream {
		return ErrForeignStream
	}
	if receipt.Sequence == 0 {
		return ErrInvalidSequence
	}
	if receipt.Sequence <= t.lastAcked {
		return ErrSequenceReplay
	}
	if receipt.Sequence > t.lastSent {
		return ErrAckAhead
	}
	t.lastAcked = receipt.Sequence
	return nil
}

func (t *AckTracker) LastAcked() uint64 {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastAcked
}
