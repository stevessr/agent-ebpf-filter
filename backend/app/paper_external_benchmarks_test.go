package app

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/internal/domainforwardproxy"
	"agent-ebpf-filter/internal/heuristics"
	"agent-ebpf-filter/pb"
)

// ExternalBenchmarkReport captures all large-scale public benchmark & upgraded ablation results.
type ExternalBenchmarkReport struct {
	GeneratedAt        string                        `json:"generatedAt"`
	BashFullML         []FullBashMLAblationRow       `json:"bashFullML"`
	InjecAgent         []InjecAgentAblationRow       `json:"injecAgent"`
	PrivacyLens        []PrivacyLensAblationRow      `json:"privacyLens"`
	AttributionMulti   []AttributionMultiAblationRow `json:"attributionMulti"`
	WebShellHeuristic  []WebShellAblationRow         `json:"webShellHeuristic"`
	SemanticContention []SemanticRuleAblationRow     `json:"semanticContention"`
}

type FullBashMLAblationRow struct {
	Variant             string  `json:"variant"`
	Dataset             string  `json:"dataset"`
	TotalSamples        int     `json:"totalSamples"`
	Seeds               int     `json:"seeds"`
	LeakagePairs        int     `json:"leakagePairs"`
	ValAccMean          float64 `json:"valAccMean"`
	ValAccStd           float64 `json:"valAccStd"`
	BlockRecallMean     float64 `json:"blockRecallMean"`
	BlockRecallStd      float64 `json:"blockRecallStd"`
	BenignFPRMean       float64 `json:"benignFPRMean"`
	BenignFPRStd        float64 `json:"benignFPRStd"`
	BalancedAccMean     float64 `json:"balancedAccMean"`
	WilsonBlockRecLow95 float64 `json:"wilsonBlockRecLow95"`
	WilsonBlockRecHi95  float64 `json:"wilsonBlockRecHi95"`
}

type InjecAgentAblationRow struct {
	Mode             string  `json:"mode"`
	TotalCases       int     `json:"totalCases"`
	DHCases          int     `json:"dhCases"`
	DSCases          int     `json:"dsCases"`
	DetectedTotal    int     `json:"detectedTotal"`
	DetectedDH       int     `json:"detectedDH"`
	DetectedDS       int     `json:"detectedDS"`
	DetectionRate    float64 `json:"detectionRate"`
	DHDetectionRate  float64 `json:"dhDetectionRate"`
	DSDetectionRate  float64 `json:"dsDetectionRate"`
	SemanticMismatch int     `json:"semanticMismatch"`
	TokenExfilRisk   int     `json:"tokenExfilRisk"`
	UnexpectedEgress int     `json:"unexpectedEgress"`
	P50LatencyNs     int64   `json:"p50LatencyNs"`
	P95LatencyNs     int64   `json:"p95LatencyNs"`
}

type PrivacyLensAblationRow struct {
	Mode              string  `json:"mode"`
	TotalTrajectories int     `json:"totalTrajectories"`
	BenignControls    int     `json:"benignControls"`
	LeakedCount       int     `json:"leakedCount"`
	LeakageRate       float64 `json:"leakageRate"`
	ProtectedCount    int     `json:"protectedCount"`
	ProtectionRate    float64 `json:"protectionRate"`
	RestoreVerified   int     `json:"restoreVerified"`
	BenignUtilityPass int     `json:"benignUtilityPass"`
	BenignUtilityRate float64 `json:"benignUtilityRate"`
	P50LatencyUs      float64 `json:"p50LatencyUs"`
	P95LatencyUs      float64 `json:"p95LatencyUs"`
}

type AttributionMultiAblationRow struct {
	Mode            string  `json:"mode"`
	TotalEvents     int     `json:"totalEvents"`
	RunMatched      int     `json:"runMatched"`
	ToolMatched     int     `json:"toolMatched"`
	Misattributed   int     `json:"misattributed"`
	Unattributed    int     `json:"unattributed"`
	RunAccRate      float64 `json:"runAccRate"`
	ToolAccRate     float64 `json:"toolAccRate"`
	MisattrRate     float64 `json:"misattrRate"`
	AlertRecallRate float64 `json:"alertRecallRate"`
	BenignFP        int     `json:"benignFP"`
}

type WebShellAblationRow struct {
	Variant        string  `json:"variant"`
	WebShellCases  int     `json:"webShellCases"`
	BenignCLICases int     `json:"benignCLICases"`
	TP             int     `json:"tp"`
	FP             int     `json:"fp"`
	Precision      float64 `json:"precision"`
	Recall         float64 `json:"recall"`
	F1             float64 `json:"f1"`
}

