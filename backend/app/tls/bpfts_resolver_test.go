package tls

import (
	"debug/elf"
	"os"
	"testing"

	"agent-ebpf-filter/app/bpfts"
)

func TestBpfTSDiscoverKnownStrippedSSLOffsetsExactPatterns(t *testing.T) {
	data := make([]byte, 0x4000)
	readOff := 0x240
	writeOff := readOff + 0xCA0
	copy(data[readOff:], bsSSLRead.pattern)
	copy(data[writeOff:], bsSSLWrite.pattern)
	copy(data[0x3000:], []byte("SSL_read\x00SSL_write\x00"))

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

func TestBpfTSUprobeResolverUsesExistingELFSymbol(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	file, err := elf.Open(path)
	if err != nil {
		t.Fatalf("elf.Open() error = %v", err)
	}
	symbols, err := file.Symbols()
	if err != nil || len(symbols) == 0 {
		_ = file.Close()
		t.Skip("test binary has no regular ELF symbol table")
	}
	targetSymbol := ""
	for _, symbol := range symbols {
		if symbol.Name != "" {
			targetSymbol = symbol.Name
			break
		}
	}
	_ = file.Close()
	if targetSymbol == "" {
		t.Skip("test binary has no named ELF symbols")
	}

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
