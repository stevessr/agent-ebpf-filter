//go:build linux

// Package componentshm implements an opt-in, single-producer/single-consumer
// process-shared telemetry ring. It does not replace authenticated control IPC,
// perform policy actions, persist records, or own privileged kernel resources.
package componentshm

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	Version         uint32 = 1
	HeaderBytes            = 256
	MinSlots               = 2
	MaxSlots               = 4096
	MinPayload             = 64
	MaxPayload             = 16384
	maxMappingBytes        = HeaderBytes + MaxSlots*(MaxPayload+8)
)

var (
	ErrInvalidRing = errors.New("invalid shared ring descriptor or header")
	ErrRingFull    = errors.New("shared event ring full")
	ErrTooLarge    = errors.New("shared event exceeds slot capacity")
	ErrRingClosed  = errors.New("shared event ring closed")
	ErrNoRecord    = errors.New("shared event ring empty")
	ErrInvalidRole = errors.New("shared event ring requires one writer or one reader")
	ErrInvalidAck  = errors.New("shared event ring receipt is stale or beyond published data")
)

const (
	offMagic    = 0
	offVersion  = 8
	offCapacity = 12
	offStream   = 24
	offPayload  = 16
	offHead     = 64
	offClosed   = 192
	offRejected = 200
)

const magic = "AEBPSHM1"

// Stats are best-effort atomic snapshots, not durable delivery receipts.
type Stats struct {
	Published uint64
	Consumed  uint64
	Queued    uint64
	Rejected  uint64
	Capacity  uint64
	Closed    bool
}

// Ring has exactly one writing owner and one reading owner. The memory file
// must be inherited or passed only to authenticated, trusted participants.
// The publisher holds the sole writable mapping, created before applying
// F_SEAL_FUTURE_WRITE. The Engine maps event memory read-only and cannot
// advance the recycling watermark; authenticated UDS ACKs are required.
type Ring struct {
	memory    []byte
	memfd     *os.File
	wakefd    *os.File
	slots     uint64
	width     uint64
	limit     int
	writer    bool
	closed    bool
	confirmed atomic.Uint64 // publisher-private authenticated ACK cursor
	readTail  uint64        // reader-private cursor; never in shared memory
}

// align each record to an 8-byte boundary so wrap offsets remain bounded.
func slotStride(payload int) uint64 {
	return uint64((payload + 4 + 7) &^ 7)
}

func checkConfig(slots, payload int) error {
	if slots < MinSlots || slots > MaxSlots || slots&(slots-1) != 0 ||
		payload < MinPayload || payload > MaxPayload {
		return ErrInvalidRing
	}
	return nil
}

func sizeFor(slots, payload int) int {
	return HeaderBytes + int(uint64(slots)*slotStride(payload))
}

