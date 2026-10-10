// Package eventqueue is the non-blocking admission boundary for events handed
// from collectors to a single broadcaster. It owns neither metrics nor events.
package eventqueue

import "strings"

// Result distinguishes successful ownership transfer from a dropped event.
type Result uint8

const (
	Accepted Result = iota
	NilItem
	QueueUnavailable
	QueueFull
)

// Offer transfers ownership only if the send succeeds. Producers must not
// access or mutate item after Accepted. The consumer owns channel lifecycle:
// all producers must have stopped before the channel is closed.
func Offer[T any](queue chan<- *T, item *T) Result {
	if item == nil {
		return NilItem
	}
	if queue == nil {
		return QueueUnavailable
	}
	select {
	case queue <- item:
		return Accepted
	default:
		return QueueFull
	}
}

// DropReason retains existing collector health metric labels.
func DropReason(source string, result Result) string {
	if result == Accepted {
		return ""
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "unknown"
	}
	switch result {
	case NilItem:
		return source + ":nil_event"
	case QueueUnavailable:
		return source + ":queue_unavailable"
	case QueueFull:
		return source + ":queue_full"
	default:
		return source + ":queue_unavailable"
	}
}
