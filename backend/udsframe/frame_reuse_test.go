package udsframe_test

import (
	"bytes"
	"net"
	"testing"

	"agent-ebpf-filter/pb"
	"agent-ebpf-filter/udsframe"
	"google.golang.org/protobuf/proto"
)

func TestReadLimitIntoReusesBufferWhenLargeEnough(t *testing.T) {
	t.Parallel()
	var stream bytes.Buffer
	for _, payload := range [][]byte{[]byte("first"), []byte("second frame that is longer")} {
		if err := udsframe.Write(&stream, payload); err != nil {
			t.Fatal(err)
		}
	}
	buf := make([]byte, 0, 8)
	first, err := udsframe.ReadLimitInto(&stream, buf, 64)
	if err != nil || string(first) != "first" {
		t.Fatalf("first = %q, %v", first, err)
	}
	if &first[0] != &buf[:1][0] {
		t.Fatal("small frame did not reuse the caller's buffer")
	}
	second, err := udsframe.ReadLimitInto(&stream, first, 64)
	if err != nil || string(second) != "second frame that is longer" {
		t.Fatalf("second = %q, %v", second, err)
	}
	if cap(second) < len(second) || (cap(buf) >= len(second)) {
		t.Fatal("larger frame should have allocated a new buffer")
	}
	if _, err := udsframe.ReadLimitInto(&stream, nil, 0); err == nil {
		t.Fatal("invalid limit accepted")
	}
}

// A reused frame buffer must be safe to overwrite once proto.Unmarshal has
// returned: protobuf copies the field data it retains.
func TestUnmarshalledRequestDoesNotAliasFrameBuffer(t *testing.T) {
	t.Parallel()
	payload, err := proto.Marshal(&pb.WrapperRequest{Pid: 7, Comm: "codex", Args: []string{"exec", "--danger"}})
	if err != nil {
		t.Fatal(err)
	}
	frame := append([]byte(nil), payload...)
	req := &pb.WrapperRequest{}
	if err := proto.Unmarshal(frame, req); err != nil {
		t.Fatal(err)
	}
	for i := range frame {
		frame[i] = 0xff
	}
	if req.GetComm() != "codex" || len(req.GetArgs()) != 2 || req.GetArgs()[1] != "--danger" {
		t.Fatalf("request aliased the overwritten frame buffer: %+v", req)
	}
}

func TestWriteGatherOverUnixSocket(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	payload := bytes.Repeat([]byte("agent-wrapper"), 1000)
	errCh := make(chan error, 1)
	go func() { errCh <- udsframe.Write(client, payload) }()
	got, err := udsframe.Read(server)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %d bytes", len(got))
	}

	// A real unix socket takes the writev path.
	listener, err := net.Listen("unix", t.TempDir()+"/frame.sock")
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	dialed, err := net.Dial("unix", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer dialed.Close()
	serverConn := <-accepted
	defer serverConn.Close()
	go func() { errCh <- udsframe.Write(dialed, payload) }()
	got, err = udsframe.Read(serverConn)
	if err != nil {
		t.Fatalf("Read(unix) error = %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Write(unix) error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("unix socket payload mismatch")
	}
}
