package udsframe_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"agent-ebpf-filter/pb"
	"agent-ebpf-filter/udsframe"
	"google.golang.org/protobuf/proto"
)

type limitedWriter struct {
	w     io.Writer
	limit int
}

func (w limitedWriter) Write(payload []byte) (int, error) {
	if len(payload) > w.limit {
		payload = payload[:w.limit]
	}
	return w.w.Write(payload)
}

type limitedReader struct {
	r     io.Reader
	limit int
}

func (r limitedReader) Read(payload []byte) (int, error) {
	if len(payload) > r.limit {
		payload = payload[:r.limit]
	}
	return r.r.Read(payload)
}

func TestFrameRoundTripHandlesFragmentedIOAndSequentialMessages(t *testing.T) {
	t.Parallel()

	messages := []proto.Message{
		&pb.WrapperRequest{Pid: 42, Comm: "codex", Args: []string{"exec", "a value larger than the old read assumptions"}},
		&pb.WrapperResponse{Action: pb.WrapperResponse_REWRITE, RewrittenArgs: []string{"safe", "value"}},
	}
	var stream bytes.Buffer
	writer := limitedWriter{w: &stream, limit: 3}
	for _, message := range messages {
		payload, err := proto.Marshal(message)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if err := udsframe.Write(writer, payload); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	reader := limitedReader{r: &stream, limit: 2}
	requestPayload, err := udsframe.Read(reader)
	if err != nil {
		t.Fatalf("Read(request) error = %v", err)
	}
	var request pb.WrapperRequest
	if err := proto.Unmarshal(requestPayload, &request); err != nil {
		t.Fatalf("Unmarshal(request) error = %v", err)
	}
	if request.Pid != 42 || request.Comm != "codex" || len(request.Args) != 2 {
		t.Fatalf("request pid=%d comm=%q args=%d", request.Pid, request.Comm, len(request.Args))
	}

	responsePayload, err := udsframe.Read(reader)
	if err != nil {
		t.Fatalf("Read(response) error = %v", err)
	}
	var response pb.WrapperResponse
	if err := proto.Unmarshal(responsePayload, &response); err != nil {
		t.Fatalf("Unmarshal(response) error = %v", err)
	}
	if response.Action != pb.WrapperResponse_REWRITE || len(response.RewrittenArgs) != 2 {
		t.Fatalf("response action=%s rewrittenArgs=%d", response.Action, len(response.RewrittenArgs))
	}
}

func TestFrameRejectsInvalidPayloadSizes(t *testing.T) {
	t.Parallel()

	if err := udsframe.Write(io.Discard, nil); !errors.Is(err, udsframe.ErrInvalidPayloadSize) {
		t.Fatalf("Write(nil) error = %v", err)
	}
	oversized := make([]byte, udsframe.MaxPayloadSize+1)
	if err := udsframe.Write(io.Discard, oversized); !errors.Is(err, udsframe.ErrInvalidPayloadSize) {
		t.Fatalf("Write(oversized) error = %v", err)
	}

	invalidHeader := []byte{0xff, 0xff, 0xff, 0xff}
	if _, err := udsframe.Read(bytes.NewReader(invalidHeader)); !errors.Is(err, udsframe.ErrInvalidPayloadSize) {
		t.Fatalf("Read(oversized header) error = %v", err)
	}
}
