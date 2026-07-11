package app

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
)

// ---- moved from backend/zz_merged_backend.go section server_uds.go ----

type udsConnectionSet struct {
	mu     sync.Mutex
	closed bool
	conns  map[net.Conn]struct{}
}

func newUDSConnectionSet() *udsConnectionSet {
	return &udsConnectionSet{conns: make(map[net.Conn]struct{})}
}

func (s *udsConnectionSet) Add(conn net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.conns[conn] = struct{}{}
	return true
}

func (s *udsConnectionSet) Remove(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
}

func (s *udsConnectionSet) CloseAll() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	connections := make([]net.Conn, 0, len(s.conns))
	for conn := range s.conns {
		connections = append(connections, conn)
	}
	s.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
}

func removeUDSSocketIfSame(path string, expected os.FileInfo) error {
	if expected == nil {
		return nil
	}
	current, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !os.SameFile(expected, current) {
		return nil
	}
	return os.Remove(path)
}

func startUDSServer(ctx context.Context, broadcast chan *pb.Event) {
	if info, err := os.Lstat(udsPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			log.Printf("[ERROR] refusing to replace non-socket UDS path %s", udsPath)
			return
		}
		if conn, dialErr := net.DialTimeout("unix", udsPath, 100*time.Millisecond); dialErr == nil {
			_ = conn.Close()
			log.Printf("[ERROR] refusing to replace live UDS socket %s", udsPath)
			return
		}
		current, statErr := os.Lstat(udsPath)
		if statErr != nil || !os.SameFile(info, current) {
			log.Printf("[ERROR] UDS path %s changed while checking for a stale socket", udsPath)
			return
		}
		if err := os.Remove(udsPath); err != nil {
			log.Printf("[ERROR] failed to remove stale UDS socket %s: %v", udsPath, err)
			return
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("[ERROR] failed to inspect UDS path %s: %v", udsPath, err)
		return
	}
	l, err := net.Listen("unix", udsPath)
	if err != nil {
		log.Printf("[ERROR] failed to listen on UDS %s: %v", udsPath, err)
		return
	}
	if unixListener, ok := l.(*net.UnixListener); ok {
		unixListener.SetUnlinkOnClose(false)
	}
	createdInfo, err := os.Lstat(udsPath)
	if err != nil {
		_ = l.Close()
		log.Printf("[ERROR] failed to inspect created UDS %s: %v", udsPath, err)
		return
	}
	defer func() {
		_ = l.Close()
		if err := removeUDSSocketIfSame(udsPath, createdInfo); err != nil {
			log.Printf("[WARN] failed to clean up UDS %s: %v", udsPath, err)
		}
	}()
	if err := os.Chmod(udsPath, 0600); err != nil {
		log.Printf("[ERROR] failed to secure UDS %s: %v", udsPath, err)
		return
	}
	if uid, gid, ok := platform.OriginalInvokerIDs(); ok {
		if err := os.Chown(udsPath, int(uid), int(gid)); err != nil {
			log.Printf("[ERROR] failed to assign UDS %s to original invoker: %v", udsPath, err)
			return
		}
	}
	serveUDSListener(ctx, l, broadcast, verifyUDSPeerCredentials)
}