func TestExternalPublicBenchmarksAndAblations(t *testing.T) {
	origTracked := trackedProcessContexts
	origSemanticState := semanticAlertsState
	origToolBaseline := toolBaseline
	origMLRuntime := ml.SnapshotMLRuntime()
	defer func() {
		trackedProcessContexts = origTracked
		semanticAlertsState = origSemanticState
		toolBaseline = origToolBaseline
		ml.ReplaceMLRuntime(origMLRuntime)
	}()
	ml.ReplaceMLRuntime(ml.MLRuntimeSnapshot{
		Config:  DefaultMLConfig(),
		Enabled: false,
	})

	report := ExternalBenchmarkReport{
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339),
		BashFullML:         runFullBashDatasetsAblation(t),
		InjecAgent:         runInjecAgentBenchmark(t),
		PrivacyLens:        runPrivacyLensBenchmark(t),
		AttributionMulti:   runUpgradedAttributionAblation(t),
		WebShellHeuristic:  runWebShellAblation(t),
		SemanticContention: runSemanticRuleAblation(t),
	}

	outPath := filepath.Join("..", "..", "reports", "ablation-paper-20261119", "external_benchmarks_summary.json")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		t.Fatalf("mkdir report dir: %v", err)
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal external benchmark report: %v", err)
	}
	if err := os.WriteFile(outPath, encoded, 0o644); err != nil {
		t.Fatalf("write external benchmark report: %v", err)
	}
	t.Logf("External benchmark report written to %s\n%s", outPath, string(encoded))
}

