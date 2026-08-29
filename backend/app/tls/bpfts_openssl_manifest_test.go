package tls

import (
	"testing"

	"agent-ebpf-filter/app/bpfts"
)

func testBpfTSOpenSSLManifest() bpfts.Manifest {
	return bpfts.Manifest{
		Version: bpfts.ManifestVersion,
		Source:  "tls-openssl.ts",
		Probes: []bpfts.ManifestProbe{
			{Name: "write", Section: "uprobe", Kind: "uprobe", Target: "SSL_write"},
			{Name: "readEnter", Section: "uprobe", Kind: "uprobe", Target: "SSL_read"},
			{Name: "readComplete", Section: "uretprobe", Kind: "uretprobe", Target: "SSL_read"},
		},
		Maps: []bpfts.ManifestMap{
			{Name: "pendingReadBuffers", Kind: "hash", MaxEntries: 16384},
			{Name: "pendingReadConnections", Kind: "hash", MaxEntries: 16384},
			{Name: bpfTSOpenSSLRingName, Kind: "ringbuf", MaxEntries: 1 << 20},
			{Name: bpfTSOpenSSLScratchName, Kind: "percpu_array", MaxEntries: 1},
		},
	}
}

func TestBpfTSOpenSSLManifestAcceptsCanonicalABI(t *testing.T) {
	if err := validateBpfTSOpenSSLManifest(testBpfTSOpenSSLManifest()); err != nil {
		t.Fatalf("validateBpfTSOpenSSLManifest() error = %v", err)
	}
}

func TestBpfTSOpenSSLManifestRejectsExtraProbe(t *testing.T) {
	manifest := testBpfTSOpenSSLManifest()
	manifest.Probes = append(manifest.Probes, bpfts.ManifestProbe{
		Name: "extra", Section: "uprobe", Kind: "uprobe", Target: "SSL_write",
	})
	if err := validateBpfTSOpenSSLManifest(manifest); err == nil {
		t.Fatal("canonical ABI accepted extra probe")
	}
}

func TestBpfTSOpenSSLManifestRejectsRingSchemaDrift(t *testing.T) {
	manifest := testBpfTSOpenSSLManifest()
	for index := range manifest.Maps {
		if manifest.Maps[index].Name == bpfTSOpenSSLRingName {
			manifest.Maps[index].MaxEntries = 65536
		}
	}
	if err := validateBpfTSOpenSSLManifest(manifest); err == nil {
		t.Fatal("canonical ABI accepted ring capacity drift")
	}
}

func TestBpfTSOpenSSLManifestRejectsScratchSchemaDrift(t *testing.T) {
	manifest := testBpfTSOpenSSLManifest()
	for index := range manifest.Maps {
		if manifest.Maps[index].Name == bpfTSOpenSSLScratchName {
			manifest.Maps[index].Kind = "array"
		}
	}
	if err := validateBpfTSOpenSSLManifest(manifest); err == nil {
		t.Fatal("canonical ABI accepted scratch map type drift")
	}
}

func TestBpfTSOpenSSLManifestRejectsDifferentSource(t *testing.T) {
	manifest := testBpfTSOpenSSLManifest()
	manifest.Source = "tls-read.ts"
	if err := validateBpfTSOpenSSLManifest(manifest); err == nil {
		t.Fatal("canonical ABI accepted another source")
	}
}
