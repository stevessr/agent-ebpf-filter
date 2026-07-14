package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeMicrophoneDeviceName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default empty", want: "default"},
		{name: "default explicit", input: " default ", want: "default"},
		{name: "alsa", input: "hw:12,3", want: "hw:12,3"},
		{name: "pulse", input: "alsa_input.pci-0000_00_1f.3.analog-stereo", want: "alsa_input.pci-0000_00_1f.3.analog-stereo"},
		{name: "whitespace", input: "pulse source", wantErr: true},
		{name: "argument-like", input: "--help", wantErr: true},
		{name: "path", input: "/tmp/device", wantErr: true},
		{name: "too long", input: strings.Repeat("a", microphoneDeviceMaxBytes+1), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeMicrophoneDeviceName(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("normalizeMicrophoneDeviceName(%q) = %q, want error", test.input, got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("normalizeMicrophoneDeviceName(%q) = %q, %v; want %q", test.input, got, err, test.want)
			}
		})
	}
}

func TestRunBoundedHardwareCommand(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		payload, err := runBoundedHardwareCommand(context.Background(), time.Second, 64, "sh", "-c", "printf hello")
		if err != nil || string(payload) != "hello" {
			t.Fatalf("command output = %q, %v", payload, err)
		}
	})

	t.Run("output limit", func(t *testing.T) {
		_, err := runBoundedHardwareCommand(context.Background(), time.Second, 16, "sh", "-c", "printf 12345678901234567")
		if !errors.Is(err, errHardwareCommandOutputTooLarge) {
			t.Fatalf("command error = %v, want output limit", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		started := time.Now()
		_, err := runBoundedHardwareCommand(context.Background(), 25*time.Millisecond, 64, "sh", "-c", "exec sleep 5")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("command error = %v, want deadline exceeded", err)
		}
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("timed-out command returned after %s", elapsed)
		}
	})
}
