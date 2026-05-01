package main

import (
	"math"
	"strings"
	"sync"
	"time"
)

// Feature vector design (128 dimensions) inspired by:
// - Forrest et al. "A Sense of Self for Unix Processes" (stide n-gram model)
// - LIGHT-HIDS (2509.13464): DeepSVDD + Isolation Forest hybrid
// - eBPF ransomware detection (2409.06452): per-syscall frequency + entropy features

const FeatureDim = 128

// RecentWrapperEvent holds a summary of a recent wrapper_intercept event
type RecentWrapperEvent struct {
	Comm         string
	Category     string
	Action       string
	AnomalyScore float64
	Timestamp    time.Time
}

// RecentHistoryBuffer is a sliding window of recent wrapper intercept events
type RecentHistoryBuffer struct {
	mu       sync.RWMutex
	events   []RecentWrapperEvent
	maxSize  int
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

func (fe *FeatureExtractor) Extract(comm string, args []string, user string, pid uint32) [FeatureDim]float64 {
	var f [FeatureDim]float64

	// ── Group A: Command/Process Features [0-31] ──
	// BehaviorCategory one-hot (15 dims, 0-14)
	classification := ClassifyBehavior(comm, args)
	for _, cat := range classification.Categories {
		if int(cat) < 15 {
			f[int(cat)] = 1.0
		}
	}

	// Binary flags [15-25]
	f[15] = boolToFloat(isShell(comm))                // is_shell
	f[16] = boolToFloat(isPackageManager(comm))        // is_package_manager
	f[17] = boolToFloat(isAgentCLI(comm))              // is_agent_cli
	f[18] = boolToFloat(user == "root")                // is_root_user
	f[19] = boolToFloat(hasNetworkArgs(args))           // has_network_args
	f[20] = boolToFloat(hasFileArgs(args))              // has_file_args
	f[21] = boolToFloat(hasRedirect(args))              // has_redirection
	f[22] = boolToFloat(hasPipeChain(args))             // has_pipe
	f[23] = boolToFloat(len(args) > 10)                 // many_args
	f[24] = boolToFloat(strings.Contains(strings.Join(args, " "), "/dev/")) // dev_access
	f[25] = boolToFloat(hasSudoInArgs(args))            // sudo_in_args

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
		f[32] = meanLen / 256.0  // mean arg length (normalized)
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
		f[35] = shannonEntropy(strings.Join(args, "")) // path-like entropy
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
	f[38] = boolToFloat(strings.Contains(allArgs, "/etc/"))
	f[39] = boolToFloat(strings.Contains(allArgs, "/proc/"))
	f[40] = boolToFloat(strings.Contains(allArgs, "/sys/"))
	f[41] = boolToFloat(strings.Contains(allArgs, "/var/log/"))
	f[42] = boolToFloat(strings.Contains(allArgs, "/root/"))
	f[43] = boolToFloat(strings.Contains(allArgs, "/home/"))
	f[44] = boolToFloat(strings.Contains(allArgs, "/tmp/"))
	f[45] = boolToFloat(strings.Contains(allArgs, "~/.ssh"))
	f[46] = boolToFloat(strings.Contains(allArgs, "~/.gnupg"))
	f[47] = boolToFloat(strings.Contains(allArgs, "/boot/"))

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
	f[58] = boolToFloat(hasURLPattern(allArgs))
	f[59] = boolToFloat(hasIPPattern(allArgs))

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
	f[63] = boolToFloat(hasEnvironmentVar(args))

	// ── Group C: Embedding Projection [64-95] ──
	_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
	// Take first 32 dims of the 64-dim LSH embedding
	copy(f[64:96], emb.Vector[:32])

	// ── Group D: Recent History Aggregates [96-111] ──
	history := fe.history.Snapshot()
	if len(history) > 0 {
		// Frequency of this comm in window
		commMatch := 0
		blockCount, alertCount := 0, 0
		var sumAnomaly, sumSqAnomaly float64
		categorySet := make(map[string]struct{})
		for _, h := range history {
			if h.Comm == comm {
				commMatch++
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
		}
		n := float64(len(history))
		f[96] = float64(commMatch) / n
		f[97] = float64(blockCount) / n
		f[98] = float64(alertCount) / n
		f[99] = sumAnomaly / n // mean anomaly
		f[100] = sumSqAnomaly/n - f[99]*f[99] // variance
		if f[100] < 0 {
			f[100] = 0
		}
		f[101] = float64(len(categorySet)) / 15.0 // category diversity
		f[102] = float64(len(history)) / float64(fe.history.maxSize) // buffer fill ratio
	}

	// ── Group E: Event Rate Features [112-119] ──
	now := time.Now()
	recentCutoff := now.Add(-1 * time.Second)
	recentCount := 0
	distinctPids := make(map[uint32]struct{})
	for _, h := range history {
		if h.Timestamp.After(recentCutoff) {
			recentCount++
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

	// ── Group F: Reserved [120-127] ──
	// Left as zeros for future use

	fe.updateStats(f)
	return f
}

// AddHistory adds a wrapper event to the history buffer
func (fe *FeatureExtractor) AddHistory(comm, category, action string, anomalyScore float64) {
	fe.history.Add(RecentWrapperEvent{
		Comm:         comm,
		Category:     category,
		Action:       action,
		AnomalyScore: anomalyScore,
		Timestamp:    time.Now(),
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

// ── Helper functions ──

func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

func isShell(comm string) bool {
	shells := map[string]bool{"bash": true, "zsh": true, "fish": true, "sh": true, "dash": true, "ksh": true, "tcsh": true, "ash": true}
	return shells[comm]
}

func isPackageManager(comm string) bool {
	pms := map[string]bool{
		"apt": true, "apt-get": true, "yum": true, "dnf": true, "pacman": true,
		"zypper": true, "brew": true, "pip": true, "pip3": true, "npm": true,
		"yarn": true, "pnpm": true, "go": true, "cargo": true, "gem": true,
		"snap": true, "flatpak": true, "nix-env": true, "rpm": true, "dpkg": true,
	}
	return pms[comm]
}

func isAgentCLI(comm string) bool {
	agents := map[string]bool{
		"claude": true, "gemini": true, "codex": true, "kiro-cli": true,
		"gh": true, "cursor": true,
	}
	return agents[comm]
}

func hasNetworkArgs(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") ||
			strings.HasPrefix(a, "ftp://") || strings.HasPrefix(a, "ws://") ||
			strings.Contains(a, ":") && !strings.Contains(a, "/") {
			return true
		}
	}
	return false
}

func hasFileArgs(args []string) bool {
	for _, a := range args {
		if strings.Contains(a, "/") && !strings.HasPrefix(a, "-") &&
			!strings.HasPrefix(a, "http") {
			return true
		}
	}
	return false
}

func hasRedirect(args []string) bool {
	for _, a := range args {
		if a == ">" || a == ">>" || a == "<" || a == "2>" || a == "&>" || a == "|" {
			return true
		}
	}
	return false
}

func hasPipeChain(args []string) bool {
	for _, a := range args {
		if a == "|" {
			return true
		}
	}
	return false
}

func hasSudoInArgs(args []string) bool {
	for _, a := range args {
		if a == "sudo" || a == "doas" || a == "pkexec" {
			return true
		}
	}
	return false
}

func hasURLPattern(s string) bool {
	return strings.Contains(s, "http://") || strings.Contains(s, "https://") ||
		strings.Contains(s, "ftp://")
}

func hasIPPattern(s string) bool {
	// Simple IP-like pattern check
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ':' || r == '/' || r == '@' || r == '.'
	})
	for _, p := range parts {
		if len(p) > 0 && (strings.Count(p, ".") == 3 || strings.Count(p, ".") == 4) {
			return true
		}
	}
	return false
}

func hasEnvironmentVar(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "$") || strings.Contains(a, "=${") {
			return true
		}
	}
	return false
}

// shannonEntropy computes Shannon entropy of a string (0-1 normalized)
func shannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	counts := make(map[byte]int)
	for i := 0; i < len(s); i++ {
		counts[s[i]]++
	}
	var entropy float64
	n := float64(len(s))
	for _, c := range counts {
		p := float64(c) / n
		entropy -= p * math.Log2(p)
	}
	// Normalize by log2(256) = 8
	return entropy / 8.0
}
