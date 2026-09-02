package tls

import (
	"debug/elf"
	"os"
	"path/filepath"
	"testing"
)

func writeScanFixture(t *testing.T, data []byte) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scan.bin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("os.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func TestScanMaskedFileRangeFindsPatternAcrossChunkBoundary(t *testing.T) {
	pattern := []byte{0xde, 0xad, 0xbe, 0xef, 0x42, 0x24}
	offset := strippedSSLScanChunkSize - 3
	data := make([]byte, strippedSSLScanChunkSize+64)
	copy(data[offset:], pattern)
	file := writeScanFixture(t, data)

	match, err := firstExactFileMatch(file, []strippedSSLFileRange{{offset: 0, size: int64(len(data))}}, pattern)
	if err != nil {
		t.Fatalf("firstExactFileMatch() error = %v", err)
	}
	if match != offset {
		t.Fatalf("match = %#x, want %#x", match, offset)
	}
}

func TestScanMaskedFileRangePreservesWildcardMask(t *testing.T) {
	pattern := []byte{0xaa, 0x00, 0xcc}
	mask := []byte{0xff, 0x00, 0xff}
	data := []byte{0x00, 0xaa, 0x77, 0xcc, 0x00}
	file := writeScanFixture(t, data)

	match, err := firstMaskedFileMatch(file, []strippedSSLFileRange{{offset: 0, size: int64(len(data))}}, pattern, mask)
	if err != nil {
		t.Fatalf("firstMaskedFileMatch() error = %v", err)
	}
	if match != 1 {
		t.Fatalf("match = %d, want 1", match)
	}
}

func TestCountMaskedFileMatchesKeepsCountBeyondStoredPair(t *testing.T) {
	pattern := []byte{0x11, 0x22, 0x33}
	data := make([]byte, 64)
	for _, offset := range []int{4, 20, 48} {
		copy(data[offset:], pattern)
	}
	file := writeScanFixture(t, data)

	firstTwo, count, err := countMaskedFileMatches(
		file,
		[]strippedSSLFileRange{{offset: 0, size: int64(len(data))}},
		pattern,
		[]byte{0xff, 0xff, 0xff},
	)
	if err != nil {
		t.Fatalf("countMaskedFileMatches() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
	if len(firstTwo) != 2 || firstTwo[0] != 4 || firstTwo[1] != 20 {
		t.Fatalf("first matches = %v, want [4 20]", firstTwo)
	}
}

func TestExecutableFileRangesFiltersSortsAndMergesLoads(t *testing.T) {
	parsed := &elf.File{Progs: []*elf.Prog{
		// Intentionally out of order and overlapping/adjacent.
		{ProgHeader: elf.ProgHeader{Type: elf.PT_LOAD, Flags: elf.PF_R | elf.PF_X, Off: 0x1800, Filesz: 0x900}},
		{ProgHeader: elf.ProgHeader{Type: elf.PT_LOAD, Flags: elf.PF_R | elf.PF_W, Off: 0x5000, Filesz: 0x400}},
		{ProgHeader: elf.ProgHeader{Type: elf.PT_LOAD, Flags: elf.PF_R | elf.PF_X, Off: 0x1000, Filesz: 0x900}},
		{ProgHeader: elf.ProgHeader{Type: elf.PT_NOTE, Flags: elf.PF_R | elf.PF_X, Off: 0x3000, Filesz: 0x100}},
		{ProgHeader: elf.ProgHeader{Type: elf.PT_LOAD, Flags: elf.PF_R | elf.PF_X, Off: 0x2100, Filesz: 0x300}},
		{ProgHeader: elf.ProgHeader{Type: elf.PT_LOAD, Flags: elf.PF_R | elf.PF_X, Off: 0x6000, Filesz: 0}},
	}}
	ranges, err := executableFileRanges(parsed)
	if err != nil {
		t.Fatalf("executableFileRanges() error = %v", err)
	}
	if len(ranges) != 1 || ranges[0].offset != 0x1000 || ranges[0].size != 0x1400 {
		t.Fatalf("executable ranges = %+v, want merged [0x1000,0x2400)", ranges)
	}
}

func TestExecutableFileRangesRejectsOffsetOverflow(t *testing.T) {
	parsed := &elf.File{Progs: []*elf.Prog{
		{ProgHeader: elf.ProgHeader{
			Type:   elf.PT_LOAD,
			Flags:  elf.PF_R | elf.PF_X,
			Off:    uint64(maxInt64) - 7,
			Filesz: 16,
		}},
	}}
	if _, err := executableFileRanges(parsed); err == nil {
		t.Fatal("overflowing executable file range was accepted")
	}
}

func TestScanMaskedFileRangeRejectsNegativeOrOverflowingRange(t *testing.T) {
	file := writeScanFixture(t, make([]byte, 64))
	pattern := []byte{1}
	for name, fileRange := range map[string]strippedSSLFileRange{
		"negative": {offset: -1, size: 1},
		"overflow": {offset: maxInt64 - 1, size: 4},
	} {
		t.Run(name, func(t *testing.T) {
			err := scanMaskedFileRange(file, fileRange.offset, fileRange.size, pattern, nil, func(int64) bool { return true })
			if err == nil {
				t.Fatalf("invalid scan range %+v was accepted", fileRange)
			}
		})
	}
}
