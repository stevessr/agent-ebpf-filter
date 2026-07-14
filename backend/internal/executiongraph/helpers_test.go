package executiongraph

import (
	"testing"
	"time"
)

func TestParseIntervalBoundsNumericInputWithoutOverflow(t *testing.T) {
	tests := []struct {
		raw  string
		want time.Duration
	}{
		{raw: "", want: 1500 * time.Millisecond},
		{raw: "1", want: 500 * time.Millisecond},
		{raw: "1500", want: 1500 * time.Millisecond},
		{raw: "30001", want: 30 * time.Second},
		{raw: "9223372036854775807", want: 30 * time.Second},
		{raw: "2s", want: 2 * time.Second},
		{raw: "invalid", want: 1500 * time.Millisecond},
	}
	for _, test := range tests {
		if got := ParseInterval(test.raw); got != test.want {
			t.Errorf("ParseInterval(%q) = %s, want %s", test.raw, got, test.want)
		}
	}
}
