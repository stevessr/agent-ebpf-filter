package app

import (
	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/internal/behavior"
	"math"
	"strings"
	"sync"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section features.go ----

// Feature vector design (128 dimensions) inspired by:
type RecentWrapperEvent struct {
	Comm         string
	Category     string
	Action       string
	AnomalyScore float64
	Timestamp    time.Time
	Pid          uint32
	User         string
	ArgsLen      int
	ArgsCount    int
}

// RecentHistoryBuffer is a sliding window of recent wrapper intercept events
type RecentHistoryBuffer struct {
	mu      sync.RWMutex
	events  []RecentWrapperEvent
	maxSize int
}

func newRecentHistoryBuffer(size int) *RecentHistoryBuffer {
	if size <= 0 {
		size = 100
	}
	return &RecentHistoryBuffer{maxSize: size}
}

func (b *RecentHistoryBuffer) Add(e RecentWrapperEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
	if len(b.events) > b.maxSize {
		copy(b.events, b.events[len(b.events)-b.maxSize:])
		b.events = b.events[:b.maxSize]
	}
}

func (b *RecentHistoryBuffer) Snapshot() []RecentWrapperEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]RecentWrapperEvent, len(b.events))
	copy(out, b.events)
	return out
}

func (b *RecentHistoryBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.events)
}

// FeatureExtractor builds a 128-dim feature vector from wrapper request context
type FeatureExtractor struct {
	history *RecentHistoryBuffer
	// Running statistics for normalization
	mu          sync.RWMutex
	minVals     [FeatureDim]float64
	maxVals     [FeatureDim]float64
	sampleCount int
}

var globalFeatureExtractor = &FeatureExtractor{
	history: newRecentHistoryBuffer(100),
}

var globalEmbedder = behavior.NewInstructionEmbedder()