// ============================================================================
//  1. Full bash-datasets (2,187 raw commands -> deduplicated) ML Ablation
//     Compares Naive Row Split (2x repeated log) vs MingJing Grouped Split
//     vs Blanket /etc Path Redaction across 10 seeds with Mean ± Std & Wilson 95% CI
//
// ============================================================================
func runFullBashDatasetsAblation(t *testing.T) []FullBashMLAblationRow {
	t.Helper()
	safePath := filepath.Join("..", "..", "research", "datasets", "bash-datasets", "taglio4_safe.jsonl")
	unsafePath := filepath.Join("..", "..", "research", "datasets", "bash-datasets", "taglio4_unsafe.jsonl")

	loadRows := func(path, name string) []remoteDatasetRow {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		parsed, err := pullRemoteDataset(remoteDatasetRequest{
			SourceName:     name,
			Format:         "jsonl",
			Content:        string(raw),
			Limit:          remoteDatasetAbsoluteRecordLimit,
			LabelMode:      "preserve",
			ImportAll:      true,
			CleanSensitive: true,
		})
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		return parsed.Rows
	}

	allRows := append(loadRows(safePath, "taglio4_safe.jsonl"), loadRows(unsafePath, "taglio4_unsafe.jsonl")...)
	extractor := &FeatureExtractor{history: newRecentHistoryBuffer(1)}

	seen := make(map[string]int32)
	var dedupRows []remoteDatasetRow
	var dedupSamples []ml.TrainingSample
	for _, row := range allRows {
		s := buildRemoteDatasetSampleWithExtractor(row, "preserve", true, extractor)
		neutralizeOfflineDecisionClockFeatures(&s)
		if !s.IsLabeled() || (s.Label != 0 && s.Label != 1) {
			continue
		}
		key := fmt.Sprintf("%s\x00%s", s.Comm, strings.Join(s.Args, "\x00"))
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = s.Label
		dedupRows = append(dedupRows, row)
		dedupSamples = append(dedupSamples, s)
	}

	hp := DefaultDecisionTrainingHyperparameters()
	const numSeeds = 10
	var out []FullBashMLAblationRow

	// Variant A: Naive Row-Wise Split on 2x repeated log (demonstrates real leakage on 2k corpus)
	{
		var repeated []ml.TrainingSample
		for _, s := range dedupSamples {
			repeated = append(repeated, s, s)
		}
		rng := rand.New(rand.NewSource(20261119))
		perm := rng.Perm(len(repeated))
		valSize := len(repeated) / 5
		trainStore := ml.NewTrainingDataStore(len(repeated) + 10)
		trainCmds := make(map[string]bool)
		var valRaw []ml.TrainingSample
		for i, idx := range perm {
			s := repeated[idx]
			if i < valSize {
				valRaw = append(valRaw, s)
			} else {
				trainCmds[s.CommandLine] = true
				trainStore.Add(s)
			}
		}
		leakPairs := 0
		for _, s := range valRaw {
			if trainCmds[s.CommandLine] {
				leakPairs++
			}
		}
		cfg := ml.DefaultMLConfig()
		cfg.ModelType = ml.ModelRandomForest
		cfg.NumTrees = hp.NumTrees
		cfg.MaxDepth = hp.MaxDepth
		cfg.MinSamplesLeaf = hp.MinSamplesLeaf

		var accs, blkRecs, fprs, bals []float64
		totalTP, totalBlock := 0, 0
		for seed := int64(0); seed < numSeeds; seed++ {
			cfg.TrainingSeed = seed + 1
			model, _ := ml.GlobalTrainer.TrainWithConfig(trainStore, cfg)
			if model == nil {
				continue
			}
			tp, tn, fp, fn := evalSamples(model, valRaw)
			acc, rec, fpr, bal := calcMetrics(tp, tn, fp, fn)
			accs = append(accs, acc)
			blkRecs = append(blkRecs, rec)
			fprs = append(fprs, fpr)
			bals = append(bals, bal)
			totalTP += tp
			totalBlock += tp + fn
		}
		lo, hi := wilson95(totalTP/numSeeds, totalBlock/numSeeds)
		out = append(out, FullBashMLAblationRow{
			Variant:             "Naive Row Split (2x Repeated Log)",
			Dataset:             "bash-datasets-full-2x",
			TotalSamples:        len(repeated),
			Seeds:               numSeeds,
			LeakagePairs:        leakPairs,
			ValAccMean:          meanFloat(accs),
			ValAccStd:           stdFloat(accs),
			BlockRecallMean:     meanFloat(blkRecs),
			BlockRecallStd:      stdFloat(blkRecs),
			BenignFPRMean:       meanFloat(fprs),
			BenignFPRStd:        stdFloat(fprs),
			BalancedAccMean:     meanFloat(bals),
			WilsonBlockRecLow95: lo,
			WilsonBlockRecHi95:  hi,
		})
	}

	// Variant B: MingJing Grouped Behavior Holdout (grouped_behavior_v1) on Full Deduplicated Corpus
	{
		train, _, _, validation, err := ml.PrepareAutoTuneSplit(dedupSamples, 0.2)
		if err != nil {
			t.Fatalf("PrepareAutoTuneSplit full bash failed: %v", err)
		}
		var accs, blkRecs, fprs, bals []float64
		totalTP, totalBlock := 0, 0
		for seed := int64(0); seed < numSeeds; seed++ {
			model := ml.BuildAutoTuneForest(train, hp.NumTrees, hp.MaxDepth, hp.MinSamplesLeaf, seed+1)
			model.Prune(train)
			tp, tn, fp, fn := evalSamples(model, validation)
			acc, rec, fpr, bal := calcMetrics(tp, tn, fp, fn)
			accs = append(accs, acc)
			blkRecs = append(blkRecs, rec)
			fprs = append(fprs, fpr)
			bals = append(bals, bal)
			totalTP += tp
			totalBlock += tp + fn
		}
		lo, hi := wilson95(totalTP/numSeeds, totalBlock/numSeeds)
		out = append(out, FullBashMLAblationRow{
			Variant:             "MingJing Grouped Holdout (Full 2k)",
			Dataset:             "bash-datasets-full-dedup",
			TotalSamples:        len(dedupSamples),
			Seeds:               numSeeds,
			LeakagePairs:        0,
			ValAccMean:          meanFloat(accs),
			ValAccStd:           stdFloat(accs),
			BlockRecallMean:     meanFloat(blkRecs),
			BlockRecallStd:      stdFloat(blkRecs),
			BenignFPRMean:       meanFloat(fprs),
			BenignFPRStd:        stdFloat(fprs),
			BalancedAccMean:     meanFloat(bals),
			WilsonBlockRecLow95: lo,
			WilsonBlockRecHi95:  hi,
		})
	}

	// Variant C: Grouped Holdout w/o /etc Risk-Target Preservation on Full Deduplicated Corpus
	{
		var blinded []ml.TrainingSample
		for _, row := range dedupRows {
			bRow := row
			for _, target := range []string{"/etc/shadow", "/etc/passwd", "/etc/sudoers", "/etc/ssh/sshd_config", "/etc/hosts", "/etc/group", "/etc/fstab"} {
				bRow.CommandLine = strings.ReplaceAll(bRow.CommandLine, target, "***")
				for i := range bRow.Args {
					bRow.Args[i] = strings.ReplaceAll(bRow.Args[i], target, "***")
				}
			}
			s := buildRemoteDatasetSampleWithExtractor(bRow, "preserve", true, extractor)
			neutralizeOfflineDecisionClockFeatures(&s)
			blinded = append(blinded, s)
		}
		train, _, _, validation, err := ml.PrepareAutoTuneSplit(blinded, 0.2)
		if err != nil {
			t.Fatalf("PrepareAutoTuneSplit blinded full bash failed: %v", err)
		}
		var accs, blkRecs, fprs, bals []float64
		totalTP, totalBlock := 0, 0
		for seed := int64(0); seed < numSeeds; seed++ {
			model := ml.BuildAutoTuneForest(train, hp.NumTrees, hp.MaxDepth, hp.MinSamplesLeaf, seed+1)
			model.Prune(train)
			tp, tn, fp, fn := evalSamples(model, validation)
			acc, rec, fpr, bal := calcMetrics(tp, tn, fp, fn)
			accs = append(accs, acc)
			blkRecs = append(blkRecs, rec)
			fprs = append(fprs, fpr)
			bals = append(bals, bal)
			totalTP += tp
			totalBlock += tp + fn
		}
		lo, hi := wilson95(totalTP/numSeeds, totalBlock/numSeeds)
		out = append(out, FullBashMLAblationRow{
			Variant:             "w/o /etc Risk-Target Preservation (Full 2k)",
			Dataset:             "bash-datasets-full-dedup",
			TotalSamples:        len(blinded),
			Seeds:               numSeeds,
			LeakagePairs:        0,
			ValAccMean:          meanFloat(accs),
			ValAccStd:           stdFloat(accs),
			BlockRecallMean:     meanFloat(blkRecs),
			BlockRecallStd:      stdFloat(blkRecs),
			BenignFPRMean:       meanFloat(fprs),
			BenignFPRStd:        stdFloat(fprs),
			BalancedAccMean:     meanFloat(bals),
			WilsonBlockRecLow95: lo,
			WilsonBlockRecHi95:  hi,
		})
	}
	return out
}

