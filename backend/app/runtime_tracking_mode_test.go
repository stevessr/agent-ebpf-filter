package app

import "testing"

func TestTrackingModeFlags(t *testing.T) {
	tests := []struct {
		exact, prefix bool
		want          uint32
	}{
		{false, false, 0},
		{true, false, trackingModePathExact},
		{false, true, trackingModePathPrefix},
		{true, true, trackingModePathExact | trackingModePathPrefix},
	}
	for _, tt := range tests {
		if got := trackingModeFlags(tt.exact, tt.prefix); got != tt.want {
			t.Fatalf("trackingModeFlags(%v,%v)=%d want %d", tt.exact, tt.prefix, got, tt.want)
		}
	}
}
