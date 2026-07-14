package wsstream

import (
	"errors"
	"testing"
	"time"
)

type testMessageWriter struct {
	deadline     time.Time
	messageType  int
	payload      []byte
	deadlineErr  error
	writeErr     error
	writeInvoked bool
}

func (writer *testMessageWriter) SetWriteDeadline(deadline time.Time) error {
	writer.deadline = deadline
	return writer.deadlineErr
}

func (writer *testMessageWriter) WriteMessage(messageType int, payload []byte) error {
	writer.writeInvoked = true
	writer.messageType = messageType
	writer.payload = append([]byte(nil), payload...)
	return writer.writeErr
}

type testJSONWriter struct {
	testMessageWriter
	value any
}

func (writer *testJSONWriter) WriteJSON(value any) error {
	writer.writeInvoked = true
	writer.value = value
	return writer.writeErr
}

func TestWriteMessageSetsDeadlineAndPropagatesErrors(t *testing.T) {
	started := time.Now()
	writer := &testMessageWriter{}
	if err := WriteMessage(writer, 2, []byte("payload")); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	if !writer.writeInvoked || writer.messageType != 2 || string(writer.payload) != "payload" {
		t.Fatalf("writer state = %#v", writer)
	}
	if writer.deadline.Before(started.Add(DefaultWriteTimeout-time.Second)) || writer.deadline.After(time.Now().Add(DefaultWriteTimeout+time.Second)) {
		t.Fatalf("write deadline = %s, want approximately now + %s", writer.deadline, DefaultWriteTimeout)
	}

	deadlineErr := errors.New("deadline failed")
	writer = &testMessageWriter{deadlineErr: deadlineErr}
	if err := WriteMessage(writer, 1, nil); !errors.Is(err, deadlineErr) || writer.writeInvoked {
		t.Fatalf("deadline failure = %v, writeInvoked=%v", err, writer.writeInvoked)
	}

	writeErr := errors.New("write failed")
	writer = &testMessageWriter{writeErr: writeErr}
	if err := WriteMessage(writer, 1, nil); !errors.Is(err, writeErr) {
		t.Fatalf("write failure = %v, want %v", err, writeErr)
	}
}

func TestWriteJSONSetsDeadline(t *testing.T) {
	writer := &testJSONWriter{}
	value := map[string]string{"status": "ok"}
	if err := WriteJSON(writer, value); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	if !writer.writeInvoked || writer.deadline.IsZero() || writer.value == nil {
		t.Fatalf("writer state = %#v", writer)
	}
}

func TestIntervalMillisecondsBoundsInput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{name: "invalid uses fallback", raw: "invalid", want: 2 * time.Second},
		{name: "empty uses fallback", raw: "", want: 2 * time.Second},
		{name: "zero uses minimum", raw: "0", want: MinStreamInterval},
		{name: "negative uses minimum", raw: "-1", want: MinStreamInterval},
		{name: "below minimum", raw: "10", want: MinStreamInterval},
		{name: "valid", raw: "1500", want: 1500 * time.Millisecond},
		{name: "above maximum", raw: "120000", want: MaxStreamInterval},
		{name: "overflow", raw: "9223372036854775807", want: MaxStreamInterval},
		{name: "parse overflow uses fallback", raw: "92233720368547758070", want: 2 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IntervalMilliseconds(test.raw, 2*time.Second, MinStreamInterval, MaxStreamInterval); got != test.want {
				t.Fatalf("IntervalMilliseconds(%q) = %s, want %s", test.raw, got, test.want)
			}
		})
	}
	if got := IntervalMilliseconds("1", time.Duration(1<<63-1), time.Second, 500*time.Millisecond); got != time.Second {
		t.Fatalf("invalid bounds result = %s, want 1s", got)
	}
}
