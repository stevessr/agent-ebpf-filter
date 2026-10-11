package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"runtime"
	"testing"
	"time"
)

func TestDoctorRootlessIsolatedSelfTest(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("SO_PEERCRED is Linux-only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := runDoctor(ctx)
	if err != nil {
		t.Fatalf("doctor failed: %v (%+v)", err, result)
	}
	if result.Scope != "isolated_local_self_test" || result.Status != "pass" {
		t.Fatalf("incorrect scope or result: %+v", result)
	}
	if len(result.Checks) != 5 {
		t.Fatalf("unexpected number of checks: %+v", result.Checks)
	}
	for _, check := range result.Checks {
		if check.Status != "pass" || check.Name == "" {
			t.Fatalf("failed check: %+v", check)
		}
	}
}

func TestDoctorReportsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := runDoctor(ctx)
	if err == nil || result.Status != "fail" || len(result.Checks) != 1 ||
		result.Checks[0].Name != "context" || result.Checks[0].Status != "fail" {
		t.Fatalf("cancelled run unexpectedly succeeded: result=%+v err=%v", result, err)
	}
}

func TestDoctorJSONOutputAndOptions(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("SO_PEERCRED is Linux-only")
	}
	var out bytes.Buffer
	var errorsOut bytes.Buffer
	if code := runCLI([]string{"--json", "--timeout=5s"}, &out, &errorsOut); code != 0 {
		t.Fatalf("doctor --json exited %d: %s", code, errorsOut.String())
	}
	var result doctorReport
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "pass" || result.Scope != "isolated_local_self_test" ||
		len(result.Checks) != 5 {
		t.Fatalf("invalid json report: %+v", result)
	}
	if strings.Contains(out.String(), "secret") || strings.Contains(out.String(), "doctor-event") {
		t.Fatal("json diagnostics should not disclose credentials or payload bytes")
	}
	for _, args := range [][]string{
		{"--timeout=0"}, {"--timeout=-1s"}, {"--unknown"},
		{"extra"}, {"--json", "unexpected"},
	} {
		out.Reset()
		errorsOut.Reset()
		if code := runCLI(args, &out, &errorsOut); code != 2 {
			t.Fatalf("invalid args %q returned %d", args, code)
		}
	}
}

func TestDoctorJSONFailureHasVisibleCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := runDoctor(ctx)
	if err == nil || result.Error == "" || len(result.Checks) != 1 ||
		result.Checks[0].Status != "fail" {
		t.Fatalf("failure was not preserved in JSON report: %+v %v", result, err)
	}
}
