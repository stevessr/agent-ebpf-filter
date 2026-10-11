//go:build linux

package componentshm

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func readerMirror(t *testing.T, source *Ring) *Ring {
	t.Helper()
	mem, wake, err := source.Files()
	if err != nil { t.Fatal(err) }
	memdup, err := unix.Dup(int(mem.Fd()))
	if err != nil { t.Fatal(err) }
	wakedup, err := unix.Dup(int(wake.Fd()))
	if err != nil {
		_ = unix.Close(memdup)
		t.Fatal(err)
	}
	reader, err := OpenReader(
		os.NewFile(uintptr(memdup), "test-memfd"),
		os.NewFile(uintptr(wakedup), "test-eventfd"),
	)
	if err != nil {
		_ = unix.Close(memdup)
		_ = unix.Close(wakedup)
		t.Fatal(err)
	}
	t.Cleanup(func(){ _ = reader.Close() })
	return reader
}

func TestRingCapacityWrapBackpressureAndOwnedRead(t *testing.T) {
	writer, err := Create(2, 65)
	if err != nil { t.Fatal(err) }
	defer writer.Close()
	reader := readerMirror(t, writer)
	if writer.MaxPayload() != 65 || writer.Capacity() != 2 {
		t.Fatalf("capacity config ignored: %d %d", writer.MaxPayload(), writer.Capacity())
	}
	payload := []byte(strings.Repeat("A", 65))
	if err := writer.TryWrite(payload); err != nil { t.Fatal(err) }
	if err := writer.TryWrite([]byte("second")); err != nil { t.Fatal(err) }
	if err := writer.TryWrite([]byte("rejected")); !errors.Is(err, ErrRingFull) {
		t.Fatalf("full ring returned %v", err)
	}
	stats := writer.Stats()
	if stats.Queued != 2 || stats.Rejected != 1 || stats.Published != 2 {
		t.Fatalf("broken backpressure counters: %+v", stats)
	}
	first, err := reader.ReadCopy()
	if err != nil { t.Fatal(err) }
	if len(first) != 65 { t.Fatalf("lost final slot byte: %d", len(first)) }
	first[0] = 'Z'
	if err := writer.TryWrite([]byte("wrapped")); err != nil { t.Fatal(err) }
	second, err := reader.ReadCopy()
	if err != nil { t.Fatal(err) }
	third, err := reader.ReadCopy()
	if err != nil { t.Fatal(err) }
	if string(second) != "second" || string(third) != "wrapped" || len(first) != 65 {
		t.Fatalf("out of order or retained borrowed memory: %q %q %q", first, second, third)
	}
	if _, err := reader.ReadCopy(); !errors.Is(err, ErrNoRecord) {
		t.Fatalf("empty ring returned %v", err)
	}
	if err := writer.Finish(); err != nil { t.Fatal(err) }
	if _, err := reader.ReadCopy(); !errors.Is(err, ErrRingClosed) {
		t.Fatalf("finished ring returned %v", err)
	}
	if err := writer.TryWrite([]byte("after")); !errors.Is(err, ErrRingClosed) {
		t.Fatalf("write after finish returned %v", err)
	}
}

func TestConfigAndInvalidReader(t *testing.T) {
	for _, tc := range [][2]int{{1,64},{3,64},{4097,64},{2,63},{2,16385}} {
		if ring,err := Create(tc[0],tc[1]); !errors.Is(err, ErrInvalidRing) {
			if ring != nil { _ = ring.Close() }
			t.Fatalf("config %v accepted: %v",tc,err)
		}
	}
	if _,err:=OpenReader(nil,nil); !errors.Is(err,ErrInvalidRing) { t.Fatal(err) }
	writer, err:=Create(2,64)
	if err != nil { t.Fatal(err) }
	defer writer.Close()
	reader:=readerMirror(t,writer)
	if _,_,err:=reader.Files(); !errors.Is(err,ErrInvalidRole) { t.Fatal(err) }
	if err:=reader.TryWrite([]byte("wrong-role")); !errors.Is(err,ErrInvalidRole) { t.Fatal(err) }
	if _,err:=writer.ReadCopy(); !errors.Is(err,ErrInvalidRole) { t.Fatal(err) }
	if err:=writer.TryWrite(nil); !errors.Is(err,ErrTooLarge) { t.Fatal(err) }
	if err:=writer.TryWrite(make([]byte,65)); !errors.Is(err,ErrTooLarge) { t.Fatal(err) }
	ctx,cancel:=context.WithTimeout(context.Background(),30*time.Millisecond)
	defer cancel()
	if err:=reader.WaitReady(ctx); !errors.Is(err,context.DeadlineExceeded) { t.Fatalf("wait timeout: %v",err) }
	if err:=writer.Finish(); err!=nil { t.Fatal(err) }
	if err:=reader.WaitReady(context.Background()); !errors.Is(err,ErrRingClosed) { t.Fatal(err) }
}

func TestConsumerRejectsTamperedHeaderAndRecord(t *testing.T) {
	w,err:=Create(4,64)
	if err!=nil { t.Fatal(err) }
	defer w.Close()
	// Missing seals / wrong descriptor rejected without assuming a filesystem.
	dummy,err:=os.CreateTemp(t.TempDir(),"invalid-file")
	if err!=nil { t.Fatal(err) }
	defer dummy.Close()
	_,wake,_:=w.Files()
	if _,err:=OpenReader(dummy,wake); !errors.Is(err,ErrInvalidRing) { t.Fatalf("unsealed fd accepted: %v",err) }
	r:=readerMirror(t,w)
	if err:=w.TryWrite([]byte("ok")); err!=nil { t.Fatal(err) }
	binary.LittleEndian.PutUint32(w.memory[HeaderBytes:HeaderBytes+4],65)
	if _,err:=r.ReadCopy(); !errors.Is(err,ErrInvalidRing) { t.Fatalf("corrupt length accepted: %v",err) }
}

