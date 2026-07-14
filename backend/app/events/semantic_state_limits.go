package events

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"strings"
	"time"
)

const (
	SemanticStateMaxContextEntries = 4096
	SemanticStateMaxFileEntries    = 8192
	SemanticStateMaxEntries        = SemanticStateMaxContextEntries*4 + SemanticStateMaxFileEntries
	SemanticStateMaxContextBytes   = 512
	SemanticStateMaxPathBytes      = 2048
	SemanticStateMaxValueBytes     = 1024
	SemanticStateMaxModeBytes      = 128
	SemanticExtraInfoMaxScanBytes  = 64 * 1024
	SemanticPromptDigestMaxBytes   = 256
	SemanticStateGCInterval        = 5 * time.Second
)

type SemanticAlertStateStatus struct {
	RecentSecrets                 int       `json:"recentSecrets"`
	RecentExecutables             int       `json:"recentExecutables"`
	ForkWindows                   int       `json:"forkWindows"`
	AgenticLoopWindows            int       `json:"agenticLoopWindows"`
	RecentFileMutations           int       `json:"recentFileMutations"`
	Entries                       int       `json:"entries"`
	MaxEntries                    int       `json:"maxEntries"`
	ExpiredEvictionsTotal         uint64    `json:"expiredEvictionsTotal"`
	CapacityEvictionsTotal        uint64    `json:"capacityEvictionsTotal"`
	TruncatedStateValuesTotal     uint64    `json:"truncatedStateValuesTotal"`
	IgnoredOversizedMetadataTotal uint64    `json:"ignoredOversizedMetadataTotal"`
	LastSweepAt                   time.Time `json:"lastSweepAt,omitempty"`
}

func boundSemanticStateString(value string, maxBytes int) (string, bool) {
	originalLength := len(value)
	value = strings.TrimSpace(value)
	if value == "" || maxBytes <= 0 {
		return "", value != ""
	}
	if len(value) <= maxBytes {
		if len(value) != originalLength {
			value = strings.Clone(value)
		}
		return value, false
	}
	return semanticBoundWithDigest(value, maxBytes, semanticStateDigest(value)), true
}

func boundSemanticStatePair(left, right string, maxBytes int) (string, bool) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" && right == "" {
		return "", false
	}
	if maxBytes <= 0 {
		return "", true
	}
	if len(left) <= maxBytes && len(right) <= maxBytes && len(left)+1+len(right) <= maxBytes {
		return left + "|" + right, false
	}
	prefix := left
	if prefix == "" {
		prefix = right
	}
	return semanticBoundWithDigest(prefix, maxBytes, semanticStateDigest(left, right)), true
}

func boundSemanticStatePrefixed(prefix, value string, maxBytes int) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if maxBytes <= 0 {
		return "", true
	}
	if len(prefix)+len(value) <= maxBytes {
		return prefix + value, false
	}
	prefixValue := value
	if available := maxBytes - len(prefix); available > 0 && len(prefixValue) > available {
		prefixValue = prefixValue[:available]
	} else if available <= 0 {
		prefixValue = ""
	}
	return semanticBoundWithDigest(prefix+prefixValue, maxBytes, semanticStateDigest(prefix, value)), true
}

func semanticBoundWithDigest(prefix string, maxBytes int, digest [sha256.Size]byte) string {
	var encoded [16]byte
	hex.Encode(encoded[:], digest[:8])
	if maxBytes <= len(encoded) {
		return string(encoded[:maxBytes])
	}
	prefixBytes := maxBytes - len(encoded) - 1
	if len(prefix) > prefixBytes {
		prefix = prefix[:prefixBytes]
	}
	prefix = strings.ToValidUTF8(prefix, "?")
	if len(prefix) > prefixBytes {
		prefix = prefix[:prefixBytes]
	}
	return prefix + "~" + string(encoded[:])
}

func semanticStateDigest(values ...string) [sha256.Size]byte {
	digest := sha256.New()
	var length [8]byte
	for _, value := range values {
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = digest.Write(length[:])
		writeSemanticDigestString(digest, value)
	}
	var sum [sha256.Size]byte
	digest.Sum(sum[:0])
	return sum
}

func writeSemanticDigestString(digest hash.Hash, value string) {
	var buffer [4096]byte
	for offset := 0; offset < len(value); {
		count := copy(buffer[:], value[offset:])
		_, _ = digest.Write(buffer[:count])
		offset += count
	}
}

func semanticStateExpired(now, observedAt time.Time, ttl time.Duration) bool {
	if observedAt.IsZero() {
		return true
	}
	return ttl > 0 && now.After(observedAt.Add(ttl))
}