// ============================================================================
//  2. InjecAgent (UIUC, 1,054 Indirect Prompt Injection Cases: 510 DH + 544 DS)
//     Maps every InjecAgent (User Tool -> Hijacked Attacker Tool) into OS/tool
//     execution events and compares Kernel-Only (No Tool Intent) vs Full MingJing
//     Cross-Layer Semantic Attribution.
//
// ============================================================================
type injecAgentCase struct {
	UserTool      string   `json:"User Tool"`
	AttackerTools []string `json:"Attacker Tools"`
	AttackType    string   `json:"Attack Type"`
}

func runInjecAgentBenchmark(t *testing.T) []InjecAgentAblationRow {
	t.Helper()
	dhPath := filepath.Join("..", "..", "research", "datasets", "InjecAgent", "data", "test_cases_dh_base.json")
	dsPath := filepath.Join("..", "..", "research", "datasets", "InjecAgent", "data", "test_cases_ds_base.json")

	load := func(p string) []injecAgentCase {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read InjecAgent %s: %v", p, err)
		}
		var list []injecAgentCase
		if err := json.Unmarshal(raw, &list); err != nil {
			t.Fatalf("parse InjecAgent %s: %v", p, err)
		}
		return list
	}
	dhCases := load(dhPath)
	dsCases := load(dsPath)

	type taggedCase struct {
		isDS bool
		c    injecAgentCase
	}
	all := make([]taggedCase, 0, len(dhCases)+len(dsCases))
	for _, c := range dhCases {
		all = append(all, taggedCase{isDS: false, c: c})
	}
	for _, c := range dsCases {
		all = append(all, taggedCase{isDS: true, c: c})
	}

	modes := []string{
		"Kernel-Only (No Tool Intent)",
		"Full MingJing Cross-Layer",
	}
	var out []InjecAgentAblationRow

	for _, mode := range modes {
		row := InjecAgentAblationRow{
			Mode:       mode,
			TotalCases: len(all),
			DHCases:    len(dhCases),
			DSCases:    len(dsCases),
		}
		latencies := make([]int64, 0, len(all))

		for idx, item := range all {
			trackedProcessContexts = newProcessContextStore()
			resetSemanticAlertState()
			toolBaseline = newToolBaselineStore()

			rootPID := uint32(10000 + idx*2)
			childPID := rootPID + 1
			runID := fmt.Sprintf("injec-%d", idx)
			// Normalize user tool name to canonical read-only intent when InjecAgent user tool is Get/Read/Search/List
			declaredTool := normalizeInjecUserTool(item.c.UserTool)

			if mode == "Full MingJing Cross-Layer" {
				trackedProcessContexts.Set(rootPID, events.ProcessContext{
					RootAgentPid: rootPID,
					AgentRunID:   runID,
					TaskID:       "task-" + runID,
					ToolCallID:   "tc-" + runID,
					ToolName:     declaredTool,
					Cwd:          "/workspace/agent",
				})
			}

			// Synthesize OS-level observable behavior of the hijacked attacker tool(s)
			var evs []*pb.Event
			if item.isDS {
				// Data Stealing (2-stage): read sensitive vault/token store -> outbound exfiltration (e.g. GmailSendEmail / API egress)
				evs = append(evs,
					&pb.Event{Pid: childPID, Ppid: rootPID, Type: "openat", Comm: "python3", Path: "/workspace/agent/.env"},
					&pb.Event{Pid: childPID, Ppid: rootPID, Type: "network_connect", Comm: "python3", NetEndpoint: "smtp.attacker.example:443", NetDirection: "connect", DstPort: 443},
				)
			} else {
				// Direct Harm: unauthorized external API/state mutation call (e.g. SmartLock / BankTransfer / RepoDelete)
				evs = append(evs,
					&pb.Event{Pid: childPID, Ppid: rootPID, Type: "network_connect", Comm: "python3", NetEndpoint: "api.external-service.example:443", NetDirection: "connect", DstPort: 443},
				)
			}

			start := time.Now()
			seenCodes := make(map[string]bool)
			for _, ev := range evs {
				if mode == "Full MingJing Cross-Layer" {
					ev = enrichEventContext(ev)
				}
				alerts := buildSemanticAlerts(ev)
				for _, a := range alerts {
					seenCodes[strings.TrimSpace(a.GetComm())] = true
				}
			}
			latencies = append(latencies, time.Since(start).Nanoseconds())

			if seenCodes["SEMANTIC_MISMATCH"] {
				row.SemanticMismatch++
			}
			if seenCodes["TOKEN_EXFIL_RISK"] {
				row.TokenExfilRisk++
			}
			if seenCodes["UNEXPECTED_NETWORK_EGRESS"] {
				row.UnexpectedEgress++
			}
			// A case is counted as intent-hijack detected if SEMANTIC_MISMATCH or TOKEN_EXFIL_RISK fires
			// (for Kernel-Only without tool intent, UNEXPECTED_NETWORK_EGRESS alone cannot distinguish benign API calls from hijacked read-only tools)
			detected := seenCodes["SEMANTIC_MISMATCH"] || seenCodes["TOKEN_EXFIL_RISK"]
			if detected {
				row.DetectedTotal++
				if item.isDS {
					row.DetectedDS++
				} else {
					row.DetectedDH++
				}
			}
		}

		row.DetectionRate = ratio(row.DetectedTotal, row.TotalCases)
		row.DHDetectionRate = ratio(row.DetectedDH, row.DHCases)
		row.DSDetectionRate = ratio(row.DetectedDS, row.DSCases)
		row.P50LatencyNs = percentileNs(latencies, 50)
		row.P95LatencyNs = percentileNs(latencies, 95)
		out = append(out, row)
	}
	return out
}

