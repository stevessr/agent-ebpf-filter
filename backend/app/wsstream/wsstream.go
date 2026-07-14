package wsstream

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultWriteTimeout = 5 * time.Second
	ControlReadLimit    = 64 << 10
	MinStreamInterval   = 500 * time.Millisecond
	MaxStreamInterval   = time.Minute
)

type MessageWriter interface {
	SetWriteDeadline(deadline time.Time) error
	WriteMessage(messageType int, data []byte) error
}

type JSONWriter interface {
	SetWriteDeadline(deadline time.Time) error
	WriteJSON(value any) error
}

func WriteMessage(writer MessageWriter, messageType int, payload []byte) error {
	if writer == nil {
		return errors.New("websocket message writer is nil")
	}
	if err := writer.SetWriteDeadline(time.Now().Add(DefaultWriteTimeout)); err != nil {
		return err
	}
	return writer.WriteMessage(messageType, payload)
}

func WriteJSON(writer JSONWriter, value any) error {
	if writer == nil {
		return errors.New("websocket JSON writer is nil")
	}
	if err := writer.SetWriteDeadline(time.Now().Add(DefaultWriteTimeout)); err != nil {
		return err
	}
	return writer.WriteJSON(value)
}

// IntervalMilliseconds parses a numeric millisecond query parameter and
// clamps it to a safe range. Invalid input uses the supplied fallback.
func IntervalMilliseconds(raw string, fallback, minimum, maximum time.Duration) time.Duration {
	if minimum <= 0 {
		minimum = time.Millisecond
	}
	if maximum < minimum {
		maximum = minimum
	}
	fallback = clampDuration(fallback, minimum, maximum)

	milliseconds, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return fallback
	}
	if milliseconds <= 0 {
		return minimum
	}
	maxMilliseconds := int64(maximum / time.Millisecond)
	if milliseconds > maxMilliseconds {
		return maximum
	}
	return clampDuration(time.Duration(milliseconds)*time.Millisecond, minimum, maximum)
}

func clampDuration(value, minimum, maximum time.Duration) time.Duration {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
