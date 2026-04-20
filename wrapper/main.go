package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
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
	cmdArgs := os.Args[2:]

	// Try to connect to the backend UDS
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
		_, err = conn.Write(data)
		if err == nil {
			buf := make([]byte, 4096)
			n, err := conn.Read(buf)
			if err == nil {
				resp := &pb.WrapperResponse{}
				if err := proto.Unmarshal(buf[:n], resp); err == nil {
					handleDecision(resp, &cmdName, &cmdArgs)
				}
			}
		}
	} else {
		// If backend is not available, just allow but maybe log it
		// fmt.Printf("Warning: Could not connect to backend socket: %v\n", err)
	}

	// Execute the command
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
			// fmt.Printf("🔄 Command Rewritten\n")
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

	// Use syscall.Exec to replace the current process (proper wrapper behavior)
	env := os.Environ()
	fullArgs := append([]string{name}, args...)
	err = syscall.Exec(path, fullArgs, env)
	if err != nil {
		log.Fatalf("Execution failed: %v", err)
	}
}
