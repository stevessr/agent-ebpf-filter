package app

import (
	"errors"
	"fmt"
	"testing"
)

// ---- moved from backend/zz_merged_backend_test.go section runtimeebpf_test.go ----

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