func TestReaderChildProcess(t *testing.T) {
	if os.Getenv("COMPONENT_SHM_TEST_CHILD")!="1" { return }
	mem:=os.NewFile(3,"inherited-ring-memfd")
	wake:=os.NewFile(4,"inherited-eventfd")
	r,err:=OpenReader(mem,wake)
	if err!=nil {
		fmt.Fprintf(os.Stderr,"open inherited ring: %v\n",err)
		os.Exit(8)
	}
	defer r.Close()
	ctx,cancel:=context.WithTimeout(context.Background(),7*time.Second)
	defer cancel()
	count:=0
	for {
		err=r.WaitReady(ctx)
		if errors.Is(err,ErrRingClosed) { break }
		if err!=nil { fmt.Fprintf(os.Stderr,"wait: %v\n",err);os.Exit(9) }
		for {
			record,err:=r.ReadCopy()
			if errors.Is(err,ErrNoRecord) { break }
			if errors.Is(err,ErrRingClosed) { break }
			if err!=nil { fmt.Fprintf(os.Stderr,"record: %v\n",err);os.Exit(10) }
			if string(record)!=strconv.Itoa(count) {
				fmt.Fprintf(os.Stderr,"sequence mismatch: got %q want %d\n",record,count)
				os.Exit(11)
			}
			count++
		}
	}
	if count!=250 { fmt.Fprintf(os.Stderr,"wrong count %d\n",count);os.Exit(12) }
	fmt.Fprintf(os.Stdout,"verified=%d\n",count)
}

func TestRealProcessInheritedFDsAndBatchedWakeups(t *testing.T) {
	w,err:=Create(16,64)
	if err!=nil { t.Fatal(err) }
	defer w.Close()
	mem,wake,err:=w.Files()
	if err!=nil { t.Fatal(err) }
	exe,err:=os.Executable()
	if err!=nil { t.Fatal(err) }
	ctx,cancel:=context.WithTimeout(context.Background(),8*time.Second)
	defer cancel()
	cmd:=exec.CommandContext(ctx,exe,"-test.run=^TestReaderChildProcess$")
	cmd.Env=append(os.Environ(),"COMPONENT_SHM_TEST_CHILD=1")
	cmd.ExtraFiles=[]*os.File{mem,wake}
	stdout,err:=cmd.StdoutPipe()
	if err!=nil { t.Fatal(err) }
	cmd.Stderr=os.Stderr
	if err:=cmd.Start(); err!=nil { t.Fatal(err) }
	var rejected int
	for i:=0;i<250;i++ {
		payload:=[]byte(strconv.Itoa(i))
		for {
			err=w.TryWrite(payload)
			if err==nil { break }
			if !errors.Is(err,ErrRingFull) { t.Fatal(err) }
			rejected++
			if rejected>200000 { t.Fatal("consumer made no progress") }
			// The initial wakeup is necessary when the first buffer saturates.
			if err:=w.Notify();err!=nil { t.Fatal(err) }
			time.Sleep(100*time.Microsecond)
		}
		if i%8==7 {
			if err:=w.Notify();err!=nil { t.Fatal(err) }
		}
	}
	if err:=w.Finish();err!=nil { t.Fatal(err) }
	out,err:=io.ReadAll(stdout)
	if err!=nil { t.Fatal(err) }
	if err:=cmd.Wait();err!=nil { t.Fatalf("inherited FD subprocess: %v",err) }
	if !strings.Contains(string(out),"verified=250") {
		t.Fatalf("unexpected subprocess output: %q",out)
	}
	stats:=w.Stats()
	if stats.Published!=250 || stats.Consumed!=250 || stats.Queued!=0 {
		t.Fatalf("process-shared cursor mismatch: %+v",stats)
	}
}

func BenchmarkRingBatch64(b *testing.B) {
	w,err:=Create(128,128)
	if err!=nil { b.Fatal(err) }
	defer w.Close()
	// Reader can immediately consume in the same process for transfer
	// microbenchmarking only; this is not an end-to-end kernel benchmark.
	mem,wake,_:=w.Files()
	md,err:=unix.Dup(int(mem.Fd()))
	if err!=nil { b.Fatal(err) }
	wd,err:=unix.Dup(int(wake.Fd()))
	if err!=nil { b.Fatal(err) }
	r,err:=OpenReader(os.NewFile(uintptr(md),"reader-memfd"),os.NewFile(uintptr(wd),"reader-eventfd"))
	if err!=nil { b.Fatal(err) }
	defer r.Close()
	record:=[]byte("bounded telemetry projection")
	b.ResetTimer()
	for i:=0;i<b.N;i++ {
		for j:=0;j<64;j++ {
			if err:=w.TryWrite(record);err!=nil { b.Fatal(err) }
		}
		if err:=w.Notify();err!=nil { b.Fatal(err) }
		for j:=0;j<64;j++ {
			if _,err:=r.ReadCopy();err!=nil { b.Fatal(err) }
		}
	}
}
