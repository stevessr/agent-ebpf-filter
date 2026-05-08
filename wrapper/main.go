package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

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
	conn, err := net.DialTimeout("unix", udsPath, 500*time.Millisecond)
	if err == nil {
		defer conn.Close()

		// Don't block forever on a stuck backend
		conn.SetDeadline(time.Now().Add(2 * time.Second))

		req := &pb.WrapperRequest{
			Pid:            uint32(os.Getpid()),
			Comm:           cmdName,
			Args:           cmdArgs,
			User:           os.Getenv("USER"),
			AgentRunId:     firstEnv("AGENT_EBPF_AGENT_RUN_ID", "AGENT_RUN_ID"),
			ConversationId: firstEnv("AGENT_EBPF_CONVERSATION_ID", "AGENT_CONVERSATION_ID"),
			TurnId:         firstEnv("AGENT_EBPF_TURN_ID", "AGENT_TURN_ID"),
			ToolCallId:     firstEnv("AGENT_EBPF_TOOL_CALL_ID", "AGENT_TOOL_CALL_ID"),
			ToolName:       firstEnv("AGENT_EBPF_TOOL_NAME", "AGENT_TOOL_NAME"),
			TraceId:        firstEnv("AGENT_EBPF_TRACE_ID", "TRACE_ID"),
			SpanId:         firstEnv("AGENT_EBPF_SPAN_ID", "SPAN_ID"),
			RootAgentPid:   parseEnvUint32("AGENT_EBPF_ROOT_AGENT_PID", "ROOT_AGENT_PID"),
			Decision:       strings.ToUpper(firstEnv("AGENT_EBPF_DECISION", "AGENT_DECISION")),
			RiskScore:      parseEnvFloat64("AGENT_EBPF_RISK_SCORE", "AGENT_RISK_SCORE"),
			ContainerId:    firstEnv("AGENT_EBPF_CONTAINER_ID", "CONTAINER_ID"),
		}
		req.ArgvDigest = buildArgvDigest(req.Comm, req.Args)

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

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func parseEnvUint32(keys ...string) uint32 {
	for _, key := range keys {
		var parsed uint32
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	return 0
}

func parseEnvFloat64(keys ...string) float64 {
	for _, key := range keys {
		var parsed float64
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			if _, err := fmt.Sscanf(value, "%f", &parsed); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func buildArgvDigest(comm string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	if trimmed := strings.TrimSpace(comm); trimmed != "" {
		parts = append(parts, trimmed)
	}
	for _, arg := range args {
		if trimmed := strings.TrimSpace(arg); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}