func normalizeInjecUserTool(tool string) string {
	lower := strings.ToLower(tool)
	for _, kw := range []string{"get", "read", "search", "list", "view"} {
		if strings.Contains(lower, kw) {
			return "read_" + lower
		}
	}
	return lower
}

// ============================================================================
//  3. PrivacyLens (NeurIPS, 493 Agent Privacy Trajectories + 493 Benign Controls)
//     Evaluates Out-of-Prompt HTTP Privacy Gateway with Warmup + Steady-State P50/P95
//
// ============================================================================
type privacyLensItem struct {
	Name       string `json:"name"`
	Trajectory struct {
		UserEmail       string   `json:"user_email"`
		UserInstruction string   `json:"user_instruction"`
		FinalAction     string   `json:"final_action"`
		SensitiveItems  []string `json:"sensitive_info_items"`
	} `json:"trajectory"`
}

func runPrivacyLensBenchmark(t *testing.T) []PrivacyLensAblationRow {
	t.Helper()
	plPath := filepath.Join("..", "..", "research", "datasets", "PrivacyLens", "data", "main_data.json")
	raw, err := os.ReadFile(plPath)
	if err != nil {
		t.Fatalf("read PrivacyLens %s: %v", plPath, err)
	}
	var items []privacyLensItem
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("parse PrivacyLens %s: %v", plPath, err)
	}

	type plReq struct {
		body        []byte
		secretEmail string
		isBenign    bool
	}
	reqs := make([]plReq, 0, len(items)*2)
	for i, it := range items {
		email := strings.TrimSpace(it.Trajectory.UserEmail)
		if email == "" {
			email = fmt.Sprintf("user%d@privacylens.example", i)
		}
		// Sensitive outbound tool payload carrying user_email & action context from PrivacyLens
		payloadObj := map[string]string{
			"action":  it.Trajectory.FinalAction,
			"sender":  email,
			"content": fmt.Sprintf("Forwarding context for %s regarding %s", email, it.Name),
		}
		b, _ := json.Marshal(payloadObj)
		reqs = append(reqs, plReq{body: b, secretEmail: email, isBenign: false})

		// Paired benign functional control payload (same action schema, no PII)
		benignObj := map[string]string{
			"action":  it.Trajectory.FinalAction,
			"status":  "ok",
			"content": fmt.Sprintf("Completed workflow step %d with schema check", i),
		}
		bb, _ := json.Marshal(benignObj)
		reqs = append(reqs, plReq{body: bb, isBenign: true})
	}

	modes := []struct {
		label   string
		enabled bool
		action  string
	}{
		{"Disabled (Passthrough)", false, "warn"},
		{"Warn-Only (Audit)", true, "warn"},
		{"MingJing Redact + Restore", true, "redact"},
		{"MingJing Strict Block", true, "block"},
	}

	var out []PrivacyLensAblationRow
	for _, m := range modes {
		engine := domainforwardproxy.NewPrivacyEngine(domainforwardproxy.PrivacySettings{
			Enabled:         m.enabled,
			Action:          m.action,
			RestoreResponse: true,
			Categories:      []string{"secret", "email", "phone"},
		})

		// Warmup pass (50 requests) to eliminate Go regex/sync.Once cold-start skew
		if m.enabled {
			for w := 0; w < 50 && w < len(reqs); w++ {
				_, _, _, _ = engine.Apply("application/json", reqs[w].body)
			}
		}

		row := PrivacyLensAblationRow{
			Mode:              m.label,
			TotalTrajectories: len(items),
			BenignControls:    len(items),
		}
		latenciesNs := make([]int64, 0, len(reqs))

		for _, r := range reqs {
			start := time.Now()
			if !m.enabled {
				latenciesNs = append(latenciesNs, time.Since(start).Nanoseconds())
				if r.isBenign {
					row.BenignUtilityPass++
				} else {
					row.LeakedCount++
				}
				continue
			}

			outBytes, session, report, err := engine.Apply("application/json", r.body)
			latenciesNs = append(latenciesNs, time.Since(start).Nanoseconds())

			if r.isBenign {
				if err == nil && !report.Blocked && string(outBytes) == string(r.body) {
					row.BenignUtilityPass++
				}
				continue
			}

			if err != nil || report.Blocked {
				row.ProtectedCount++
				continue
			}
			if strings.Contains(string(outBytes), r.secretEmail) {
				row.LeakedCount++
			} else {
				row.ProtectedCount++
				if session != nil {
					if restored, ok := session.Restore(outBytes); ok && strings.Contains(string(restored), r.secretEmail) {
						row.RestoreVerified++
					}
				}
			}
		}

		row.LeakageRate = ratio(row.LeakedCount, row.TotalTrajectories)
		row.ProtectionRate = ratio(row.ProtectedCount, row.TotalTrajectories)
		row.BenignUtilityRate = ratio(row.BenignUtilityPass, row.BenignControls)
		row.P50LatencyUs = float64(percentileNs(latenciesNs, 50)) / 1000.0
		row.P95LatencyUs = float64(percentileNs(latenciesNs, 95)) / 1000.0
		out = append(out, row)
	}
	return out
}

