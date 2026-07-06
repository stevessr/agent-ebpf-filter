package app

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
)

// ---- moved from backend/zz_merged_backend.go section server_uds.go ----

func startUDSServer(broadcast chan *pb.Event) {
	_ = os.Remove(udsPath)
	l, err := net.Listen("unix", udsPath)
	if err != nil {
		return
	}
	_ = os.Chmod(udsPath, 0600)
	if uid, gid, ok := platform.OriginalInvokerIDs(); ok {
		_ = os.Chown(udsPath, int(uid), int(gid))
	}
	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			if err := verifyUDSPeerCredentials(c); err != nil {
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
				ctx := buildProcessContextFromWrapperRequest(req, decision, riskScore)
				trackedProcessContexts.Set(req.Pid, ctx)

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
					RootAgentPid:   ctx.RootAgentPid,
					AgentRunId:     ctx.AgentRunID,
					TaskId:         ctx.TaskID,
					ConversationId: ctx.ConversationID,
					TurnId:         ctx.TurnID,
					ToolCallId:     ctx.ToolCallID,
					ToolName:       ctx.ToolName,
					TraceId:        ctx.TraceID,
					SpanId:         ctx.SpanID,
					Decision:       ctx.Decision,
					RiskScore:      ctx.RiskScore,
					ContainerId:    ctx.ContainerID,
					ArgvDigest:     ctx.ArgvDigest,
					Cwd:            ctx.Cwd,
				}, "uds_wrapper_intercept")

				// ── Async TLS attach for wrapper-registered PIDs ──
				if tlsCaptureController != nil && req.Pid > 0 && resolvedAction != pb.WrapperResponse_BLOCK {
					pid := req.Pid
					comm := req.Comm
					reqBinPath := req.BinaryPath // wrapper may know the path before exec
					go func() {
						// Use the binary path supplied by the wrapper (if available)
						// so we can start attach immediately without waiting for exec.
						binPath := strings.TrimSpace(reqBinPath)
						if binPath == "" {
							// Fall back: give the target process time to exec
							// (syscall.Exec replaces the wrapper binary
							// with the actual command).
							time.Sleep(500 * time.Millisecond)

							var err error
							binPath, err = os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
							if err != nil || binPath == "" {
								log.Printf("[tls] wrapper-attach: PID %d (%s): cannot read exe after exec: %v", pid, comm, err)
								return
							}
						}

						manager, err := tlsCaptureController.EnsureStarted()
						if err != nil {
							log.Printf("[tls] wrapper-attach: PID %d (%s): EnsureStarted failed: %v", pid, comm, err)
							return
						}

						result := manager.AttachExecutable(binPath, int(pid), "")
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
						RootAgentPid:  ctx.RootAgentPid,
						AgentRunId:    ctx.AgentRunID,
						TaskId:        ctx.TaskID,
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
