package main

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

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

				// ── Decision fusion ──
				resolvedAction, reason := resolveAction(
					req, ruleAction, rulePriority,
					classification, anomalyScore, mlPrediction, mlConfig,
				)

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
					sample := TrainingSample{
						Features:     features,
						Label:        -1, // unlabeled initially
						CommandLine:  joinCommandLine(req.Comm, req.Args),
						Comm:         req.Comm,
						Args:         req.Args,
						Category:     classification.PrimaryCategory,
						AnomalyScore: anomalyScore,
						Timestamp:    time.Now(),
					}
					globalTrainingStore.Add(sample)
				}

				globalFeatureExtractor.AddHistory(
					req.Comm,
					classification.PrimaryCategory,
					actionLabel[mlPrediction.Action],
					anomalyScore,
				)

				decision := actionLabel[int32(resolvedAction)]
				riskScore := maxFloat64(anomalyScore, mlPrediction.Confidence)
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

				select {
				case broadcast <- &pb.Event{
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
				}:
				default:
				}
				out, _ := proto.Marshal(resp)
				_, _ = c.Write(out)
			}
		}(conn)
	}
}
