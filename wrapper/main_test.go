package main

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
	"agent-ebpf-filter/udsframe"
	"google.golang.org/protobuf/proto"
)

func TestExchangeWrapperDecisionUsesFramedProtocolForLargePayload(t *testing.T) {
	t.Parallel()

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	deadline := time.Now().Add(2 * time.Second)
	_ = server.SetDeadline(deadline)
	_ = client.SetDeadline(deadline)

	serverErr := make(chan error, 1)
	go func() {
		payload, err := udsframe.Read(server)
		if err != nil {
			serverErr <- err
			return
		}
		var request pb.WrapperRequest
		if err := proto.Unmarshal(payload, &request); err != nil {
			serverErr <- err
			return
		}
		if len(request.Args) != 1 || len(request.Args[0]) <= 4096 {
			serverErr <- fmt.Errorf("unexpected wrapper request: args=%d firstArgBytes=%d", len(request.Args), firstArgBytes(request.Args))
			return
		}
		responsePayload, err := proto.Marshal(&pb.WrapperResponse{
			Action:         pb.WrapperResponse_BLOCK,
			Classification: &pb.BehaviorClassification{PrimaryCategory: "destructive"},
			Message:        "blocked by compatibility test",
		})
		if err == nil {
			err = udsframe.Write(server, responsePayload)
		}
		serverErr <- err
	}()

	response, err := exchangeWrapperDecision(client, &pb.WrapperRequest{
		Pid:  42,
		Comm: "codex",
		Args: []string{strings.Repeat("x", 8192)},
	})
	if err != nil {
		t.Fatalf("exchangeWrapperDecision() error = %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server exchange error = %v", err)
	}
	if response.Action != pb.WrapperResponse_BLOCK || response.Message != "blocked by compatibility test" {
		t.Fatalf("response = %#v", response)
	}
}

func firstArgBytes(args []string) int {
	if len(args) == 0 {
		return 0
	}
	return len(args[0])
}


func TestPrepareCommandArgsPreservesDshAppArgumentsVerbatim(t *testing.T) {
	raw := []string{"headless", "  keep spacing  ", ""}
	got := prepareCommandArgs("dsh", raw, false)
	if len(got) != len(raw) || got[0] != "headless" || got[1] != "  keep spacing  " || got[2] != "" {
		t.Fatalf("prepareCommandArgs(dsh) = %#v, want %#v", got, raw)
	}
	got[1] = "changed"
	if raw[1] != "  keep spacing  " {
		t.Fatal("prepareCommandArgs must return an independent argv slice")
	}
}

func TestPrepareCommandArgsKeepsLegacySanitizationForOtherCommands(t *testing.T) {
	got := prepareCommandArgs("git", []string{" push ", "", " origin "}, false)
	want := []string{"push", "origin"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("prepareCommandArgs(git) = %#v, want %#v", got, want)
	}
}

func TestBuildArgvDigestPreservesExactArgumentBoundaries(t *testing.T) {
	plain := buildArgvDigest("dsh", []string{"headless", "task"})
	spaced := buildArgvDigest("dsh", []string{"headless", " task "})
	withEmpty := buildArgvDigest("dsh", []string{"headless", "task", ""})
	if plain == "" || spaced == "" || withEmpty == "" {
		t.Fatal("expected non-empty digests")
	}
	if plain == spaced || plain == withEmpty || spaced == withEmpty {
		t.Fatalf("exact argv variants collapsed to the same digest: plain=%s spaced=%s empty=%s", plain, spaced, withEmpty)
	}
}


func TestPrepareCommandArgsVerbatimModePreservesAnyCommand(t *testing.T) {
	raw := []string{"", "  keep  ", "--flag"}
	got := prepareCommandArgs("python", raw, true)
	if len(got) != len(raw) || got[0] != "" || got[1] != "  keep  " || got[2] != "--flag" {
		t.Fatalf("prepareCommandArgs(verbatim) = %#v, want %#v", got, raw)
	}
	got[1] = "changed"
	if raw[1] != "  keep  " {
		t.Fatal("verbatim argv must be copied")
	}
}

func TestDshPolicyProviderFailsClosedWhenBackendIsUnavailable(t *testing.T) {
	cases := []struct{
		sandbox string
		dshExec bool
		require bool
	}{
		{sandbox:"off", dshExec:false, require:false},
		{sandbox:"readonly", dshExec:false, require:true},
		{sandbox:"workspace", dshExec:false, require:true},
		{sandbox:"off", dshExec:true, require:true},
	}
	for _, tc := range cases {
		if got := requiresBackendPolicy(tc.sandbox, tc.dshExec); got != tc.require {
			t.Errorf("requiresBackendPolicy(%q, %v) = %v, want %v", tc.sandbox, tc.dshExec, got, tc.require)
		}
	}
}