// ============================================================================
//  4. Upgraded Attribution Ablation (Run-Level vs Tool-Call-Level Accuracy)
//     19 runtime-replay.v1 scenarios + 16 concurrent multi-agent 3-hop trees
//
// ============================================================================
func runUpgradedAttributionAblation(t *testing.T) []AttributionMultiAblationRow {
	t.Helper()
	catalog := loadRuntimeReplayCatalog(t)

	type concAgent struct {
		runID    string
		toolCall string
		toolName string
		rootPID  uint32
		midPID   uint32
		leafPID  uint32
		cgroupID uint64
		comm     string
		target   string
	}
	comms := []string{"python", "git", "node", "bash"}
	concAgents := make([]concAgent, 0, 16)
	for i := 0; i < 16; i++ {
		base := uint32(20000 + i*10)
		concAgents = append(concAgents, concAgent{
			runID:    fmt.Sprintf("conc-run-%02d", i),
			toolCall: fmt.Sprintf("conc-tc-%02d", i),
			toolName: "inspect_repo",
			rootPID:  base,
			midPID:   base + 1,
			leafPID:  base + 2,
			cgroupID: uint64(5000 + i),
			comm:     comms[i%len(comms)],
			target:   fmt.Sprintf("/workspace/repo/src/mod_%02d.go", i),
		})
	}

	modes := []string{
		"No-Context (Kernel Only)",
		"Bare-Comm Heuristic (Naive)",
		"PID-Only (No Tool Intent)",
		"Wrapper/Hook-Only (No Multi-Hop)",
		"Full MingJing Tri-Modal",
	}

	out := make([]AttributionMultiAblationRow, 0, len(modes))
	for _, mode := range modes {
		row := AttributionMultiAblationRow{Mode: mode}
		alertsExpected, alertsMatched := 0, 0

		// Part A: 19 runtime-replay.v1 scenarios
		for _, scenario := range catalog.Scenarios {
			trackedProcessContexts = newProcessContextStore()
			resetSemanticAlertState()
			toolBaseline = newToolBaselineStore()

			var seedReq *pb.WrapperRequest
			if scenario.Seed != nil && scenario.Seed.WrapperRequest != nil {
				seedReq = scenario.Seed.WrapperRequest.toProto()
				action := simulateWrapperDecision(seedReq)
				switch mode {
				case "PID-Only (No Tool Intent)":
					trackedProcessContexts.Set(seedReq.Pid, events.ProcessContext{
						RootAgentPid: seedReq.RootAgentPid,
						AgentRunID:   seedReq.AgentRunId,
					})
				case "Wrapper/Hook-Only (No Multi-Hop)", "Full MingJing Tri-Modal":
					trackedProcessContexts.Set(seedReq.Pid, buildProcessContextFromWrapperRequest(seedReq, action, 0.1))
				}
			}

			observed := make(map[string]struct{})
			for _, raw := range scenario.Events {
				ev := raw.toProto()
				switch mode {
				case "Bare-Comm Heuristic (Naive)":
					if seedReq != nil && ev.Comm == seedReq.Comm {
						ev.AgentRunId = seedReq.AgentRunId
					}
				case "PID-Only (No Tool Intent)", "Full MingJing Tri-Modal":
					ev = enrichEventContext(ev)
				case "Wrapper/Hook-Only (No Multi-Hop)":
					if ctx, ok := trackedProcessContexts.Get(ev.Pid); ok {
						events.ApplyProcessContextToEvent(ev, ctx)
					}
				}
				for _, a := range buildSemanticAlerts(ev) {
					if c := strings.TrimSpace(a.GetComm()); c != "" {
						observed[c] = struct{}{}
					}
				}
				if scenario.ExpectContextInheritance && seedReq != nil && raw.PID != seedReq.Pid {
					row.TotalEvents++
					gotRun := strings.TrimSpace(ev.GetAgentRunId())
					gotTool := strings.TrimSpace(ev.GetToolCallId())
					if gotRun == "" {
						row.Unattributed++
					} else if gotRun != seedReq.GetAgentRunId() {
						row.Misattributed++
					} else {
						row.RunMatched++
						if gotTool == seedReq.GetToolCallId() {
							row.ToolMatched++
						}
					}
				}
			}

			alertsExpected += len(scenario.ExpectedAlertCodes)
			for _, exp := range scenario.ExpectedAlertCodes {
				if _, ok := observed[exp]; ok {
					alertsMatched++
				}
			}
			if scenario.Class == "benign" {
				row.BenignFP += len(observed)
			}
		}

		// Part B: 16 Concurrent Multi-Agent 3-Hop Process Trees
		trackedProcessContexts = newProcessContextStore()
		resetSemanticAlertState()
		commOwner := make(map[string]string)
		for _, a := range concAgents {
			commOwner[a.comm] = a.runID
			switch mode {
			case "PID-Only (No Tool Intent)":
				trackedProcessContexts.Set(a.rootPID, events.ProcessContext{
					RootAgentPid: a.rootPID,
					AgentRunID:   a.runID,
				})
			case "Wrapper/Hook-Only (No Multi-Hop)", "Full MingJing Tri-Modal":
				trackedProcessContexts.Set(a.rootPID, events.ProcessContext{
					RootAgentPid: a.rootPID,
					AgentRunID:   a.runID,
					ToolCallID:   a.toolCall,
					ToolName:     a.toolName,
					Cwd:          "/workspace/repo",
				})
			}
		}
		for _, a := range concAgents {
			f1 := &pb.Event{Pid: a.rootPID, Ppid: 1, Type: "process_fork", Comm: a.comm, ExtraInfo: fmt.Sprintf("child_pid=%d", a.midPID), CgroupId: a.cgroupID}
			f2 := &pb.Event{Pid: a.midPID, Ppid: a.rootPID, Type: "process_fork", Comm: "sh", ExtraInfo: fmt.Sprintf("child_pid=%d", a.leafPID), CgroupId: a.cgroupID}
			leaf := &pb.Event{Pid: a.leafPID, Ppid: a.midPID, Type: "openat", Comm: a.comm, Path: a.target, CgroupId: a.cgroupID}
			for _, ev := range []*pb.Event{f1, f2, leaf} {
				switch mode {
				case "Bare-Comm Heuristic (Naive)":
					if owner, ok := commOwner[ev.Comm]; ok {
						ev.AgentRunId = owner
					}
				case "PID-Only (No Tool Intent)", "Full MingJing Tri-Modal":
					enrichEventContext(ev)
				case "Wrapper/Hook-Only (No Multi-Hop)":
					if ctx, ok := trackedProcessContexts.Get(ev.Pid); ok {
						events.ApplyProcessContextToEvent(ev, ctx)
					}
				}
			}
			row.TotalEvents++
			gotRun := strings.TrimSpace(leaf.GetAgentRunId())
			gotTool := strings.TrimSpace(leaf.GetToolCallId())
			if gotRun == "" {
				row.Unattributed++
			} else if gotRun != a.runID {
				row.Misattributed++
			} else {
				row.RunMatched++
				if gotTool == a.toolCall {
					row.ToolMatched++
				}
			}
		}

		row.RunAccRate = ratio(row.RunMatched, row.TotalEvents)
		row.ToolAccRate = ratio(row.ToolMatched, row.TotalEvents)
		row.MisattrRate = ratio(row.Misattributed, row.TotalEvents)
		row.AlertRecallRate = ratio(alertsMatched, alertsExpected)
		out = append(out, row)
	}
	return out
}