func (fe *FeatureExtractor) Extract(comm string, args []string, user string, pid uint32) [FeatureDim]float64 {
	var f [FeatureDim]float64

	// ── Group A: Command/Process Features [0-31] ──
	// BehaviorCategory one-hot (15 dims, 0-14)
	classification := behavior.ClassifyBehavior(comm, args)
	for _, cat := range classification.Categories {
		if int(cat) < 15 {
			f[int(cat)] = 1.0
		}
	}

	// Binary flags [15-25]
	f[15] = platform.BoolToFloat(ml.IsShell(comm))                                   // is_shell
	f[16] = platform.BoolToFloat(ml.IsPackageManager(comm))                          // is_package_manager
	f[17] = platform.BoolToFloat(ml.IsAgentCLI(comm))                                // is_agent_cli
	f[18] = platform.BoolToFloat(user == "root")                                     // is_root_user
	f[19] = platform.BoolToFloat(ml.HasNetworkArgs(args))                            // has_network_args
	f[20] = platform.BoolToFloat(ml.HasFileArgs(args))                               // has_file_args
	f[21] = platform.BoolToFloat(ml.HasRedirect(args))                               // has_redirection
	f[22] = platform.BoolToFloat(ml.HasPipeChain(args))                              // has_pipe
	f[23] = platform.BoolToFloat(len(args) > 10)                                     // many_args
	f[24] = platform.BoolToFloat(strings.Contains(strings.Join(args, " "), "/dev/")) // dev_access
	f[25] = platform.BoolToFloat(ml.HasSudoInArgs(args))                             // sudo_in_args

	// Confidence encoding [26-27]
	switch classification.Confidence {
	case "high":
		f[26] = 1.0
	case "medium":
		f[27] = 1.0
	}

	// Command length stats [28-31]
	commLen := float64(len(comm)) / 16.0
	if commLen > 1.0 {
		commLen = 1.0
	}
	f[28] = commLen
	f[29] = float64(len(args)) / 20.0
	if f[29] > 1.0 {
		f[29] = 1.0
	}

	// ── Group B: Argument Statistical Features [32-63] ──
	if len(args) > 0 {
		var sumLen, sumSqLen float64
		for _, a := range args {
			l := float64(len(a))
			sumLen += l
			sumSqLen += l * l
		}
		meanLen := sumLen / float64(len(args))
		f[32] = meanLen / 256.0 // mean arg length (normalized)
		if f[32] > 1.0 {
			f[32] = 1.0
		}
		variance := sumSqLen/float64(len(args)) - meanLen*meanLen
		f[33] = math.Sqrt(math.Abs(variance)) / 256.0 // std dev
		if f[33] > 1.0 {
			f[33] = 1.0
		}
		f[34] = sumLen / 4096.0 // total arg bytes
		if f[34] > 1.0 {
			f[34] = 1.0
		}
		f[35] = platform.ShannonEntropy(strings.Join(args, "")) // path-like entropy
	}

	// Flag vs positional counts [36-37]
	flagCount := 0
	posCount := 0
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flagCount++
		} else {
			posCount++
		}
	}
	f[36] = float64(flagCount) / 20.0
	if f[36] > 1.0 {
		f[36] = 1.0
	}
	f[37] = float64(posCount) / 20.0
	if f[37] > 1.0 {
		f[37] = 1.0
	}

	// Sensitive path flags [38-47]
	allArgs := strings.Join(args, " ")
	f[38] = platform.BoolToFloat(strings.Contains(allArgs, "/etc/"))
	f[39] = platform.BoolToFloat(strings.Contains(allArgs, "/proc/"))
	f[40] = platform.BoolToFloat(strings.Contains(allArgs, "/sys/"))
	f[41] = platform.BoolToFloat(strings.Contains(allArgs, "/var/log/"))
	f[42] = platform.BoolToFloat(strings.Contains(allArgs, "/root/"))
	f[43] = platform.BoolToFloat(strings.Contains(allArgs, "/home/"))
	f[44] = platform.BoolToFloat(strings.Contains(allArgs, "/tmp/"))
	f[45] = platform.BoolToFloat(strings.Contains(allArgs, "~/.ssh"))
	f[46] = platform.BoolToFloat(strings.Contains(allArgs, "~/.gnupg"))
	f[47] = platform.BoolToFloat(strings.Contains(allArgs, "/boot/"))

	// File extension histogram top 10 [48-57]
	extCounts := make(map[string]int)
	topExts := []string{".go", ".py", ".js", ".ts", ".json", ".yaml", ".toml", ".md", ".sh", ".txt"}
	for _, a := range args {
		for _, ext := range topExts {
			if strings.HasSuffix(a, ext) {
				extCounts[ext]++
			}
		}
	}
	for i, ext := range topExts {
		c := float64(extCounts[ext])
		if c > 3 {
			c = 3
		}
		f[48+i] = c / 3.0
	}

	// URL/IP patterns in args [58-59]
	f[58] = platform.BoolToFloat(ml.HasURLPattern(allArgs))
	f[59] = platform.BoolToFloat(ml.HasIPPattern(allArgs))

	// Redirection operators count [60-61]
	redirectCount := 0
	for _, a := range args {
		if a == ">" || a == ">>" || a == "<" || a == "2>" || a == "&>" {
			redirectCount++
		}
	}
	f[60] = float64(redirectCount) / 5.0
	if f[60] > 1.0 {
		f[60] = 1.0
	}

	pipeCount := 0
	for _, a := range args {
		if a == "|" {
			pipeCount++
		}
	}
	f[61] = float64(pipeCount) / 5.0
	if f[61] > 1.0 {
		f[61] = 1.0
	}

	// Argument uniqueness ratio [62-63]
	uniqueArgs := make(map[string]struct{})
	for _, a := range args {
		uniqueArgs[a] = struct{}{}
	}
	if len(args) > 0 {
		f[62] = float64(len(uniqueArgs)) / float64(len(args))
	}
	f[63] = platform.BoolToFloat(ml.HasEnvironmentVar(args))

	// ── Group C: Embedding Projection [64-95] ──
	_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
	// Take first 32 dims of the 64-dim LSH embedding
	copy(f[64:96], emb.Vector[:32])

	// ── Group D: Recent History Aggregates [96-111] ──
	history := fe.history.Snapshot()
	now := time.Now()
	if len(history) > 0 {
		// Frequency of this comm in window
		commMatch := 0
		blockCount, alertCount := 0, 0
		var sumAnomaly, sumSqAnomaly float64
		categorySet := make(map[string]struct{})
		sensitiveCount := 0
		netCount := 0
		rootCount := 0
		distinctComms := make(map[string]struct{})
		distinctUsers := make(map[string]struct{})
		var lastCommTime time.Time

		for _, h := range history {
			if h.Comm == comm {
				commMatch++
				if lastCommTime.IsZero() || h.Timestamp.After(lastCommTime) {
					lastCommTime = h.Timestamp
				}
			}
			switch h.Action {
			case "BLOCK":
				blockCount++
			case "ALERT":
				alertCount++
			}
			sumAnomaly += h.AnomalyScore
			sumSqAnomaly += h.AnomalyScore * h.AnomalyScore
			categorySet[h.Category] = struct{}{}

			if h.Category == "SENSITIVE" || h.Category == "FILE_DELETE" || h.Category == "PROCESS_KILL" {
				sensitiveCount++
			}
			if h.Category == "NETWORK" {
				netCount++
			}
			if h.User == "root" {
				rootCount++
			}
			distinctComms[h.Comm] = struct{}{}
			if h.User != "" {
				distinctUsers[h.User] = struct{}{}
			}
		}
		n := float64(len(history))
		f[96] = float64(commMatch) / n
		f[97] = float64(blockCount) / n
		f[98] = float64(alertCount) / n
		f[99] = sumAnomaly / n                // mean anomaly
		f[100] = sumSqAnomaly/n - f[99]*f[99] // variance
		if f[100] < 0 {
			f[100] = 0
		}
		f[101] = float64(len(categorySet)) / 15.0 // category diversity

		// Anomaly trend: recent half vs older half
		if len(history) >= 4 {
			mid := len(history) / 2
			var recentMean, olderMean float64
			for j, h := range history {
				if j >= mid {
					recentMean += h.AnomalyScore
				} else {
					olderMean += h.AnomalyScore
				}
			}
			recentMean /= float64(len(history) - mid)
			olderMean /= float64(mid)
			f[102] = (recentMean - olderMean + 1.0) / 2.0
		} else {
			f[102] = 0.5
		}

		// Time-weighted recent activity (last 5 events)
		if len(history) >= 3 {
			recentComm := 0
			tail := history
			if len(tail) > 5 {
				tail = tail[len(tail)-5:]
			}
			for _, h := range tail {
				if h.Comm == comm {
					recentComm++
				}
			}
			f[103] = float64(recentComm) / float64(len(tail))
		}

		// Command repetition burst
		if commMatch > 1 {
			f[104] = math.Min(float64(commMatch-1)/10.0, 1.0)
		}

		// Comm-specific alert rate
		commAlerts := 0
		for _, h := range history {
			if h.Comm == comm && h.Action == "ALERT" {
				commAlerts++
			}
		}
		if commMatch > 0 {
			f[105] = float64(commAlerts) / float64(commMatch)
		}

		// New features on previously unassigned dimensions [106-111]
		f[106] = float64(sensitiveCount) / n
		f[107] = float64(netCount) / n
		f[108] = float64(rootCount) / n
		f[109] = float64(len(distinctComms)) / 20.0
		if f[109] > 1.0 {
			f[109] = 1.0
		}
		if !lastCommTime.IsZero() {
			diff := now.Sub(lastCommTime).Seconds()
			if diff < 0 {
				diff = 0
			}
			f[110] = 1.0 / (1.0 + diff)
		} else {
			f[110] = 0.0
		}
		f[111] = float64(len(distinctUsers)) / 5.0
		if f[111] > 1.0 {
			f[111] = 1.0
		}
	}

	// ── Group E: Event Rate Features [112-119] ──
	recentCutoff := now.Add(-1 * time.Second)
	recentCount := 0
	distinctPids := make(map[uint32]struct{})
	sumArgsLen := 0.0
	for _, h := range history {
		sumArgsLen += float64(h.ArgsLen)
		if h.Timestamp.After(recentCutoff) {
			recentCount++
			distinctPids[h.Pid] = struct{}{}
		}
	}
	f[112] = float64(recentCount) / 50.0 // events per second (cap at 50)
	if f[112] > 1.0 {
		f[112] = 1.0
	}
	f[113] = float64(len(distinctPids)) / 20.0
	if f[113] > 1.0 {
		f[113] = 1.0
	}

	// Timestamp features [114-115]
	f[114] = float64(now.Hour()) / 24.0   // hour of day
	f[115] = float64(now.Weekday()) / 7.0 // day of week

	// New features on previously unassigned dimensions [116-119]
	f[116] = math.Sin(2.0 * math.Pi * float64(now.Hour()) / 24.0) // cyclic hour sin, remapped to [0,1] below
	f[117] = math.Cos(2.0 * math.Pi * float64(now.Hour()) / 24.0) // cyclic hour cos, remapped to [0,1] below
	isWeekend := now.Weekday() == time.Saturday || now.Weekday() == time.Sunday
	f[118] = platform.BoolToFloat(isWeekend)
	if len(history) > 0 {
		f[119] = (sumArgsLen / float64(len(history))) / 256.0
		if f[119] > 1.0 {
			f[119] = 1.0
		}
	} else {
		f[119] = 0.0
	}

	// ── Group F: Network Audit Features [120-127] ──
	cmdline := strings.Join(args, " ")
	netAudit := AuditNetworkBehavior(comm, cmdline)
	f[120] = netAudit.RiskScore / 100.0 // normalized risk score
	f[121] = platform.BoolToFloat(netAudit.Flags.SuspiciousPort)
	f[122] = platform.BoolToFloat(netAudit.Flags.ReverseShell)
	f[123] = platform.BoolToFloat(netAudit.Flags.DataExfil)
	f[124] = platform.BoolToFloat(netAudit.Flags.DNSTunnel)
	f[125] = platform.BoolToFloat(netAudit.Flags.ClearTextProto)
	f[126] = platform.BoolToFloat(netAudit.Flags.UnusualTarget)
	f[127] = platform.BoolToFloat(netAudit.Flags.PortScan)

	f = normalizeFeatureVector(f)
	fe.updateStats(f)
	return f
}

// AddHistory adds a wrapper event to the history buffer
func (fe *FeatureExtractor) AddHistory(comm, category, action string, anomalyScore float64, pid uint32, user string, argsLen int, argsCount int) {
	fe.history.Add(RecentWrapperEvent{
		Comm:         comm,
		Category:     category,
		Action:       action,
		AnomalyScore: anomalyScore,
		Timestamp:    time.Now(),
		Pid:          pid,
		User:         user,
		ArgsLen:      argsLen,
		ArgsCount:    argsCount,
	})
}

func (fe *FeatureExtractor) updateStats(f [FeatureDim]float64) {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	if fe.sampleCount == 0 {
		fe.minVals = f
		fe.maxVals = f
	} else {
		for i := range f {
			if f[i] < fe.minVals[i] {
				fe.minVals[i] = f[i]
			}
			if f[i] > fe.maxVals[i] {
				fe.maxVals[i] = f[i]
			}
		}
	}
	fe.sampleCount++
}

// GetHistoryBuffer returns the shared history buffer
func (fe *FeatureExtractor) GetHistoryBuffer() *RecentHistoryBuffer {
	return fe.history
}
