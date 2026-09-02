package tls

import (
	"debug/elf"
	"os"
	"testing"

	"agent-ebpf-filter/app/bpfts"
)

func exactBoringSSLFixture(includeNames bool) ([]byte, int, int) {
	data := make([]byte, 0x4000)
	readOff := 0x240
	writeOff := readOff + 0xCA0
	copy(data[readOff:], bsSSLRead.pattern)
	copy(data[writeOff:], bsSSLWrite.pattern)
	if includeNames {
		copy(data[0x3000:], []byte("SSL_read\x00SSL_write\x00"))
	}
	return data, readOff, writeOff
}

func assertExactBoringSSLOffsets(t *testing.T, data []byte, readOff, writeOff int) {
	t.Helper()
	offsets, err := discoverKnownStrippedSSLOffsets(data)
	if err != nil {
		t.Fatalf("discoverKnownStrippedSSLOffsets() error = %v", err)
	}
	if got := offsets["SSL_read"]; got != uint64(readOff) {
		t.Fatalf("SSL_read offset = %#x, want %#x", got, readOff)
	}
	if got := offsets["SSL_write"]; got != uint64(writeOff) {
		t.Fatalf("SSL_write offset = %#x, want %#x", got, writeOff)
	}
}

func TestBpfTSDiscoverKnownStrippedSSLOffsetsExactPatterns(t *testing.T) {
	data, readOff, writeOff := exactBoringSSLFixture(true)
	assertExactBoringSSLOffsets(t, data, readOff, writeOff)
}

func TestBpfTSDiscoverExactPatternsSurviveRemovedSSLStrings(t *testing.T) {
	data, readOff, writeOff := exactBoringSSLFixture(false)
	assertExactBoringSSLOffsets(t, data, readOff, writeOff)
}

func TestBpfTSDiscoverKnownStrippedSSLOffsetsFailsClosedOnAmbiguousPrefix(t *testing.T) {
	data := make([]byte, 0x5000)
	copy(data[0x4000:], []byte("SSL_read\x00SSL_write\x00"))
	for _, offset := range []int{0x200, 0x900, 0x1200} {
		copy(data[offset:], osslSSLCommonPrefix.pattern)
	}
	if _, err := discoverKnownStrippedSSLOffsets(data); err == nil {
		t.Fatal("expected ambiguous common-prefix scan to fail closed")
	}
}

func executableWithSymbol(t *testing.T) (string, string) {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	file, err := elf.Open(path)
	if err != nil {
		t.Fatalf("elf.Open() error = %v", err)
	}
	defer file.Close()
	symbols, err := file.Symbols()
	if err != nil || len(symbols) == 0 {
		t.Skip("test binary has no regular ELF symbol table")
	}
	for _, symbol := range symbols {
		if symbol.Name != "" {
			return path, symbol.Name
		}
	}
	t.Skip("test binary has no named ELF symbols")
	return "", ""
}

func TestBpfTSUprobeResolverUsesExistingELFSymbol(t *testing.T) {
	path, targetSymbol := executableWithSymbol(t)
	resolver := NewBpfTSUprobeResolver(path, 4321)
	target, err := resolver(bpfts.ManifestProbe{
		Name:    "testProbe",
		Kind:    "uprobe",
		Section: "uprobe",
		Target:  targetSymbol,
	})
	if err != nil {
		t.Fatalf("resolver() error = %v", err)
	}
	if target.Path != path || target.Symbol != targetSymbol || target.PID != 4321 || target.Address != 0 {
		t.Fatalf("unexpected resolved target: %+v", target)
	}
}

func TestBpfTSResolverSkipsStrippedScanForUnrelatedSymbolMiss(t *testing.T) {
	path, _ := executableWithSymbol(t)
	resolver := &bpfTSTLSResolver{path: path}
	_, err := resolver.resolve(bpfts.ManifestProbe{
		Name:    "missing",
		Kind:    "uprobe",
		Section: "uprobe",
		Target:  "__bpf_ts_definitely_missing_symbol__",
	})
	if err == nil {
		t.Fatal("expected unrelated missing symbol to fail closed")
	}
	if resolver.offsets != nil || resolver.offsetErr != nil {
		t.Fatalf("unrelated symbol miss unexpectedly triggered stripped scan: offsets=%v err=%v", resolver.offsets, resolver.offsetErr)
	}
}

func TestBpfTSUprobeResolverRejectsUnknownTarget(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	resolver := NewBpfTSUprobeResolver(path, 0)
	if _, err := resolver(bpfts.ManifestProbe{
		Name:    "missing",
		Kind:    "uprobe",
		Section: "uprobe",
		Target:  "__bpf_ts_definitely_missing_symbol__",
	}); err == nil {
		t.Fatal("expected unknown target to fail closed")
	}
}
