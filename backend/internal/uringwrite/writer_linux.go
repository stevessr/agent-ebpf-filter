//go:build linux && (amd64 || arm64)

// Package uringwrite implements opt-in, ordered io_uring writes to append-only
// log files. It intentionally does not own the file descriptor it is given.
package uringwrite

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	ioUringSetup      = 425 // Linux amd64/arm64 syscall numbers
	ioUringEnter      = 426
	ioUringOpWrite    = 23
	ioUringGetEvents  = 1
	ioUringSingleMmap = 1
	ioUringCurrentPos = 1 << 3
	ioUringOffSQRing  = 0
	ioUringOffCQRing  = 0x8000000
	ioUringOffSQEs    = 0x10000000
)

type sqOffsets struct {
	Head, Tail, Mask, Entries, Flags, Dropped, Array, Reserved uint32
	Reserved2                                                  uint64
}

type cqOffsets struct {
	Head, Tail, Mask, Entries, Overflow, CQEs, Flags, Reserved uint32
	Reserved2                                                  uint64
}

type ringParams struct {
	SQEntries, CQEntries, Flags, SQCPU, SQIdle, Features, WQFD uint32
	Reserved                                                   [3]uint32
	SQ                                                         sqOffsets
	CQ                                                         cqOffsets
}

type sqEntry struct {
	Opcode, Flags            uint8
	Priority                 uint16
	FD                       int32
	Offset, Address          uint64
	Length, RWFlags          uint32
	UserData                 uint64
	BufferIndex, Personality uint16
	SpliceFD                 int32
	Address3                 uint64
	Pad                      uint64
}

type cqEntry struct {
	UserData uint64
	Result   int32
	Flags    uint32
}

// Writer has only one outstanding SQE, preserving on-disk audit-chain order.
// Write returns after completion (not just submission), so callers' flush
// barriers and persisted counters keep their original semantics.
//
// A Writer is not safe for concurrent use; a single log writer goroutine owns it.
type Writer struct {
	ringFD, fileFD                    int
	sqRing, cqRing, sqes              []byte
	sqHead, sqTail, sqMask, sqEntries *uint32
	sqArray                           *uint32
	cqHead, cqTail, cqMask            *uint32
	cqeOffset                         uint32
	closed                            bool
}

// New initializes a ring for an existing O_APPEND regular file. No I/O is
// submitted on failure, so a caller may safely retain the normal write path.
// Close only releases ring resources, never the original file.
func New(file *os.File) (*Writer, error) {
	if file == nil {
		return nil, errors.New("io_uring: nil file")
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("io_uring requires a regular file")
	}
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, file.Fd(), syscall.F_GETFL, 0)
	if errno != 0 {
		return nil, errno
	}
	if flags&syscall.O_APPEND == 0 {
		return nil, errors.New("io_uring requires O_APPEND")
	}
	var params ringParams
	fd, _, errno := syscall.Syscall(ioUringSetup, 2, uintptr(unsafe.Pointer(&params)), 0)
	if errno != 0 {
		return nil, fmt.Errorf("io_uring_setup: %w", errno)
	}
	w := &Writer{ringFD: int(fd), fileFD: int(file.Fd())}
	ok := false
	defer func() {
		if !ok {
			_ = w.Close()
		}
	}()
	if params.Features&ioUringCurrentPos == 0 {
		return nil, errors.New("io_uring lacks IORING_FEAT_RW_CUR_POS")
	}
	if params.SQEntries == 0 || params.CQEntries == 0 {
		return nil, errors.New("io_uring returned empty rings")
	}
	sqSize := int(params.SQ.Array) + int(params.SQEntries)*4
	cqSize := int(params.CQ.CQEs) + int(params.CQEntries)*int(unsafe.Sizeof(cqEntry{}))
	if sqSize <= 0 || cqSize <= 0 {
		return nil, errors.New("io_uring returned invalid ring sizes")
	}
	single := params.Features&ioUringSingleMmap != 0
	if single && cqSize > sqSize {
		sqSize = cqSize
	}
	w.sqRing, err = syscall.Mmap(w.ringFD, ioUringOffSQRing, sqSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("map io_uring SQ: %w", err)
	}
	if single {
		w.cqRing = w.sqRing
	} else {
		w.cqRing, err = syscall.Mmap(w.ringFD, ioUringOffCQRing, cqSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if err != nil {
			return nil, fmt.Errorf("map io_uring CQ: %w", err)
		}
	}
	w.sqes, err = syscall.Mmap(w.ringFD, ioUringOffSQEs, int(params.SQEntries)*int(unsafe.Sizeof(sqEntry{})), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("map io_uring SQEs: %w", err)
	}
	if !fits(w.sqRing, params.SQ.Head, 4) ||
		!fits(w.sqRing, params.SQ.Tail, 4) ||
		!fits(w.sqRing, params.SQ.Mask, 4) ||
		!fits(w.sqRing, params.SQ.Entries, 4) ||
		!fits(w.sqRing, params.SQ.Array, params.SQEntries*4) ||
		!fits(w.cqRing, params.CQ.Head, 4) ||
		!fits(w.cqRing, params.CQ.Tail, 4) ||
		!fits(w.cqRing, params.CQ.Mask, 4) ||
		!fits(w.cqRing, params.CQ.CQEs, params.CQEntries*uint32(unsafe.Sizeof(cqEntry{}))) {
		return nil, errors.New("io_uring returned invalid offsets")
	}
	w.sqHead = word(w.sqRing, params.SQ.Head)
	w.sqTail = word(w.sqRing, params.SQ.Tail)
	w.sqMask = word(w.sqRing, params.SQ.Mask)
	w.sqEntries = word(w.sqRing, params.SQ.Entries)
	w.sqArray = word(w.sqRing, params.SQ.Array)
	w.cqHead = word(w.cqRing, params.CQ.Head)
	w.cqTail = word(w.cqRing, params.CQ.Tail)
	w.cqMask = word(w.cqRing, params.CQ.Mask)
	w.cqeOffset = params.CQ.CQEs
	ok = true
	return w, nil
}