func Create(slots, payload int) (_ *Ring, err error) {
	if err := checkConfig(slots, payload); err != nil {
		return nil, err
	}
	fd, err := unix.MemfdCreate("agent-ebpf-component-events", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, err
	}
	memoryFile := os.NewFile(uintptr(fd), "component-event-memfd")
	defer func() {
		if err != nil {
			_ = memoryFile.Close()
		}
	}()
	size := sizeFor(slots, payload)
	if err = unix.Ftruncate(fd, int64(size)); err != nil {
		return nil, err
	}
	// The parent maps RW before sealing. F_SEAL_FUTURE_WRITE denies new
	// writable mappings and write(2), including with an inherited FD.
	memory, err := unix.Mmap(fd, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	_, err = unix.FcntlInt(uintptr(fd), unix.F_ADD_SEALS,
		unix.F_SEAL_SHRINK|unix.F_SEAL_GROW|unix.F_SEAL_FUTURE_WRITE|unix.F_SEAL_SEAL)
	if err != nil {
		_ = unix.Munmap(memory)
		return nil, err
	}
	var stream [16]byte
	if _, randErr := rand.Read(stream[:]); randErr != nil || stream == ([16]byte{}) {
		_ = unix.Munmap(memory)
		if randErr != nil {
			return nil, randErr
		}
		return nil, ErrInvalidRing
	}
	copy(memory[offMagic:offMagic+8], magic)
	copy(memory[offStream:offStream+16], stream[:])
	binary.LittleEndian.PutUint32(memory[offVersion:offVersion+4], Version)
	binary.LittleEndian.PutUint32(memory[offCapacity:offCapacity+4], uint32(slots))
	binary.LittleEndian.PutUint32(memory[offPayload:offPayload+4], uint32(payload))
	wake, wakeErr := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if wakeErr != nil {
		_ = unix.Munmap(memory)
		return nil, wakeErr
	}
	return &Ring{
		memory: memory, memfd: memoryFile,
		wakefd: os.NewFile(uintptr(wake), "component-event-wakeup"),
		slots:  uint64(slots), width: slotStride(payload), limit: payload, writer: true,
	}, nil
}

// OpenReader takes ownership of the supplied descriptor files on success
// only. The caller must close them on failure. Descriptors must come from a
// trusted process launcher, never an untrusted pathname or environment value.
func OpenReader(memoryFile, wakeFile *os.File) (*Ring, error) {
	if memoryFile == nil || wakeFile == nil {
		return nil, ErrInvalidRing
	}
	fd := int(memoryFile.Fd())
	wakefd := int(wakeFile.Fd())
	seals, err := unix.FcntlInt(uintptr(fd), unix.F_GET_SEALS, 0)
	if err != nil || seals&(unix.F_SEAL_SHRINK|unix.F_SEAL_GROW|unix.F_SEAL_FUTURE_WRITE|unix.F_SEAL_SEAL) !=
		(unix.F_SEAL_SHRINK|unix.F_SEAL_GROW|unix.F_SEAL_FUTURE_WRITE|unix.F_SEAL_SEAL) {
		return nil, ErrInvalidRing
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return nil, err
	}
	if st.Size < HeaderBytes || st.Size > maxMappingBytes {
		return nil, ErrInvalidRing
	}
	var wakeStat unix.Stat_t
	if err := unix.Fstat(wakefd, &wakeStat); err != nil || wakeStat.Mode&unix.S_IFMT != 0 {
		// eventfd is an anonymous inode (mode 0600 but no file type).
		return nil, ErrInvalidRing
	}
	memory, err := unix.Mmap(fd, 0, int(st.Size), unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	reject := func() (*Ring, error) {
		_ = unix.Munmap(memory)
		return nil, ErrInvalidRing
	}
	if string(memory[offMagic:offMagic+8]) != magic ||
		binary.LittleEndian.Uint32(memory[offVersion:offVersion+4]) != Version {
		return reject()
	}
	slots := int(binary.LittleEndian.Uint32(memory[offCapacity : offCapacity+4]))
	payload := int(binary.LittleEndian.Uint32(memory[offPayload : offPayload+4]))
	if checkConfig(slots, payload) != nil || int(st.Size) != sizeFor(slots, payload) ||
		string(memory[offStream:offStream+16]) == string(make([]byte, 16)) {
		return reject()
	}
	return &Ring{
		memory: memory, memfd: memoryFile, wakefd: wakeFile,
		slots: uint64(slots), width: slotStride(payload), limit: payload,
	}, nil
}

func (r *Ring) cursor(off int) *uint64 {
	return (*uint64)(unsafe.Pointer(&r.memory[off]))
}

func (r *Ring) load(off int) uint64 {
	return atomic.LoadUint64(r.cursor(off))
}

func (r *Ring) store(off int, value uint64) {
	atomic.StoreUint64(r.cursor(off), value)
}

func (r *Ring) alive(writer bool) bool {
	return r != nil && !r.closed && len(r.memory) >= HeaderBytes && r.writer == writer
}

// Stream identifies this ring lifetime. It is public metadata, not a secret
// or authorization credential. It lets authenticated control ACKs bind to this
// ring and reject stale receipts from previous subprocess lifetimes.
func (r *Ring) Stream() [16]byte {
	var id [16]byte
	if r != nil && !r.closed && len(r.memory) >= HeaderBytes {
		copy(id[:], r.memory[offStream:offStream+16])
	}
	return id
}

func (r *Ring) Capacity() int {
	if r == nil {
		return 0
	}
	return int(r.slots)
}

func (r *Ring) MaxPayload() int {
	if r == nil {
		return 0
	}
	return r.limit
}

// Files returns descriptors owned by the ring, intended for an explicit
// child ExtraFiles list or authenticated SCM_RIGHTS handoff. Do not close
// these files independently while the Ring is active.
func (r *Ring) Files() (*os.File, *os.File, error) {
	if !r.alive(true) {
		return nil, nil, ErrInvalidRole
	}
	return r.memfd, r.wakefd, nil
}

// TryWrite never blocks or triggers a syscall. If full, it refuses admission
// and counts one loss; callers must not treat refused records as delivered.
// Only a single producer goroutine/process may call TryWrite.
func (r *Ring) TryWrite(data []byte) error {
	if !r.alive(true) {
		return ErrInvalidRole
	}
	if r.load(offClosed) != 0 {
		return ErrRingClosed
	}
	if len(data) == 0 || len(data) > r.MaxPayload() {
		return ErrTooLarge
	}
	head, tail := r.load(offHead), r.confirmed.Load()
	if head < tail || head-tail > r.slots {
		return ErrInvalidRing
	}
	if head-tail == r.slots {
		atomic.AddUint64(r.cursor(offRejected), 1)
		return ErrRingFull
	}
	off := HeaderBytes + int((head%r.slots)*r.width)
	binary.LittleEndian.PutUint32(r.memory[off:off+4], uint32(len(data)))
	copy(r.memory[off+4:off+4+len(data)], data)
	// Release publication after payload write. Reader's atomic head load
	// acquires the full slot before it reads.
	r.store(offHead, head+1)
	return nil
}

// Confirm releases published slots after the Collector verifies an
// authenticated, monotonic, matching-stream cumulative UDS ACK.
func (r *Ring) Confirm(sequence uint64) error {
	if !r.alive(true) {
		return ErrInvalidRole
	}
	old, head := r.confirmed.Load(), r.load(offHead)
	if sequence <= old || sequence > head || head-old > r.slots {
		return ErrInvalidAck
	}
	r.confirmed.Store(sequence)
	return nil
}

// Notify performs exactly one eventfd write per caller-chosen batch.
// A saturated eventfd is already signalled, so EAGAIN is not a loss.
func (r *Ring) Notify() error {
	if !r.alive(true) {
		return ErrInvalidRole
	}
	var value [8]byte
	binary.LittleEndian.PutUint64(value[:], 1)
	n, err := unix.Write(int(r.wakefd.Fd()), value[:])
	if errors.Is(err, unix.EAGAIN) {
		return nil
	}
	if err != nil {
		return err
	}
	if n != len(value) {
		return ioErrShortWrite
	}
	return nil
}

var ioErrShortWrite = errors.New("short eventfd notification write")

// ReadCopy returns owned bytes. Only a single reader goroutine/process may
// call ReadCopy. The tail is published only after the slot copy completes.
func (r *Ring) ReadCopy() ([]byte, error) {
	if !r.alive(false) {
		return nil, ErrInvalidRole
	}
	tail, head := r.readTail, r.load(offHead)
	if head < tail || head-tail > r.slots {
		return nil, ErrInvalidRing
	}
	if tail == head {
		if r.load(offClosed) != 0 {
			return nil, ErrRingClosed
		}
		return nil, ErrNoRecord
	}
	off := HeaderBytes + int((tail%r.slots)*r.width)
	length := int(binary.LittleEndian.Uint32(r.memory[off : off+4]))
	if length <= 0 || length > r.MaxPayload() {
		return nil, ErrInvalidRing
	}
	data := append([]byte(nil), r.memory[off+4:off+4+length]...)
	// Reader tail is private. The writer may reclaim the slot only after
	// verifying a matching authenticated cumulative UDS ACK.
	r.readTail = tail + 1
	return data, nil
}

// WaitReady checks occupancy before polling to avoid missed wakeups. It
// never blocks indefinitely: the 100ms poll window bounds cancellation
// response if the publisher dies without notifying.
func (r *Ring) WaitReady(ctx context.Context) error {
	if !r.alive(false) || ctx == nil {
		return ErrInvalidRole
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		tail, head := r.readTail, r.load(offHead)
		if head < tail || head-tail > r.slots {
			return ErrInvalidRing
		}
		if head != tail {
			return nil
		}
		if r.load(offClosed) != 0 {
			return ErrRingClosed
		}
		ms := 100
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return context.DeadlineExceeded
			}
			if remaining < time.Duration(ms)*time.Millisecond {
				ms = int((remaining + time.Millisecond - 1) / time.Millisecond)
			}
		}
		pfds := []unix.PollFd{{Fd: int32(r.wakefd.Fd()), Events: unix.POLLIN}}
		_, err := unix.Poll(pfds, ms)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("wait for eventfd: %w", err)
		}
		if pfds[0].Revents&(unix.POLLERR|unix.POLLNVAL|unix.POLLHUP) != 0 {
			return ErrInvalidRing
		}
		if pfds[0].Revents&unix.POLLIN != 0 {
			var buf [8]byte
			_, err := unix.Read(int(r.wakefd.Fd()), buf[:])
			if err != nil && !errors.Is(err, unix.EAGAIN) {
				return err
			}
		}
	}
}

