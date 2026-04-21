package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"agent-ebpf-filter/pb"
	"google.golang.org/protobuf/proto"
)

const udsPath = "/tmp/agent-ebpf.sock"

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: agent-wrapper <command> [args...]")
		os.Exit(1)
	}

	cmdName := os.Args[1]
	rawArgs := os.Args[2:]

	// Simple information cleaning: trim whitespace and remove empty args
	cmdArgs := []string{}
	for _, arg := range rawArgs {
		trimmed := strings.TrimSpace(arg)
		if trimmed != "" {
			cmdArgs = append(cmdArgs, trimmed)
		}
	}

	fmt.Printf("[DEBUG] Dialing %s...\n", udsPath)
	conn, err := net.Dial("unix", udsPath)
	if err == nil {
		defer conn.Close()

		req := &pb.WrapperRequest{
			Pid:  uint32(os.Getpid()),
			Comm: cmdName,
			Args: cmdArgs,
			User: os.Getenv("USER"),
		}

		data, _ := proto.Marshal(req)
		fmt.Printf("[DEBUG] Writing %d bytes to socket...\n", len(data))
		_, err = conn.Write(data)
		if err == nil {
			fmt.Printf("[DEBUG] Waiting for response...\n")
			buf := make([]byte, 4096)
			n, err := conn.Read(buf)
			if err == nil {
				fmt.Printf("[DEBUG] Read %d bytes\n", n)
				resp := &pb.WrapperResponse{}
				if err := proto.Unmarshal(buf[:n], resp); err == nil {
					handleDecision(resp, &cmdName, &cmdArgs)
				} else {
					fmt.Printf("[DEBUG] Unmarshal error: %v\n", err)
				}
			} else {
				fmt.Printf("[DEBUG] Read error: %v\n", err)
			}
		} else {
			fmt.Printf("[DEBUG] Write error: %v\n", err)
		}
	} else {
		fmt.Printf("[DEBUG] Dial error: %v\n", err)
	}

	fmt.Printf("[DEBUG] Executing %s %v\n", cmdName, cmdArgs)
	execute(cmdName, cmdArgs)
}

func handleDecision(resp *pb.WrapperResponse, name *string, args *[]string) {
	switch resp.Action {
	case pb.WrapperResponse_BLOCK:
		fmt.Printf("❌ Execution Blocked: %s\n", resp.Message)
		os.Exit(1)
	case pb.WrapperResponse_ALERT:
		fmt.Printf("⚠️  Security Alert: %s\n", resp.Message)
	case pb.WrapperResponse_REWRITE:
		if len(resp.RewrittenArgs) > 0 {
			*name = resp.RewrittenArgs[0]
			*args = resp.RewrittenArgs[1:]
		}
	}
}

func execute(name string, args []string) {
	path, err := exec.LookPath(name)
	if err != nil {
		fmt.Printf("Error: command not found: %s\n", name)
		os.Exit(127)
	}
	fmt.Printf("[DEBUG] Found path: %s\n", path)

	env := os.Environ()
	fullArgs := append([]string{name}, args...)
	err = syscall.Exec(path, fullArgs, env)
	if err != nil {
		log.Fatalf("Execution failed: %v", err)
	}
}
