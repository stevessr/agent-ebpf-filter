package app

import (
	"os"
	"strings"
	"testing"
)

func ioCorrelationBlock(t *testing.T, src, name, side string) string {
	t.Helper()
	marker := "int tracepoint__syscalls__sys_" + side + "_" + name + "("
	start := strings.Index(src, marker)
	if start < 0 {
		t.Fatalf("missing %s %s handler", side, name)
	}
	end := strings.Index(src[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("missing end of %s %s handler", side, name)
	}
	return src[start : start+end]
}

func TestCompactIOCorrelationSourceContract(t *testing.T) {
	commonBytes, err := os.ReadFile("../ebpf/agent_tracker_common.h")
	if err != nil {
		t.Fatal(err)
	}
	syscallBytes, err := os.ReadFile("../ebpf/agent_tracker_syscalls.h")
	if err != nil {
		t.Fatal(err)
	}
	common := string(commonBytes)
	syscalls := string(syscallBytes)

	for _, needle := range []string{
		"_Static_assert(sizeof(struct exit_meta) == 88",
		"_Static_assert(sizeof(struct exit_io_meta) == 64",
		"__uint(max_entries, 6144);",
		"} exit_io_ctx SEC(\".maps\");",
		"__uint(max_entries, 4096);",
		"store_exit_io_meta",
		"consume_exit_io_meta",
		"fill_from_exit_io_meta",
	} {
		if !strings.Contains(common, needle) {
			t.Fatalf("missing compact I/O contract %q", needle)
		}
	}

	for _, name := range []string{"read", "write", "readv", "writev"} {
		enter := ioCorrelationBlock(t, syscalls, name, "enter")
		exit := ioCorrelationBlock(t, syscalls, name, "exit")
		if !strings.Contains(enter, "struct exit_io_meta") ||
			!strings.Contains(enter, "store_exit_io_meta") ||
			strings.Contains(enter, "store_exit_meta(") {
			t.Fatalf("%s enter does not exclusively use exit_io_ctx", name)
		}
		if !strings.Contains(exit, "struct exit_io_meta") ||
			!strings.Contains(exit, "consume_exit_io_meta") ||
			!strings.Contains(exit, "fill_from_exit_io_meta") ||
			strings.Contains(exit, "consume_exit_meta(") {
			t.Fatalf("%s exit does not exclusively use exit_io_ctx", name)
		}
	}
}
