package main

import (
	"net"
	"os"
	"strings"

	"agent-ebpf-filter/pb"
	"google.golang.org/protobuf/proto"
)

func startUDSServer(broadcast chan *pb.Event) {
	_ = os.Remove(udsPath)
	l, err := net.Listen("unix", udsPath)
	if err != nil {
		return
	}
	_ = os.Chmod(udsPath, 0666)
	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			buf := make([]byte, 4096)
			for {
				n, err := c.Read(buf)
				if err != nil {
					return
				}
				req := &pb.WrapperRequest{}
				if err := proto.Unmarshal(buf[:n], req); err != nil {
					continue
				}
				resp := &pb.WrapperResponse{Action: pb.WrapperResponse_ALLOW}
				rulesMu.RLock()
				rule, ok := wrapperRules[req.Comm]
				rulesMu.RUnlock()
				if ok {
					switch rule.Action {
					case "BLOCK":
						resp.Action = pb.WrapperResponse_BLOCK
						resp.Message = "Blocked by policy"
					case "ALERT":
						resp.Action = pb.WrapperResponse_ALERT
						resp.Message = "Security alert"
					case "REWRITE":
						resp.Action = pb.WrapperResponse_REWRITE
						resp.RewrittenArgs = rule.RewrittenCmd
					}
				}
				select {
				case broadcast <- &pb.Event{Pid: req.Pid, Comm: req.Comm, Type: "wrapper_intercept", Tag: "Wrapper", Path: strings.Join(append([]string{req.Comm}, req.Args...), " ")}:
				default:
				}
				out, _ := proto.Marshal(resp)
				_, _ = c.Write(out)
			}
		}(conn)
	}
}
