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