func serveUDSListener(ctx context.Context, l net.Listener, broadcast chan *pb.Event, verifyPeer func(net.Conn) error) {
	if ctx == nil || l == nil {
		return
	}
	if verifyPeer == nil {
		verifyPeer = verifyUDSPeerCredentials
	}
	connections := newUDSConnectionSet()
	var handlers sync.WaitGroup
	defer handlers.Wait()
	defer connections.CloseAll()
	serverDone := make(chan struct{})
	defer close(serverDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = l.Close()
			connections.CloseAll()
		case <-serverDone:
		}
	}()
	for {
		conn, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("[ERROR] UDS accept failed: %v", err)
			return
		}
		if !connections.Add(conn) {
			_ = conn.Close()
			return
		}
		handlers.Add(1)
		go func(c net.Conn) {
			defer handlers.Done()
			defer connections.Remove(c)
			defer c.Close()
			if err := verifyPeer(c); err != nil {
				return
			}
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

				rulesMu.RLock()
				rule, hasRule := wrapperRules[req.Comm]
				rulesMu.RUnlock()

				ruleAction := ""
				rulePriority := 0
				if hasRule {
					ruleAction = rule.Action
					rulePriority = rule.Priority
				}

				// ── Layer 1: Rule-based classification + embedding + anomaly scoring ──
				classification, embedding := globalEmbedder.ClassifyAndEmbed(req.Comm, req.Args)
				globalEmbedder.RegisterVocab(fmt.Sprintf("process %s performed wrapper_intercept on %s %s tagged Wrapper",
					req.Comm, req.Comm, strings.Join(req.Args, " ")))

				// Only cluster if we have enough history (avoid cold-start noise)
				globalEmbedder.AddToCluster(embedding)
				anomalyScore := globalEmbedder.ComputeAnomalyScore(embedding)

				// ── Network audit ──
				cmdline := strings.Join(req.Args, " ")
				netAudit := AuditNetworkBehavior(req.Comm, cmdline)

				// ── Layer 2: ML random forest prediction ──
				features := globalFeatureExtractor.Extract(req.Comm, req.Args, req.User, req.Pid)
				var mlPrediction Prediction
				if mlEnabled && mlModelLoaded {
					mlPrediction = mlEngine.Predict(features)
				}

				// ── Check if process is trusted health dataset generator ──
				isHealthGenerator := false
				if ctx, ok := trackedProcessContexts.Get(req.Pid); ok {
					if _, isHealth := healthGeneratorPIDs.Load(ctx.RootAgentPid); isHealth {
						isHealthGenerator = true
					}
				}
				if _, isHealth := healthGeneratorPIDs.Load(req.Pid); isHealth {
					isHealthGenerator = true
				}

				// ── Decision fusion ──
				resolvedAction, reason := resolveAction(
					req, ruleAction, rulePriority,
					classification, anomalyScore, mlPrediction, mlConfig,
				)

				if isHealthGenerator {
					resolvedAction = pb.WrapperResponse_ALLOW
					reason = "Trusted health dataset generator process (ALLOW)"
				}

				// ── Apply REWRITE logic ──
				resp := &pb.WrapperResponse{
					Action:         resolvedAction,
					Classification: classification,
					AnomalyScore:   anomalyScore,
				}

				if mlEnabled && mlModelLoaded {
					resp.MlScore = mlPrediction.Confidence
					resp.MlAction = actionLabel[mlPrediction.Action]
					resp.MlReasoning = mlReasoning(mlPrediction, anomalyScore, classification)
				}

				resp.Message = reason

				if resolvedAction == pb.WrapperResponse_REWRITE && hasRule {
					resp.Action = pb.WrapperResponse_REWRITE
					if rule.Regex != "" {
						fullArgs := strings.Join(req.Args, " ")
						re, err := regexp.Compile(rule.Regex)
						if err == nil {
							newFull := re.ReplaceAllString(fullArgs, rule.Replacement)
							resp.RewrittenArgs = strings.Fields(newFull)
						} else {
							resp.RewrittenArgs = rule.RewrittenCmd
						}
					} else {
						resp.RewrittenArgs = rule.RewrittenCmd
					}
				}

				// ── Record to training store and history buffer ──
				if mlEnabled && globalTrainingStore != nil {
					labelVal := int32(-1) // unlabeled initially
					userLabelVal := ""
					if isHealthGenerator {
						labelVal = 0 // ALLOW
						userLabelVal = "health-generator"
					}
					sample := TrainingSample{
						Features:     features,
						Label:        labelVal,
						CommandLine:  joinCommandLine(req.Comm, req.Args),
						Comm:         req.Comm,
						Args:         req.Args,
						Category:     classification.PrimaryCategory,
						AnomalyScore: anomalyScore,
						Timestamp:    time.Now(),
						UserLabel:    userLabelVal,
					}
					globalTrainingStore.Add(sample)
				}

				globalFeatureExtractor.AddHistory(
					req.Comm,
					classification.PrimaryCategory,
					actionLabel[mlPrediction.Action],
					anomalyScore,
					req.Pid,
					req.User,
					len(strings.Join(req.Args, " ")),
					len(req.Args),
				)

				decision := actionLabel[int32(resolvedAction)]
				riskScore := platform.MaxFloat64(anomalyScore, mlPrediction.Confidence)
				processCtx := buildProcessContextFromWrapperRequest(req, decision, riskScore)
				trackedProcessContexts.Set(req.Pid, processCtx)

				// Register wrapper PID in eBPF agent_pids
				if trackerMaps.AgentPids != nil {
					_ = trackerMaps.AgentPids.Put(req.Pid, getTagID("Wrapper"))
				}
				if trackerMaps.TrackedComms != nil {
					var k [16]byte
					copy(k[:], req.Comm)
					_ = trackerMaps.TrackedComms.Put(k, getTagID("Wrapper"))
				}

				enqueueBroadcastEvent(broadcast, &pb.Event{
					Pid:            req.Pid,
					Comm:           req.Comm,
					Type:           "wrapper_intercept",
					EventType:      pb.EventType_WRAPPER_INTERCEPT,
					Tag:            "Wrapper",
					Path:           strings.Join(append([]string{req.Comm}, req.Args...), " "),
					Behavior:       classification,
					ExtraInfo:      fmt.Sprintf("net_audit:%s risk:%.0f", netAudit.RiskLevel, netAudit.RiskScore),
					SchemaVersion:  eventSchemaVersion,
					RootAgentPid:   processCtx.RootAgentPid,
					AgentRunId:     processCtx.AgentRunID,
					TaskId:         processCtx.TaskID,
					ConversationId: processCtx.ConversationID,
					TurnId:         processCtx.TurnID,
					ToolCallId:     processCtx.ToolCallID,
					ToolName:       processCtx.ToolName,
					TraceId:        processCtx.TraceID,
					SpanId:         processCtx.SpanID,
					Decision:       processCtx.Decision,
					RiskScore:      processCtx.RiskScore,
					ContainerId:    processCtx.ContainerID,
					ArgvDigest:     processCtx.ArgvDigest,
					Cwd:            processCtx.Cwd,
				}, "uds_wrapper_intercept")

				// ── Async TLS attach for wrapper-registered PIDs ──
				if tlsCaptureController != nil && runtimeSettingsStore.Snapshot().TlsCaptureEnabled && req.Pid > 0 && resolvedAction != pb.WrapperResponse_BLOCK {
					pid := req.Pid
					comm := req.Comm
					reqBinPath := req.BinaryPath // wrapper may know the path before exec
					handlers.Add(1)
					go func() {
						defer handlers.Done()
						select {
						case <-ctx.Done():
							return
						default:
						}
						if !runtimeSettingsStore.Snapshot().TlsCaptureEnabled {
							return
						}
						// Use the binary path supplied by the wrapper (if available)
						// so we can start attach immediately without waiting for exec.
						binPath := strings.TrimSpace(reqBinPath)
						if binPath == "" {
							// Fall back: give the target process time to exec
							// (syscall.Exec replaces the wrapper binary
							// with the actual command).
							timer := time.NewTimer(500 * time.Millisecond)
							select {
							case <-ctx.Done():
								timer.Stop()
								return
							case <-timer.C:
							}

							var err error
							binPath, err = os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
							if err != nil || binPath == "" {
								log.Printf("[tls] wrapper-attach: PID %d (%s): cannot read exe after exec: %v", pid, comm, err)
								return
							}
						}
						if !runtimeSettingsStore.Snapshot().TlsCaptureEnabled {
							return
						}
						select {
						case <-ctx.Done():
							return
						default:
						}

						result := tlsCaptureController.AttachExecutable(binPath, int(pid), "")
						if result.Error != "" {
							log.Printf("[tls] wrapper-attach: PID %d (%s, %s): %s", pid, comm, binPath, result.Error)
						} else {
							log.Printf("[tls] wrapper-attach: PID %d (%s) attached via %s/%s (library=%s)", pid, comm, result.TargetKind, result.Library, binPath)
						}
					}()
				}

				// ── Observer mode: tell the frontend to navigate to the observe page ──
				if req.Observer && req.Pid > 0 {
					enqueueBroadcastEvent(broadcast, &pb.Event{
						Pid:           req.Pid,
						Comm:          req.Comm,
						Type:          "wrapper_intercept",
						EventType:     pb.EventType_OBSERVE_NAVIGATE,
						Tag:           "Observer",
						Path:          req.Comm,
						ExtraInfo:     fmt.Sprintf("auto-observe pid=%d", req.Pid),
						SchemaVersion: eventSchemaVersion,
						RootAgentPid:  processCtx.RootAgentPid,
						AgentRunId:    processCtx.AgentRunID,
						TaskId:        processCtx.TaskID,
					}, "uds_observe_navigate")
				}

				out, _ := proto.Marshal(resp)
				_, _ = c.Write(out)
			}
		}(conn)
	}
}

func verifyUDSPeerCredentials(conn net.Conn) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return errors.New("unexpected UDS connection type")
	}

	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return err
	}

	var cred *unix.Ucred
	var credErr error
	if err := rawConn.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if credErr != nil {
		return credErr
	}
	if cred == nil {
		return errors.New("missing peer credentials")
	}

	if _, ok := allowedControlPlaneUIDs()[cred.Uid]; !ok {
		return fmt.Errorf("unauthorized UDS peer uid %d", cred.Uid)
	}
	return nil
}
