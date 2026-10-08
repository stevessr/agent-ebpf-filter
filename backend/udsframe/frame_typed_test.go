package udsframe

import (
	"bytes"
	"testing"
)

func TestWriteTypedRoundTrip(t *testing.T) {
	var stream bytes.Buffer
	if err := WriteTyped(&stream, 7, []byte("native-payload")); err != nil {
		t.Fatal(err)
	}
	frame, err := Read(&stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) != len("native-payload")+1 {
		t.Fatalf("frame length = %d", len(frame))
	}
	if frame[0] != 7 {
		t.Fatalf("kind = %d", frame[0])
	}
	if got := string(frame[1:]); got != "native-payload" {
		t.Fatalf("payload = %q", got)
	}
}