// ============================================================================
//  5. Web-Service Shell-Spawn Heuristic Ablation (31 Real Runtime Cases)
//     Compares Naive Interpreter-Parent Alerting vs MingJing Positive-Evidence Rule
//
// ============================================================================
func runWebShellAblation(t *testing.T) []WebShellAblationRow {
	t.Helper()
	type wsCase struct {
		isWebShell bool
		cmd        string
		args       []string
		parentArgv []string
	}
	cases := []wsCase{
		// 17 True Web-Service Shell Spawns
		{true, "/bin/sh", []string{"-c", "id"}, []string{"/usr/sbin/php-fpm8.3", "-F"}},
		{true, "/bin/sh", []string{"-c", "curl x"}, []string{"php-fpm: pool www"}},
		{true, "bash", nil, []string{"/usr/bin/php-cgi"}},
		{true, "bash", nil, []string{"php", "-S", "0.0.0.0:8080", "-t", "public"}},
		{true, "sh", nil, []string{"php", "artisan", "serve"}},
		{true, "sh", []string{"-c", "id"}, []string{"uvicorn", "api:app", "--host", "0.0.0.0"}},
		{true, "sh", []string{"-c", "id"}, []string{"python3", "-m", "uvicorn", "api:app"}},
		{true, "bash", nil, []string{"/usr/local/bin/gunicorn", "project.wsgi:application"}},
		{true, "bash", nil, []string{"python3", "manage.py", "runserver"}},
		{true, "sh", nil, []string{"python3", "-m", "http.server", "8000"}},
		{true, "bash", nil, []string{"flask", "run", "--host", "0.0.0.0"}},
		{true, "sh", nil, []string{"python3", "-m", "flask", "run"}},
		{true, "env", []string{"/bin/sh", "-c", "id"}, []string{"node", "dist/server.js"}},
		{true, "/bin/bash", nil, []string{"tsx", "src/server.ts"}},
		{true, "sh", nil, []string{"deno", "run", "--allow-net", "server.ts"}},
		{true, "bash", nil, []string{"next", "start"}},
		{true, "busybox", []string{"ash", "-c", "id"}, []string{"node", "node_modules/next/dist/bin/next", "start"}},
		// 14 Benign Developer CLI / Test / Build Spawns
		{false, "bash", nil, []string{"python3", "-m", "pytest"}},
		{false, "sh", nil, []string{"python3", "-m", "celery", "worker"}},
		{false, "bash", nil, []string{"php", "script.php"}},
		{false, "sh", nil, []string{"php", "-s", "script.php"}},
		{false, "bash", nil, []string{"php", "artisan", "queue:work"}},
		{false, "bash", nil, []string{"node", "scripts/build.js"}},
		{false, "sh", nil, []string{"node", "--test", "tests/a.test.js"}},
		{false, "sh", nil, []string{"node", "--test", "tests/server.js"}},
		{false, "bash", nil, []string{"tsx", "scripts/generate.ts"}},
		{false, "sh", nil, []string{"tsx", "scripts/generate.ts", "--port", "8080"}},
		{false, "sh", nil, []string{"bun", "install"}},
		{false, "sh", nil, []string{"npm", "run", "dev"}},
		{false, "bash", nil, []string{"python", "app.py"}},
		{false, "shellcheck", []string{"x.sh"}, []string{"node", "dist/server.js"}},
	}

	variants := []string{
		"Naive Interpreter-Parent Rule",
		"Full MingJing Positive-Evidence Rule",
	}
	var out []WebShellAblationRow
	for _, v := range variants {
		row := WebShellAblationRow{Variant: v}
		for _, tc := range cases {
			if tc.isWebShell {
				row.WebShellCases++
			} else {
				row.BenignCLICases++
			}
			alerted := false
			if v == "Naive Interpreter-Parent Rule" {
				// Naive rule alerts whenever php/python/node/tsx/bun/deno/uvicorn/gunicorn/flask/next spawns any shell or *sh* binary
				p0 := strings.ToLower(filepath.Base(tc.parentArgv[0]))
				isInterp := strings.Contains(p0, "php") || strings.Contains(p0, "python") || strings.Contains(p0, "node") ||
					strings.Contains(p0, "tsx") || strings.Contains(p0, "deno") || strings.Contains(p0, "bun") ||
					strings.Contains(p0, "npm") || strings.Contains(p0, "uvicorn") || strings.Contains(p0, "gunicorn") ||
					strings.Contains(p0, "flask") || strings.Contains(p0, "next")
				c0 := strings.ToLower(filepath.Base(tc.cmd))
				isShellLike := strings.Contains(c0, "sh") || c0 == "env" || c0 == "busybox"
				alerted = isInterp && isShellLike
			} else {
				_, alerted = heuristics.MatchWebShellSpawn(tc.cmd, tc.args, tc.parentArgv, 1234)
			}
			if tc.isWebShell && alerted {
				row.TP++
			}
			if !tc.isWebShell && alerted {
				row.FP++
			}
		}
		if row.TP+row.FP > 0 {
			row.Precision = float64(row.TP) / float64(row.TP+row.FP)
		}
		if row.WebShellCases > 0 {
			row.Recall = float64(row.TP) / float64(row.WebShellCases)
		}
		if row.Precision+row.Recall > 0 {
			row.F1 = 2 * row.Precision * row.Recall / (row.Precision + row.Recall)
		}
		out = append(out, row)
	}
	return out
}

func evalSamples(model ml.Model, samples []ml.TrainingSample) (tp, tn, fp, fn int) {
	for _, s := range samples {
		pred := model.Predict(s.Features).Action
		if s.Label == 1 {
			if pred == 1 {
				tp++
			} else {
				fn++
			}
		} else {
			if pred == 0 {
				tn++
			} else {
				fp++
			}
		}
	}
	return
}

func calcMetrics(tp, tn, fp, fn int) (acc, rec, fpr, bal float64) {
	if tp+tn+fp+fn > 0 {
		acc = float64(tp+tn) / float64(tp+tn+fp+fn)
	}
	if tp+fn > 0 {
		rec = float64(tp) / float64(tp+fn)
	}
	if tn+fp > 0 {
		fpr = float64(fp) / float64(tn+fp)
	}
	tnr := 1.0 - fpr
	bal = (rec + tnr) / 2.0
	return
}

func meanFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func stdFloat(vals []float64) float64 {
	if len(vals) <= 1 {
		return 0
	}
	m := meanFloat(vals)
	sumSq := 0.0
	for _, v := range vals {
		d := v - m
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(vals)-1))
}

func unusedSHA256Guard(b []byte) [32]byte {
	return sha256.Sum256(b)
}
