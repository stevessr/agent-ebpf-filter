package app

import (
	"os"
	"sync"
)

const signalProgramLogFrameCountCacheCapacity = 256

type signalProgramLogFileSignature struct {
	size       int64
	modifiedNs int64
}

type signalProgramLogFrameCountCacheEntry struct {
	signature signalProgramLogFileSignature
	frames    int
	errorText string
	access    uint64
}

// signalProgramLogFrameCountCache prevents the status endpoint (polled by the
// UI) from rescanning up to 128 MiB of framed data every few seconds. Entries
// are keyed by the file metadata observed through the already validated file
// descriptor, and successful appends advance a known count in O(1).
type signalProgramLogFrameCountCache struct {
	mu       sync.Mutex
	entries  map[string]signalProgramLogFrameCountCacheEntry
	capacity int
	sequence uint64
}

var signalProgramLogFrameCountCacheStore = newSignalProgramLogFrameCountCache(signalProgramLogFrameCountCacheCapacity)

func newSignalProgramLogFrameCountCache(capacity int) *signalProgramLogFrameCountCache {
	if capacity <= 0 {
		capacity = 1
	}
	return &signalProgramLogFrameCountCache{
		entries:  make(map[string]signalProgramLogFrameCountCacheEntry, capacity),
		capacity: capacity,
	}
}

func signalProgramLogSignature(info os.FileInfo) signalProgramLogFileSignature {
	if info == nil {
		return signalProgramLogFileSignature{}
	}
	return signalProgramLogFileSignature{
		size:       info.Size(),
		modifiedNs: info.ModTime().UnixNano(),
	}
}

func (c *signalProgramLogFrameCountCache) Lookup(path string, signature signalProgramLogFileSignature) (frames int, errorText string, ok bool) {
	if c == nil || path == "" {
		return 0, "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[path]
	if !ok || entry.signature != signature {
		return 0, "", false
	}
	c.sequence++
	entry.access = c.sequence
	c.entries[path] = entry
	return entry.frames, entry.errorText, true
}

func (c *signalProgramLogFrameCountCache) Remember(path string, signature signalProgramLogFileSignature, frames int, errorText string) {
	if c == nil || path == "" {
		return
	}
	if frames < 0 {
		frames = 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sequence++
	if _, exists := c.entries[path]; !exists && len(c.entries) >= c.capacity {
		c.evictOldestLocked()
	}
	c.entries[path] = signalProgramLogFrameCountCacheEntry{
		signature: signature,
		frames:    frames,
		errorText: errorText,
		access:    c.sequence,
	}
}

func (c *signalProgramLogFrameCountCache) Advance(
	path string,
	before signalProgramLogFileSignature,
	after signalProgramLogFileSignature,
) {
	if c == nil || path == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sequence++
	if before.size == 0 {
		if _, exists := c.entries[path]; !exists && len(c.entries) >= c.capacity {
			c.evictOldestLocked()
		}
		c.entries[path] = signalProgramLogFrameCountCacheEntry{
			signature: after,
			frames:    1,
			access:    c.sequence,
		}
		return
	}
	entry, ok := c.entries[path]
	if !ok || entry.signature != before {
		delete(c.entries, path)
		return
	}
	entry.signature = after
	if entry.errorText == "" {
		entry.frames++
	}
	entry.access = c.sequence
	c.entries[path] = entry
}

func (c *signalProgramLogFrameCountCache) evictOldestLocked() {
	var oldestPath string
	var oldestAccess uint64
	for path, entry := range c.entries {
		if oldestPath == "" || entry.access < oldestAccess {
			oldestPath = path
			oldestAccess = entry.access
		}
	}
	if oldestPath != "" {
		delete(c.entries, oldestPath)
	}
}
