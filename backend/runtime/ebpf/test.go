package main

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsMissingTracepointError(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("attach syscalls/sys_enter_lstat: reading file \"/sys/kernel/tracing/events/syscalls/sys_enter_lstat/id\": open /sys/kernel/tracing/events/syscalls/sys_enter_lstat/id: no such file or directory")
	if !isMissingTracepointError(err) {
		t.Fatalf("expected missing tracepoint error to be detected")
	}

	if isMissingTracepointError(errors.New("permission denied")) {
		t.Fatalf("unexpectedly classified a non-not-found error as missing tracepoint")
	}
}
