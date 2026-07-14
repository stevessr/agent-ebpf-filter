package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	hardwareDiscoveryTimeout        = 3 * time.Second
	hardwareDiscoveryMaxTimeout     = 10 * time.Second
	hardwareDiscoveryMaxOutputBytes = 1 << 20
	hardwareProcessWaitDelay        = time.Second
	microphoneDeviceMaxBytes        = 256
	hardwareCommandConcurrency      = 4
	microphoneCaptureConcurrency    = 4
	hardwareDeviceListLimit         = 256
	hardwareDeviceLabelMaxBytes     = 512
)

var (
	errHardwareCommandOutputTooLarge = errors.New("hardware command output exceeds the size limit")
	alsaMicrophoneDevicePattern      = regexp.MustCompile(`^hw:[0-9]+,[0-9]+$`)
	pulseMicrophoneDevicePattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]*$`)
	hardwareCommandSlots             = make(chan struct{}, hardwareCommandConcurrency)
	microphoneCaptureSlots           = make(chan struct{}, microphoneCaptureConcurrency)
)

func normalizeMicrophoneDeviceName(raw string) (string, error) {
	deviceName := strings.TrimSpace(raw)
	if deviceName == "" || deviceName == "default" {
		return "default", nil
	}
	if len(deviceName) > microphoneDeviceMaxBytes {
		return "", errors.New("microphone device name is too long")
	}
	if alsaMicrophoneDevicePattern.MatchString(deviceName) || pulseMicrophoneDevicePattern.MatchString(deviceName) {
		return deviceName, nil
	}
	return "", fmt.Errorf("invalid microphone device %q", raw)
}

func runBoundedHardwareCommand(parent context.Context, timeout time.Duration, maxOutput int64, name string, args ...string) ([]byte, error) {
	if parent == nil {
		parent = context.Background()
	}
	if timeout <= 0 {
		timeout = hardwareDiscoveryTimeout
	} else if timeout > hardwareDiscoveryMaxTimeout {
		timeout = hardwareDiscoveryMaxTimeout
	}
	if maxOutput <= 0 || maxOutput > hardwareDiscoveryMaxOutputBytes {
		maxOutput = hardwareDiscoveryMaxOutputBytes
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	select {
	case hardwareCommandSlots <- struct{}{}:
		defer func() { <-hardwareCommandSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = hardwareProcessWaitDelay
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open %s output: %w", name, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	payload, readErr := io.ReadAll(io.LimitReader(stdout, maxOutput+1))
	if int64(len(payload)) > maxOutput {
		_ = stdout.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return nil, errHardwareCommandOutputTooLarge
	}
	if readErr != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("read %s output: %w", name, readErr)
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if waitErr != nil {
		return nil, fmt.Errorf("run %s: %w", name, waitErr)
	}
	return payload, nil
}