// Finish prevents further records and wakes a sleeping receiver. It does not
// imply that queued messages have been validated or acknowledged.
func (r *Ring) Finish() error {
	if !r.alive(true) {
		return ErrInvalidRole
	}
	r.store(offClosed, 1)
	return r.Notify()
}

func (r *Ring) Stats() Stats {
	if r == nil || r.closed || len(r.memory) < HeaderBytes {
		return Stats{}
	}
	head := r.load(offHead)
	var tail uint64
	if r.writer {
		tail = r.confirmed.Load()
	} else {
		tail = r.readTail
	}
	queued := uint64(0)
	if head >= tail && head-tail <= r.slots {
		queued = head - tail
	}
	return Stats{
		Published: head, Consumed: tail, Queued: queued,
		Rejected: r.load(offRejected), Capacity: r.slots,
		Closed: r.load(offClosed) != 0,
	}
}

// Close unmaps the ring and closes both owned descriptors. The owning caller
// must first stop all goroutines using this mapping; Close is not concurrent
// safe and does not call Finish implicitly.
func (r *Ring) Close() error {
	if r == nil || r.closed {
		return nil
	}
	r.closed = true
	err := unix.Munmap(r.memory)
	r.memory = nil
	e1 := r.memfd.Close()
	e2 := r.wakefd.Close()
	if err != nil {
		return err
	}
	if e1 != nil {
		return e1
	}
	return e2
}