func fits(b []byte, offset, size uint32) bool {
	return offset%4 == 0 && uint64(offset)+uint64(size) <= uint64(len(b))
}

func word(b []byte, offset uint32) *uint32 {
	return (*uint32)(unsafe.Pointer(&b[offset]))
}

func (w *Writer) Write(p []byte) (int, error) {
	if w == nil || w.closed {
		return 0, os.ErrClosed
	}
	if len(p) == 0 {
		return 0, nil
	}
	if uint64(len(p)) > uint64(^uint32(0)) {
		return 0, errors.New("io_uring write exceeds uint32 size")
	}
	tail := atomic.LoadUint32(w.sqTail)
	if tail-atomic.LoadUint32(w.sqHead) >= atomic.LoadUint32(w.sqEntries) {
		return 0, errors.New("io_uring submission queue full")
	}
	index := tail & atomic.LoadUint32(w.sqMask)
	entry := (*sqEntry)(unsafe.Pointer(&w.sqes[uint64(index)*uint64(unsafe.Sizeof(sqEntry{}))]))
	*entry = sqEntry{
		Opcode:   ioUringOpWrite,
		FD:       int32(w.fileFD),
		Offset:   ^uint64(0),
		Address:  uint64(uintptr(unsafe.Pointer(&p[0]))),
		Length:   uint32(len(p)),
		UserData: 1,
	}
	*(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(w.sqArray)) + uintptr(index)*4)) = index
	atomic.StoreUint32(w.sqTail, tail+1)

	// Retry an interrupted enter without creating another SQE; never switch
	// to write(2) after submission, as completion may already be in progress.
	for {
		n, _, errno := syscall.Syscall6(ioUringEnter, uintptr(w.ringFD), 1, 0, 0, 0, 0)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			runtime.KeepAlive(p)
			return 0, fmt.Errorf("io_uring submit: %w", errno)
		}
		if n == 1 {
			break
		}
	}
	for {
		head := atomic.LoadUint32(w.cqHead)
		if head != atomic.LoadUint32(w.cqTail) {
			cqe := (*cqEntry)(unsafe.Pointer(&w.cqRing[uint64(w.cqeOffset)+uint64(head&atomic.LoadUint32(w.cqMask))*uint64(unsafe.Sizeof(cqEntry{}))]))
			result, userData := cqe.Result, cqe.UserData
			atomic.StoreUint32(w.cqHead, head+1)
			runtime.KeepAlive(p)
			if userData != 1 {
				return 0, errors.New("unexpected io_uring completion identifier")
			}
			if result < 0 {
				return 0, fmt.Errorf("io_uring completion: %w", syscall.Errno(-result))
			}
			if int(result) != len(p) {
				return int(result), io.ErrShortWrite
			}
			return len(p), nil
		}
		_, _, errno := syscall.Syscall6(ioUringEnter, uintptr(w.ringFD), 0, 1, ioUringGetEvents, 0, 0)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			runtime.KeepAlive(p)
			return 0, fmt.Errorf("io_uring wait: %w", errno)
		}
	}
}

// Close unmaps the ring and closes the ring FD, leaving the log file open.
func (w *Writer) Close() error {
	if w == nil || w.closed {
		return nil
	}
	w.closed = true
	var err error
	if len(w.sqes) != 0 {
		err = errors.Join(err, syscall.Munmap(w.sqes))
	}
	if len(w.cqRing) != 0 && (len(w.sqRing) == 0 || &w.cqRing[0] != &w.sqRing[0]) {
		err = errors.Join(err, syscall.Munmap(w.cqRing))
	}
	if len(w.sqRing) != 0 {
		err = errors.Join(err, syscall.Munmap(w.sqRing))
	}
	return errors.Join(err, syscall.Close(w.ringFD))
}
